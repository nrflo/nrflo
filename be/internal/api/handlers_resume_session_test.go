package api

import (
	"database/sql"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"be/internal/clock"
	"be/internal/db"
	"be/internal/model"
	"be/internal/repo"
	"be/internal/ws"
)

// newResumeTestServer creates a minimal Server for resume-session handler tests.
// It does not need orchestrator or ptyManager.
func newResumeTestServer(t *testing.T) (*Server, string) {
	t.Helper()
	dbPath := t.TempDir() + "/resume_handler_test.db"
	pool, err := db.NewPoolPath(dbPath, db.DefaultPoolConfig())
	if err != nil {
		t.Fatalf("newResumeTestServer: create pool: %v", err)
	}
	t.Cleanup(func() { pool.Close() })

	hub := ws.NewHub(clock.Real())
	go hub.Run()
	t.Cleanup(hub.Stop)

	s := &Server{
		dataPath: dbPath,
		wsHub:    hub,
		clock:    clock.Real(),
	}
	return s, dbPath
}

// insertResumeTestSession inserts the minimal DB records needed to test
// handleResumeSession and handleResumeSessionProject.
// modelID is the raw string stored in agent_sessions.model_id (e.g. "claude:sonnet").
// Pass an empty string to insert a NULL model_id.
func insertResumeTestSession(t *testing.T, dbPath, sessionID, projectID string, status model.AgentSessionStatus, modelID string) {
	t.Helper()
	database, err := db.OpenPath(dbPath)
	if err != nil {
		t.Fatalf("insertResumeTestSession: open db: %v", err)
	}
	defer database.Close()

	now := time.Now().UTC().Format(time.RFC3339Nano)

	_, err = database.Exec(`INSERT OR IGNORE INTO projects (id, name, created_at, updated_at) VALUES (?, ?, ?, ?)`,
		projectID, projectID+" Project", now, now)
	if err != nil {
		t.Fatalf("insertResumeTestSession: insert project: %v", err)
	}

	wfID := "test-wf-rs-" + projectID
	_, err = database.Exec(`INSERT OR IGNORE INTO workflows (project_id, id, description, scope_type, phases, created_at, updated_at)
		VALUES (?, ?, 'Resume Test WF', 'ticket', '[]', ?, ?)`, projectID, wfID, now, now)
	if err != nil {
		t.Fatalf("insertResumeTestSession: insert workflow: %v", err)
	}

	wfiID := "wfi-rs-" + sessionID
	_, err = database.Exec(`INSERT OR IGNORE INTO workflow_instances (id, project_id, ticket_id, workflow_id, status, scope_type, findings, created_at, updated_at)
		VALUES (?, ?, 'TKT-RS', ?, 'active', 'ticket', '{}', ?, ?)`, wfiID, projectID, wfID, now, now)
	if err != nil {
		t.Fatalf("insertResumeTestSession: insert wfi: %v", err)
	}

	var modelIDVal interface{}
	if modelID != "" {
		modelIDVal = modelID
	}
	_, err = database.Exec(`INSERT OR IGNORE INTO agent_sessions
		(id, project_id, ticket_id, workflow_instance_id, phase, agent_type, model_id, status,
		 result, result_reason, pid, findings, context_left, ancestor_session_id,
		 spawn_command, prompt_context, restart_count, started_at, ended_at, created_at, updated_at)
		VALUES (?, ?, 'TKT-RS', ?, 'phase1', 'implementor', ?, ?,
		        NULL, NULL, NULL, NULL, NULL, NULL, NULL, NULL, 0, ?, NULL, ?, ?)`,
		sessionID, projectID, wfiID, modelIDVal, string(status), now, now, now)
	if err != nil {
		t.Fatalf("insertResumeTestSession: insert session: %v", err)
	}
}

// --- validateResumeSession unit tests ---

