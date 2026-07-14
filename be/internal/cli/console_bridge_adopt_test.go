package cli

import (
	"context"
	"testing"
)

// TestAdoptConsoleSession_NoCreateUsesInjectedBearerAndProject covers case 6:
// with a pre-minted session adopted (as `nrflo_server console` injects via
// NRFLO_CONSOLE_TOKEN/NRFLO_CONSOLE_SESSION_ID), the bridge performs no
// create call, uses the injected bearer + pinned project on tools/list and
// tools/call, and never closes the session it does not own.
func TestAdoptConsoleSession_NoCreateUsesInjectedBearerAndProject(t *testing.T) {
	f := newFakeConsoleServer(t)
	f.sessionToken = "adopted-tok" // the pre-minted session the fake server already knows about
	f.tools = []consoleTool{{Name: "project_status"}}
	f.toolResp["project_status"] = toolCallResp{output: "ok"}

	c := &nrfloHTTPClient{base: f.url(), hc: f.srv.Client()}
	c.adoptConsoleSession("sess-adopted", "adopted-tok", "proj-pinned")

	if len(f.createReqs) != 0 {
		t.Fatalf("createReqs = %d, want 0 (adopt must never open a new session)", len(f.createReqs))
	}

	if _, err := c.listConsoleTools(context.Background()); err != nil {
		t.Fatalf("listConsoleTools: %v", err)
	}
	if len(f.listReqs) != 1 || f.listReqs[0].auth != "Bearer adopted-tok" || f.listReqs[0].project != "proj-pinned" {
		t.Errorf("tools/list req = %+v, want bearer adopted-tok + project proj-pinned", f.listReqs)
	}

	if _, _, err := c.callConsoleTool(context.Background(), "project_status", nil); err != nil {
		t.Fatalf("callConsoleTool: %v", err)
	}
	if len(f.callReqs) != 1 || f.callReqs[0].auth != "Bearer adopted-tok" || f.callReqs[0].project != "proj-pinned" {
		t.Errorf("tools/call req = %+v, want bearer adopted-tok + project proj-pinned", f.callReqs)
	}

	c.closeConsoleSession()
	if len(f.closeReqs) != 0 {
		t.Errorf("closeReqs = %d, want 0 (an adopted session is owned by the parent `console` command, not this bridge)", len(f.closeReqs))
	}
}

// TestAdoptConsoleSession_401ReexchangesWithServiceTokenAndClosesSelfOwnedSession
// covers case 6: a forced 401 on an adopted session still re-exchanges with
// the service token (single retry, same as the non-adopted path), and the
// FRESH session minted by that re-exchange — which this client now owns — is
// closed on shutdown, unlike the original adopted session.
func TestAdoptConsoleSession_401ReexchangesWithServiceTokenAndClosesSelfOwnedSession(t *testing.T) {
	f := newFakeConsoleServer(t)
	f.sessionToken = "adopted-tok"
	f.toolResp["project_status"] = toolCallResp{output: "ok"}

	c := &nrfloHTTPClient{base: f.url(), serviceToken: f.serviceToken, hc: f.srv.Client()}
	c.adoptConsoleSession("sess-adopted", "adopted-tok", "proj-pinned")
	f.unauthorizedRemaining = 1 // the adopted session was swept idle server-side

	if _, _, err := c.callConsoleTool(context.Background(), "project_status", nil); err != nil {
		t.Fatalf("callConsoleTool: %v", err)
	}
	if len(f.createReqs) != 1 {
		t.Fatalf("createReqs = %d, want 1 (a single re-exchange using the service token)", len(f.createReqs))
	}
	if got := f.createReqs[0]; got.auth != "Bearer "+f.serviceToken || got.project != "proj-pinned" {
		t.Errorf("re-exchange create req = %+v, want service token + the pinned project proj-pinned", got)
	}

	c.closeConsoleSession()
	if len(f.closeReqs) != 1 {
		t.Fatalf("closeReqs = %d, want 1 (the re-exchanged session is self-owned and must close)", len(f.closeReqs))
	}
	if got := f.closeReqs[0].auth; got == "Bearer adopted-tok" {
		t.Errorf("close should use the NEW session's bearer, not the original adopted token: %q", got)
	}
}
