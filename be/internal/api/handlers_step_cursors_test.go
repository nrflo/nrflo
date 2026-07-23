package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"be/internal/clock"
	"be/internal/db"
)

// newStepCursorsHandlerServer seeds a project/workflow/instance chain for
// GET /workflow-instances/{iid}/steps tests, mirroring newTraceHandlerServer.
func newStepCursorsHandlerServer(t *testing.T) *Server {
	t.Helper()
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "stepcursors.db")
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
		{`INSERT INTO projects (id, name, created_at, updated_at) VALUES ('sc-proj', 'sc-proj', ?, ?)`, []interface{}{now, now}},
		{`INSERT INTO workflows (project_id, id, description, scope_type, created_at, updated_at) VALUES ('sc-proj', 'wf', '', 'ticket', ?, ?)`, []interface{}{now, now}},
		{`INSERT INTO workflow_instances (id, project_id, ticket_id, workflow_id, status, scope_type, created_at, updated_at) VALUES ('sc-wfi', 'sc-proj', '', 'wf', 'active', 'ticket', ?, ?)`, []interface{}{now, now}},
		{`INSERT INTO workflow_instances (id, project_id, ticket_id, workflow_id, status, scope_type, created_at, updated_at) VALUES ('sc-wfi-empty', 'sc-proj', '', 'wf', 'active', 'ticket', ?, ?)`, []interface{}{now, now}},
	} {
		if _, err := pool.Exec(q.sql, q.args...); err != nil {
			t.Fatalf("seed: %v\nsql: %s", err, q.sql)
		}
	}

	stepsJSON := `[{"step_id":"s1","title":"Step One","instruction":"do 1"},{"step_id":"s2","title":"Step Two","instruction":"do 2"}]`
	if _, err := pool.Exec(`
		INSERT INTO agent_step_cursors (workflow_instance_id, node_id, steps_snapshot, revision, current_index, completed, rejections, created_at, updated_at)
		VALUES ('sc-wfi', 'node-a', ?, 1, 0, '[]', '{}', ?, ?)`, stepsJSON, now, now); err != nil {
		t.Fatalf("seed cursor: %v", err)
	}

	return &Server{pool: pool, clock: clock.Real()}
}

func getStepCursors(t *testing.T, srv *Server, iid string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/workflow-instances/"+iid+"/steps", nil)
	req.SetPathValue("iid", iid)
	rec := httptest.NewRecorder()
	srv.handleGetStepCursors(rec, req)
	return rec
}

func TestHandleGetStepCursors_HappyPathPopulated(t *testing.T) {
	t.Parallel()
	srv := newStepCursorsHandlerServer(t)
	rec := getStepCursors(t, srv, "sc-wfi")
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
	var body map[string]interface{}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("unmarshal body: %v", err)
	}
	if body["workflow_instance_id"] != "sc-wfi" {
		t.Errorf("workflow_instance_id = %v, want sc-wfi", body["workflow_instance_id"])
	}
	cursors, ok := body["cursors"].(map[string]interface{})
	if !ok {
		t.Fatalf("cursors = %#v, want a populated object", body["cursors"])
	}
	if _, present := cursors["node-a"]; !present {
		t.Errorf("cursors = %+v, want key node-a", cursors)
	}
}

func TestHandleGetStepCursors_EmptyInstanceReturnsEmptyCursors(t *testing.T) {
	t.Parallel()
	srv := newStepCursorsHandlerServer(t)
	rec := getStepCursors(t, srv, "sc-wfi-empty")
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
	var body map[string]interface{}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("unmarshal body: %v", err)
	}
	cursors, ok := body["cursors"].(map[string]interface{})
	if !ok {
		t.Fatalf("cursors = %#v, want an (empty) object", body["cursors"])
	}
	if len(cursors) != 0 {
		t.Errorf("cursors = %+v, want empty", cursors)
	}
}

func TestHandleGetStepCursors_EmptyIIDReturns400(t *testing.T) {
	t.Parallel()
	srv := newStepCursorsHandlerServer(t)
	rec := getStepCursors(t, srv, "")
	if rec.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400; body = %s", rec.Code, rec.Body.String())
	}
}

func TestHandleGetStepCursors_UnknownInstanceReturns404(t *testing.T) {
	t.Parallel()
	srv := newStepCursorsHandlerServer(t)
	rec := getStepCursors(t, srv, "does-not-exist")
	if rec.Code != http.StatusNotFound {
		t.Errorf("status = %d, want 404; body = %s", rec.Code, rec.Body.String())
	}
}
