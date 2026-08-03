package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"be/internal/clock"
	"be/internal/db"
	"be/internal/types"
)

// newSessionsHandlerServer builds a Server + two projects, each with one
// agent_sessions row, for the session-listing endpoint tests.
func newSessionsHandlerServer(t *testing.T) *Server {
	t.Helper()
	dbPath := filepath.Join(t.TempDir(), "sessions_list.db")
	if err := apiCopyTemplateDB(dbPath); err != nil {
		t.Fatalf("copy template DB: %v", err)
	}
	pool, err := db.OpenPoolExisting(dbPath, db.DefaultPoolConfig())
	if err != nil {
		t.Fatalf("open pool: %v", err)
	}
	t.Cleanup(func() { pool.Close() })

	now := "2025-01-01T00:00:00Z"
	for _, q := range []struct {
		sql  string
		args []interface{}
	}{
		{`INSERT INTO projects (id, name, created_at, updated_at) VALUES ('proj-sl-a', 'A', ?, ?)`, []interface{}{now, now}},
		{`INSERT INTO projects (id, name, created_at, updated_at) VALUES ('proj-sl-b', 'B', ?, ?)`, []interface{}{now, now}},
		{`INSERT INTO agent_sessions (id, project_id, ticket_id, phase, agent_type, status, started_at, created_at, updated_at)
			VALUES ('s-sl-a', 'proj-sl-a', '', 'p', 'analyzer', 'running', ?, ?, ?)`, []interface{}{now, now, now}},
		{`INSERT INTO agent_sessions (id, project_id, ticket_id, phase, agent_type, status, started_at, created_at, updated_at)
			VALUES ('s-sl-b', 'proj-sl-b', '', 'p', 'analyzer', 'running', ?, ?, ?)`, []interface{}{now, now, now}},
	} {
		if _, err := pool.Exec(q.sql, q.args...); err != nil {
			t.Fatalf("seed: %v\nsql: %s", err, q.sql)
		}
	}
	return &Server{pool: pool, clock: clock.Real()}
}

func TestHandleListSessions_MissingProject_400(t *testing.T) {
	t.Parallel()
	srv := newSessionsHandlerServer(t)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/sessions", nil)
	rec := httptest.NewRecorder()
	srv.handleListSessions(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", rec.Code)
	}
}

func TestHandleListSessions_ScopesToProject(t *testing.T) {
	t.Parallel()
	srv := newSessionsHandlerServer(t)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/sessions?project=proj-sl-a", nil)
	rec := httptest.NewRecorder()
	srv.handleListSessions(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
	var resp types.SessionListResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(resp.Sessions) != 1 || resp.Sessions[0].SessionID != "s-sl-a" {
		t.Errorf("Sessions = %+v, want only s-sl-a (proj-sl-b must not leak in)", resp.Sessions)
	}
}

func TestHandleListSessions_CrossProjectIsolation(t *testing.T) {
	t.Parallel()
	srv := newSessionsHandlerServer(t)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/sessions?project=proj-sl-b", nil)
	rec := httptest.NewRecorder()
	srv.handleListSessions(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d", rec.Code)
	}
	var resp types.SessionListResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	for _, s := range resp.Sessions {
		if s.SessionID == "s-sl-a" {
			t.Errorf("proj-sl-a's session leaked into proj-sl-b's listing: %+v", resp.Sessions)
		}
	}
}

func TestHandleListSessionsGlobal_ReturnsRowsFromMultipleProjects(t *testing.T) {
	t.Parallel()
	srv := newSessionsHandlerServer(t)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/sessions/global", nil)
	rec := httptest.NewRecorder()
	srv.handleListSessionsGlobal(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
	var resp types.SessionListResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	seen := map[string]bool{}
	for _, s := range resp.Sessions {
		seen[s.SessionID] = true
	}
	if !seen["s-sl-a"] || !seen["s-sl-b"] {
		t.Errorf("Sessions = %+v, want both s-sl-a and s-sl-b (no X-Project required)", resp.Sessions)
	}
}

func TestHandleListSessionsGlobal_NoProjectRequired(t *testing.T) {
	t.Parallel()
	srv := newSessionsHandlerServer(t)
	// No ?project and no X-Project header at all — the global route must not 400.
	req := httptest.NewRequest(http.MethodGet, "/api/v1/sessions/global", nil)
	rec := httptest.NewRecorder()
	srv.handleListSessionsGlobal(rec, req)
	if rec.Code != http.StatusOK {
		t.Errorf("status = %d, want 200 (global route has no project requirement)", rec.Code)
	}
}
