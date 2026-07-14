package cli

import (
	"bytes"
	"context"
	"strings"
	"sync"
	"testing"
)

// TestOpenConsoleSession_UsesServiceTokenAndResolvedProject covers case 1:
// openConsoleSession POSTs with the SERVICE bearer and the resolved
// X-Project; the returned token (not the service token) is used on every
// subsequent /console/tools request.
func TestOpenConsoleSession_UsesServiceTokenAndResolvedProject(t *testing.T) {
	f := newFakeConsoleServer(t)
	c := &nrfloHTTPClient{base: f.url(), serviceToken: f.serviceToken, defaultProject: "p1", hc: f.srv.Client()}
	c.cwdResolved = true

	if err := c.openConsoleSession(context.Background()); err != nil {
		t.Fatalf("openConsoleSession: %v", err)
	}
	if len(f.createReqs) != 1 {
		t.Fatalf("create-session calls = %d, want 1", len(f.createReqs))
	}
	if got := f.createReqs[0]; got.auth != "Bearer "+f.serviceToken || got.project != "p1" {
		t.Errorf("create-session req = %+v, want service token + project p1", got)
	}
	if c.consoleToken == "" || c.consoleToken == c.serviceToken {
		t.Fatalf("consoleToken not set to the session token: %q", c.consoleToken)
	}

	f.tools = []consoleTool{{Name: "project_status"}}
	f.toolResp["project_status"] = toolCallResp{output: "ok"}
	if _, err := c.listConsoleTools(context.Background()); err != nil {
		t.Fatalf("listConsoleTools: %v", err)
	}
	if _, _, err := c.callConsoleTool(context.Background(), "project_status", nil); err != nil {
		t.Fatalf("callConsoleTool: %v", err)
	}
	for _, got := range append(append([]reqLog{}, f.listReqs...), f.callReqs[0].reqLog) {
		if got.auth != "Bearer "+c.consoleToken {
			t.Errorf("console tool route used %q, want the console token %q (never the service token)", got.auth, "Bearer "+c.consoleToken)
		}
	}
}

// TestCallConsoleTool_401TriggersSingleReexchange and
// TestCallConsoleTool_401DoesNotLoopForever cover case 6.
func TestCallConsoleTool_401TriggersSingleReexchange(t *testing.T) {
	f := newFakeConsoleServer(t)
	f.toolResp["project_status"] = toolCallResp{output: "ok"}
	c := f.openedClient(t, "p1") // 1 create-session call so far
	f.unauthorizedRemaining = 1  // the next authed request fails once

	out, isErr, err := c.callConsoleTool(context.Background(), "project_status", nil)
	if err != nil {
		t.Fatalf("callConsoleTool: %v", err)
	}
	if out != "ok" || isErr {
		t.Errorf("out=%q isErr=%v, want ok/false", out, isErr)
	}
	if len(f.createReqs) != 2 {
		t.Errorf("create-session calls = %d, want 2 (initial open + one re-exchange)", len(f.createReqs))
	}
}

func TestCallConsoleTool_401DoesNotLoopForever(t *testing.T) {
	f := newFakeConsoleServer(t)
	c := f.openedClient(t, "p1")
	f.unauthorizedRemaining = 1000 // always unauthorized

	if _, _, err := c.callConsoleTool(context.Background(), "whatever", nil); err == nil {
		t.Fatal("expected an error when the retry also gets 401")
	}
	if len(f.createReqs) != 2 {
		t.Errorf("create-session calls = %d, want exactly 2 (initial open + single retry, no loop)", len(f.createReqs))
	}
}

// TestConcurrentToolCalls_SingleReexchangeNoDuplicateSessions guards the stdio
// loop's concurrency contract: runMCPStdioLoopWithCancel dispatches every
// request in its own goroutine against one shared client, so a session swept
// idle server-side makes N goroutines see 401 at once. The re-exchange is
// single-flighted — exactly one new session is opened (a per-goroutine
// re-exchange would leak all but the last) and the mutable session state is
// mutex-guarded (run under -race).
func TestConcurrentToolCalls_SingleReexchangeNoDuplicateSessions(t *testing.T) {
	const n = 8
	f := newFakeConsoleServer(t)
	f.toolResp["project_status"] = toolCallResp{output: "ok"}
	c := f.openedClient(t, "p1") // 1 create-session call so far

	// Simulate the server-side idle sweep: the session the client holds is gone,
	// so any request still bearing that token 401s, while the token minted by a
	// re-exchange works. Modelled by killing the fake's current session token
	// rather than by a forced-401 counter, so the assertion holds for every
	// goroutine interleaving (a goroutine whose first attempt lands after
	// another's re-exchange simply succeeds with the fresh token).
	f.mu.Lock()
	f.sessionToken = ""
	f.mu.Unlock()

	var wg sync.WaitGroup
	errs := make([]error, n)
	for i := range n {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			_, _, errs[i] = c.callConsoleTool(context.Background(), "project_status", nil)
		}(i)
	}
	wg.Wait()

	for i, err := range errs {
		if err != nil {
			t.Errorf("concurrent callConsoleTool[%d]: %v", i, err)
		}
	}
	if len(f.createReqs) != 2 {
		t.Errorf("create-session calls = %d, want 2 (initial open + ONE single-flighted re-exchange for %d concurrent 401s)", len(f.createReqs), n)
	}
}

// TestCloseConsoleSession_FiresOnShutdown covers case 7: driving
// runMCPStdioLoopWithCancel with an EOF-ing reader, then closing the session
// (mirroring RunE's deferred close), fires the close request with the console
// bearer.
func TestCloseConsoleSession_FiresOnShutdown(t *testing.T) {
	f := newFakeConsoleServer(t)
	c := f.openedClient(t, "p1")

	in := strings.NewReader(`{"jsonrpc":"2.0","id":1,"method":"initialize"}` + "\n")
	var out bytes.Buffer
	err := runMCPStdioLoopWithCancel(context.Background(), in, &out, func(ctx context.Context, req mcpRequest) *mcpResponse {
		return dispatchExternalMCP(ctx, req, c)
	})
	if err != nil {
		t.Fatalf("runMCPStdioLoopWithCancel: %v", err)
	}
	c.closeConsoleSession()

	if len(f.closeReqs) != 1 {
		t.Fatalf("close calls = %d, want 1", len(f.closeReqs))
	}
	if got := f.closeReqs[0].auth; got != "Bearer "+c.consoleToken {
		t.Errorf("close should use the console bearer, got %q want %q", got, "Bearer "+c.consoleToken)
	}
}
