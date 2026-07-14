package integration

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"path/filepath"
	"testing"

	"be/internal/clock"
	"be/internal/db"
	"be/internal/model"
	"be/internal/repo"
	"be/internal/service"
	"be/internal/types"
)

func postJSON(t *testing.T, client *http.Client, url, project string, body map[string]string) (*http.Response, []byte) {
	t.Helper()
	bodyJSON, _ := json.Marshal(body)
	req, _ := http.NewRequest("POST", url, bytes.NewBuffer(bodyJSON))
	req.Header.Set("Content-Type", "application/json")
	if project != "" {
		req.Header.Set("X-Project", project)
	}
	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("postJSON %s: %v", url, err)
	}
	respBody, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	return resp, respBody
}

func doWorkflowContinue(t *testing.T, client *http.Client, baseURL, ticketID, project string, body map[string]string) (*http.Response, []byte) {
	t.Helper()
	return postJSON(t, client, baseURL+"/api/v1/tickets/"+ticketID+"/workflow/continue", project, body)
}

func doWorkflowFail(t *testing.T, client *http.Client, baseURL, ticketID, project string, body map[string]string) (*http.Response, []byte) {
	t.Helper()
	return postJSON(t, client, baseURL+"/api/v1/tickets/"+ticketID+"/workflow/fail", project, body)
}

func doWorkflowContinueProject(t *testing.T, client *http.Client, baseURL, projectID string, body map[string]string) (*http.Response, []byte) {
	t.Helper()
	return postJSON(t, client, baseURL+"/api/v1/projects/"+projectID+"/workflow/continue", "", body)
}

func doWorkflowFailProject(t *testing.T, client *http.Client, baseURL, projectID string, body map[string]string) (*http.Response, []byte) {
	t.Helper()
	return postJSON(t, client, baseURL+"/api/v1/projects/"+projectID+"/workflow/fail", "", body)
}

const pauseFinding = `{"paused_after_layer":0,"resume_layer":1,"event":{},"timestamp":"2026-01-01T00:00:00Z"}`

func insertPauseFinding(t *testing.T, dbPath, wfiID string) {
	t.Helper()
	database, err := db.OpenPathExisting(dbPath)
	if err != nil {
		t.Fatalf("insertPauseFinding: failed to open DB: %v", err)
	}
	defer database.Close()
	_, err = database.Exec(`
		INSERT INTO findings (id, scope, scope_id, key, value, workflow_instance_id, created_source, updated_source, created_at, updated_at)
		VALUES (lower(hex(randomblob(16))), 'workflow_instance', ?, '_pause', ?, ?, 'orchestrator', 'orchestrator', datetime('now'), datetime('now'))`,
		wfiID, pauseFinding, wfiID)
	if err != nil {
		t.Fatalf("insertPauseFinding: %v", err)
	}
}

func TestContinueWorkflowHandler_HappyPath(t *testing.T) {
	dbDir := t.TempDir()
	dbPath := filepath.Join(dbDir, "test.db")
	if err := copyTemplateDB(dbPath); err != nil {
		t.Fatalf("failed to copy template DB: %v", err)
	}

	seedProjectWithRoot(t, dbPath, "proj")
	seedWorkflowDef(t, dbPath, "proj")
	seedTicketAndWorkflow(t, dbPath, "proj", "TICK-1")

	database, err := db.OpenPathExisting(dbPath)
	if err != nil {
		t.Fatalf("failed to open DB: %v", err)
	}
	pool := db.WrapAsPool(database)
	wfiRepo := repo.NewWorkflowInstanceRepo(pool, clock.Real())
	wi, err := wfiRepo.GetByTicketAndWorkflow("proj", "TICK-1", "test")
	if err != nil {
		t.Fatalf("failed to get workflow instance: %v", err)
	}
	wfiRepo.UpdateStatus(wi.ID, model.WorkflowInstanceWaiting)
	database.Close()

	insertPauseFinding(t, dbPath, wi.ID)

	baseURL, client := startAPIServer(t, dbPath)

	resp, body := doWorkflowContinue(t, client, baseURL, "TICK-1", "proj", map[string]string{
		"workflow": "test",
	})

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", resp.StatusCode, string(body))
	}

	var result map[string]string
	json.Unmarshal(body, &result)
	if result["status"] != "continuing" {
		t.Fatalf("expected status=continuing, got %q", result["status"])
	}
	if result["instance_id"] == "" {
		t.Fatal("expected non-empty instance_id in response")
	}

	// ContinueWorkflow sets status=active synchronously before launching goroutine
	database, err = db.OpenPathExisting(dbPath)
	if err != nil {
		t.Fatalf("failed to open DB for verification: %v", err)
	}
	pool = db.WrapAsPool(database)
	wfiRepo = repo.NewWorkflowInstanceRepo(pool, clock.Real())
	wi, err = wfiRepo.Get(wi.ID)
	database.Close()
	if err != nil {
		t.Fatalf("failed to get workflow instance: %v", err)
	}
	if wi.Status != model.WorkflowInstanceActive {
		t.Errorf("expected status=active, got %s", wi.Status)
	}
}

