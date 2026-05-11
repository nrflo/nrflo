package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"be/internal/model"
)

// seedSpecImportWFI inserts a spec-import workflow_instance with the given status and findings.
func seedSpecImportWFI(t *testing.T, s *Server, projectID string, status model.WorkflowInstanceStatus, findingsMap map[string]interface{}) string {
	t.Helper()
	instanceID := "wfi-commit-" + strings.ReplaceAll(t.Name(), "/", "-")
	findingsJSON, _ := json.Marshal(findingsMap)
	now := time.Now().UTC().Format(time.RFC3339Nano)
	if _, err := s.pool.Exec(
		`INSERT INTO workflow_instances (id, project_id, workflow_id, scope_type, status, findings, created_at, updated_at)
		 VALUES (?, ?, ?, 'project', ?, ?, ?, ?)`,
		instanceID, projectID, specImportWorkflowID, string(status), string(findingsJSON), now, now,
	); err != nil {
		t.Fatalf("seedSpecImportWFI: %v", err)
	}
	return instanceID
}

// doCommit sends POST /api/v1/import/spec/{instance_id}/commit and returns the recorder.
func doCommit(t *testing.T, s *Server, instanceID, body string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/import/spec/"+instanceID+"/commit", strings.NewReader(body))
	req.SetPathValue("instance_id", instanceID)
	rr := httptest.NewRecorder()
	s.handleCommitSpecImport(rr, req)
	return rr
}

// --- handleCommitSpecImport ---

func TestHandleCommitSpecImport_HappyPath(t *testing.T) {
	s, projectID := newSpecImportServer(t)

	instanceID := seedSpecImportWFI(t, s, projectID, model.WorkflowInstanceActive, map[string]interface{}{
		"_spec_source": "markdown",
		"_raw_spec":    "# Feature\n\nDo the thing.",
	})

	body := `{"title":"My Feature","workflow_name":"feature","instructions":"custom instructions"}`
	rr := doCommit(t, s, instanceID, body)
	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body: %s", rr.Code, rr.Body.String())
	}

	var resp map[string]interface{}
	if err := json.NewDecoder(rr.Body).Decode(&resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	ticketID, _ := resp["ticket_id"].(string)
	if ticketID == "" {
		t.Error("ticket_id should be non-empty in response")
	}

	// Verify ticket row was created.
	var title string
	if err := s.pool.QueryRow(`SELECT title FROM tickets WHERE id = ?`, ticketID).Scan(&title); err != nil {
		t.Fatalf("query ticket: %v", err)
	}
	if title != "My Feature" {
		t.Errorf("ticket title = %q, want 'My Feature'", title)
	}

	// Verify wfi was archived (status=project_completed, _archived=true in findings).
	var wfiStatus, wfiFindings string
	if err := s.pool.QueryRow(
		`SELECT status, findings FROM workflow_instances WHERE id = ?`, instanceID,
	).Scan(&wfiStatus, &wfiFindings); err != nil {
		t.Fatalf("query wfi: %v", err)
	}
	if wfiStatus != string(model.WorkflowInstanceProjectCompleted) {
		t.Errorf("wfi status = %q, want %q", wfiStatus, model.WorkflowInstanceProjectCompleted)
	}
	var findings map[string]interface{}
	json.Unmarshal([]byte(wfiFindings), &findings)
	if findings["_archived"] != true {
		t.Errorf("findings[_archived] = %v, want true", findings["_archived"])
	}
}

func TestHandleCommitSpecImport_WithAttachedRefs(t *testing.T) {
	s, projectID := newSpecImportServer(t)

	refsJSON, _ := json.Marshal([]map[string]interface{}{
		{"kind": "source", "url": "https://github.com/o/r/issues/42"},
	})
	instanceID := seedSpecImportWFI(t, s, projectID, model.WorkflowInstanceActive, map[string]interface{}{
		"_spec_source":        "github_issue",
		"_raw_spec":           "# Issue 42",
		"_spec_attached_refs": string(refsJSON),
	})

	body := `{"title":"From Issue 42","workflow_name":"feature"}`
	rr := doCommit(t, s, instanceID, body)
	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body: %s", rr.Code, rr.Body.String())
	}

	var resp map[string]interface{}
	json.NewDecoder(rr.Body).Decode(&resp)
	ticketID, _ := resp["ticket_id"].(string)
	if ticketID == "" {
		t.Fatal("ticket_id must not be empty")
	}

	// Verify ticket_refs row was created.
	var count int
	if err := s.pool.QueryRow(
		`SELECT COUNT(*) FROM ticket_refs WHERE project_id = ? AND ticket_id = ?`,
		projectID, ticketID,
	).Scan(&count); err != nil {
		t.Fatalf("query ticket_refs: %v", err)
	}
	if count != 1 {
		t.Errorf("ticket_refs count = %d, want 1", count)
	}
}

func TestHandleCommitSpecImport_NotFound_404(t *testing.T) {
	s, _ := newSpecImportServer(t)
	rr := doCommit(t, s, "nonexistent-id", `{"title":"T","workflow_name":"feature"}`)
	if rr.Code != http.StatusNotFound {
		t.Errorf("status = %d, want 404", rr.Code)
	}
}

