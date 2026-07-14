package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"be/internal/service"
)

// nrfloHTTPClient is the external MCP proxy's only HTTP knowledge: it opens a
// console session against a running `nrflo_server serve` (service-token
// authenticated) and then speaks the console tool routes with the session's
// own bearer for the rest of the process lifetime. It holds no DB pool or
// orchestrator — every tool call is a REST request into the live server.
//
// The stdio loop dispatches every request in its own goroutine
// (runMCPStdioLoopWithCancel), so all mutable session state is guarded by mu.
type nrfloHTTPClient struct {
	base           string // e.g. http://127.0.0.1:6587
	serviceToken   string // NRFLO_MCP_TOKEN — used only to open/reopen the console session
	defaultProject string // NRFLO_PROJECT; falls back to cwd match, then the global project
	hc             *http.Client

	mu             sync.Mutex // guards the session + cwd-cache fields below
	consoleToken   string     // console session bearer — used for tools/list and tools/call
	sessionID      string
	sessionProject string // project the session is scoped to; pinned at first open
	cwdResolved    bool   // cwd→project auto-detect, cached only after a successful lookup
	cwdProjectID   string

	reopenMu sync.Mutex // single-flights the 401 session re-exchange
}

// consoleTool mirrors the wire shape of one GET /api/v1/console/tools entry.
type consoleTool struct {
	Name        string          `json:"name"`
	Description string          `json:"description"`
	InputSchema json.RawMessage `json:"input_schema"`
}

// do sends one REST request with the current session bearer — the console
// token once the session is open, else the service token.
func (c *nrfloHTTPClient) do(ctx context.Context, project, method, path string, body any, out any) error {
	c.mu.Lock()
	bearer := c.consoleToken
	c.mu.Unlock()
	if bearer == "" {
		bearer = c.serviceToken
	}
	return c.doAs(ctx, bearer, project, method, path, body, out)
}

// doAs sends one REST request with an explicit bearer. Session open/reopen
// uses it to force the SERVICE token even while a (stale) console token is
// still installed.
func (c *nrfloHTTPClient) doAs(ctx context.Context, bearer, project, method, path string, body any, out any) error {
	var rdr io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			return err
		}
		rdr = bytes.NewReader(b)
	}
	req, err := http.NewRequestWithContext(ctx, method, c.base+path, rdr)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+bearer)
	req.Header.Set("X-Project", project)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := c.hc.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close() //nolint:errcheck
	data, _ := io.ReadAll(resp.Body)
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return &httpStatusError{status: resp.StatusCode, body: fmt.Errorf("nrflo %s %s -> %d: %s", method, path, resp.StatusCode, strings.TrimSpace(string(data)))}
	}
	if out != nil && len(data) > 0 {
		if err := json.Unmarshal(data, out); err != nil {
			return fmt.Errorf("decode %s response: %w", path, err)
		}
	}
	return nil
}

// httpStatusError carries the HTTP status alongside the formatted error so
// callers (the 401 re-exchange retry) can branch on it without re-parsing.
type httpStatusError struct {
	status int
	body   error
}

func (e *httpStatusError) Error() string { return e.body.Error() }

// resolveSessionProject picks the project the console session is opened
// against, in order: cwd auto-detect (proxy's working directory matched
// against project root paths) → NRFLO_PROJECT → the hidden global project. It
// never errors. Called only from openConsoleSession — the project is pinned
// for the connection, never re-derived per tool call (the console tool schemas
// carry no `project` arg, and a console session is project-scoped server-side).
func (c *nrfloHTTPClient) resolveSessionProject(ctx context.Context) string {
	if p := c.cwdProject(ctx); p != "" {
		return p
	}
	if c.defaultProject != "" {
		return c.defaultProject
	}
	return service.GlobalProjectID
}

// sessionProjectID returns the project the open session is scoped to.
func (c *nrfloHTTPClient) sessionProjectID() string {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.sessionProject
}

