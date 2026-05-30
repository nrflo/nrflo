package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"be/internal/clock"
	"be/internal/db"

	"path/filepath"
)

func TestHandleGetActiveWorkflows_MixedScopesAndSpecImport(t *testing.T) {
	s, database := newActiveWorkflowsServer(t)
	defer database.Close()

	seedProjectOnly(t, database, "proj-mix", "Mix Project", "wf-ticket", "ticket")
	// Add a second workflow for project scope
	_, err := database.Exec(`INSERT INTO workflows (project_id, id, description, scope_type, created_at, updated_at)
		VALUES ('proj-mix', 'wf-proj', 'WF project', 'project', datetime('now'), datetime('now'))`)
	if err != nil {
		t.Fatalf("insert project workflow: %v", err)
	}
	// Add specImport workflow FK
	_, err = database.Exec(`INSERT INTO workflows (project_id, id, description, scope_type, created_at, updated_at)
		VALUES ('proj-mix', ?, 'spec import wf', 'project', datetime('now'), datetime('now'))`, specImportWorkflowID)
	if err != nil {
		t.Fatalf("insert spec import workflow: %v", err)
	}

	insertWorkflowInstance(t, database, "wfi-ticket-active", "proj-mix", "TKT-1", "wf-ticket", "active", "ticket")
	insertWorkflowInstance(t, database, "wfi-proj-active", "proj-mix", "", "wf-proj", "active", "project")
	insertWorkflowInstance(t, database, "wfi-spec-active", "proj-mix", "", specImportWorkflowID, "active", "project")
	insertWorkflowInstance(t, database, "wfi-completed", "proj-mix", "TKT-2", "wf-ticket", "completed", "ticket")

	req := httptest.NewRequest(http.MethodGet, "/api/v1/workflows/active", nil)
	rr := httptest.NewRecorder()
	s.handleGetActiveWorkflows(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rr.Code)
	}
	var resp map[string]interface{}
	if err := json.NewDecoder(rr.Body).Decode(&resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	workflows, ok := resp["workflows"].([]interface{})
	if !ok {
		t.Fatalf("workflows field missing: %v", resp)
	}
	// Only ticket-active and proj-active; spec-import and completed excluded
	if len(workflows) != 2 {
		t.Errorf("workflows count = %d, want 2", len(workflows))
	}
	seen := map[string]bool{}
	for _, w := range workflows {
		id := w.(map[string]interface{})["instance_id"].(string)
		seen[id] = true
	}
	if !seen["wfi-ticket-active"] {
		t.Errorf("wfi-ticket-active not in response")
	}
	if !seen["wfi-proj-active"] {
		t.Errorf("wfi-proj-active not in response")
	}
	count, _ := resp["count"].(float64)
	if count != 2 {
		t.Errorf("count = %v, want 2", count)
	}
}

func TestHandleGetActiveWorkflows_NoProjectHeaderRequired(t *testing.T) {
	s, database := newActiveWorkflowsServer(t)
	defer database.Close()

	// No X-Project header — must return 200 (global endpoint)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/workflows/active", nil)
	rr := httptest.NewRecorder()
	s.handleGetActiveWorkflows(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("status = %d, want 200 (no project header required)", rr.Code)
	}
}

func TestHandleGetActiveWorkflows_ContentType(t *testing.T) {
	s, database := newActiveWorkflowsServer(t)
	defer database.Close()

	req := httptest.NewRequest(http.MethodGet, "/api/v1/workflows/active", nil)
	rr := httptest.NewRecorder()
	s.handleGetActiveWorkflows(rr, req)

	ct := rr.Header().Get("Content-Type")
	if ct != "application/json" {
		t.Errorf("Content-Type = %q, want application/json", ct)
	}
}

