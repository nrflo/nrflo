package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"be/internal/model"
	"be/internal/service"
)

func seedConsoleProject(t *testing.T, s *Server, projectID string) {
	t.Helper()
	if _, err := s.pool.Exec(`INSERT OR IGNORE INTO projects (id, name, created_at, updated_at)
		VALUES (?, 'p', datetime('now'), datetime('now'))`, projectID); err != nil {
		t.Fatalf("seed project: %v", err)
	}
}

// seedWorkflowAgentForConsoleTest inserts a kind='workflow_agent' session so kind-guard
// behavior can be verified against the console close route.
func seedWorkflowAgentForConsoleTest(t *testing.T, s *Server, projectID, id string) {
	t.Helper()
	now := time.Now().UTC().Format(time.RFC3339Nano)
	if _, err := s.pool.Exec(`INSERT OR IGNORE INTO workflows (project_id, id, description, scope_type, created_at, updated_at)
		VALUES (?, 'wf', '', 'project', ?, ?)`, projectID, now, now); err != nil {
		t.Fatalf("workflow: %v", err)
	}
	wfiID := "wfi-" + id
	if _, err := s.pool.Exec(`INSERT INTO workflow_instances (id, project_id, ticket_id, workflow_id, status, scope_type, created_at, updated_at)
		VALUES (?, ?, '', 'wf', 'active', 'project', ?, ?)`, wfiID, projectID, now, now); err != nil {
		t.Fatalf("wfi: %v", err)
	}
	if _, err := s.pool.Exec(`INSERT INTO agent_sessions
		(id, project_id, ticket_id, workflow_instance_id, phase, agent_type, model_id, status, kind, spawn_token, created_at, updated_at)
		VALUES (?, ?, '', ?, 'p', 'a', 'sonnet', 'user_interactive', 'workflow_agent', ?, ?, ?)`,
		id, projectID, wfiID, "tok-"+id, now, now); err != nil {
		t.Fatalf("workflow agent session: %v", err)
	}
}

func createConsoleReq(project string) *http.Request {
	url := "/api/v1/console/sessions"
	if project != "" {
		url += "?project=" + project
	}
	return httptest.NewRequest(http.MethodPost, url, nil)
}

func closeConsoleReq(sid string) *http.Request {
	r := httptest.NewRequest(http.MethodPost, "/api/v1/console/sessions/"+sid+"/close", nil)
	r.SetPathValue("sid", sid)
	return r
}

func TestHandleCreateConsoleSession_MissingProject_Returns400(t *testing.T) {
	s := newServerWithAuth(t)
	adminID := createTestUser(t, s, "admin1@test.com", model.UserRoleAdmin, false)
	cookie := injectSession(t, s, adminID)

	chain := s.sessionMgr.LoadAndSave(s.requireProjectAdmin(http.HandlerFunc(s.handleCreateConsoleSession)))
	req := createConsoleReq("")
	req.AddCookie(cookie)
	rr := httptest.NewRecorder()
	chain.ServeHTTP(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400; body=%s", rr.Code, rr.Body.String())
	}
}

func TestHandleCreateConsoleSession_UnknownProject_Returns404(t *testing.T) {
	s := newServerWithAuth(t)
	adminID := createTestUser(t, s, "admin2@test.com", model.UserRoleAdmin, false)
	cookie := injectSession(t, s, adminID)

	chain := s.sessionMgr.LoadAndSave(s.requireProjectAdmin(http.HandlerFunc(s.handleCreateConsoleSession)))
	req := createConsoleReq("no-such-project")
	req.AddCookie(cookie)
	rr := httptest.NewRecorder()
	chain.ServeHTTP(rr, req)

	if rr.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404; body=%s", rr.Code, rr.Body.String())
	}
}

func createConsoleReqBody(project, body string) *http.Request {
	r := httptest.NewRequest(http.MethodPost, "/api/v1/console/sessions?project="+project, strings.NewReader(body))
	return r
}

func TestHandleCreateConsoleSession_ValidTicketHint_StoredAndEchoed(t *testing.T) {
	s := newServerWithAuth(t)
	seedConsoleProject(t, s, "proj-console-tk")
	now := time.Now().UTC().Format(time.RFC3339Nano)
	if _, err := s.pool.Exec(`INSERT INTO tickets (id, project_id, title, created_at, updated_at, created_by) VALUES (?,?,?,?,?,'t')`,
		"NRF-9", "proj-console-tk", "T", now, now); err != nil {
		t.Fatalf("insert ticket: %v", err)
	}
	adminID := createTestUser(t, s, "admin-tk@test.com", model.UserRoleAdmin, false)
	cookie := injectSession(t, s, adminID)

	chain := s.sessionMgr.LoadAndSave(s.requireProjectAdmin(http.HandlerFunc(s.handleCreateConsoleSession)))
	req := createConsoleReqBody("proj-console-tk", `{"ticket_id":"NRF-9"}`)
	req.AddCookie(cookie)
	rr := httptest.NewRecorder()
	chain.ServeHTTP(rr, req)

	if rr.Code != http.StatusCreated {
		t.Fatalf("status = %d, want 201; body=%s", rr.Code, rr.Body.String())
	}
	var body map[string]string
	if err := json.Unmarshal(rr.Body.Bytes(), &body); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if body["ticket_id"] != "NRF-9" {
		t.Errorf("ticket_id = %q, want NRF-9 (validated + echoed)", body["ticket_id"])
	}
	var stored string
	if err := s.pool.QueryRow(`SELECT ticket_id FROM agent_sessions WHERE id = ?`, body["session_id"]).Scan(&stored); err != nil {
		t.Fatalf("query stored ticket: %v", err)
	}
	// agent_sessions lowercases ticket_id on write (repo convention); reads are
	// case-insensitive, so ticket_current still resolves it.
	if !strings.EqualFold(stored, "NRF-9") {
		t.Errorf("stored ticket_id = %q, want NRF-9 (case-insensitive)", stored)
	}
}

