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

	orch := orchestrator.New(dbPath, hub, clock.Real(), nil)
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

// TestHandleTakeControl_MissingProject verifies that omitting ?project= returns 400.
func TestHandleTakeControl_MissingProject(t *testing.T) {
	s := &Server{} // no orchestrator
	body := `{"workflow":"test","session_id":"sess-1"}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/tickets/TKT-1/workflow/take-control",
		strings.NewReader(body))
	req.SetPathValue("id", "TKT-1")
	rr := httptest.NewRecorder()
	s.handleTakeControl(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", rr.Code)
	}
	assertErrorContains(t, rr, "X-Project")
}

// TestHandleTakeControl_MissingTicketID verifies that a missing ticket ID returns 400.
func TestHandleTakeControl_MissingTicketID(t *testing.T) {
	s := &Server{} // orchestrator nil — check happens after projectID
	body := `{"workflow":"test","session_id":"sess-1"}`
	// No path value "id" set → extractID returns ""
	req := httptest.NewRequest(http.MethodPost,
		withProject("/api/v1/tickets//workflow/take-control", "proj"),
		strings.NewReader(body))
	rr := httptest.NewRecorder()
	s.handleTakeControl(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", rr.Code)
	}
	assertErrorContains(t, rr, "ticket ID")
}

// TestHandleTakeControl_OrchestratorNil verifies 503 when orchestrator is not set.
func TestHandleTakeControl_OrchestratorNil(t *testing.T) {
	s := &Server{orchestrator: nil}
	body := `{"workflow":"test","session_id":"sess-1"}`
	req := httptest.NewRequest(http.MethodPost,
		withProject("/api/v1/tickets/TKT-1/workflow/take-control", "proj"),
		strings.NewReader(body))
	req.SetPathValue("id", "TKT-1")
	rr := httptest.NewRecorder()
	s.handleTakeControl(rr, req)

	if rr.Code != http.StatusServiceUnavailable {
		t.Errorf("status = %d, want 503", rr.Code)
	}
	assertErrorContains(t, rr, "orchestrator not available")
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

// TestHandleTakeControl_MissingWorkflow verifies 400 when workflow is not in body.
func TestHandleTakeControl_MissingWorkflow(t *testing.T) {
	s := newTakeControlServer(t)
	body := `{"session_id":"sess-1"}`
	req := httptest.NewRequest(http.MethodPost,
		withProject("/api/v1/tickets/TKT-1/workflow/take-control", "proj"),
		strings.NewReader(body))
	req.SetPathValue("id", "TKT-1")
	rr := httptest.NewRecorder()
	s.handleTakeControl(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", rr.Code)
	}
	assertErrorContains(t, rr, "workflow name is required")
}

// TestHandleTakeControl_MissingSessionID verifies 400 when session_id is omitted.
func TestHandleTakeControl_MissingSessionID(t *testing.T) {
	s := newTakeControlServer(t)
	body := `{"workflow":"test"}`
	req := httptest.NewRequest(http.MethodPost,
		withProject("/api/v1/tickets/TKT-1/workflow/take-control", "proj"),
		strings.NewReader(body))
	req.SetPathValue("id", "TKT-1")
	rr := httptest.NewRecorder()
	s.handleTakeControl(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", rr.Code)
	}
	assertErrorContains(t, rr, "session_id is required")
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

// TestHandleExitInteractive_MissingProject verifies 400 for missing ?project=.
func TestHandleExitInteractive_MissingProject(t *testing.T) {
	s := &Server{}
	body := `{"workflow":"test","session_id":"sess-1"}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/tickets/TKT-1/workflow/exit-interactive",
		strings.NewReader(body))
	req.SetPathValue("id", "TKT-1")
	rr := httptest.NewRecorder()
	s.handleExitInteractive(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", rr.Code)
	}
	assertErrorContains(t, rr, "X-Project")
}

// TestHandleExitInteractive_OrchestratorNil verifies 503 when orchestrator is nil.
func TestHandleExitInteractive_OrchestratorNil(t *testing.T) {
	s := &Server{orchestrator: nil}
	body := `{"workflow":"test","session_id":"sess-1"}`
	req := httptest.NewRequest(http.MethodPost,
		withProject("/api/v1/tickets/TKT-1/workflow/exit-interactive", "proj"),
		strings.NewReader(body))
	req.SetPathValue("id", "TKT-1")
	rr := httptest.NewRecorder()
	s.handleExitInteractive(rr, req)

	if rr.Code != http.StatusServiceUnavailable {
		t.Errorf("status = %d, want 503", rr.Code)
	}
	assertErrorContains(t, rr, "orchestrator not available")
}

// TestHandleExitInteractive_MissingWorkflow verifies 400 when workflow is omitted.
func TestHandleExitInteractive_MissingWorkflow(t *testing.T) {
	s := newTakeControlServer(t)
	body := `{"session_id":"sess-1"}`
	req := httptest.NewRequest(http.MethodPost,
		withProject("/api/v1/tickets/TKT-1/workflow/exit-interactive", "proj"),
		strings.NewReader(body))
	req.SetPathValue("id", "TKT-1")
	rr := httptest.NewRecorder()
	s.handleExitInteractive(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", rr.Code)
	}
	assertErrorContains(t, rr, "workflow name is required")
}

