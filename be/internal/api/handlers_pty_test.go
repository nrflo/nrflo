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
	"be/internal/orchestrator"
	ptyPkg "be/internal/pty"
	"be/internal/repo"
	"be/internal/ws"
)

// newPtyTestServer creates a Server with real DB + orchestrator + ptyManager
// for PTY handler tests.
func newPtyTestServer(t *testing.T) (*Server, string) {
	t.Helper()
	dbPath := filepath.Join(t.TempDir(), "pty_handler_test.db")
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
	s := &Server{
		dataPath:     dbPath,
		pool:         pool,
		orchestrator: orch,
		ptyManager:   ptyPkg.NewManager(),
		wsHub:        hub,
		clock:        clock.Real(),
	}
	return s, dbPath
}

// insertPtyTestSession inserts minimal records (project, workflow, WFI,
// agent_session) into the test DB so the handler's DB lookups succeed.
func insertPtyTestSession(t *testing.T, dbPath, sessionID string, status model.AgentSessionStatus) {
	t.Helper()
	database, err := db.OpenPath(dbPath)
	if err != nil {
		t.Fatalf("insertPtyTestSession: open db: %v", err)
	}
	defer database.Close()

	now := time.Now().UTC().Format(time.RFC3339Nano)

	_, err = database.Exec(`INSERT OR IGNORE INTO projects (id, name, created_at, updated_at) VALUES ('proj-pty', 'PTY Test Project', ?, ?)`, now, now)
	if err != nil {
		t.Fatalf("insertPtyTestSession: insert project: %v", err)
	}

	_, err = database.Exec(`INSERT OR IGNORE INTO workflows (project_id, id, description, scope_type, created_at, updated_at)
		VALUES ('proj-pty', 'test-wf', 'PTY Test WF', 'ticket', ?, ?)`, now, now)
	if err != nil {
		t.Fatalf("insertPtyTestSession: insert workflow: %v", err)
	}

	wfiID := "wfi-pty-test"
	_, err = database.Exec(`INSERT OR IGNORE INTO workflow_instances (id, project_id, ticket_id, workflow_id, status, scope_type, findings, created_at, updated_at)
		VALUES (?, 'proj-pty', 'TKT-PTY', 'test-wf', 'active', 'ticket', '{}', ?, ?)`, wfiID, now, now)
	if err != nil {
		t.Fatalf("insertPtyTestSession: insert wfi: %v", err)
	}

	_, err = database.Exec(`INSERT OR IGNORE INTO agent_sessions
		(id, project_id, ticket_id, workflow_instance_id, phase, agent_type, status,
		 result, result_reason, pid, findings, context_left, ancestor_session_id,
		 spawn_command, prompt_context, restart_count, started_at, ended_at, created_at, updated_at)
		VALUES (?, 'proj-pty', 'TKT-PTY', ?, 'phase1', 'analyzer', ?,
		        NULL, NULL, NULL, NULL, NULL, NULL, NULL, NULL, 0, ?, NULL, ?, ?)`,
		sessionID, wfiID, string(status), now, now, now)
	if err != nil {
		t.Fatalf("insertPtyTestSession: insert session: %v", err)
	}
}

// TestHandlePtyWebSocket_MissingSessionID verifies that an empty session_id
// path param returns 400.
func TestHandlePtyWebSocket_MissingSessionID(t *testing.T) {
	s, _ := newPtyTestServer(t)
	// No path value for session_id set — PathValue returns "".
	req := httptest.NewRequest(http.MethodGet, "/api/v1/pty/", nil)
	rr := httptest.NewRecorder()
	s.handlePtyWebSocket(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", rr.Code)
	}
	assertErrorContains(t, rr, "session_id required")
}

// TestHandlePtyWebSocket_SessionNotFound verifies that a nonexistent session
// returns 404.
func TestHandlePtyWebSocket_SessionNotFound(t *testing.T) {
	s, _ := newPtyTestServer(t)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/pty/no-such-session", nil)
	req.SetPathValue("session_id", "no-such-session")
	rr := httptest.NewRecorder()
	s.handlePtyWebSocket(rr, req)

	if rr.Code != http.StatusNotFound {
		t.Errorf("status = %d, want 404", rr.Code)
	}
	assertErrorContains(t, rr, "session not found")
}

// TestHandlePtyWebSocket_WrongStatus verifies that a session not in
// user_interactive status returns 400.
func TestHandlePtyWebSocket_WrongStatus(t *testing.T) {
	s, dbPath := newPtyTestServer(t)
	insertPtyTestSession(t, dbPath, "sess-wrong-status", model.AgentSessionRunning)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/pty/sess-wrong-status", nil)
	req.SetPathValue("session_id", "sess-wrong-status")
	rr := httptest.NewRecorder()
	s.handlePtyWebSocket(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", rr.Code)
	}
	assertErrorContains(t, rr, "user_interactive")
}