func TestValidateResumeSession(t *testing.T) {
	cases := []struct {
		name        string
		session     *model.AgentSession
		wantErr     bool
		errContains string
	}{
		{
			name:        "null model_id",
			session:     &model.AgentSession{ModelID: sql.NullString{Valid: false}, Status: model.AgentSessionCompleted},
			wantErr:     true,
			errContains: "no model_id",
		},
		{
			name:        "empty model_id string",
			session:     &model.AgentSession{ModelID: sql.NullString{String: "", Valid: true}, Status: model.AgentSessionCompleted},
			wantErr:     true,
			errContains: "no model_id",
		},
		{
			name:        "opencode model_id",
			session:     &model.AgentSession{ModelID: sql.NullString{String: "opencode:gpt-4o", Valid: true}, Status: model.AgentSessionCompleted},
			wantErr:     true,
			errContains: "does not support resume",
		},
		{
			name:        "running session",
			session:     &model.AgentSession{ModelID: sql.NullString{String: "claude:sonnet", Valid: true}, Status: model.AgentSessionRunning},
			wantErr:     true,
			errContains: "terminal state",
		},
		{
			name:        "user_interactive session",
			session:     &model.AgentSession{ModelID: sql.NullString{String: "claude:sonnet", Valid: true}, Status: model.AgentSessionUserInteractive},
			wantErr:     true,
			errContains: "terminal state",
		},
		{
			name:    "completed claude session",
			session: &model.AgentSession{ModelID: sql.NullString{String: "claude:sonnet", Valid: true}, Status: model.AgentSessionCompleted},
			wantErr: false,
		},
		{
			name:    "failed claude session",
			session: &model.AgentSession{ModelID: sql.NullString{String: "claude:opus", Valid: true}, Status: model.AgentSessionFailed},
			wantErr: false,
		},
		{
			name:    "timeout claude session",
			session: &model.AgentSession{ModelID: sql.NullString{String: "claude:haiku", Valid: true}, Status: model.AgentSessionTimeout},
			wantErr: false,
		},
		{
			name:    "interactive_completed claude session",
			session: &model.AgentSession{ModelID: sql.NullString{String: "claude:sonnet", Valid: true}, Status: model.AgentSessionInteractiveCompleted},
			wantErr: false,
		},
		{
			name:    "skipped claude session",
			session: &model.AgentSession{ModelID: sql.NullString{String: "claude:sonnet", Valid: true}, Status: model.AgentSessionSkipped},
			wantErr: false,
		},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			err := validateResumeSession(tc.session)
			if tc.wantErr {
				if err == nil {
					t.Errorf("validateResumeSession() = nil, want error")
					return
				}
				if tc.errContains != "" && !strings.Contains(err.Error(), tc.errContains) {
					t.Errorf("error = %q, want to contain %q", err.Error(), tc.errContains)
				}
			} else if err != nil {
				t.Errorf("validateResumeSession() = %v, want nil", err)
			}
		})
	}
}

// --- handleResumeSession tests ---

func TestHandleResumeSession_MissingProject(t *testing.T) {
	s, _ := newResumeTestServer(t)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/tickets/TKT-1/workflow/resume-session",
		strings.NewReader(`{"session_id":"sess-1"}`))
	rr := httptest.NewRecorder()
	s.handleResumeSession(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", rr.Code)
	}
	assertErrorContains(t, rr, "X-Project")
}

func TestHandleResumeSession_MissingSessionID(t *testing.T) {
	s, _ := newResumeTestServer(t)
	req := httptest.NewRequest(http.MethodPost,
		withProject("/api/v1/tickets/TKT-1/workflow/resume-session", "proj-rs"),
		strings.NewReader(`{}`))
	rr := httptest.NewRecorder()
	s.handleResumeSession(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", rr.Code)
	}
	assertErrorContains(t, rr, "session_id is required")
}

func TestHandleResumeSession_SessionNotFound(t *testing.T) {
	s, _ := newResumeTestServer(t)
	req := httptest.NewRequest(http.MethodPost,
		withProject("/api/v1/tickets/TKT-1/workflow/resume-session", "proj-rs"),
		strings.NewReader(`{"session_id":"no-such-session-rs"}`))
	rr := httptest.NewRecorder()
	s.handleResumeSession(rr, req)

	if rr.Code != http.StatusNotFound {
		t.Errorf("status = %d, want 404", rr.Code)
	}
}

func TestHandleResumeSession_WrongProject(t *testing.T) {
	s, dbPath := newResumeTestServer(t)
	insertResumeTestSession(t, dbPath, "sess-wp-rs", "proj-rs-correct", model.AgentSessionCompleted, "claude:sonnet")

	req := httptest.NewRequest(http.MethodPost,
		withProject("/api/v1/tickets/TKT-1/workflow/resume-session", "proj-rs-wrong"),
		strings.NewReader(`{"session_id":"sess-wp-rs"}`))
	rr := httptest.NewRecorder()
	s.handleResumeSession(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", rr.Code)
	}
	assertErrorContains(t, rr, "does not belong")
}

func TestHandleResumeSession_NonClaudeSession(t *testing.T) {
	s, dbPath := newResumeTestServer(t)
	insertResumeTestSession(t, dbPath, "sess-nc-rs", "proj-rs-nc", model.AgentSessionCompleted, "opencode:gpt-4o")

	req := httptest.NewRequest(http.MethodPost,
		withProject("/api/v1/tickets/TKT-1/workflow/resume-session", "proj-rs-nc"),
		strings.NewReader(`{"session_id":"sess-nc-rs"}`))
	rr := httptest.NewRecorder()
	s.handleResumeSession(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", rr.Code)
	}
	assertErrorContains(t, rr, "does not support resume")
}

func TestHandleResumeSession_RunningSession(t *testing.T) {
	s, dbPath := newResumeTestServer(t)
	insertResumeTestSession(t, dbPath, "sess-run-rs", "proj-rs-run", model.AgentSessionRunning, "claude:sonnet")

	req := httptest.NewRequest(http.MethodPost,
		withProject("/api/v1/tickets/TKT-1/workflow/resume-session", "proj-rs-run"),
		strings.NewReader(`{"session_id":"sess-run-rs"}`))
	rr := httptest.NewRecorder()
	s.handleResumeSession(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", rr.Code)
	}
	assertErrorContains(t, rr, "terminal state")
}

