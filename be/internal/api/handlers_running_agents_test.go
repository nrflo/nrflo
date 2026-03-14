package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
	"time"

	"be/internal/clock"
	"be/internal/db"
)

// newRunningAgentsServer creates a Server with a temp DB for running-agents handler tests.
// Returns the server and an open DB connection for test data setup.
func newRunningAgentsServer(t *testing.T) (*Server, *db.DB) {
	t.Helper()
	dbPath := filepath.Join(t.TempDir(), "running_agents_handler_test.db")
	database, err := db.OpenPath(dbPath)
	if err != nil {
		t.Fatalf("failed to open db: %v", err)
	}
	pool, err := db.NewPoolPath(dbPath, db.DefaultPoolConfig())
	if err != nil {
		t.Fatalf("failed to create pool: %v", err)
	}
	t.Cleanup(func() { pool.Close() })
	s := &Server{dataPath: dbPath, pool: pool, clock: clock.Real()}
	return s, database
}

// insertHandlerSession inserts an agent_session row for handler tests.
func insertHandlerSession(t *testing.T, database *db.DB, id, wfiID, projectID, status, startedAt string) {
	t.Helper()
	now := time.Now().UTC().Format(time.RFC3339Nano)
	_, err := database.Exec(`
		INSERT INTO agent_sessions
		(id, project_id, ticket_id, workflow_instance_id, phase, agent_type, model_id, status, started_at, created_at, updated_at)
		VALUES (?, ?, 'TKT-1', ?, 'impl', 'implementor', 'sonnet', ?, ?, ?, ?)`,
		id, projectID, wfiID, status, startedAt, now, now)
	if err != nil {
		t.Fatalf("insertHandlerSession(%s): %v", id, err)
	}
}

// seedProject inserts a project and returns its workflow-instance ID.
func seedProject(t *testing.T, database *db.DB, projectID, projectName string) string {
	t.Helper()
	_, err := database.Exec(`INSERT INTO projects (id, name, created_at, updated_at)
		VALUES (?, ?, datetime('now'), datetime('now'))`, projectID, projectName)
	if err != nil {
		t.Fatalf("seedProject(%s): %v", projectID, err)
	}
	_, err = database.Exec(`INSERT INTO workflows (project_id, id, description, scope_type, phases, created_at, updated_at)
		VALUES (?, 'wf-1', 'WF', 'ticket', '[]', datetime('now'), datetime('now'))`, projectID)
	if err != nil {
		t.Fatalf("seedProject workflow(%s): %v", projectID, err)
	}
	wfiID := "wfi-" + projectID
	_, err = database.Exec(`INSERT INTO workflow_instances (id, project_id, ticket_id, workflow_id, status, scope_type, findings, created_at, updated_at)
		VALUES (?, ?, 'TKT-1', 'wf-1', 'active', 'ticket', '{}', datetime('now'), datetime('now'))`, wfiID, projectID)
	if err != nil {
		t.Fatalf("seedProject wfi(%s): %v", projectID, err)
	}
	return wfiID
}

func TestHandleGetRunningAgents_EmptyList(t *testing.T) {
	s, database := newRunningAgentsServer(t)
	defer database.Close()

	req := httptest.NewRequest(http.MethodGet, "/api/v1/agents/running", nil)
	rr := httptest.NewRecorder()
	s.handleGetRunningAgents(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rr.Code)
	}

	var resp map[string]interface{}
	if err := json.NewDecoder(rr.Body).Decode(&resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}
	agents, ok := resp["agents"].([]interface{})
	if !ok {
		t.Fatalf("agents field missing or wrong type: %v", resp["agents"])
	}
	if len(agents) != 0 {
		t.Errorf("agents = %d, want 0", len(agents))
	}
	count, ok := resp["count"].(float64)
	if !ok {
		t.Fatalf("count field missing or wrong type: %v", resp["count"])
	}
	if count != 0 {
		t.Errorf("count = %v, want 0", count)
	}
}

func TestHandleGetRunningAgents_ResponseShape(t *testing.T) {
	s, database := newRunningAgentsServer(t)
	defer database.Close()

	wfiID := seedProject(t, database, "proj-shape", "Shape Project")
	startedAt := time.Now().UTC().Add(-60 * time.Second).Format(time.RFC3339Nano)
	insertHandlerSession(t, database, "sess-shape-1", wfiID, "proj-shape", "running", startedAt)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/agents/running", nil)
	rr := httptest.NewRecorder()
	s.handleGetRunningAgents(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rr.Code)
	}

	var resp map[string]interface{}
	if err := json.NewDecoder(rr.Body).Decode(&resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	agents, ok := resp["agents"].([]interface{})
	if !ok || len(agents) != 1 {
		t.Fatalf("expected 1 agent, got %v", resp["agents"])
	}

	agent := agents[0].(map[string]interface{})
	requiredFields := []string{"session_id", "project_id", "project_name", "ticket_id",
		"workflow_id", "agent_type", "model_id", "phase", "started_at", "elapsed_sec"}
	for _, field := range requiredFields {
		if _, exists := agent[field]; !exists {
			t.Errorf("response missing field %q", field)
		}
	}

	if agent["session_id"] != "sess-shape-1" {
		t.Errorf("session_id = %v, want sess-shape-1", agent["session_id"])
	}
	if agent["project_id"] != "proj-shape" {
		t.Errorf("project_id = %v, want proj-shape", agent["project_id"])
	}
	if agent["project_name"] != "Shape Project" {
		t.Errorf("project_name = %v, want Shape Project", agent["project_name"])
	}
	if agent["workflow_id"] != "wf-1" {
		t.Errorf("workflow_id = %v, want wf-1", agent["workflow_id"])
	}

	count, _ := resp["count"].(float64)
	if count != 1 {
		t.Errorf("count = %v, want 1", count)
	}
}

