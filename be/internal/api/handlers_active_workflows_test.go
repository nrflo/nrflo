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
	if err := apiCopyTemplateDB(dbPath); err != nil {
		t.Fatalf("copy template DB: %v", err)
	}
	database, err := db.OpenPathExisting(dbPath)
	if err != nil {
		t.Fatalf("failed to open db: %v", err)
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

func TestHandleGetActiveWorkflows_DelegateHostExcluded(t *testing.T) {
	s, database := newActiveWorkflowsServer(t)
	defer database.Close()

	seedProjectOnly(t, database, "proj-dh", "DH Project", "_delegate_host", "project")
	insertWorkflowInstance(t, database, "wfi-delegate", "proj-dh", "", "_delegate_host", "active", "project")

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
		t.Errorf("workflows = %d, want 0 (_delegate_host filtered)", len(workflows))
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
