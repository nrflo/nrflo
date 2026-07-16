package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"be/internal/clock"
	"be/internal/db"
	"be/internal/model"
	ptyPkg "be/internal/pty"
	"be/internal/repo"
	"be/internal/ws"
)

// newResumeTestServer creates a minimal Server for resume-session handler tests.
// It needs no orchestrator, but does need a ptyManager: the handler registers the
// adapter-built resume launch before flipping status. Registering a launch spawns
// nothing — only pty.Manager.Create() would, and these tests never call it.
func newResumeTestServer(t *testing.T) (*Server, string) {
	t.Helper()
	dbPath := t.TempDir() + "/resume_handler_test.db"
	if err := apiCopyTemplateDB(dbPath); err != nil {
		t.Fatalf("copy template DB: %v", err)
	}
	pool, err := db.OpenPoolExisting(dbPath, db.DefaultPoolConfig())
	if err != nil {
		t.Fatalf("newResumeTestServer: create pool: %v", err)
	}
	t.Cleanup(func() { pool.Close() })

	hub := ws.NewHub(clock.Real())
	go hub.Run()
	t.Cleanup(hub.Stop)

	s := &Server{
		dataPath:   dbPath,
		pool:       pool,
		wsHub:      hub,
		clock:      clock.Real(),
		ptyManager: ptyPkg.NewManager(),
	}
	return s, dbPath
}

// insertResumeTestSession inserts the minimal DB records needed to test
// handleResumeSession and handleResumeSessionProject.
// modelID is the raw string stored in agent_sessions.model_id (e.g. "claude:sonnet-5").
// Pass an empty string to insert a NULL model_id.
func insertResumeTestSession(t *testing.T, dbPath, sessionID, projectID string, status model.AgentSessionStatus, modelID string) {
	t.Helper()
	database, err := db.OpenPathExisting(dbPath)
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
	_, err = database.Exec(`INSERT OR IGNORE INTO workflows (project_id, id, description, scope_type, created_at, updated_at)
		VALUES (?, ?, 'Resume Test WF', 'ticket', ?, ?)`, projectID, wfID, now, now)
	if err != nil {
		t.Fatalf("insertResumeTestSession: insert workflow: %v", err)
	}

	wfiID := "wfi-rs-" + sessionID
	_, err = database.Exec(`INSERT OR IGNORE INTO workflow_instances (id, project_id, ticket_id, workflow_id, status, scope_type, created_at, updated_at)
		VALUES (?, ?, 'TKT-RS', ?, 'active', 'ticket', ?, ?)`, wfiID, projectID, wfID, now, now)
	if err != nil {
		t.Fatalf("insertResumeTestSession: insert wfi: %v", err)
	}

	var modelIDVal interface{}
	if modelID != "" {
		modelIDVal = modelID
	}
	_, err = database.Exec(`INSERT OR IGNORE INTO agent_sessions
		(id, project_id, ticket_id, workflow_instance_id, phase, agent_type, model_id, status,
		 result, result_reason, pid, context_left, ancestor_session_id,
		 spawn_command, prompt, restart_count, started_at, ended_at, created_at, updated_at)
		VALUES (?, ?, 'TKT-RS', ?, 'phase1', 'implementor', ?, ?,
		        NULL, NULL, NULL, NULL, NULL, NULL, NULL, 0, ?, NULL, ?, ?)`,
		sessionID, projectID, wfiID, modelIDVal, string(status), now, now, now)
	if err != nil {
		t.Fatalf("insertResumeTestSession: insert session: %v", err)
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
	insertResumeTestSession(t, dbPath, "sess-wp-rs", "proj-rs-correct", model.AgentSessionCompleted, "claude:sonnet-5")

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
	insertResumeTestSession(t, dbPath, "sess-nc-rs", "proj-rs-nc", model.AgentSessionCompleted, "codex:gpt-5.3-codex")

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
	insertResumeTestSession(t, dbPath, "sess-run-rs", "proj-rs-run", model.AgentSessionRunning, "claude:sonnet-5")

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
	insertResumeTestSession(t, dbPath, "sess-happy-rs", "proj-rs-happy", model.AgentSessionCompleted, "claude:sonnet-5")

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
	database, err := db.OpenPathExisting(dbPath)
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

	// The PTY manager has no default command: without a registered launch the
	// PTY attach that the UI opens next fails with "no PTY launch registered".
	assertResumeLaunchRegistered(t, s, "sess-happy-rs")
}

// assertResumeLaunchRegistered verifies the resume handler registered a claude
// resume launch for the session with the PTY manager.
func assertResumeLaunchRegistered(t *testing.T, s *Server, sessionID string) {
	t.Helper()
	launch, ok := s.ptyManager.PendingLaunch(sessionID)
	if !ok {
		t.Fatalf("no PTY launch registered for session %s", sessionID)
	}
	// exec.Command resolves a PATH lookup to an absolute path.
	if filepath.Base(launch.Command) != "claude" {
		t.Errorf("launch.Command = %q, want claude", launch.Command)
	}
	args := strings.Join(launch.Args, " ")
	if !strings.Contains(args, "--resume "+sessionID) {
		t.Errorf("launch args missing --resume %s: %s", sessionID, args)
	}
	// The CLI rejects --session-id alongside --resume.
	if strings.Contains(args, "--session-id") {
		t.Errorf("resume launch must not pass --session-id: %s", args)
	}
}