// openConsoleSession exchanges the SERVICE token for a console session:
// POST /api/v1/console/sessions. The project is resolved on the first open and
// reused verbatim by any later re-exchange, so a connection never silently
// migrates to another project. On failure the error message includes the HTTP
// status/body and the resolved project id, so a misconfigured NRFLO_PROJECT/cwd
// is diagnosable from the MCP client's stderr.
func (c *nrfloHTTPClient) openConsoleSession(ctx context.Context) error {
	project := c.sessionProjectID()
	if project == "" {
		project = c.resolveSessionProject(ctx)
	}
	var res struct {
		SessionID string `json:"session_id"`
		Token     string `json:"token"`
	}
	if err := c.doAs(ctx, c.serviceToken, project, http.MethodPost, "/api/v1/console/sessions", nil, &res); err != nil {
		return fmt.Errorf("open console session for project %q: %w", project, err)
	}
	c.mu.Lock()
	c.sessionID, c.consoleToken, c.sessionProject = res.SessionID, res.Token, project
	c.mu.Unlock()
	return nil
}

// closeConsoleSession best-effort closes the console session with its own
// bearer. It builds its own short-lived context because the stdio loop's
// parent context is already cancelled by the time shutdown runs this. Errors
// are ignored: closing is a courtesy, never a reason to block process exit.
func (c *nrfloHTTPClient) closeConsoleSession() {
	c.mu.Lock()
	sid := c.sessionID
	c.mu.Unlock()
	if sid == "" {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	_ = c.do(ctx, c.sessionProjectID(), http.MethodPost, "/api/v1/console/sessions/"+url.PathEscape(sid)+"/close", nil, nil)
}

// listConsoleTools fetches the server-owned tool catalogue for this session's
// project — GET /api/v1/console/tools.
func (c *nrfloHTTPClient) listConsoleTools(ctx context.Context) ([]consoleTool, error) {
	var res struct {
		Tools []consoleTool `json:"tools"`
	}
	err := c.withSessionRetry(ctx, func() error {
		return c.do(ctx, c.sessionProjectID(), http.MethodGet, "/api/v1/console/tools", nil, &res)
	})
	if err != nil {
		return nil, err
	}
	return res.Tools, nil
}

// callConsoleTool invokes one console tool by name — POST
// /api/v1/console/tools/{name}/call with body {"arguments": <raw args>}.
func (c *nrfloHTTPClient) callConsoleTool(ctx context.Context, name string, args json.RawMessage) (string, bool, error) {
	if len(args) == 0 {
		args = json.RawMessage("{}")
	}
	var res struct {
		Output  string `json:"output"`
		IsError bool   `json:"is_error"`
	}
	err := c.withSessionRetry(ctx, func() error {
		body := map[string]any{"arguments": args}
		return c.do(ctx, c.sessionProjectID(), http.MethodPost,
			"/api/v1/console/tools/"+url.PathEscape(name)+"/call", body, &res)
	})
	if err != nil {
		return "", false, err
	}
	return res.Output, res.IsError, nil
}

// withSessionRetry runs call once; if it fails with HTTP 401 (the console
// session was swept idle server-side, ConsoleService.SweepIdle), it
// re-exchanges the service token for a fresh console session and retries call
// exactly once — never loops.
func (c *nrfloHTTPClient) withSessionRetry(ctx context.Context, call func() error) error {
	c.mu.Lock()
	used := c.consoleToken
	c.mu.Unlock()

	err := call()
	var statusErr *httpStatusError
	if err == nil || !errors.As(err, &statusErr) || statusErr.status != http.StatusUnauthorized {
		return err
	}
	if err := c.reopenSession(ctx, used); err != nil {
		return err
	}
	return call()
}

// reopenSession re-exchanges the service token for a fresh console session,
// single-flighted: concurrent 401s (every stdio request runs in its own
// goroutine) would otherwise each open a session and leak all but the last.
// If another goroutine already replaced the token that failed, its session is
// reused and the caller simply retries.
func (c *nrfloHTTPClient) reopenSession(ctx context.Context, staleToken string) error {
	c.reopenMu.Lock()
	defer c.reopenMu.Unlock()
	c.mu.Lock()
	current := c.consoleToken
	c.mu.Unlock()
	if current != staleToken {
		return nil // another goroutine already re-exchanged — retry with its token
	}
	return c.openConsoleSession(ctx)
}
