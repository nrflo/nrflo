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

// newSessionFlowHandlerServer builds a Server + one project with a
// caller/worker session pair (a delegation edge) for the flow/stats
// endpoint tests.
func newSessionFlowHandlerServer(t *testing.T) *Server {
	t.Helper()
	dbPath := filepath.Join(t.TempDir(), "session_flow.db")
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
		{`INSERT INTO projects (id, name, created_at, updated_at) VALUES ('proj-sf', 'P', ?, ?)`, []interface{}{now, now}},
		{`INSERT INTO agent_sessions (id, project_id, ticket_id, phase, agent_type, status, started_at, created_at, updated_at)
			VALUES ('s-sf-root', 'proj-sf', '', 'p', 'caller', 'completed', ?, ?, ?)`, []interface{}{now, now, now}},
		{`INSERT INTO agent_sessions (id, project_id, ticket_id, phase, node_id, agent_type, status, started_at, created_at, updated_at)
			VALUES ('s-sf-worker', 'proj-sf', '', '_delegate', '_delegate', '_t1_executor', 'completed', ?, ?, ?)`, []interface{}{now, now, now}},
		{`INSERT INTO delegations (id, caller_session_id, workflow_instance_id, project_id, tier, brief, fanout, worker_session_ids, spawn_errors, depth, fanout_done, status, created_at)
			VALUES ('wfi-sf.d1', 's-sf-root', '', 'proj-sf', 'executor', '', 1, '["s-sf-worker"]', '[""]', 1, 1, 'completed', ?)`, []interface{}{now}},
	} {
		if _, err := pool.Exec(q.sql, q.args...); err != nil {
			t.Fatalf("seed: %v\nsql: %s", err, q.sql)
		}
	}
	return &Server{pool: pool, clock: clock.Real()}
}

func getSessionFlow(t *testing.T, srv *Server, sid string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/sessions/"+sid+"/flow", nil)
	req.SetPathValue("sid", sid)
	rec := httptest.NewRecorder()
	srv.handleGetSessionFlow(rec, req)
	return rec
}

func getSessionStats(t *testing.T, srv *Server, sid string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/sessions/"+sid+"/stats", nil)
	req.SetPathValue("sid", sid)
	rec := httptest.NewRecorder()
	srv.handleGetSessionStats(rec, req)
	return rec
}

func TestHandleGetSessionFlow_HappyPath(t *testing.T) {
	t.Parallel()
	srv := newSessionFlowHandlerServer(t)
	rec := getSessionFlow(t, srv, "s-sf-root")
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
	var flow types.SessionFlowResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &flow); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if flow.RootSessionID != "s-sf-root" {
		t.Errorf("RootSessionID = %q, want s-sf-root", flow.RootSessionID)
	}
	if len(flow.Nodes) != 2 {
		t.Fatalf("len(Nodes) = %d, want 2", len(flow.Nodes))
	}
	if len(flow.Edges) != 1 || flow.Edges[0].Kind != "delegate" {
		t.Errorf("Edges = %+v, want one delegate edge", flow.Edges)
	}
}

func TestHandleGetSessionFlow_UnknownSession_404(t *testing.T) {
	t.Parallel()
	srv := newSessionFlowHandlerServer(t)
	rec := getSessionFlow(t, srv, "no-such-session")
	if rec.Code != http.StatusNotFound {
		t.Errorf("status = %d, want 404", rec.Code)
	}
}

func TestHandleGetSessionFlow_MissingSID_400(t *testing.T) {
	t.Parallel()
	srv := newSessionFlowHandlerServer(t)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/sessions//flow", nil)
	rec := httptest.NewRecorder()
	srv.handleGetSessionFlow(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", rec.Code)
	}
}

func TestHandleGetSessionStats_HappyPath(t *testing.T) {
	t.Parallel()
	srv := newSessionFlowHandlerServer(t)
	if _, err := srv.pool.Exec(`UPDATE agent_sessions SET cost_estimate = 1.0 WHERE id = 's-sf-root'`); err != nil {
		t.Fatalf("seed cost: %v", err)
	}
	if _, err := srv.pool.Exec(`UPDATE agent_sessions SET cost_estimate = 2.0 WHERE id = 's-sf-worker'`); err != nil {
		t.Fatalf("seed cost: %v", err)
	}

	rec := getSessionStats(t, srv, "s-sf-root")
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
	var stats types.SessionStatsResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &stats); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if stats.SelfCostUSD != 1.0 {
		t.Errorf("SelfCostUSD = %v, want 1.0", stats.SelfCostUSD)
	}
	if stats.SubtreeCostUSD != 3.0 {
		t.Errorf("SubtreeCostUSD = %v, want 3.0 (root+worker)", stats.SubtreeCostUSD)
	}
}

func TestHandleGetSessionStats_UnknownSession_404(t *testing.T) {
	t.Parallel()
	srv := newSessionFlowHandlerServer(t)
	rec := getSessionStats(t, srv, "no-such-session")
	if rec.Code != http.StatusNotFound {
		t.Errorf("status = %d, want 404", rec.Code)
	}
}