func TestHandleResumeSession_HappyPath(t *testing.T) {
	s, dbPath := newResumeTestServer(t)
	insertResumeTestSession(t, dbPath, "sess-happy-rs", "proj-rs-happy", model.AgentSessionCompleted, "claude:sonnet")

	req := httptest.NewRequest(http.MethodPost,
		withProject("/api/v1/tickets/TKT-1/workflow/resume-session", "proj-rs-happy"),
		strings.NewReader(`{"session_id":"sess-happy-rs"}`))
	rr := httptest.NewRecorder()
	s.handleResumeSession(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("status = %d, want 200; body = %s", rr.Code, rr.Body.String())
	}

	var resp map[string]string
	if err := json.NewDecoder(rr.Body).Decode(&resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if resp["status"] != "interactive" {
		t.Errorf("status = %q, want interactive", resp["status"])
	}
	if resp["session_id"] != "sess-happy-rs" {
		t.Errorf("session_id = %q, want sess-happy-rs", resp["session_id"])
	}

	// Verify the DB status was updated to user_interactive.
	database, err := db.OpenPath(dbPath)
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	defer database.Close()
	r := repo.NewAgentSessionRepo(database, clock.Real())
	updated, err := r.Get("sess-happy-rs")
	if err != nil {
		t.Fatalf("get session: %v", err)
	}
	if updated.Status != model.AgentSessionUserInteractive {
		t.Errorf("session status = %q, want user_interactive", updated.Status)
	}
}

// --- handleResumeSessionProject tests ---

func TestHandleResumeSessionProject_MissingProjectID(t *testing.T) {
	s, _ := newResumeTestServer(t)
	// No path value "id" set → r.PathValue("id") returns "".
	req := httptest.NewRequest(http.MethodPost, "/api/v1/projects//workflow/resume-session",
		strings.NewReader(`{"session_id":"sess-1"}`))
	rr := httptest.NewRecorder()
	s.handleResumeSessionProject(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", rr.Code)
	}
	assertErrorContains(t, rr, "project ID required")
}

func TestHandleResumeSessionProject_MissingSessionID(t *testing.T) {
	s, _ := newResumeTestServer(t)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/projects/proj-rsp/workflow/resume-session",
		strings.NewReader(`{}`))
	req.SetPathValue("id", "proj-rsp")
	rr := httptest.NewRecorder()
	s.handleResumeSessionProject(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", rr.Code)
	}
	assertErrorContains(t, rr, "session_id is required")
}

func TestHandleResumeSessionProject_SessionNotFound(t *testing.T) {
	s, _ := newResumeTestServer(t)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/projects/proj-rsp/workflow/resume-session",
		strings.NewReader(`{"session_id":"no-such-session-rsp"}`))
	req.SetPathValue("id", "proj-rsp")
	rr := httptest.NewRecorder()
	s.handleResumeSessionProject(rr, req)

	if rr.Code != http.StatusNotFound {
		t.Errorf("status = %d, want 404", rr.Code)
	}
}

func TestHandleResumeSessionProject_NonClaudeSession(t *testing.T) {
	s, dbPath := newResumeTestServer(t)
	insertResumeTestSession(t, dbPath, "sess-nc-rsp", "proj-rsp-nc", model.AgentSessionCompleted, "opencode:gpt-4o")

	req := httptest.NewRequest(http.MethodPost, "/api/v1/projects/proj-rsp-nc/workflow/resume-session",
		strings.NewReader(`{"session_id":"sess-nc-rsp"}`))
	req.SetPathValue("id", "proj-rsp-nc")
	rr := httptest.NewRecorder()
	s.handleResumeSessionProject(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", rr.Code)
	}
	assertErrorContains(t, rr, "does not support resume")
}

func TestHandleResumeSessionProject_HappyPath(t *testing.T) {
	s, dbPath := newResumeTestServer(t)
	insertResumeTestSession(t, dbPath, "sess-happy-rsp", "proj-rsp-happy", model.AgentSessionFailed, "claude:opus")

	req := httptest.NewRequest(http.MethodPost, "/api/v1/projects/proj-rsp-happy/workflow/resume-session",
		strings.NewReader(`{"session_id":"sess-happy-rsp"}`))
	req.SetPathValue("id", "proj-rsp-happy")
	rr := httptest.NewRecorder()
	s.handleResumeSessionProject(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("status = %d, want 200; body = %s", rr.Code, rr.Body.String())
	}

	var resp map[string]string
	if err := json.NewDecoder(rr.Body).Decode(&resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if resp["status"] != "interactive" {
		t.Errorf("status = %q, want interactive", resp["status"])
	}
	if resp["session_id"] != "sess-happy-rsp" {
		t.Errorf("session_id = %q, want sess-happy-rsp", resp["session_id"])
	}

	// Verify the DB status was updated to user_interactive.
	database, err := db.OpenPath(dbPath)
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	defer database.Close()
	r := repo.NewAgentSessionRepo(database, clock.Real())
	updated, err := r.Get("sess-happy-rsp")
	if err != nil {
		t.Fatalf("get session: %v", err)
	}
	if updated.Status != model.AgentSessionUserInteractive {
		t.Errorf("session status = %q, want user_interactive", updated.Status)
	}
}