func TestHandleCreateConsoleSession_UnknownTicketHint_DroppedSilently(t *testing.T) {
	s := newServerWithAuth(t)
	seedConsoleProject(t, s, "proj-console-tk2")
	adminID := createTestUser(t, s, "admin-tk2@test.com", model.UserRoleAdmin, false)
	cookie := injectSession(t, s, adminID)

	chain := s.sessionMgr.LoadAndSave(s.requireProjectAdmin(http.HandlerFunc(s.handleCreateConsoleSession)))
	req := createConsoleReqBody("proj-console-tk2", `{"ticket_id":"not-a-ticket"}`)
	req.AddCookie(cookie)
	rr := httptest.NewRecorder()
	chain.ServeHTTP(rr, req)

	if rr.Code != http.StatusCreated {
		t.Fatalf("status = %d, want 201 (unknown ticket is not an error); body=%s", rr.Code, rr.Body.String())
	}
	var body map[string]string
	if err := json.Unmarshal(rr.Body.Bytes(), &body); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if body["ticket_id"] != "" {
		t.Errorf("ticket_id = %q, want empty (unknown branch dropped silently)", body["ticket_id"])
	}
}

func TestHandleCreateConsoleSession_AdminCookie_Returns201WithToken(t *testing.T) {
	s := newServerWithAuth(t)
	seedConsoleProject(t, s, "proj-console-admin")
	adminID := createTestUser(t, s, "admin3@test.com", model.UserRoleAdmin, false)
	cookie := injectSession(t, s, adminID)

	chain := s.sessionMgr.LoadAndSave(s.requireProjectAdmin(http.HandlerFunc(s.handleCreateConsoleSession)))
	req := createConsoleReq("proj-console-admin")
	req.AddCookie(cookie)
	rr := httptest.NewRecorder()
	chain.ServeHTTP(rr, req)

	if rr.Code != http.StatusCreated {
		t.Fatalf("status = %d, want 201; body=%s", rr.Code, rr.Body.String())
	}
	var body map[string]string
	if err := json.Unmarshal(rr.Body.Bytes(), &body); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if body["session_id"] == "" || body["token"] == "" {
		t.Fatalf("body = %+v, want non-empty session_id and token", body)
	}
}

func TestHandleCreateConsoleSession_NonAdminHuman_Returns403(t *testing.T) {
	s := newServerWithAuth(t)
	seedConsoleProject(t, s, "proj-console-nonadmin")
	userID := createTestUser(t, s, "user1@test.com", model.UserRoleViewer, false)
	cookie := injectSession(t, s, userID)

	chain := s.sessionMgr.LoadAndSave(s.requireProjectAdmin(http.HandlerFunc(s.handleCreateConsoleSession)))
	req := createConsoleReq("proj-console-nonadmin")
	req.AddCookie(cookie)
	rr := httptest.NewRecorder()
	chain.ServeHTTP(rr, req)

	if rr.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want 403; body=%s", rr.Code, rr.Body.String())
	}
}

func TestHandleCreateConsoleSession_ProjectServiceToken_MatchingProject_Returns201(t *testing.T) {
	s := newServerWithAuth(t)
	seedConsoleProject(t, s, "proj-console-svc")
	_, plain := seedServiceToken(t, s, "proj-console-svc", "ci")

	chain := s.sessionMgr.LoadAndSave(s.requireProjectAdmin(http.HandlerFunc(s.handleCreateConsoleSession)))
	req := createConsoleReq("proj-console-svc")
	req.Header.Set("Authorization", "Bearer "+plain)
	rr := httptest.NewRecorder()
	chain.ServeHTTP(rr, req)

	if rr.Code != http.StatusCreated {
		t.Fatalf("status = %d, want 201; body=%s", rr.Code, rr.Body.String())
	}
}

func TestHandleCreateConsoleSession_ProjectServiceToken_Mismatched_Returns403(t *testing.T) {
	s := newServerWithAuth(t)
	seedConsoleProject(t, s, "proj-console-svc-b")
	_, plain := seedServiceToken(t, s, "proj-console-svc-a", "ci")

	chain := s.sessionMgr.LoadAndSave(s.requireProjectAdmin(http.HandlerFunc(s.handleCreateConsoleSession)))
	req := createConsoleReq("proj-console-svc-b")
	req.Header.Set("Authorization", "Bearer "+plain)
	rr := httptest.NewRecorder()
	chain.ServeHTTP(rr, req)

	if rr.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want 403; body=%s", rr.Code, rr.Body.String())
	}
}

func TestHandleCreateConsoleSession_GlobalServiceToken_Returns201(t *testing.T) {
	s := newServerWithAuth(t)
	seedConsoleProject(t, s, "proj-console-global")
	svc := service.NewServiceTokenService(s.pool, s.clock)
	_, plain, err := svc.Create("", "global-ci", "", "global")
	if err != nil {
		t.Fatalf("create global token: %v", err)
	}

	chain := s.sessionMgr.LoadAndSave(s.requireProjectAdmin(http.HandlerFunc(s.handleCreateConsoleSession)))
	req := createConsoleReq("proj-console-global")
	req.Header.Set("Authorization", "Bearer "+plain)
	rr := httptest.NewRecorder()
	chain.ServeHTTP(rr, req)

	if rr.Code != http.StatusCreated {
		t.Fatalf("status = %d, want 201; body=%s", rr.Code, rr.Body.String())
	}
}