func TestHandleGetRunningAgents_ElapsedSec(t *testing.T) {
	fixedNow := time.Date(2025, 6, 1, 12, 0, 0, 0, time.UTC)
	testClock := clock.NewTest(fixedNow)

	dbPath := filepath.Join(t.TempDir(), "elapsed_test.db")
	database, err := db.OpenPath(dbPath)
	if err != nil {
		t.Fatalf("failed to open db: %v", err)
	}
	defer database.Close()

	pool, err := db.NewPoolPath(dbPath, db.DefaultPoolConfig())
	if err != nil {
		t.Fatalf("failed to create pool: %v", err)
	}
	defer pool.Close()

	s := &Server{dataPath: dbPath, pool: pool, clock: testClock}
	wfiID := seedProject(t, database, "proj-elapsed", "Elapsed Project")
	startedAt := fixedNow.Add(-120 * time.Second).Format(time.RFC3339Nano)
	insertHandlerSession(t, database, "sess-elapsed", wfiID, "proj-elapsed", "running", startedAt)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/agents/running", nil)
	rr := httptest.NewRecorder()
	s.handleGetRunningAgents(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rr.Code)
	}

	var resp map[string]interface{}
	if err := json.NewDecoder(rr.Body).Decode(&resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}
	agents := resp["agents"].([]interface{})
	if len(agents) != 1 {
		t.Fatalf("expected 1 agent, got %d", len(agents))
	}
	elapsedSec := agents[0].(map[string]interface{})["elapsed_sec"].(float64)
	if elapsedSec != 120 {
		t.Errorf("elapsed_sec = %v, want 120", elapsedSec)
	}
}

func TestHandleGetRunningAgents_LimitCustom(t *testing.T) {
	s, database := newRunningAgentsServer(t)
	defer database.Close()

	wfiID := seedProject(t, database, "proj-limit", "Limit Project")
	now := time.Now().UTC()
	for i := 0; i < 5; i++ {
		startedAt := now.Add(time.Duration(-i) * time.Minute).Format(time.RFC3339Nano)
		insertHandlerSession(t, database, "sess-limit-"+string(rune('A'+i)), wfiID, "proj-limit", "running", startedAt)
	}

	req := httptest.NewRequest(http.MethodGet, "/api/v1/agents/running?limit=3", nil)
	rr := httptest.NewRecorder()
	s.handleGetRunningAgents(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rr.Code)
	}

	var resp map[string]interface{}
	if err := json.NewDecoder(rr.Body).Decode(&resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}
	agents := resp["agents"].([]interface{})
	if len(agents) != 3 {
		t.Errorf("agents count = %d, want 3 (limit param applied)", len(agents))
	}
	count := resp["count"].(float64)
	if count != 3 {
		t.Errorf("count = %v, want 3", count)
	}
}

func TestHandleGetRunningAgents_LimitCappedAt100(t *testing.T) {
	s, database := newRunningAgentsServer(t)
	defer database.Close()

	// Insert fewer sessions than the cap; just verify limit > 100 doesn't error.
	wfiID := seedProject(t, database, "proj-cap", "Cap Project")
	now := time.Now().UTC()
	for i := 0; i < 3; i++ {
		startedAt := now.Add(time.Duration(-i) * time.Minute).Format(time.RFC3339Nano)
		insertHandlerSession(t, database, "sess-cap-"+string(rune('A'+i)), wfiID, "proj-cap", "running", startedAt)
	}

	req := httptest.NewRequest(http.MethodGet, "/api/v1/agents/running?limit=999", nil)
	rr := httptest.NewRecorder()
	s.handleGetRunningAgents(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 (limit capped, not rejected)", rr.Code)
	}
	var resp map[string]interface{}
	if err := json.NewDecoder(rr.Body).Decode(&resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}
	// All 3 sessions returned (cap is 100, we only have 3)
	agents := resp["agents"].([]interface{})
	if len(agents) != 3 {
		t.Errorf("agents count = %d, want 3", len(agents))
	}
}

func TestHandleGetRunningAgents_NoProjectHeaderRequired(t *testing.T) {
	s, database := newRunningAgentsServer(t)
	defer database.Close()

	// No X-Project header and no ?project= param — must return 200 (global endpoint).
	req := httptest.NewRequest(http.MethodGet, "/api/v1/agents/running", nil)
	rr := httptest.NewRecorder()
	s.handleGetRunningAgents(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("status = %d, want 200 (no project header required)", rr.Code)
	}
}