// TestHandlePtyWebSocket_WrongStatus_Completed verifies that interactive_completed
// sessions also return 400 (session already finished).
func TestHandlePtyWebSocket_WrongStatus_Completed(t *testing.T) {
	s, dbPath := newPtyTestServer(t)
	insertPtyTestSession(t, dbPath, "sess-ic", model.AgentSessionInteractiveCompleted)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/pty/sess-ic", nil)
	req.SetPathValue("session_id", "sess-ic")
	rr := httptest.NewRecorder()
	s.handlePtyWebSocket(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", rr.Code)
	}
}

// TestHandlePtyWebSocket_WrongStatus_Failed verifies failed sessions return 400.
func TestHandlePtyWebSocket_WrongStatus_Failed(t *testing.T) {
	s, dbPath := newPtyTestServer(t)
	insertPtyTestSession(t, dbPath, "sess-failed-pty", model.AgentSessionFailed)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/pty/sess-failed-pty", nil)
	req.SetPathValue("session_id", "sess-failed-pty")
	rr := httptest.NewRecorder()
	s.handlePtyWebSocket(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", rr.Code)
	}
}

// TestHandlePtyWebSocket_ValidSessionFailsUpgrade verifies that a session in
// user_interactive status passes all validation and fails only at the WebSocket
// upgrade step (httptest.ResponseRecorder is not a real connection).
func TestHandlePtyWebSocket_ValidSessionFailsUpgrade(t *testing.T) {
	s, dbPath := newPtyTestServer(t)
	insertPtyTestSession(t, dbPath, "sess-valid-pty", model.AgentSessionUserInteractive)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/pty/sess-valid-pty", nil)
	req.SetPathValue("session_id", "sess-valid-pty")
	rr := httptest.NewRecorder()
	s.handlePtyWebSocket(rr, req)

	// The handler will fail at WebSocket upgrade (not a real HTTP connection),
	// but it must NOT return 404 or 400 from the validation phase.
	// Acceptable responses: 400 (upgrade error) or 500 — not 404.
	if rr.Code == http.StatusNotFound {
		t.Errorf("status = 404, validation should have passed for user_interactive session")
	}
	// Must not be a 400 from our validation — any upgrade failure is fine.
	body := rr.Body.String()
	if strings.Contains(body, "session not found") {
		t.Errorf("got 'session not found' for a valid session")
	}
	if strings.Contains(body, "session status is") {
		t.Errorf("got status error for user_interactive session")
	}
}

// TestBuildPtyEnv_ContainsRequiredKeys verifies that buildPtyEnv includes all
// required environment variables.
func TestBuildPtyEnv_ContainsRequiredKeys(t *testing.T) {
	session := &model.AgentSession{
		ID:                 "env-sess-1",
		ProjectID:          "my-project",
		WorkflowInstanceID: "wfi-env-1",
	}
	project := &model.Project{}

	env := buildPtyEnv(session, project)

	envMap := make(map[string]string)
	for _, e := range env {
		parts := strings.SplitN(e, "=", 2)
		if len(parts) == 2 {
			envMap[parts[0]] = parts[1]
		}
	}

	requiredKeys := []string{
		"TERM",
		"NRFLO_PROJECT",
		"NRF_WORKFLOW_INSTANCE_ID",
		"NRF_SESSION_ID",
	}
	for _, key := range requiredKeys {
		if _, ok := envMap[key]; !ok {
			t.Errorf("buildPtyEnv missing required key %q", key)
		}
	}

	if envMap["TERM"] != "xterm-256color" {
		t.Errorf("TERM = %q, want xterm-256color", envMap["TERM"])
	}
	if envMap["NRFLO_PROJECT"] != "my-project" {
		t.Errorf("NRFLO_PROJECT = %q, want my-project", envMap["NRFLO_PROJECT"])
	}
	if envMap["NRF_WORKFLOW_INSTANCE_ID"] != "wfi-env-1" {
		t.Errorf("NRF_WORKFLOW_INSTANCE_ID = %q, want wfi-env-1", envMap["NRF_WORKFLOW_INSTANCE_ID"])
	}
	if envMap["NRF_SESSION_ID"] != "env-sess-1" {
		t.Errorf("NRF_SESSION_ID = %q, want env-sess-1", envMap["NRF_SESSION_ID"])
	}
}

