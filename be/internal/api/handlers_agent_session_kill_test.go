package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"be/internal/db"
)

// killTestFixture seeds the minimum data needed for kill handler tests.
type killTestFixture struct {
	projID    string
	wfiID     string
	sessionID string
}

func seedKillFixture(t *testing.T, pool *db.Pool) *killTestFixture {
	t.Helper()
	now := "2025-07-01T09:00:00Z"
	f := &killTestFixture{
		projID:    "kill-proj",
		wfiID:     "kill-wfi",
		sessionID: "kill-sess",
	}
	stmts := []struct {
		sql  string
		args []interface{}
	}{
		{`INSERT INTO projects (id, name, created_at, updated_at) VALUES (?, 'KP', ?, ?)`, []interface{}{f.projID, now, now}},
		{`INSERT INTO workflows (project_id, id, description, scope_type, created_at, updated_at) VALUES (?, 'kill-wf', '', 'project', ?, ?)`, []interface{}{f.projID, now, now}},
		{`INSERT INTO workflow_instances (id, project_id, ticket_id, workflow_id, status, scope_type, created_at, updated_at) VALUES (?, ?, '', 'kill-wf', 'active', 'project', ?, ?)`, []interface{}{f.wfiID, f.projID, now, now}},
		{`INSERT INTO agent_sessions (id, project_id, ticket_id, workflow_instance_id, phase, agent_type, status, pid, created_at, updated_at) VALUES (?, ?, '', ?, 'ph', 'ag', 'running', 12345, ?, ?)`, []interface{}{f.sessionID, f.projID, f.wfiID, now, now}},
	}
	for _, s := range stmts {
		if _, err := pool.Exec(s.sql, s.args...); err != nil {
			t.Fatalf("seedKillFixture: %v", err)
		}
	}
	return f
}

func insertKillSession(t *testing.T, pool *db.Pool, id, projID, wfiID, status string) {
	t.Helper()
	now := "2025-07-01T09:00:00Z"
	if _, err := pool.Exec(
		`INSERT INTO agent_sessions (id, project_id, ticket_id, workflow_instance_id, phase, agent_type, status, created_at, updated_at) VALUES (?, ?, '', ?, 'ph', 'ag', ?, ?, ?)`,
		id, projID, wfiID, status, now, now,
	); err != nil {
		t.Fatalf("insertKillSession(%s): %v", id, err)
	}
}

func TestHandleKillAgentSession_MissingProject(t *testing.T) {
	s := &Server{}
	req := httptest.NewRequest(http.MethodPost, "/api/v1/agent-sessions/sess-1/kill", nil)
	req.SetPathValue("id", "sess-1")
	rr := httptest.NewRecorder()
	s.handleKillAgentSession(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", rr.Code)
	}
	assertErrorContains(t, rr, "project")
}

func TestHandleKillAgentSession_MissingSessionID(t *testing.T) {
	s := &Server{}
	req := httptest.NewRequest(http.MethodPost,
		withProject("/api/v1/agent-sessions//kill", "some-proj"), nil)
	// No SetPathValue("id", ...) → extractID returns "".
	rr := httptest.NewRecorder()
	s.handleKillAgentSession(rr, req)

	// Missing project is checked first; "some-proj" provided so project check passes.
	// Then session ID check fires.
	if rr.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", rr.Code)
	}
}

func TestHandleKillAgentSession_NotFound(t *testing.T) {
	s := newTakeControlServer(t)
	req := httptest.NewRequest(http.MethodPost,
		withProject("/api/v1/agent-sessions/no-such-sess/kill", "proj-x"), nil)
	req.SetPathValue("id", "no-such-sess")
	rr := httptest.NewRecorder()
	s.handleKillAgentSession(rr, req)

	if rr.Code != http.StatusNotFound {
		t.Errorf("status = %d, want 404", rr.Code)
	}
}

func TestHandleKillAgentSession_CrossProject(t *testing.T) {
	s := newTakeControlServer(t)
	f := seedKillFixture(t, s.pool)

	// Request with a different project than the session's project.
	req := httptest.NewRequest(http.MethodPost,
		withProject("/api/v1/agent-sessions/"+f.sessionID+"/kill", "wrong-proj"), nil)
	req.SetPathValue("id", f.sessionID)
	rr := httptest.NewRecorder()
	s.handleKillAgentSession(rr, req)

	if rr.Code != http.StatusForbidden {
		t.Errorf("status = %d, want 403", rr.Code)
	}
	assertErrorContains(t, rr, "project")
}

func TestHandleKillAgentSession_NotAlive_Completed(t *testing.T) {
	s := newTakeControlServer(t)
	f := seedKillFixture(t, s.pool)

	// Insert a completed session (not running/user_interactive).
	insertKillSession(t, s.pool, "kill-done", f.projID, f.wfiID, "completed")

	req := httptest.NewRequest(http.MethodPost,
		withProject("/api/v1/agent-sessions/kill-done/kill", f.projID), nil)
	req.SetPathValue("id", "kill-done")
	rr := httptest.NewRecorder()
	s.handleKillAgentSession(rr, req)

	if rr.Code != http.StatusConflict {
		t.Errorf("status = %d, want 409", rr.Code)
	}
	assertErrorContains(t, rr, "not_alive")
}

