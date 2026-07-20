package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"be/internal/db"
)

// insertHandoffDigestSession seeds the projects/workflows/workflow_instances/
// agent_sessions chain newContextLedgerServer's helper needs, with an
// explicit node_id (insertCtxLedgerSession leaves it blank), since the
// handoff-digest handler keys off (workflow_instance_id, node_id).
func insertHandoffDigestSession(t *testing.T, pool *db.Pool, id, projectID, wfiID, nodeID string) {
	t.Helper()
	now := "2025-08-01T09:00:00Z"
	for _, q := range []struct {
		sql  string
		args []interface{}
	}{
		{`INSERT INTO projects (id, name, created_at, updated_at) VALUES (?, 'HD Project', ?, ?)`, []interface{}{projectID, now, now}},
		{`INSERT INTO workflows (project_id, id, description, scope_type, created_at, updated_at) VALUES (?, 'hd-wf', '', 'project', ?, ?)`, []interface{}{projectID, now, now}},
		{`INSERT INTO workflow_instances (id, project_id, ticket_id, workflow_id, status, scope_type, created_at, updated_at) VALUES (?, ?, '', 'hd-wf', 'active', 'project', ?, ?)`, []interface{}{wfiID, projectID, now, now}},
		{`INSERT INTO agent_sessions (id, project_id, ticket_id, workflow_instance_id, phase, node_id, agent_type, status, created_at, updated_at) VALUES (?, ?, '', ?, 'ph', ?, 'ag', 'running', ?, ?)`, []interface{}{id, projectID, wfiID, nodeID, now, now}},
	} {
		if _, err := pool.Exec(q.sql, q.args...); err != nil {
			t.Fatalf("insertHandoffDigestSession seed: %v", err)
		}
	}
}

func insertHandoffDigestRow(t *testing.T, pool *db.Pool, wfiID, nodeID, projectID string, version, foldCount int, content string) {
	t.Helper()
	now := "2025-08-01T09:05:00Z"
	if _, err := pool.Exec(
		`INSERT INTO refinery_autonomous_digests (workflow_instance_id, node_id, project_id, version, content, fold_count, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		wfiID, nodeID, projectID, version, content, foldCount, now, now,
	); err != nil {
		t.Fatalf("insertHandoffDigestRow seed: %v", err)
	}
}

func TestHandleGetHandoffDigest_MissingSessionID(t *testing.T) {
	s := &Server{}
	req := httptest.NewRequest(http.MethodGet, "/api/v1/sessions//handoff-digest", nil)
	// No SetPathValue("id", ...) -> extractID returns "".
	rr := httptest.NewRecorder()
	s.handleGetHandoffDigest(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", rr.Code)
	}
	assertErrorContains(t, rr, "session ID required")
}

func TestHandleGetHandoffDigest_ProjectHeader_SessionNotFound(t *testing.T) {
	s, _ := newContextLedgerServer(t)
	req := httptest.NewRequest(http.MethodGet,
		withProject("/api/v1/sessions/missing-sess/handoff-digest", "hd-proj"), nil)
	req.SetPathValue("id", "missing-sess")
	rr := httptest.NewRecorder()
	s.handleGetHandoffDigest(rr, req)

	if rr.Code != http.StatusNotFound {
		t.Errorf("status = %d, want 404", rr.Code)
	}
	assertErrorContains(t, rr, "session not found")
}

func TestHandleGetHandoffDigest_ProjectMismatch_Forbidden(t *testing.T) {
	s, pool := newContextLedgerServer(t)
	insertHandoffDigestSession(t, pool, "hd-sess-1", "hd-proj-owner", "hd-sess-1-wfi", "node-1")

	req := httptest.NewRequest(http.MethodGet,
		withProject("/api/v1/sessions/hd-sess-1/handoff-digest", "hd-proj-other"), nil)
	req.SetPathValue("id", "hd-sess-1")
	rr := httptest.NewRecorder()
	s.handleGetHandoffDigest(rr, req)

	if rr.Code != http.StatusForbidden {
		t.Errorf("status = %d, want 403", rr.Code)
	}
	assertErrorContains(t, rr, "does not belong to this project")
}

func TestHandleGetHandoffDigest_SessionExists_NoDigestRow(t *testing.T) {
	s, pool := newContextLedgerServer(t)
	insertHandoffDigestSession(t, pool, "hd-sess-2", "hd-proj-owner", "hd-sess-2-wfi", "node-2")

	req := httptest.NewRequest(http.MethodGet,
		withProject("/api/v1/sessions/hd-sess-2/handoff-digest", "hd-proj-owner"), nil)
	req.SetPathValue("id", "hd-sess-2")
	rr := httptest.NewRecorder()
	s.handleGetHandoffDigest(rr, req)

	if rr.Code != http.StatusNotFound {
		t.Errorf("status = %d, want 404", rr.Code)
	}
	assertErrorContains(t, rr, "no handoff digest")
}

func TestHandleGetHandoffDigest_DigestPresent_OK(t *testing.T) {
	s, pool := newContextLedgerServer(t)
	wfiID, nodeID := "hd-sess-3-wfi", "node-3"
	insertHandoffDigestSession(t, pool, "hd-sess-3", "hd-proj-owner", wfiID, nodeID)
	insertHandoffDigestRow(t, pool, wfiID, nodeID, "hd-proj-owner", 2, 2, "the folded digest content")

	req := httptest.NewRequest(http.MethodGet,
		withProject("/api/v1/sessions/hd-sess-3/handoff-digest", "hd-proj-owner"), nil)
	req.SetPathValue("id", "hd-sess-3")
	rr := httptest.NewRecorder()
	s.handleGetHandoffDigest(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200, body = %s", rr.Code, rr.Body.String())
	}
	var body map[string]interface{}
	if err := json.Unmarshal(rr.Body.Bytes(), &body); err != nil {
		t.Fatalf("unmarshal body: %v", err)
	}
	if body["content"] != "the folded digest content" {
		t.Errorf("content = %v, want %q", body["content"], "the folded digest content")
	}
	if v, ok := body["version"].(float64); !ok || int(v) != 2 {
		t.Errorf("version = %v, want 2", body["version"])
	}
	if v, ok := body["fold_count"].(float64); !ok || int(v) != 2 {
		t.Errorf("fold_count = %v, want 2", body["fold_count"])
	}
	if body["updated_at"] == nil || body["updated_at"] == "" {
		t.Errorf("updated_at missing/empty: %v", body["updated_at"])
	}
}
