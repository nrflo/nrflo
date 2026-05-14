package api

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

// ── Project-scoped kill-interactive ──────────────────────────────────────────

// TestHandleKillInteractiveProject_MissingProjectID verifies 400 when project ID absent.
func TestHandleKillInteractiveProject_MissingProjectID(t *testing.T) {
	s := &Server{}
	req := httptest.NewRequest(http.MethodPost, "/api/v1/projects//workflow/kill-interactive",
		strings.NewReader(`{"workflow":"test","session_id":"sess-1"}`))
	rr := httptest.NewRecorder()
	s.handleKillInteractiveProject(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", rr.Code)
	}
	assertErrorContains(t, rr, "project ID required")
}

// TestHandleKillInteractiveProject_OrchestratorNil verifies 503.
func TestHandleKillInteractiveProject_OrchestratorNil(t *testing.T) {
	s := &Server{orchestrator: nil}
	req := httptest.NewRequest(http.MethodPost, "/api/v1/projects/proj-1/workflow/kill-interactive",
		strings.NewReader(`{"workflow":"test","session_id":"sess-1"}`))
	req.SetPathValue("id", "proj-1")
	rr := httptest.NewRecorder()
	s.handleKillInteractiveProject(rr, req)

	if rr.Code != http.StatusServiceUnavailable {
		t.Errorf("status = %d, want 503", rr.Code)
	}
	assertErrorContains(t, rr, "orchestrator not available")
}

// TestHandleKillInteractiveProject_InvalidBody verifies 400 for malformed JSON.
func TestHandleKillInteractiveProject_InvalidBody(t *testing.T) {
	s := newTakeControlServer(t)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/projects/proj-1/workflow/kill-interactive",
		strings.NewReader("{bad json"))
	req.SetPathValue("id", "proj-1")
	rr := httptest.NewRecorder()
	s.handleKillInteractiveProject(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", rr.Code)
	}
	assertErrorContains(t, rr, "invalid request body")
}

// TestHandleKillInteractiveProject_MissingWorkflow verifies 400 when workflow omitted.
func TestHandleKillInteractiveProject_MissingWorkflow(t *testing.T) {
	s := newTakeControlServer(t)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/projects/proj-1/workflow/kill-interactive",
		strings.NewReader(`{"session_id":"sess-1"}`))
	req.SetPathValue("id", "proj-1")
	rr := httptest.NewRecorder()
	s.handleKillInteractiveProject(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", rr.Code)
	}
	assertErrorContains(t, rr, "workflow name is required")
}

// TestHandleKillInteractiveProject_MissingSessionID verifies 400 when session_id omitted.
func TestHandleKillInteractiveProject_MissingSessionID(t *testing.T) {
	s := newTakeControlServer(t)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/projects/proj-1/workflow/kill-interactive",
		strings.NewReader(`{"workflow":"test"}`))
	req.SetPathValue("id", "proj-1")
	rr := httptest.NewRecorder()
	s.handleKillInteractiveProject(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", rr.Code)
	}
	assertErrorContains(t, rr, "session_id is required")
}

// TestHandleKillInteractiveProject_UnknownSession verifies 404 for nonexistent session.
func TestHandleKillInteractiveProject_UnknownSession(t *testing.T) {
	s := newTakeControlServer(t)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/projects/proj-1/workflow/kill-interactive",
		strings.NewReader(`{"workflow":"test","session_id":"no-such-sess-proj"}`))
	req.SetPathValue("id", "proj-1")
	rr := httptest.NewRecorder()
	s.handleKillInteractiveProject(rr, req)

	if rr.Code != http.StatusNotFound {
		t.Errorf("status = %d, want 404", rr.Code)
	}
}

// TestHandleKillInteractiveProject_SessionNotInInteractive verifies 400 when
// session is not in user_interactive status.
func TestHandleKillInteractiveProject_SessionNotInInteractive(t *testing.T) {
	s := newTakeControlServer(t)
	now := time.Now().UTC().Format(time.RFC3339Nano)
	if _, err := s.pool.Exec(`INSERT OR IGNORE INTO projects (id, name, created_at, updated_at) VALUES ('proj', 'P', ?, ?)`, now, now); err != nil {
		t.Fatalf("seed project: %v", err)
	}
	if _, err := s.pool.Exec(`INSERT OR IGNORE INTO workflows (project_id, id, description, scope_type, created_at, updated_at) VALUES ('proj','wf2','W','project',?,?)`, now, now); err != nil {
		t.Fatalf("seed workflow: %v", err)
	}
	wfiID := "wfi-proj-kill-ni"
	if _, err := s.pool.Exec(`INSERT INTO workflow_instances (id, project_id, ticket_id, workflow_id, status, scope_type, findings, created_at, updated_at) VALUES (?, 'proj', '', 'wf2', 'active', 'project', '{}', ?, ?)`, wfiID, now, now); err != nil {
		t.Fatalf("seed wfi: %v", err)
	}
	sessID := "sess-proj-not-interactive"
	if _, err := s.pool.Exec(`
		INSERT INTO agent_sessions (id, project_id, ticket_id, workflow_instance_id, phase, agent_type, status, restart_count, started_at, created_at, updated_at)
		VALUES (?, 'proj', '', ?, 'phase', 'agent', 'running', 0, ?, ?, ?)`,
		sessID, wfiID, now, now, now); err != nil {
		t.Fatalf("seed session: %v", err)
	}

	req := httptest.NewRequest(http.MethodPost, "/api/v1/projects/proj/workflow/kill-interactive",
		strings.NewReader(`{"workflow":"wf2","session_id":"`+sessID+`"}`))
	req.SetPathValue("id", "proj")
	rr := httptest.NewRecorder()
	s.handleKillInteractiveProject(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400 (not user_interactive)", rr.Code)
	}
}
