package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"be/internal/clock"
	"be/internal/orchestrator"
	"be/internal/ws"

	"be/internal/db"
)

// newTakeControlServer creates a Server with a real orchestrator pointed at
// a temporary DB for take-control handler tests.
func newTakeControlServer(t *testing.T) *Server {
	t.Helper()
	dbPath := filepath.Join(t.TempDir(), "tc_handler_test.db")
	if err := apiCopyTemplateDB(dbPath); err != nil {
		t.Fatalf("copy template DB: %v", err)
	}
	pool, err := db.OpenPoolExisting(dbPath, db.DefaultPoolConfig())
	if err != nil {
		t.Fatalf("failed to create pool: %v", err)
	}
	t.Cleanup(func() { pool.Close() })

	hub := ws.NewHub(clock.Real())
	go hub.Run()
	t.Cleanup(hub.Stop)

	orch := orchestrator.New(dbPath, hub, clock.Real(), nil, "")
	return &Server{orchestrator: orch, clock: clock.Real(), pool: pool}
}

// withProject returns a URL with ?project=<id> query param, which is how
// handler tests pass the project ID without going through projectMiddleware.
func withProject(path, projectID string) string {
	if strings.Contains(path, "?") {
		return path + "&project=" + projectID
	}
	return path + "?project=" + projectID
}

// guardCase describes one guard-ladder row for the workflow-control handlers.
// server selects which Server fixture to invoke:
//   - "": &Server{} (no orchestrator, no pool) — for the project/ID guards
//   - "nilorch": &Server{orchestrator: nil} — same as "" but explicit
//   - "real": newTakeControlServer(t) — for body-validation and lookup guards
type guardCase struct {
	name       string
	server     string
	withProj   bool // ticket handlers: append ?project=
	setPath    bool // set r.PathValue("id")
	body       string
	wantStatus int
	wantErr    string // substring expected in the "error" field ("" = skip check)
}

func runGuardCase(t *testing.T, tc guardCase, pathBase, pathID string,
	handler func(s *Server) http.HandlerFunc) {
	t.Helper()

	var s *Server
	switch tc.server {
	case "real":
		s = newTakeControlServer(t)
	default:
		s = &Server{}
	}

	path := pathBase
	if tc.withProj {
		path = withProject(pathBase, "proj")
	}
	req := httptest.NewRequest(http.MethodPost, path, strings.NewReader(tc.body))
	if tc.setPath {
		req.SetPathValue("id", pathID)
	}
	rr := httptest.NewRecorder()
	handler(s)(rr, req)

	if rr.Code != tc.wantStatus {
		t.Errorf("status = %d, want %d", rr.Code, tc.wantStatus)
	}
	if tc.wantErr != "" {
		assertErrorContains(t, rr, tc.wantErr)
	}
}

// TestHandleTakeControl_Guards exercises the take-control (ticket-scoped) guard ladder.
func TestHandleTakeControl_Guards(t *testing.T) {
	cases := []guardCase{
		{"missing_project", "", false, true, `{"workflow":"test","session_id":"sess-1"}`, http.StatusBadRequest, "X-Project"},
		{"missing_ticket_id", "", true, false, `{"workflow":"test","session_id":"sess-1"}`, http.StatusBadRequest, "ticket ID"},
		{"orchestrator_nil", "", true, true, `{"workflow":"test","session_id":"sess-1"}`, http.StatusServiceUnavailable, "orchestrator not available"},
		{"missing_workflow", "real", true, true, `{"session_id":"sess-1"}`, http.StatusBadRequest, "workflow name is required"},
		{"missing_session_id", "real", true, true, `{"workflow":"test"}`, http.StatusBadRequest, "session_id is required"},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			runGuardCase(t, tc, "/api/v1/tickets/TKT-1/workflow/take-control", "TKT-1",
				func(s *Server) http.HandlerFunc { return s.handleTakeControl })
		})
	}
}

// TestHandleTakeControl_InvalidBody verifies 400 for malformed JSON.
func TestHandleTakeControl_InvalidBody(t *testing.T) {
	s := newTakeControlServer(t)
	req := httptest.NewRequest(http.MethodPost,
		withProject("/api/v1/tickets/TKT-1/workflow/take-control", "proj"),
		strings.NewReader("{bad json"))
	req.SetPathValue("id", "TKT-1")
	rr := httptest.NewRecorder()
	s.handleTakeControl(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", rr.Code)
	}
	assertErrorContains(t, rr, "invalid request body")
}

// TestHandleTakeControl_NoRunningOrchestration verifies 404 when no workflow is running.
func TestHandleTakeControl_NoRunningOrchestration(t *testing.T) {
	s := newTakeControlServer(t)
	// No project/ticket/WFI set up in DB — TakeControl returns "workflow not found" → 404.
	body := `{"workflow":"test","session_id":"sess-not-running"}`
	req := httptest.NewRequest(http.MethodPost,
		withProject("/api/v1/tickets/TKT-NONE/workflow/take-control", "proj-none"),
		strings.NewReader(body))
	req.SetPathValue("id", "TKT-NONE")
	rr := httptest.NewRecorder()
	s.handleTakeControl(rr, req)

	if rr.Code != http.StatusNotFound {
		t.Errorf("status = %d, want 404 (workflow not found)", rr.Code)
	}
}

