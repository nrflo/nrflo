package api

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"be/internal/model"
)

// seedTokenSession inserts a project + workflow + workflow_instance + agent_session
// row with the given spawn_token and status. Returns the session ID.
func seedTokenSession(t *testing.T, s *Server, projectID, token string, status model.AgentSessionStatus) string {
	t.Helper()
	now := time.Now().UTC().Format(time.RFC3339Nano)
	if _, err := s.pool.Exec(`INSERT OR IGNORE INTO projects (id, name, created_at, updated_at)
		VALUES (?, 'p', ?, ?)`, projectID, now, now); err != nil {
		t.Fatalf("project: %v", err)
	}
	if _, err := s.pool.Exec(`INSERT OR IGNORE INTO workflows (project_id, id, description, scope_type, created_at, updated_at)
		VALUES (?, 'wf', '', 'project', ?, ?)`, projectID, now, now); err != nil {
		t.Fatalf("workflow: %v", err)
	}
	wfiID := "wfi-" + token
	if _, err := s.pool.Exec(`INSERT INTO workflow_instances (id, project_id, ticket_id, workflow_id, status, scope_type, findings, created_at, updated_at)
		VALUES (?, ?, '', 'wf', 'active', 'project', '{}', ?, ?)`, wfiID, projectID, now, now); err != nil {
		t.Fatalf("wfi: %v", err)
	}
	sid := "sess-" + token
	if _, err := s.pool.Exec(`INSERT INTO agent_sessions
		(id, project_id, ticket_id, workflow_instance_id, phase, agent_type, model_id, status, spawn_token, created_at, updated_at)
		VALUES (?, ?, '', ?, 'p', 'a', 'sonnet', ?, ?, ?, ?)`,
		sid, projectID, wfiID, status, token, now, now); err != nil {
		t.Fatalf("session: %v", err)
	}
	return sid
}

func reqWithBearer(token string) *http.Request {
	r := httptest.NewRequest(http.MethodGet, "/", nil)
	r.Header.Set("Authorization", "Bearer "+token)
	return r
}

func TestRequireAuth_BearerToken_RunningSession_Passes(t *testing.T) {
	s := newServerWithAuth(t)
	seedTokenSession(t, s, "proj-bearer", "tok-good", model.AgentSessionRunning)

	called := false
	handler := s.requireAuth(sentinelHandler(&called))
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, reqWithBearer("tok-good"))

	if !called {
		t.Errorf("next handler not called")
	}
	if rr.Code != http.StatusOK {
		t.Errorf("status = %d, want 200", rr.Code)
	}
}

func TestRequireAuth_BearerToken_UnknownToken_Returns401(t *testing.T) {
	s := newServerWithAuth(t)
	called := false
	chain := s.sessionMgr.LoadAndSave(s.requireAuth(sentinelHandler(&called)))
	rr := httptest.NewRecorder()
	chain.ServeHTTP(rr, reqWithBearer("nope-not-a-token"))
	if called {
		t.Error("next handler should not have been called")
	}
	if rr.Code != http.StatusUnauthorized {
		t.Errorf("status = %d, want 401", rr.Code)
	}
}

func TestRequireAuth_BearerToken_TerminalSession_Returns401(t *testing.T) {
	s := newServerWithAuth(t)
	seedTokenSession(t, s, "proj-term", "tok-completed", model.AgentSessionCompleted)
	called := false
	chain := s.sessionMgr.LoadAndSave(s.requireAuth(sentinelHandler(&called)))
	rr := httptest.NewRecorder()
	chain.ServeHTTP(rr, reqWithBearer("tok-completed"))
	if called {
		t.Error("terminal-status token should be rejected")
	}
	if rr.Code != http.StatusUnauthorized {
		t.Errorf("status = %d, want 401", rr.Code)
	}
}

func TestRequireAuth_BearerToken_ProjectMismatch_Returns403(t *testing.T) {
	s := newServerWithAuth(t)
	seedTokenSession(t, s, "proj-a", "tok-a", model.AgentSessionRunning)
	called := false
	handler := s.requireAuth(sentinelHandler(&called))
	r := reqWithBearer("tok-a")
	r.Header.Set("X-Project", "proj-other")
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, r)
	if called {
		t.Error("project mismatch should be rejected")
	}
	if rr.Code != http.StatusForbidden {
		t.Errorf("status = %d, want 403", rr.Code)
	}
}

func TestRequireAuth_BearerToken_ProjectMatch_CaseInsensitive(t *testing.T) {
	s := newServerWithAuth(t)
	// seedTokenSession lowercases the project ID via repo path? It directly inserts
	// the value; we use lowercase here to match `Create()` lowercasing and also
	// confirm the EqualFold check.
	seedTokenSession(t, s, "proj-mixed", "tok-mixed", model.AgentSessionRunning)
	called := false
	handler := s.requireAuth(sentinelHandler(&called))
	r := reqWithBearer("tok-mixed")
	r.Header.Set("X-Project", "PROJ-MIXED")
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, r)
	if !called || rr.Code != http.StatusOK {
		t.Errorf("called=%v code=%d, want true/200", called, rr.Code)
	}
}

func TestRequireAdmin_BearerToken_Returns403(t *testing.T) {
	s := newServerWithAuth(t)
	seedTokenSession(t, s, "proj-admin", "tok-admin", model.AgentSessionRunning)
	called := false
	handler := s.requireAdmin(sentinelHandler(&called))
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, reqWithBearer("tok-admin"))
	if called {
		t.Error("agent token must not satisfy requireAdmin")
	}
	if rr.Code != http.StatusForbidden {
		t.Errorf("status = %d, want 403", rr.Code)
	}
}

func TestGetAgentSession_ReturnsSession_OnBearerAuth(t *testing.T) {
	s := newServerWithAuth(t)
	seedTokenSession(t, s, "proj-ctx", "tok-ctx", model.AgentSessionRunning)

	var captured *model.AgentSession
	handler := s.requireAuth(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		captured = getAgentSession(r)
	}))
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, reqWithBearer("tok-ctx"))
	if captured == nil {
		t.Fatal("getAgentSession returned nil for bearer-auth request")
	}
	if captured.ID != "sess-tok-ctx" {
		t.Errorf("session.ID = %q, want sess-tok-ctx", captured.ID)
	}
}

func TestBearerToken_Helper(t *testing.T) {
	cases := []struct {
		header string
		want   string
	}{
		{"", ""},
		{"Basic abc", ""},
		{"Bearer abc", "abc"},
		{"bearer abc", "abc"},
		{"Bearer  spaces  ", "spaces"},
	}
	for _, c := range cases {
		got := bearerToken(c.header)
		if got != c.want {
			t.Errorf("bearerToken(%q) = %q, want %q", c.header, got, c.want)
		}
	}
}