func TestHandleKillAgentSession_NotAlive_Failed(t *testing.T) {
	s := newTakeControlServer(t)
	f := seedKillFixture(t, s.pool)

	insertKillSession(t, s.pool, "kill-failed", f.projID, f.wfiID, "failed")

	req := httptest.NewRequest(http.MethodPost,
		withProject("/api/v1/agent-sessions/kill-failed/kill", f.projID), nil)
	req.SetPathValue("id", "kill-failed")
	rr := httptest.NewRecorder()
	s.handleKillAgentSession(rr, req)

	if rr.Code != http.StatusConflict {
		t.Errorf("status = %d, want 409", rr.Code)
	}
}

func TestHandleKillAgentSession_HappyPath_Running(t *testing.T) {
	s := newTakeControlServer(t)
	f := seedKillFixture(t, s.pool)

	// f.sessionID has status=running; orchestrator has no active run registered,
	// so RequestTerminalSignal returns nil → handler returns 200.
	req := httptest.NewRequest(http.MethodPost,
		withProject("/api/v1/agent-sessions/"+f.sessionID+"/kill", f.projID), nil)
	req.SetPathValue("id", f.sessionID)
	rr := httptest.NewRecorder()
	s.handleKillAgentSession(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("status = %d, want 200", rr.Code)
	}

	var body map[string]string
	if err := json.NewDecoder(rr.Body).Decode(&body); err != nil {
		t.Fatalf("decode body: %v", err)
	}
	if body["status"] != "killed" {
		t.Errorf("status = %q, want killed", body["status"])
	}
	if body["session_id"] != f.sessionID {
		t.Errorf("session_id = %q, want %q", body["session_id"], f.sessionID)
	}
}

func TestHandleKillAgentSession_HappyPath_UserInteractive(t *testing.T) {
	s := newTakeControlServer(t)
	f := seedKillFixture(t, s.pool)

	insertKillSession(t, s.pool, "kill-ui", f.projID, f.wfiID, "user_interactive")

	req := httptest.NewRequest(http.MethodPost,
		withProject("/api/v1/agent-sessions/kill-ui/kill", f.projID), nil)
	req.SetPathValue("id", "kill-ui")
	rr := httptest.NewRecorder()
	s.handleKillAgentSession(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("status = %d, want 200", rr.Code)
	}

	var body map[string]string
	if err := json.NewDecoder(rr.Body).Decode(&body); err != nil {
		t.Fatalf("decode body: %v", err)
	}
	if body["status"] != "killed" {
		t.Errorf("status = %q, want killed", body["status"])
	}
	if body["session_id"] != "kill-ui" {
		t.Errorf("session_id = %q, want kill-ui", body["session_id"])
	}
}

func TestHandleKillAgentSession_StatusCodes_Table(t *testing.T) {
	t.Parallel()
	cases := []struct {
		status   string
		wantCode int
	}{
		{"completed", http.StatusConflict},
		{"failed", http.StatusConflict},
		{"timeout", http.StatusConflict},
		{"skipped", http.StatusConflict},
		{"project_completed", http.StatusConflict},
		{"interactive_completed", http.StatusConflict},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.status, func(t *testing.T) {
			t.Parallel()
			s := newTakeControlServer(t)

			now := "2025-07-02T00:00:00Z"
			projID := "tbl-proj-" + tc.status
			wfiID := "tbl-wfi-" + tc.status
			sessID := "tbl-sess-" + tc.status

			for _, q := range []struct {
				sql  string
				args []interface{}
			}{
				{`INSERT INTO projects (id, name, created_at, updated_at) VALUES (?, 'P', ?, ?)`, []interface{}{projID, now, now}},
				{`INSERT INTO workflows (project_id, id, description, scope_type, created_at, updated_at) VALUES (?, 'wf', '', 'project', ?, ?)`, []interface{}{projID, now, now}},
				{`INSERT INTO workflow_instances (id, project_id, ticket_id, workflow_id, status, scope_type, created_at, updated_at) VALUES (?, ?, '', 'wf', 'active', 'project', ?, ?)`, []interface{}{wfiID, projID, now, now}},
				{`INSERT INTO agent_sessions (id, project_id, ticket_id, workflow_instance_id, phase, agent_type, status, created_at, updated_at) VALUES (?, ?, '', ?, 'ph', 'ag', ?, ?, ?)`, []interface{}{sessID, projID, wfiID, tc.status, now, now}},
			} {
				if _, err := s.pool.Exec(q.sql, q.args...); err != nil {
					t.Fatalf("seed exec: %v", err)
				}
			}

			req := httptest.NewRequest(http.MethodPost,
				withProject("/api/v1/agent-sessions/"+sessID+"/kill", projID), nil)
			req.SetPathValue("id", sessID)
			rr := httptest.NewRecorder()
			s.handleKillAgentSession(rr, req)

			if rr.Code != tc.wantCode {
				t.Errorf("status(%q) = %d, want %d", tc.status, rr.Code, tc.wantCode)
			}
		})
	}
}
