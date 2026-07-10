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

// newTraceHandlerServer builds a Server + seeded project/workflow/instance
// with two agent sessions and a few messages for trace endpoint tests.
func newTraceHandlerServer(t *testing.T) (*Server, string) {
	t.Helper()
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "trace.db")
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
		{`INSERT INTO projects (id, name, created_at, updated_at) VALUES ('tp', 'tp', ?, ?)`, []interface{}{now, now}},
		{`INSERT INTO workflows (project_id, id, description, scope_type, created_at, updated_at) VALUES ('tp', 'wf', '', 'ticket', ?, ?)`, []interface{}{now, now}},
		{`INSERT INTO agent_definitions (id, project_id, workflow_id, prompt, layer, created_at, updated_at) VALUES ('analyzer', 'tp', 'wf', '', 0, ?, ?)`, []interface{}{now, now}},
		{`INSERT INTO agent_definitions (id, project_id, workflow_id, prompt, layer, created_at, updated_at) VALUES ('builder', 'tp', 'wf', '', 1, ?, ?)`, []interface{}{now, now}},
		{`INSERT INTO workflow_instances (id, project_id, ticket_id, workflow_id, status, scope_type, created_at, updated_at) VALUES ('wfi-1', 'tp', '', 'wf', 'active', 'ticket', ?, ?)`, []interface{}{now, now}},
		{`INSERT INTO agent_sessions (id, project_id, ticket_id, workflow_instance_id, phase, agent_type, status, result, started_at, ended_at, created_at, updated_at)
			VALUES ('s1', 'tp', '', 'wfi-1', 'analyzer', 'analyzer', 'completed', 'pass', '2025-01-01T00:00:01Z', '2025-01-01T00:01:00Z', ?, ?)`, []interface{}{now, now}},
		{`INSERT INTO agent_sessions (id, project_id, ticket_id, workflow_instance_id, phase, agent_type, status, started_at, created_at, updated_at)
			VALUES ('s2', 'tp', '', 'wfi-1', 'builder', 'builder', 'running', '2025-01-01T00:01:00Z', ?, ?)`, []interface{}{now, now}},
		{`INSERT INTO agent_messages (session_id, seq, content, category, created_at) VALUES ('s1', 0, '[Bash] ls', 'tool', '2025-01-01T00:00:02Z')`, nil},
		{`INSERT INTO agent_messages (session_id, seq, content, category, created_at) VALUES ('s1', 1, 'oops', 'error', '2025-01-01T00:00:03Z')`, nil},
		{`INSERT INTO agent_messages (session_id, seq, content, category, created_at) VALUES ('s2', 0, 'pondering', 'thinking', '2025-01-01T00:01:01Z')`, nil},
	} {
		if _, err := pool.Exec(q.sql, q.args...); err != nil {
			t.Fatalf("seed: %v\nsql: %s", err, q.sql)
		}
	}
	return &Server{pool: pool, clock: clock.Real()}, dir
}

func getTrace(t *testing.T, srv *Server, url string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, url, nil)
	req.SetPathValue("iid", req.URL.Path[len("/api/v1/workflow-instances/"):len(req.URL.Path)-len("/trace")])
	rec := httptest.NewRecorder()
	srv.handleGetWorkflowTrace(rec, req)
	return rec
}

func TestHandleGetWorkflowTrace_HappyPath(t *testing.T) {
	t.Parallel()
	srv, _ := newTraceHandlerServer(t)
	rec := getTrace(t, srv, "/api/v1/workflow-instances/wfi-1/trace")
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
	var trace types.TraceResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &trace); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if trace.InstanceID != "wfi-1" || trace.Workflow != "wf" || trace.Status != "active" {
		t.Errorf("root = %+v", trace)
	}
	if len(trace.Lanes) != 2 || len(trace.Layers) != 2 {
		t.Fatalf("lanes=%d layers=%d, want 2/2", len(trace.Lanes), len(trace.Layers))
	}
	if got := len(trace.Lanes[0].Markers); got != 2 {
		t.Errorf("analyzer markers = %d, want 2 (tool+error; thinking excluded)", got)
	}
	if got := len(trace.Lanes[1].Markers); got != 0 {
		t.Errorf("builder markers = %d, want 0 (thinking excluded by default)", got)
	}
}

func TestHandleGetWorkflowTrace_CategoriesNarrow(t *testing.T) {
	t.Parallel()
	srv, _ := newTraceHandlerServer(t)
	rec := getTrace(t, srv, "/api/v1/workflow-instances/wfi-1/trace?categories=tool")
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d", rec.Code)
	}
	var trace types.TraceResponse
	json.Unmarshal(rec.Body.Bytes(), &trace) //nolint:errcheck
	if got := len(trace.Lanes[0].Markers); got != 1 {
		t.Errorf("markers = %d, want 1 (tool only)", got)
	}
}

func TestHandleGetWorkflowTrace_Errors(t *testing.T) {
	t.Parallel()
	srv, _ := newTraceHandlerServer(t)
	cases := []struct {
		url  string
		want int
	}{
		{"/api/v1/workflow-instances/no-such/trace", http.StatusNotFound},
		{"/api/v1/workflow-instances/wfi-1/trace?categories=bogus", http.StatusBadRequest},
		{"/api/v1/workflow-instances/wfi-1/trace?marker_limit=0", http.StatusBadRequest},
		{"/api/v1/workflow-instances/wfi-1/trace?marker_limit=99999", http.StatusBadRequest},
	}
	for _, tc := range cases {
		rec := getTrace(t, srv, tc.url)
		if rec.Code != tc.want {
			t.Errorf("%s → %d, want %d", tc.url, rec.Code, tc.want)
		}
	}
}

func TestHandleGetWorkflowTrace_EmptyIID(t *testing.T) {
	t.Parallel()
	srv, _ := newTraceHandlerServer(t)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/workflow-instances//trace", nil)
	rec := httptest.NewRecorder()
	srv.handleGetWorkflowTrace(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", rec.Code)
	}
}
