package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"be/internal/db"
)

// seedWorkflowOnly inserts a workflow FK row for an already-seeded project
// (unlike seedProjectOnly, it does not also insert the project row).
func seedWorkflowOnly(t *testing.T, database *db.DB, projectID, workflowID, scopeType string) {
	t.Helper()
	_, err := database.Exec(`INSERT INTO workflows (project_id, id, description, scope_type, created_at, updated_at)
		VALUES (?, ?, 'WF', ?, datetime('now'), datetime('now'))`, projectID, workflowID, scopeType)
	if err != nil {
		t.Fatalf("seedWorkflowOnly workflow(%s/%s): %v", projectID, workflowID, err)
	}
}

// TestHandleGetProjectWorkflow_HiddenWorkflowsExcluded verifies that
// _delegate_host and __spec_import__ instances are absent from all_workflows,
// that a normal project run is still listed, and that has_workflow is false
// when hidden instances are the only ones present.
func TestHandleGetProjectWorkflow_HiddenWorkflowsExcluded(t *testing.T) {
	s, database := newActiveWorkflowsServer(t)
	defer database.Close()

	seedProjectOnly(t, database, "proj-hidden", "Hidden Project", "wf-visible", "project")
	insertWorkflowInstance(t, database, "wfi-visible-1", "proj-hidden", "", "wf-visible", "active", "project")

	seedWorkflowOnly(t, database, "proj-hidden", "_delegate_host", "project")
	insertWorkflowInstance(t, database, "wfi-delegate-1", "proj-hidden", "", "_delegate_host", "active", "project")

	seedWorkflowOnly(t, database, "proj-hidden", "__spec_import__", "project")
	insertWorkflowInstance(t, database, "wfi-spec-1", "proj-hidden", "", "__spec_import__", "active", "project")

	req := httptest.NewRequest(http.MethodGet, "/api/v1/projects/proj-hidden/workflow", nil)
	req.SetPathValue("id", "proj-hidden")
	rr := httptest.NewRecorder()
	s.handleGetProjectWorkflow(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body: %s", rr.Code, rr.Body.String())
	}
	var resp map[string]interface{}
	if err := json.NewDecoder(rr.Body).Decode(&resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if resp["has_workflow"] != true {
		t.Errorf("has_workflow = %v, want true (visible instance present)", resp["has_workflow"])
	}
	allWorkflows, ok := resp["all_workflows"].(map[string]interface{})
	if !ok {
		t.Fatalf("all_workflows field missing or wrong type: %v", resp["all_workflows"])
	}
	if _, ok := allWorkflows["wfi-visible-1"]; !ok {
		t.Error("wfi-visible-1 should appear in all_workflows")
	}
	if _, ok := allWorkflows["wfi-delegate-1"]; ok {
		t.Error("wfi-delegate-1 (_delegate_host) must not appear in all_workflows")
	}
	if _, ok := allWorkflows["wfi-spec-1"]; ok {
		t.Error("wfi-spec-1 (__spec_import__) must not appear in all_workflows")
	}
	if len(allWorkflows) != 1 {
		t.Errorf("all_workflows len = %d, want 1", len(allWorkflows))
	}
}

// TestHandleGetProjectWorkflow_OnlyHiddenInstances_NoWorkflow verifies that
// has_workflow is false when every instance in the project is hidden.
func TestHandleGetProjectWorkflow_OnlyHiddenInstances_NoWorkflow(t *testing.T) {
	s, database := newActiveWorkflowsServer(t)
	defer database.Close()

	seedProjectOnly(t, database, "proj-only-hidden", "Only Hidden Project", "_delegate_host", "project")
	insertWorkflowInstance(t, database, "wfi-delegate-only", "proj-only-hidden", "", "_delegate_host", "active", "project")

	req := httptest.NewRequest(http.MethodGet, "/api/v1/projects/proj-only-hidden/workflow", nil)
	req.SetPathValue("id", "proj-only-hidden")
	rr := httptest.NewRecorder()
	s.handleGetProjectWorkflow(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body: %s", rr.Code, rr.Body.String())
	}
	var resp map[string]interface{}
	if err := json.NewDecoder(rr.Body).Decode(&resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if resp["has_workflow"] != false {
		t.Errorf("has_workflow = %v, want false (only hidden instances)", resp["has_workflow"])
	}
}
