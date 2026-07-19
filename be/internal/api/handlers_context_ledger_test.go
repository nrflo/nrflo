package api

import (
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"be/internal/clock"
	"be/internal/db"
)

func newContextLedgerServer(t *testing.T) (*Server, *db.Pool) {
	t.Helper()
	dbPath := filepath.Join(t.TempDir(), "ctxledger_test.db")
	if err := apiCopyTemplateDB(dbPath); err != nil {
		t.Fatalf("copy template DB: %v", err)
	}
	pool, err := db.OpenPoolExisting(dbPath, db.DefaultPoolConfig())
	if err != nil {
		t.Fatalf("new pool: %v", err)
	}
	t.Cleanup(func() { pool.Close() })
	return &Server{pool: pool, clock: clock.Real()}, pool
}

func insertCtxLedgerSession(t *testing.T, pool *db.Pool, id, projectID string) {
	t.Helper()
	now := "2025-08-01T09:00:00Z"
	for _, q := range []struct {
		sql  string
		args []interface{}
	}{
		{`INSERT INTO projects (id, name, created_at, updated_at) VALUES (?, 'CL Project', ?, ?)`, []interface{}{projectID, now, now}},
		{`INSERT INTO workflows (project_id, id, description, scope_type, created_at, updated_at) VALUES (?, 'cl-wf', '', 'project', ?, ?)`, []interface{}{projectID, now, now}},
		{`INSERT INTO workflow_instances (id, project_id, ticket_id, workflow_id, status, scope_type, created_at, updated_at) VALUES (?, ?, '', 'cl-wf', 'active', 'project', ?, ?)`, []interface{}{id + "-wfi", projectID, now, now}},
		{`INSERT INTO agent_sessions (id, project_id, ticket_id, workflow_instance_id, phase, agent_type, status, created_at, updated_at) VALUES (?, ?, '', ?, 'ph', 'ag', 'running', ?, ?)`, []interface{}{id, projectID, id + "-wfi", now, now}},
	} {
		if _, err := pool.Exec(q.sql, q.args...); err != nil {
			t.Fatalf("insertCtxLedgerSession seed: %v", err)
		}
	}
}

func TestHandleGetContextLedger_MissingSessionID(t *testing.T) {
	s := &Server{}
	req := httptest.NewRequest(http.MethodGet, "/api/v1/sessions//context-ledger", nil)
	// No SetPathValue("id", ...) -> extractID returns "".
	rr := httptest.NewRecorder()
	s.handleGetContextLedger(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", rr.Code)
	}
	assertErrorContains(t, rr, "session ID required")
}

func TestHandleGetContextLedger_NoLedger_NoProjectHeader(t *testing.T) {
	// No X-Project header: the handler skips the DB ownership check entirely
	// and goes straight to the ledger lookup, which is absent -> 404.
	s := &Server{}
	req := httptest.NewRequest(http.MethodGet, "/api/v1/sessions/no-such-session/context-ledger", nil)
	req.SetPathValue("id", "no-such-session")
	rr := httptest.NewRecorder()
	s.handleGetContextLedger(rr, req)

	if rr.Code != http.StatusNotFound {
		t.Errorf("status = %d, want 404", rr.Code)
	}
	assertErrorContains(t, rr, "no context ledger")
}

func TestHandleGetContextLedger_ProjectHeader_SessionNotFound(t *testing.T) {
	s, _ := newContextLedgerServer(t)
	req := httptest.NewRequest(http.MethodGet,
		withProject("/api/v1/sessions/missing-sess/context-ledger", "cl-proj"), nil)
	req.SetPathValue("id", "missing-sess")
	rr := httptest.NewRecorder()
	s.handleGetContextLedger(rr, req)

	if rr.Code != http.StatusNotFound {
		t.Errorf("status = %d, want 404", rr.Code)
	}
	assertErrorContains(t, rr, "session not found")
}

func TestHandleGetContextLedger_ProjectMismatch_Forbidden(t *testing.T) {
	s, pool := newContextLedgerServer(t)
	insertCtxLedgerSession(t, pool, "cl-sess-1", "cl-proj-owner")

	req := httptest.NewRequest(http.MethodGet,
		withProject("/api/v1/sessions/cl-sess-1/context-ledger", "cl-proj-other"), nil)
	req.SetPathValue("id", "cl-sess-1")
	rr := httptest.NewRecorder()
	s.handleGetContextLedger(rr, req)

	if rr.Code != http.StatusForbidden {
		t.Errorf("status = %d, want 403", rr.Code)
	}
	assertErrorContains(t, rr, "does not belong to this project")
}

func TestHandleGetContextLedger_MatchingProject_NoLedgerTracked(t *testing.T) {
	// Session exists and the project matches, but nothing has ever written to
	// this session's ledger (it either finished, was dropped, or the agent
	// never ran in a mode that tracks one) -> 404.
	s, pool := newContextLedgerServer(t)
	insertCtxLedgerSession(t, pool, "cl-sess-2", "cl-proj-owner")

	req := httptest.NewRequest(http.MethodGet,
		withProject("/api/v1/sessions/cl-sess-2/context-ledger", "cl-proj-owner"), nil)
	req.SetPathValue("id", "cl-sess-2")
	rr := httptest.NewRecorder()
	s.handleGetContextLedger(rr, req)

	if rr.Code != http.StatusNotFound {
		t.Errorf("status = %d, want 404", rr.Code)
	}
	assertErrorContains(t, rr, "no context ledger")
}
