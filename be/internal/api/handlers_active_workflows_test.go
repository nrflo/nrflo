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

// newActiveWorkflowsServer creates a Server backed by a temp DB for active-workflows handler tests.
func newActiveWorkflowsServer(t *testing.T) (*Server, *db.DB) {
	t.Helper()
	dbPath := filepath.Join(t.TempDir(), "active_workflows_test.db")
	database, err := db.OpenPath(dbPath)
	if err != nil {
		t.Fatalf("failed to open db: %v", err)
	}
	if err := apiCopyTemplateDB(dbPath); err != nil {
		t.Fatalf("copy template DB: %v", err)
	}
	pool, err := db.OpenPoolExisting(dbPath, db.DefaultPoolConfig())
	if err != nil {
		t.Fatalf("failed to create pool: %v", err)
	}
	t.Cleanup(func() { pool.Close() })
	s := &Server{dataPath: dbPath, pool: pool, clock: clock.Real()}
	return s, database
}

// insertWorkflowInstance inserts a workflow_instance row for handler tests.
func insertWorkflowInstance(t *testing.T, database *db.DB, id, projectID, ticketID, workflowID, status, scopeType string) {
	t.Helper()
	now := time.Now().UTC().Format(time.RFC3339Nano)
	_, err := database.Exec(`
		INSERT INTO workflow_instances (id, project_id, ticket_id, workflow_id, status, scope_type, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		id, projectID, ticketID, workflowID, status, scopeType, now, now)
	if err != nil {
		t.Fatalf("insertWorkflowInstance(%s): %v", id, err)
	}
}

// seedProjectOnly inserts just the project and a workflow FK row (no workflow_instance).
func seedProjectOnly(t *testing.T, database *db.DB, projectID, projectName, workflowID, workflowScopeType string) {
	t.Helper()
	_, err := database.Exec(`INSERT INTO projects (id, name, created_at, updated_at)
		VALUES (?, ?, datetime('now'), datetime('now'))`, projectID, projectName)
	if err != nil {
		t.Fatalf("seedProjectOnly(%s): %v", projectID, err)
	}
	_, err = database.Exec(`INSERT INTO workflows (project_id, id, description, scope_type, created_at, updated_at)
		VALUES (?, ?, 'WF', ?, datetime('now'), datetime('now'))`, projectID, workflowID, workflowScopeType)
	if err != nil {
		t.Fatalf("seedProjectOnly workflow(%s/%s): %v", projectID, workflowID, err)
	}
}

func TestHandleGetActiveWorkflows_EmptyList(t *testing.T) {
	s, database := newActiveWorkflowsServer(t)
	defer database.Close()

	req := httptest.NewRequest(http.MethodGet, "/api/v1/workflows/active", nil)
	rr := httptest.NewRecorder()
	s.handleGetActiveWorkflows(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rr.Code)
	}
	var resp map[string]interface{}
	if err := json.NewDecoder(rr.Body).Decode(&resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	workflows, ok := resp["workflows"].([]interface{})
	if !ok {
		t.Fatalf("workflows field missing or wrong type: %v", resp["workflows"])
	}
	if len(workflows) != 0 {
		t.Errorf("workflows = %d, want 0", len(workflows))
	}
	count, ok := resp["count"].(float64)
	if !ok {
		t.Fatalf("count field missing or wrong type: %v", resp["count"])
	}
	if count != 0 {
		t.Errorf("count = %v, want 0", count)
	}
}

func TestHandleGetActiveWorkflows_ProjectScopedInstanceReturned(t *testing.T) {
	s, database := newActiveWorkflowsServer(t)
	defer database.Close()

	seedProjectOnly(t, database, "proj-ps", "PS Project", "wf-ps", "project")
	insertWorkflowInstance(t, database, "wfi-ps-1", "proj-ps", "", "wf-ps", "active", "project")

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
	if !ok || len(workflows) != 1 {
		t.Fatalf("expected 1 workflow, got %v", resp["workflows"])
	}
	wf := workflows[0].(map[string]interface{})
	if wf["scope_type"] != "project" {
		t.Errorf("scope_type = %v, want project", wf["scope_type"])
	}
	if wf["instance_id"] != "wfi-ps-1" {
		t.Errorf("instance_id = %v, want wfi-ps-1", wf["instance_id"])
	}
	if wf["project_id"] != "proj-ps" {
		t.Errorf("project_id = %v, want proj-ps", wf["project_id"])
	}
	if wf["ticket_id"] != "" {
		t.Errorf("ticket_id = %v, want empty string", wf["ticket_id"])
	}
	if wf["workflow_id"] != "wf-ps" {
		t.Errorf("workflow_id = %v, want wf-ps", wf["workflow_id"])
	}
	if wf["status"] != "active" {
		t.Errorf("status = %v, want active", wf["status"])
	}
	count, _ := resp["count"].(float64)
	if count != 1 {
		t.Errorf("count = %v, want 1", count)
	}
}

func TestHandleGetActiveWorkflows_CompletedAndFailedExcluded(t *testing.T) {
	s, database := newActiveWorkflowsServer(t)
	defer database.Close()

	seedProjectOnly(t, database, "proj-cf", "CF Project", "wf-cf", "ticket")
	insertWorkflowInstance(t, database, "wfi-active", "proj-cf", "TKT-1", "wf-cf", "active", "ticket")
	insertWorkflowInstance(t, database, "wfi-done", "proj-cf", "TKT-2", "wf-cf", "completed", "ticket")
	insertWorkflowInstance(t, database, "wfi-fail", "proj-cf", "TKT-3", "wf-cf", "failed", "ticket")

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
	if len(workflows) != 1 {
		t.Errorf("workflows count = %d, want 1 (only active)", len(workflows))
	}
	if len(workflows) == 1 {
		id := workflows[0].(map[string]interface{})["instance_id"]
		if id != "wfi-active" {
			t.Errorf("instance_id = %v, want wfi-active", id)
		}
	}
}

func TestHandleGetActiveWorkflows_SpecImportExcluded(t *testing.T) {
	s, database := newActiveWorkflowsServer(t)
	defer database.Close()

	seedProjectOnly(t, database, "proj-si", "SI Project", specImportWorkflowID, "project")
	insertWorkflowInstance(t, database, "wfi-spec", "proj-si", "", specImportWorkflowID, "active", "project")

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
	if len(workflows) != 0 {
		t.Errorf("workflows = %d, want 0 (spec import filtered)", len(workflows))
	}
	count, _ := resp["count"].(float64)
	if count != 0 {
		t.Errorf("count = %v, want 0", count)
	}
}

func TestHandleGetActiveWorkflows_TicketScopedInstanceReturned(t *testing.T) {
	s, database := newActiveWorkflowsServer(t)
	defer database.Close()

	seedProjectOnly(t, database, "proj-ts", "TS Project", "wf-ts", "ticket")
	insertWorkflowInstance(t, database, "wfi-ts-1", "proj-ts", "TKT-42", "wf-ts", "active", "ticket")

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
	if !ok || len(workflows) != 1 {
		t.Fatalf("expected 1 workflow, got %v", resp["workflows"])
	}
	wf := workflows[0].(map[string]interface{})
	if wf["scope_type"] != "ticket" {
		t.Errorf("scope_type = %v, want ticket", wf["scope_type"])
	}
	if wf["ticket_id"] != "TKT-42" {
		t.Errorf("ticket_id = %v, want TKT-42", wf["ticket_id"])
	}
}

func TestHandleGetActiveWorkflows_ResponseShape(t *testing.T) {
	s, database := newActiveWorkflowsServer(t)
	defer database.Close()

	seedProjectOnly(t, database, "proj-shape", "Shape Project", "wf-shape", "project")
	insertWorkflowInstance(t, database, "wfi-shape-1", "proj-shape", "", "wf-shape", "active", "project")

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
	if !ok || len(workflows) != 1 {
		t.Fatalf("expected 1 workflow, got %v", resp["workflows"])
	}
	wf := workflows[0].(map[string]interface{})
	for _, field := range []string{"instance_id", "project_id", "ticket_id", "workflow_id", "scope_type", "status"} {
		if _, exists := wf[field]; !exists {
			t.Errorf("response missing field %q", field)
		}
	}
}

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