// TestHandleExitInteractive_MissingSessionID verifies 400 when session_id is omitted.
func TestHandleExitInteractive_MissingSessionID(t *testing.T) {
	s := newTakeControlServer(t)
	body := `{"workflow":"test"}`
	req := httptest.NewRequest(http.MethodPost,
		withProject("/api/v1/tickets/TKT-1/workflow/exit-interactive", "proj"),
		strings.NewReader(body))
	req.SetPathValue("id", "TKT-1")
	rr := httptest.NewRecorder()
	s.handleExitInteractive(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", rr.Code)
	}
	assertErrorContains(t, rr, "session_id is required")
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

// TestHandleTakeControlProject_MissingProjectID verifies 400 when project ID is missing.
func TestHandleTakeControlProject_MissingProjectID(t *testing.T) {
	s := &Server{}
	body := `{"workflow":"test","session_id":"sess-1","instance_id":"inst-1"}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/projects//workflow/take-control",
		strings.NewReader(body))
	// no path value set → r.PathValue("id") returns ""
	rr := httptest.NewRecorder()
	s.handleTakeControlProject(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", rr.Code)
	}
	assertErrorContains(t, rr, "project ID required")
}

// TestHandleTakeControlProject_OrchestratorNil verifies 503.
func TestHandleTakeControlProject_OrchestratorNil(t *testing.T) {
	s := &Server{orchestrator: nil}
	body := `{"workflow":"test","session_id":"sess-1","instance_id":"inst-1"}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/projects/proj-1/workflow/take-control",
		strings.NewReader(body))
	req.SetPathValue("id", "proj-1")
	rr := httptest.NewRecorder()
	s.handleTakeControlProject(rr, req)

	if rr.Code != http.StatusServiceUnavailable {
		t.Errorf("status = %d, want 503", rr.Code)
	}
}

// TestHandleTakeControlProject_MissingWorkflow verifies 400 when workflow is missing.
func TestHandleTakeControlProject_MissingWorkflow(t *testing.T) {
	s := newTakeControlServer(t)
	body := `{"session_id":"sess-1","instance_id":"inst-1"}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/projects/proj-1/workflow/take-control",
		strings.NewReader(body))
	req.SetPathValue("id", "proj-1")
	rr := httptest.NewRecorder()
	s.handleTakeControlProject(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", rr.Code)
	}
	assertErrorContains(t, rr, "workflow name is required")
}

// TestHandleTakeControlProject_MissingSessionID verifies 400 when session_id is missing.
func TestHandleTakeControlProject_MissingSessionID(t *testing.T) {
	s := newTakeControlServer(t)
	body := `{"workflow":"test","instance_id":"inst-1"}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/projects/proj-1/workflow/take-control",
		strings.NewReader(body))
	req.SetPathValue("id", "proj-1")
	rr := httptest.NewRecorder()
	s.handleTakeControlProject(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", rr.Code)
	}
	assertErrorContains(t, rr, "session_id is required")
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

// TestHandleExitInteractiveProject_MissingProjectID verifies 400 for missing project ID.
func TestHandleExitInteractiveProject_MissingProjectID(t *testing.T) {
	s := &Server{}
	body := `{"workflow":"test","session_id":"sess-1"}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/projects//workflow/exit-interactive",
		strings.NewReader(body))
	rr := httptest.NewRecorder()
	s.handleExitInteractiveProject(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", rr.Code)
	}
	assertErrorContains(t, rr, "project ID required")
}

// TestHandleExitInteractiveProject_OrchestratorNil verifies 503.
func TestHandleExitInteractiveProject_OrchestratorNil(t *testing.T) {
	s := &Server{orchestrator: nil}
	body := `{"workflow":"test","session_id":"sess-1"}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/projects/proj-1/workflow/exit-interactive",
		strings.NewReader(body))
	req.SetPathValue("id", "proj-1")
	rr := httptest.NewRecorder()
	s.handleExitInteractiveProject(rr, req)

	if rr.Code != http.StatusServiceUnavailable {
		t.Errorf("status = %d, want 503", rr.Code)
	}
}

// TestHandleExitInteractiveProject_MissingWorkflow verifies 400 for missing workflow.
func TestHandleExitInteractiveProject_MissingWorkflow(t *testing.T) {
	s := newTakeControlServer(t)
	body := `{"session_id":"sess-1"}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/projects/proj-1/workflow/exit-interactive",
		strings.NewReader(body))
	req.SetPathValue("id", "proj-1")
	rr := httptest.NewRecorder()
	s.handleExitInteractiveProject(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", rr.Code)
	}
	assertErrorContains(t, rr, "workflow name is required")
}

// TestHandleExitInteractiveProject_MissingSessionID verifies 400 for missing session_id.
func TestHandleExitInteractiveProject_MissingSessionID(t *testing.T) {
	s := newTakeControlServer(t)
	body := `{"workflow":"test"}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/projects/proj-1/workflow/exit-interactive",
		strings.NewReader(body))
	req.SetPathValue("id", "proj-1")
	rr := httptest.NewRecorder()
	s.handleExitInteractiveProject(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", rr.Code)
	}
	assertErrorContains(t, rr, "session_id is required")
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