// TestBuildPtyEnv_NoDuplicateKeys verifies that buildPtyEnv does not produce
// duplicate key entries.
func TestBuildPtyEnv_NoDuplicateKeys(t *testing.T) {
	session := &model.AgentSession{
		ID:                 "dedup-sess",
		ProjectID:          "proj-dedup",
		WorkflowInstanceID: "wfi-dedup",
	}
	project := &model.Project{}

	env := buildPtyEnv(session, project)

	seen := make(map[string]int)
	for _, e := range env {
		key := strings.SplitN(e, "=", 2)[0]
		seen[key]++
	}
	for key, count := range seen {
		if count > 1 {
			t.Errorf("duplicate env key %q appears %d times", key, count)
		}
	}
}

// TestResizeMsg_ParseValidJSON verifies that a valid resize JSON message can be
// decoded into resizeMsg.
func TestResizeMsg_ParseValidJSON(t *testing.T) {
	cases := []struct {
		input    string
		wantRows uint16
		wantCols uint16
	}{
		{`{"type":"resize","rows":24,"cols":80}`, 24, 80},
		{`{"type":"resize","rows":40,"cols":120}`, 40, 120},
		{`{"type":"resize","rows":0,"cols":0}`, 0, 0},
	}

	for _, tc := range cases {
		var msg resizeMsg
		if err := json.Unmarshal([]byte(tc.input), &msg); err != nil {
			t.Errorf("Unmarshal(%q) failed: %v", tc.input, err)
			continue
		}
		if msg.Type != "resize" {
			t.Errorf("Type = %q, want resize", msg.Type)
		}
		if msg.Rows != tc.wantRows {
			t.Errorf("Rows = %d, want %d", msg.Rows, tc.wantRows)
		}
		if msg.Cols != tc.wantCols {
			t.Errorf("Cols = %d, want %d", msg.Cols, tc.wantCols)
		}
	}
}

// TestResizeMsg_IgnoreInvalidJSON verifies that invalid JSON for resize
// messages fails to parse (the handler ignores them).
func TestResizeMsg_IgnoreInvalidJSON(t *testing.T) {
	invalids := []string{
		`not json`,
		`{"type":"other","rows":24,"cols":80}`,
		`{}`,
	}

	for _, input := range invalids {
		var msg resizeMsg
		err := json.Unmarshal([]byte(input), &msg)
		if err == nil && msg.Type == "resize" {
			t.Errorf("unexpected successful resize parse for input %q", input)
		}
	}
}

// TestCompletePtyInteractive_NilOrchestrator verifies that completePtyInteractive
// does not panic when orchestrator is nil.
func TestCompletePtyInteractive_NilOrchestrator(t *testing.T) {
	s := &Server{orchestrator: nil}
	session := &model.AgentSession{ID: "sess-nil-orch"}
	// Must not panic.
	s.completePtyInteractive(session, "test-workflow")
}

// TestPtyManager_GetReturnsNilBeforeCreate verifies the ptyManager embedded in
// the server returns nil for an unknown session.
func TestPtyManager_GetReturnsNilBeforeCreate(t *testing.T) {
	s, _ := newPtyTestServer(t)
	if got := s.ptyManager.Get("unknown"); got != nil {
		t.Errorf("ptyManager.Get(unknown) = %v, want nil", got)
	}
}

// TestHandlePtyWebSocket_StatusTable runs table-driven tests for all non-interactive
// statuses, all should return 400.
func TestHandlePtyWebSocket_StatusTable(t *testing.T) {
	s, dbPath := newPtyTestServer(t)

	cases := []struct {
		status model.AgentSessionStatus
		sessID string
	}{
		{model.AgentSessionRunning, "tbl-running"},
		{model.AgentSessionCompleted, "tbl-completed"},
		{model.AgentSessionFailed, "tbl-failed"},
		{model.AgentSessionTimeout, "tbl-timeout"},
		{model.AgentSessionContinued, "tbl-continued"},
		{model.AgentSessionInteractiveCompleted, "tbl-ic"},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(string(tc.status), func(t *testing.T) {
			insertPtyTestSession(t, dbPath, tc.sessID, tc.status)
			req := httptest.NewRequest(http.MethodGet, "/api/v1/pty/"+tc.sessID, nil)
			req.SetPathValue("session_id", tc.sessID)
			rr := httptest.NewRecorder()
			s.handlePtyWebSocket(rr, req)

			if rr.Code != http.StatusBadRequest {
				t.Errorf("status %q → HTTP %d, want 400", tc.status, rr.Code)
			}
		})
	}
}

// TestAgentSessionRepo_GetMissingSession verifies that repo.Get returns error
// for a nonexistent session, which drives the 404 in handlePtyWebSocket.
func TestAgentSessionRepo_GetMissingSession(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "repo_pty_test.db")
	database, err := db.OpenPath(dbPath)
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	defer database.Close()

	r := repo.NewAgentSessionRepo(database, clock.Real())
	_, err = r.Get("nonexistent-session-pty")
	if err == nil {
		t.Fatal("expected error for missing session, got nil")
	}
}