func TestFailWorkflowHandler_WaitingInstance(t *testing.T) {
	dbDir := t.TempDir()
	dbPath := filepath.Join(dbDir, "test.db")
	if err := copyTemplateDB(dbPath); err != nil {
		t.Fatalf("failed to copy template DB: %v", err)
	}

	seedProjectWithRoot(t, dbPath, "proj")
	seedWorkflowDef(t, dbPath, "proj")
	seedTicketAndWorkflow(t, dbPath, "proj", "TICK-1")

	database, err := db.OpenPathExisting(dbPath)
	if err != nil {
		t.Fatalf("failed to open DB: %v", err)
	}
	pool := db.WrapAsPool(database)
	wfiRepo := repo.NewWorkflowInstanceRepo(pool, clock.Real())
	wi, err := wfiRepo.GetByTicketAndWorkflow("proj", "TICK-1", "test")
	if err != nil {
		t.Fatalf("failed to get workflow instance: %v", err)
	}
	wfiRepo.UpdateStatus(wi.ID, model.WorkflowInstanceWaiting)
	database.Close()

	insertPauseFinding(t, dbPath, wi.ID)

	baseURL, client := startAPIServer(t, dbPath)

	resp, body := doWorkflowFail(t, client, baseURL, "TICK-1", "proj", map[string]string{
		"workflow": "test",
		"reason":   "test reason",
	})

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", resp.StatusCode, string(body))
	}

	var result map[string]string
	json.Unmarshal(body, &result)
	if result["status"] != "failing" {
		t.Fatalf("expected status=failing, got %q", result["status"])
	}

	// FailWorkflow on a waiting instance is synchronous (markFailed directly)
	database, err = db.OpenPathExisting(dbPath)
	if err != nil {
		t.Fatalf("failed to open DB for verification: %v", err)
	}
	pool = db.WrapAsPool(database)
	wfiRepo = repo.NewWorkflowInstanceRepo(pool, clock.Real())
	wi, err = wfiRepo.Get(wi.ID)
	database.Close()
	if err != nil {
		t.Fatalf("failed to get workflow instance: %v", err)
	}
	if wi.Status != model.WorkflowInstanceFailed {
		t.Errorf("expected status=failed, got %s", wi.Status)
	}
}

// seedProjectWorkflowWaiting seeds a project-scoped workflow instance in waiting status
// and inserts a _pause finding. Returns the instance ID.
func seedProjectWorkflowWaiting(t *testing.T, dbPath string) string {
	t.Helper()
	seedProjectWithRoot(t, dbPath, "proj")
	seedWorkflowDef(t, dbPath, "proj")

	database, err := db.OpenPathExisting(dbPath)
	if err != nil {
		t.Fatalf("seedProjectWorkflowWaiting: failed to open DB: %v", err)
	}
	_, err = database.Exec(`UPDATE workflows SET scope_type = 'project' WHERE project_id = ? AND id = ?`, "proj", "test")
	if err != nil {
		t.Fatalf("seedProjectWorkflowWaiting: update scope: %v", err)
	}
	pool := db.WrapAsPool(database)
	wi, err := service.NewWorkflowService(pool, clock.Real()).InitProjectWorkflow("proj", &types.ProjectWorkflowRunRequest{Workflow: "test"})
	if err != nil {
		t.Fatalf("seedProjectWorkflowWaiting: init: %v", err)
	}
	repo.NewWorkflowInstanceRepo(pool, clock.Real()).UpdateStatus(wi.ID, model.WorkflowInstanceWaiting)
	database.Close()

	insertPauseFinding(t, dbPath, wi.ID)
	return wi.ID
}

func TestContinueWorkflowProjectHandler_HappyPath(t *testing.T) {
	dbDir := t.TempDir()
	dbPath := filepath.Join(dbDir, "test.db")
	if err := copyTemplateDB(dbPath); err != nil {
		t.Fatalf("failed to copy template DB: %v", err)
	}

	wfiID := seedProjectWorkflowWaiting(t, dbPath)
	baseURL, client := startAPIServer(t, dbPath)

	resp, body := doWorkflowContinueProject(t, client, baseURL, "proj", map[string]string{
		"instance_id": wfiID,
	})

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", resp.StatusCode, string(body))
	}
	var result map[string]string
	json.Unmarshal(body, &result)
	if result["status"] != "continuing" {
		t.Fatalf("expected status=continuing, got %q", result["status"])
	}
}

func TestFailWorkflowProjectHandler_WaitingInstance(t *testing.T) {
	dbDir := t.TempDir()
	dbPath := filepath.Join(dbDir, "test.db")
	if err := copyTemplateDB(dbPath); err != nil {
		t.Fatalf("failed to copy template DB: %v", err)
	}

	wfiID := seedProjectWorkflowWaiting(t, dbPath)
	baseURL, client := startAPIServer(t, dbPath)

	resp, body := doWorkflowFailProject(t, client, baseURL, "proj", map[string]string{
		"instance_id": wfiID,
		"reason":      "test reason",
	})

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", resp.StatusCode, string(body))
	}
	var result map[string]string
	json.Unmarshal(body, &result)
	if result["status"] != "failing" {
		t.Fatalf("expected status=failing, got %q", result["status"])
	}

	// FailWorkflow on a waiting instance is synchronous (markFailed directly)
	database, err := db.OpenPathExisting(dbPath)
	if err != nil {
		t.Fatalf("failed to open DB for verification: %v", err)
	}
	pool := db.WrapAsPool(database)
	wi, err := repo.NewWorkflowInstanceRepo(pool, clock.Real()).Get(wfiID)
	database.Close()
	if err != nil {
		t.Fatalf("failed to get workflow instance: %v", err)
	}
	if wi.Status != model.WorkflowInstanceFailed {
		t.Errorf("expected status=failed, got %s", wi.Status)
	}
}