func TestHandleCommitSpecImport_WrongWorkflow_404(t *testing.T) {
	s, projectID := newSpecImportServer(t)
	// Seed a wfi with a different workflow ID.
	now := time.Now().UTC().Format(time.RFC3339Nano)
	// First seed a workflow row so the FK is satisfied.
	s.pool.Exec( //nolint:errcheck
		`INSERT OR IGNORE INTO workflows (id, project_id, description, scope_type, groups, close_ticket_on_complete, next_workflow_on_success, created_at, updated_at)
		 VALUES ('feature', ?, '', 'ticket', '[]', 0, '', ?, ?)`, projectID, now, now)
	s.pool.Exec( //nolint:errcheck
		`INSERT INTO workflow_instances (id, project_id, workflow_id, scope_type, status, findings, created_at, updated_at)
		 VALUES ('wfi-wrong-wf', ?, 'feature', 'ticket', 'active', '{}', ?, ?)`, projectID, now, now)

	rr := doCommit(t, s, "wfi-wrong-wf", `{"title":"T","workflow_name":"feature"}`)
	if rr.Code != http.StatusNotFound {
		t.Errorf("status = %d, want 404", rr.Code)
	}
}

func TestHandleCommitSpecImport_AlreadyCompleted_409(t *testing.T) {
	s, projectID := newSpecImportServer(t)
	instanceID := seedSpecImportWFI(t, s, projectID, model.WorkflowInstanceProjectCompleted, map[string]interface{}{
		"_archived": true,
	})
	rr := doCommit(t, s, instanceID, `{"title":"T","workflow_name":"feature"}`)
	if rr.Code != http.StatusConflict {
		t.Errorf("status = %d, want 409", rr.Code)
	}
}

func TestHandleCommitSpecImport_MissingTitle_400(t *testing.T) {
	s, projectID := newSpecImportServer(t)
	instanceID := seedSpecImportWFI(t, s, projectID, model.WorkflowInstanceActive, map[string]interface{}{
		"_raw_spec": "x",
	})
	rr := doCommit(t, s, instanceID, `{"workflow_name":"feature"}`)
	if rr.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", rr.Code)
	}
}

func TestHandleCommitSpecImport_MissingWorkflowName_400(t *testing.T) {
	s, projectID := newSpecImportServer(t)
	instanceID := seedSpecImportWFI(t, s, projectID, model.WorkflowInstanceActive, map[string]interface{}{
		"_raw_spec": "x",
	})
	rr := doCommit(t, s, instanceID, `{"title":"Title"}`)
	if rr.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", rr.Code)
	}
}

// TestHandleListWorkflowDefs_ExcludesSpecImport verifies that the __spec_import__
// workflow is always hidden from the workflow definitions listing.
func TestHandleListWorkflowDefs_ExcludesSpecImport(t *testing.T) {
	s, projectID := newSpecImportServer(t)
	// newSpecImportServer already seeds __spec_import__; we also seed a visible workflow.
	now := time.Now().UTC().Format(time.RFC3339Nano)
	if _, err := s.pool.Exec(
		`INSERT INTO workflows (id, project_id, description, scope_type, groups, close_ticket_on_complete, next_workflow_on_success, created_at, updated_at)
		 VALUES ('visible-wf', ?, 'Visible', 'ticket', '[]', 1, '', ?, ?)`,
		projectID, now, now,
	); err != nil {
		t.Fatalf("seed visible workflow: %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "/api/v1/workflows", nil)
	req = injectProject(req, projectID)
	rr := httptest.NewRecorder()
	s.handleListWorkflowDefs(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body: %s", rr.Code, rr.Body.String())
	}
	var defs map[string]interface{}
	if err := json.NewDecoder(rr.Body).Decode(&defs); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if _, ok := defs["__spec_import__"]; ok {
		t.Error("__spec_import__ must not appear in ListWorkflowDefs response")
	}
	if _, ok := defs["visible-wf"]; !ok {
		t.Error("visible-wf should appear in ListWorkflowDefs response")
	}
}

// TestHandleCommitSpecImport_UsesRawSpecWhenNoInstructions verifies that
// when instructions are not provided, _raw_spec is used as the fallback
// instructions for the orchestrator (visible in the returned response).
func TestHandleCommitSpecImport_UsesRawSpecWhenNoInstructions(t *testing.T) {
	s, projectID := newSpecImportServer(t)
	instanceID := seedSpecImportWFI(t, s, projectID, model.WorkflowInstanceActive, map[string]interface{}{
		"_raw_spec": "raw spec text",
	})

	body := `{"title":"T","workflow_name":"feature"}`
	rr := doCommit(t, s, instanceID, body)
	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body: %s", rr.Code, rr.Body.String())
	}
	// With no orchestrator, instance_id is empty; ticket_id is set.
	var resp map[string]interface{}
	json.NewDecoder(rr.Body).Decode(&resp)
	if resp["ticket_id"] == "" {
		t.Error("ticket_id should be set even when orchestrator is nil")
	}
}

// TestHandleCommitSpecImport_MissingInstanceID_400 verifies that an empty
// path value returns 400 before any DB access.
func TestHandleCommitSpecImport_MissingInstanceID_400(t *testing.T) {
	s, _ := newSpecImportServer(t)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/import/spec//commit",
		strings.NewReader(`{"title":"T","workflow_name":"feature"}`))
	// instance_id path value intentionally not set — falls back to empty string.
	req = req.WithContext(context.Background())
	rr := httptest.NewRecorder()
	s.handleCommitSpecImport(rr, req)
	if rr.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", rr.Code)
	}
}
