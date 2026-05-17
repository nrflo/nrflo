package api

import (
	"database/sql"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

// insertInteractiveSessionForKill inserts an agent_sessions row with status=user_interactive
// for handler tests that exercise KillInteractive.
func insertInteractiveSessionForKill(t *testing.T, s *Server, wfiID, projectID, ticketID, sessionID string) {
	t.Helper()
	now := time.Now().UTC().Format(time.RFC3339Nano)
	_, err := s.pool.Exec(`
		INSERT INTO agent_sessions
			(id, project_id, ticket_id, workflow_instance_id, phase, agent_type,
			 model_id, status, result, result_reason, pid,
			 context_left, ancestor_session_id, spawn_command, prompt,
			 restart_count, started_at, ended_at, created_at, updated_at)
		VALUES (?, ?, ?, ?, 'phase', 'agent',
			?, 'user_interactive', NULL, NULL, NULL,
			NULL, NULL, NULL, NULL,
			0, ?, NULL, ?, ?)`,
		sessionID, projectID, ticketID, wfiID,
		sql.NullString{String: "claude:sonnet", Valid: true},
		now, now, now,
	)
	if err != nil {
		t.Fatalf("insertInteractiveSessionForKill: %v", err)
	}
}

// ── Ticket-scoped kill-interactive ───────────────────────────────────────────

// TestHandleKillInteractive_MissingProject verifies 400 when ?project= is absent.
func TestHandleKillInteractive_MissingProject(t *testing.T) {
	s := &Server{}
	req := httptest.NewRequest(http.MethodPost, "/api/v1/tickets/TKT-1/workflow/kill-interactive",
		strings.NewReader(`{"workflow":"test","session_id":"sess-1"}`))
	req.SetPathValue("id", "TKT-1")
	rr := httptest.NewRecorder()
	s.handleKillInteractive(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", rr.Code)
	}
	assertErrorContains(t, rr, "X-Project")
}

// TestHandleKillInteractive_MissingTicketID verifies 400 when ticket ID is absent.
func TestHandleKillInteractive_MissingTicketID(t *testing.T) {
	s := &Server{}
	req := httptest.NewRequest(http.MethodPost,
		withProject("/api/v1/tickets//workflow/kill-interactive", "proj"),
		strings.NewReader(`{"workflow":"test","session_id":"sess-1"}`))
	rr := httptest.NewRecorder()
	s.handleKillInteractive(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", rr.Code)
	}
	assertErrorContains(t, rr, "ticket ID")
}

// TestHandleKillInteractive_OrchestratorNil verifies 503 when orchestrator is nil.
func TestHandleKillInteractive_OrchestratorNil(t *testing.T) {
	s := &Server{orchestrator: nil}
	req := httptest.NewRequest(http.MethodPost,
		withProject("/api/v1/tickets/TKT-1/workflow/kill-interactive", "proj"),
		strings.NewReader(`{"workflow":"test","session_id":"sess-1"}`))
	req.SetPathValue("id", "TKT-1")
	rr := httptest.NewRecorder()
	s.handleKillInteractive(rr, req)

	if rr.Code != http.StatusServiceUnavailable {
		t.Errorf("status = %d, want 503", rr.Code)
	}
	assertErrorContains(t, rr, "orchestrator not available")
}

// TestHandleKillInteractive_InvalidBody verifies 400 for malformed JSON.
func TestHandleKillInteractive_InvalidBody(t *testing.T) {
	s := newTakeControlServer(t)
	req := httptest.NewRequest(http.MethodPost,
		withProject("/api/v1/tickets/TKT-1/workflow/kill-interactive", "proj"),
		strings.NewReader("{bad json"))
	req.SetPathValue("id", "TKT-1")
	rr := httptest.NewRecorder()
	s.handleKillInteractive(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", rr.Code)
	}
	assertErrorContains(t, rr, "invalid request body")
}

// TestHandleKillInteractive_MissingWorkflow verifies 400 when workflow is omitted.
func TestHandleKillInteractive_MissingWorkflow(t *testing.T) {
	s := newTakeControlServer(t)
	req := httptest.NewRequest(http.MethodPost,
		withProject("/api/v1/tickets/TKT-1/workflow/kill-interactive", "proj"),
		strings.NewReader(`{"session_id":"sess-1"}`))
	req.SetPathValue("id", "TKT-1")
	rr := httptest.NewRecorder()
	s.handleKillInteractive(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", rr.Code)
	}
	assertErrorContains(t, rr, "workflow name is required")
}

// TestHandleKillInteractive_MissingSessionID verifies 400 when session_id is omitted.
func TestHandleKillInteractive_MissingSessionID(t *testing.T) {
	s := newTakeControlServer(t)
	req := httptest.NewRequest(http.MethodPost,
		withProject("/api/v1/tickets/TKT-1/workflow/kill-interactive", "proj"),
		strings.NewReader(`{"workflow":"test"}`))
	req.SetPathValue("id", "TKT-1")
	rr := httptest.NewRecorder()
	s.handleKillInteractive(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", rr.Code)
	}
	assertErrorContains(t, rr, "session_id is required")
}

// TestHandleKillInteractive_UnknownSession verifies 404 for a nonexistent session_id.
func TestHandleKillInteractive_UnknownSession(t *testing.T) {
	s := newTakeControlServer(t)
	req := httptest.NewRequest(http.MethodPost,
		withProject("/api/v1/tickets/TKT-NONE/workflow/kill-interactive", "proj"),
		strings.NewReader(`{"workflow":"test","session_id":"nonexistent-sess-xyz"}`))
	req.SetPathValue("id", "TKT-NONE")
	rr := httptest.NewRecorder()
	s.handleKillInteractive(rr, req)

	if rr.Code != http.StatusNotFound {
		t.Errorf("status = %d, want 404", rr.Code)
	}
}

// TestHandleKillInteractive_SessionNotInInteractive verifies 400 when session is
// not in user_interactive status.
func TestHandleKillInteractive_SessionNotInInteractive(t *testing.T) {
	s := newTakeControlServer(t)
	now := time.Now().UTC().Format(time.RFC3339Nano)
	if _, err := s.pool.Exec(`INSERT OR IGNORE INTO projects (id, name, created_at, updated_at) VALUES ('proj', 'P', ?, ?)`, now, now); err != nil {
		t.Fatalf("seed project: %v", err)
	}
	if _, err := s.pool.Exec(`INSERT OR IGNORE INTO workflows (project_id, id, description, scope_type, created_at, updated_at) VALUES ('proj','wf','W','ticket',?,?)`, now, now); err != nil {
		t.Fatalf("seed workflow: %v", err)
	}
	wfiID := "wfi-kill-ni"
	if _, err := s.pool.Exec(`INSERT INTO workflow_instances (id, project_id, ticket_id, workflow_id, status, scope_type, created_at, updated_at) VALUES (?, 'proj', 'TKT-NI', 'wf', 'active', 'ticket', ?, ?)`, wfiID, now, now); err != nil {
		t.Fatalf("seed wfi: %v", err)
	}
	sessID := "sess-not-interactive"
	if _, err := s.pool.Exec(`
		INSERT INTO agent_sessions (id, project_id, ticket_id, workflow_instance_id, phase, agent_type, status, restart_count, started_at, created_at, updated_at)
		VALUES (?, 'proj', 'TKT-NI', ?, 'phase', 'agent', 'running', 0, ?, ?, ?)`,
		sessID, wfiID, now, now, now); err != nil {
		t.Fatalf("seed session: %v", err)
	}

	req := httptest.NewRequest(http.MethodPost,
		withProject("/api/v1/tickets/TKT-NI/workflow/kill-interactive", "proj"),
		strings.NewReader(`{"workflow":"wf","session_id":"`+sessID+`"}`))
	req.SetPathValue("id", "TKT-NI")
	rr := httptest.NewRecorder()
	s.handleKillInteractive(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400 (not user_interactive)", rr.Code)
	}
}
