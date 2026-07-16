package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
)

// reqLog captures the Authorization and X-Project headers of one request.
type reqLog struct {
	auth    string
	project string
}

// toolCallLog captures one tools/call request: headers, tool name, and the raw
// "arguments" body sent.
type toolCallLog struct {
	reqLog
	name string
	args json.RawMessage
}

// toolCallResp configures how the fake server answers one tool name. A zero
// value (status 0) answers 200 with the given output/isError.
type toolCallResp struct {
	status  int
	output  string
	isError bool
	errBody string
}

// fakeConsoleServer is a minimal in-process stand-in for the four console
// endpoints the external bridge speaks (POST .../sessions, POST
// .../sessions/{sid}/close, GET .../tools, POST .../tools/{name}/call) plus
// GET /api/v1/projects for cwd auto-detect, recording every request it serves.
type fakeConsoleServer struct {
	t   *testing.T
	srv *httptest.Server

	serviceToken string
	projects     []projRoot
	tools        []consoleTool
	toolResp     map[string]toolCallResp
	// unauthorizedRemaining forces the next N console-token-authed requests
	// (tools/list or tools/call) to fail with 401, regardless of token.
	unauthorizedRemaining int

	mu           sync.Mutex
	sessionSeq   int
	sessionToken string
	createReqs   []reqLog
	closeReqs    []reqLog
	listReqs     []reqLog
	callReqs     []toolCallLog
}

func newFakeConsoleServer(t *testing.T) *fakeConsoleServer {
	t.Helper()
	f := &fakeConsoleServer{t: t, serviceToken: "svc-tok", toolResp: map[string]toolCallResp{}}
	f.srv = httptest.NewServer(http.HandlerFunc(f.handle))
	t.Cleanup(f.srv.Close)
	return f
}

func (f *fakeConsoleServer) url() string { return f.srv.URL }

// openedClient builds a client against f with cwd auto-detect disabled (tests
// of cwd resolution set it up themselves) and an already-open console session.
func (f *fakeConsoleServer) openedClient(t *testing.T, project string) *nrfloHTTPClient {
	t.Helper()
	c := &nrfloHTTPClient{base: f.url(), serviceToken: f.serviceToken, defaultProject: project, hc: f.srv.Client()}
	c.cwdResolved = true
	if err := c.openConsoleSession(context.Background()); err != nil {
		t.Fatalf("openConsoleSession: %v", err)
	}
	return c
}

func (f *fakeConsoleServer) handle(w http.ResponseWriter, r *http.Request) {
	f.mu.Lock()
	defer f.mu.Unlock()
	log := reqLog{auth: r.Header.Get("Authorization"), project: r.Header.Get("X-Project")}
	switch {
	case r.Method == http.MethodPost && r.URL.Path == "/api/v1/console/sessions":
		f.createReqs = append(f.createReqs, log)
		if log.auth != "Bearer "+f.serviceToken {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		f.sessionSeq++
		f.sessionToken = fmt.Sprintf("tok-%d", f.sessionSeq)
		writeTestJSON(w, http.StatusCreated, map[string]string{
			"session_id": fmt.Sprintf("sess-%d", f.sessionSeq),
			"token":      f.sessionToken,
		})
	case r.Method == http.MethodPost && strings.HasSuffix(r.URL.Path, "/close"):
		f.closeReqs = append(f.closeReqs, log)
		w.WriteHeader(http.StatusNoContent)
	case r.Method == http.MethodGet && r.URL.Path == "/api/v1/console/tools":
		f.listReqs = append(f.listReqs, log)
		if !f.authOKLocked(log.auth) {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		// omitempty on InputSchema: a tool with no schema must send no
		// "input_schema" key at all (not a literal JSON null), matching what a
		// real server sends and exercising the client's own-schema default.
		type wireTool struct {
			Name        string          `json:"name"`
			Description string          `json:"description"`
			InputSchema json.RawMessage `json:"input_schema,omitempty"`
		}
		wire := make([]wireTool, len(f.tools))
		for i, tl := range f.tools {
			wire[i] = wireTool(tl)
		}
		writeTestJSON(w, http.StatusOK, map[string]any{"tools": wire})
	case r.Method == http.MethodPost && strings.Contains(r.URL.Path, "/console/tools/") && strings.HasSuffix(r.URL.Path, "/call"):
		name := strings.TrimSuffix(strings.TrimPrefix(r.URL.Path, "/api/v1/console/tools/"), "/call")
		raw, _ := io.ReadAll(r.Body)
		var body struct {
			Arguments json.RawMessage `json:"arguments"`
		}
		_ = json.Unmarshal(raw, &body)
		f.callReqs = append(f.callReqs, toolCallLog{reqLog: log, name: name, args: body.Arguments})
		if !f.authOKLocked(log.auth) {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		resp, ok := f.toolResp[name]
		if !ok {
			http.Error(w, "unknown tool: "+name, http.StatusNotFound)
			return
		}
		if resp.status != 0 && resp.status != http.StatusOK {
			http.Error(w, resp.errBody, resp.status)
			return
		}
		writeTestJSON(w, http.StatusOK, map[string]any{"output": resp.output, "is_error": resp.isError, "duration_ms": 1})
	case r.Method == http.MethodGet && r.URL.Path == "/api/v1/projects":
		writeTestJSON(w, http.StatusOK, map[string]any{"projects": f.projects})
	default:
		http.Error(w, "not found", http.StatusNotFound)
	}
}

// authOKLocked reports whether auth carries the current console session
// token, consuming one pending forced-401 if configured. Caller holds f.mu.
func (f *fakeConsoleServer) authOKLocked(auth string) bool {
	if f.unauthorizedRemaining > 0 {
		f.unauthorizedRemaining--
		return false
	}
	return f.sessionToken != "" && auth == "Bearer "+f.sessionToken
}

func writeTestJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}
