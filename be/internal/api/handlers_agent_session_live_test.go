package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"be/internal/db"
)

// seedLiveTestData inserts project/workflow/wfi/session rows for live handler tests.
func seedLiveTestData(t *testing.T, pool *db.Pool) {
	t.Helper()
	now := "2025-06-01T10:00:00Z"
	for _, q := range []struct {
		sql  string
		args []interface{}
	}{
		{`INSERT INTO projects (id, name, created_at, updated_at) VALUES ('live-api-proj', 'P', ?, ?)`, []interface{}{now, now}},
		{`INSERT INTO workflows (project_id, id, description, scope_type, created_at, updated_at) VALUES ('live-api-proj', 'live-api-wf', '', 'project', ?, ?)`, []interface{}{now, now}},
		{`INSERT INTO workflow_instances (id, project_id, ticket_id, workflow_id, status, scope_type, findings, created_at, updated_at) VALUES ('live-api-wfi', 'live-api-proj', '', 'live-api-wf', 'active', 'project', '{}', ?, ?)`, []interface{}{now, now}},
		// running session with a fake pid (99999999) that PidAlive will reject — tests handler shape, not proc layer
		{`INSERT INTO agent_sessions (id, project_id, ticket_id, workflow_instance_id, phase, agent_type, status, pid, started_at, created_at, updated_at) VALUES ('live-api-s', 'live-api-proj', '', 'live-api-wfi', 'ph', 'ag', 'running', 99999999, ?, ?, ?)`, []interface{}{now, now, now}},
	} {
		if _, err := pool.Exec(q.sql, q.args...); err != nil {
			t.Fatalf("seedLiveTestData exec %q: %v", q.sql, err)
		}
	}
}

func TestHandleListLiveAgentSessions_MissingProject(t *testing.T) {
	s := &Server{}
	req := httptest.NewRequest(http.MethodGet, "/api/v1/agent-session-logs/live", nil)
	rr := httptest.NewRecorder()
	s.handleListLiveAgentSessions(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", rr.Code)
	}
	assertErrorContains(t, rr, "project")
}

func TestHandleListLiveAgentSessions_EmptyShape(t *testing.T) {
	s := newTakeControlServer(t)
	req := httptest.NewRequest(http.MethodGet,
		withProject("/api/v1/agent-session-logs/live", "no-such-proj"), nil)
	rr := httptest.NewRecorder()
	s.handleListLiveAgentSessions(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("status = %d, want 200", rr.Code)
	}

	var body map[string]interface{}
	if err := json.NewDecoder(rr.Body).Decode(&body); err != nil {
		t.Fatalf("decode body: %v", err)
	}
	if _, ok := body["sessions"]; !ok {
		t.Error("response missing 'sessions' key")
	}
	if _, ok := body["count"]; !ok {
		t.Error("response missing 'count' key")
	}
	sessions, ok := body["sessions"].([]interface{})
	if !ok {
		t.Fatalf("sessions field is not array, got %T", body["sessions"])
	}
	if len(sessions) != 0 {
		t.Errorf("sessions len = %d, want 0 (empty project)", len(sessions))
	}
	count, ok := body["count"].(float64)
	if !ok {
		t.Fatalf("count field is not number, got %T", body["count"])
	}
	if int(count) != 0 {
		t.Errorf("count = %d, want 0", int(count))
	}
}

func TestHandleListLiveAgentSessions_ResponseEnvelope(t *testing.T) {
	s := newTakeControlServer(t)
	seedLiveTestData(t, s.pool)

	req := httptest.NewRequest(http.MethodGet,
		withProject("/api/v1/agent-session-logs/live", "live-api-proj"), nil)
	rr := httptest.NewRecorder()
	s.handleListLiveAgentSessions(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("status = %d, want 200 (sessions dropped by PidAlive but envelope is valid)", rr.Code)
	}

	var body map[string]interface{}
	if err := json.NewDecoder(rr.Body).Decode(&body); err != nil {
		t.Fatalf("decode body: %v", err)
	}
	if _, ok := body["sessions"]; !ok {
		t.Error("response missing 'sessions' key")
	}
	if _, ok := body["count"]; !ok {
		t.Error("response missing 'count' key")
	}
}