func TestHandleGetActiveWorkflows_WaitingStatusExcluded(t *testing.T) {
	s, database := newActiveWorkflowsServer(t)
	defer database.Close()

	seedProjectOnly(t, database, "proj-wait", "Wait Project", "wf-wait", "ticket")
	insertWorkflowInstance(t, database, "wfi-waiting", "proj-wait", "TKT-1", "wf-wait", "waiting", "ticket")
	insertWorkflowInstance(t, database, "wfi-active2", "proj-wait", "TKT-2", "wf-wait", "active", "ticket")

	req := httptest.NewRequest(http.MethodGet, "/api/v1/workflows/active", nil)
	rr := httptest.NewRecorder()
	s.handleGetActiveWorkflows(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rr.Code)
	}
	var resp map[string]interface{}
	if err := json.NewDecoder(rr.Body).Decode(&resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	workflows := resp["workflows"].([]interface{})
	if len(workflows) != 1 {
		t.Errorf("workflows count = %d, want 1 (waiting excluded)", len(workflows))
	}
	if len(workflows) == 1 {
		id := workflows[0].(map[string]interface{})["instance_id"]
		if id != "wfi-active2" {
			t.Errorf("instance_id = %v, want wfi-active2", id)
		}
	}
}

func TestHandleGetActiveWorkflows_MultipleProjects(t *testing.T) {
	s, database := newActiveWorkflowsServer(t)
	defer database.Close()

	seedProjectOnly(t, database, "proj-a", "Project A", "wf-a", "ticket")
	seedProjectOnly(t, database, "proj-b", "Project B", "wf-b", "project")
	insertWorkflowInstance(t, database, "wfi-a", "proj-a", "TKT-1", "wf-a", "active", "ticket")
	insertWorkflowInstance(t, database, "wfi-b", "proj-b", "", "wf-b", "active", "project")

	req := httptest.NewRequest(http.MethodGet, "/api/v1/workflows/active", nil)
	rr := httptest.NewRecorder()
	s.handleGetActiveWorkflows(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rr.Code)
	}
	var resp map[string]interface{}
	if err := json.NewDecoder(rr.Body).Decode(&resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	workflows := resp["workflows"].([]interface{})
	if len(workflows) != 2 {
		t.Errorf("workflows count = %d, want 2 (cross-project)", len(workflows))
	}
	projectIDs := map[string]bool{}
	for _, w := range workflows {
		pid := w.(map[string]interface{})["project_id"].(string)
		projectIDs[pid] = true
	}
	if !projectIDs["proj-a"] || !projectIDs["proj-b"] {
		t.Errorf("expected both proj-a and proj-b in response, got %v", projectIDs)
	}
}

func TestHandleGetActiveWorkflows_ClockNotRequired(t *testing.T) {
	fixedNow := time.Date(2025, 6, 1, 12, 0, 0, 0, time.UTC)
	testClock := clock.NewTest(fixedNow)

	dbPath := filepath.Join(t.TempDir(), "active_wf_clock_test.db")
	database, err := db.OpenPath(dbPath)
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	defer database.Close()
	if err := apiCopyTemplateDB(dbPath); err != nil {
		t.Fatalf("copy template DB: %v", err)
	}
	pool, err := db.OpenPoolExisting(dbPath, db.DefaultPoolConfig())
	if err != nil {
		t.Fatalf("create pool: %v", err)
	}
	defer pool.Close()

	s := &Server{dataPath: dbPath, pool: pool, clock: testClock}
	seedProjectOnly(t, database, "proj-clk", "Clock Project", "wf-clk", "project")
	insertWorkflowInstance(t, database, "wfi-clk", "proj-clk", "", "wf-clk", "active", "project")

	req := httptest.NewRequest(http.MethodGet, "/api/v1/workflows/active", nil)
	rr := httptest.NewRecorder()
	s.handleGetActiveWorkflows(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rr.Code)
	}
	var resp map[string]interface{}
	if err := json.NewDecoder(rr.Body).Decode(&resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	workflows := resp["workflows"].([]interface{})
	if len(workflows) != 1 {
		t.Errorf("workflows = %d, want 1", len(workflows))
	}
}