// TestHandleExitInteractive_Guards exercises the exit-interactive (ticket-scoped) guard ladder.
func TestHandleExitInteractive_Guards(t *testing.T) {
	cases := []guardCase{
		{"missing_project", "", false, true, `{"workflow":"test","session_id":"sess-1"}`, http.StatusBadRequest, "X-Project"},
		{"missing_ticket_id", "", true, false, `{"workflow":"test","session_id":"sess-1"}`, http.StatusBadRequest, "ticket ID"},
		{"orchestrator_nil", "", true, true, `{"workflow":"test","session_id":"sess-1"}`, http.StatusServiceUnavailable, "orchestrator not available"},
		{"missing_workflow", "real", true, true, `{"session_id":"sess-1"}`, http.StatusBadRequest, "workflow name is required"},
		{"missing_session_id", "real", true, true, `{"workflow":"test"}`, http.StatusBadRequest, "session_id is required"},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			runGuardCase(t, tc, "/api/v1/tickets/TKT-1/workflow/exit-interactive", "TKT-1",
				func(s *Server) http.HandlerFunc { return s.handleExitInteractive })
		})
	}
}

// TestHandleExitInteractive_SessionNotFound verifies 400 when session is not found.
func TestHandleExitInteractive_SessionNotFound(t *testing.T) {
	s := newTakeControlServer(t)
	body := `{"workflow":"test","session_id":"nonexistent-session-xyz"}`
	req := httptest.NewRequest(http.MethodPost,
		withProject("/api/v1/tickets/TKT-1/workflow/exit-interactive", "proj"),
		strings.NewReader(body))
	req.SetPathValue("id", "TKT-1")
	rr := httptest.NewRecorder()
	s.handleExitInteractive(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", rr.Code)
	}
}

// TestHandleTakeControlProject_Guards exercises the take-control (project-scoped) guard ladder.
func TestHandleTakeControlProject_Guards(t *testing.T) {
	cases := []guardCase{
		{"missing_project_id", "", false, false, `{"workflow":"test","session_id":"sess-1","instance_id":"inst-1"}`, http.StatusBadRequest, "project ID required"},
		{"orchestrator_nil", "", false, true, `{"workflow":"test","session_id":"sess-1","instance_id":"inst-1"}`, http.StatusServiceUnavailable, ""},
		{"missing_workflow", "real", false, true, `{"session_id":"sess-1","instance_id":"inst-1"}`, http.StatusBadRequest, "workflow name is required"},
		{"missing_session_id", "real", false, true, `{"workflow":"test","instance_id":"inst-1"}`, http.StatusBadRequest, "session_id is required"},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			runGuardCase(t, tc, "/api/v1/projects/proj-1/workflow/take-control", "proj-1",
				func(s *Server) http.HandlerFunc { return s.handleTakeControlProject })
		})
	}
}

// TestHandleTakeControlProject_NoRunningOrchestration verifies 404 when
// no orchestration is running for the given instance.
func TestHandleTakeControlProject_NoRunningOrchestration(t *testing.T) {
	s := newTakeControlServer(t)
	body := `{"workflow":"test","session_id":"sess-1","instance_id":"nonexistent-instance"}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/projects/proj-1/workflow/take-control",
		strings.NewReader(body))
	req.SetPathValue("id", "proj-1")
	rr := httptest.NewRecorder()
	s.handleTakeControlProject(rr, req)

	if rr.Code != http.StatusNotFound {
		t.Errorf("status = %d, want 404", rr.Code)
	}
}

// TestHandleExitInteractiveProject_Guards exercises the exit-interactive (project-scoped) guard ladder.
func TestHandleExitInteractiveProject_Guards(t *testing.T) {
	cases := []guardCase{
		{"missing_project_id", "", false, false, `{"workflow":"test","session_id":"sess-1"}`, http.StatusBadRequest, "project ID required"},
		{"orchestrator_nil", "", false, true, `{"workflow":"test","session_id":"sess-1"}`, http.StatusServiceUnavailable, ""},
		{"missing_workflow", "real", false, true, `{"session_id":"sess-1"}`, http.StatusBadRequest, "workflow name is required"},
		{"missing_session_id", "real", false, true, `{"workflow":"test"}`, http.StatusBadRequest, "session_id is required"},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			runGuardCase(t, tc, "/api/v1/projects/proj-1/workflow/exit-interactive", "proj-1",
				func(s *Server) http.HandlerFunc { return s.handleExitInteractiveProject })
		})
	}
}

// TestHandleExitInteractiveProject_SessionNotFound verifies 400 for missing session.
func TestHandleExitInteractiveProject_SessionNotFound(t *testing.T) {
	s := newTakeControlServer(t)
	body := `{"workflow":"test","session_id":"no-such-session-proj"}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/projects/proj-1/workflow/exit-interactive",
		strings.NewReader(body))
	req.SetPathValue("id", "proj-1")
	rr := httptest.NewRecorder()
	s.handleExitInteractiveProject(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", rr.Code)
	}
}

// assertErrorContains is a helper that verifies the response body contains the
// expected substring in the "error" field.
func assertErrorContains(t *testing.T, rr *httptest.ResponseRecorder, want string) {
	t.Helper()
	var body map[string]string
	if err := json.NewDecoder(rr.Body).Decode(&body); err != nil {
		t.Fatalf("failed to decode error response: %v", err)
	}
	errMsg := body["error"]
	if !strings.Contains(errMsg, want) {
		t.Errorf("error = %q, want to contain %q", errMsg, want)
	}
}
