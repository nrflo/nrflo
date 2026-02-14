package integration

import (
	"bytes"
	"database/sql"
	"encoding/json"
	"io"
	"net/http"
	"path/filepath"
	"testing"

	"be/internal/db"
	"be/internal/model"
	"be/internal/repo"
	"be/internal/service"
	"be/internal/types"
)

func doRetryFailed(t *testing.T, baseURL, ticketID, project string, body map[string]string) (*http.Response, []byte) {
	t.Helper()
	bodyJSON, _ := json.Marshal(body)
	req, _ := http.NewRequest("POST", baseURL+"/api/v1/tickets/"+ticketID+"/workflow/retry-failed",
		bytes.NewBuffer(bodyJSON))
	req.Header.Set("Content-Type", "application/json")
	if project != "" {
		req.Header.Set("X-Project", project)
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	respBody, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	return resp, respBody
}

func doRetryFailedProject(t *testing.T, baseURL, projectID string, body map[string]string) (*http.Response, []byte) {
	t.Helper()
	bodyJSON, _ := json.Marshal(body)
	url := baseURL + "/api/v1/projects/" + projectID + "/workflow/retry-failed"
	if projectID == "" {
		// Handle empty project ID test case
		url = baseURL + "/api/v1/projects//workflow/retry-failed"
	}
	req, _ := http.NewRequest("POST", url, bytes.NewBuffer(bodyJSON))
	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	respBody, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	return resp, respBody
}

// seedProjectWithRoot creates a project with a root_path set
func seedProjectWithRoot(t *testing.T, dbPath, projectID string) {
	t.Helper()
	database, err := db.Open(dbPath)
	if err != nil {
		t.Fatalf("failed to open DB for seeding: %v", err)
	}
	defer database.Close()

	_, err = database.Exec(`
		INSERT OR REPLACE INTO projects (id, name, root_path, created_at, updated_at)
		VALUES (?, ?, ?, datetime('now'), datetime('now'))`, projectID, projectID, t.TempDir())
	if err != nil {
		t.Fatalf("failed to seed project with root: %v", err)
	}
}

func TestRetryFailedHandler_MissingProject(t *testing.T) {
	dbDir := t.TempDir()
	dbPath := filepath.Join(dbDir, "test.db")
	database, err := db.OpenPath(dbPath)
	if err != nil {
		t.Fatalf("failed to init DB: %v", err)
	}
	database.Close()

	baseURL := startAPIServer(t, dbPath)

	resp, body := doRetryFailed(t, baseURL, "TICK-1", "", map[string]string{
		"workflow":   "test",
		"session_id": "sess-1",
	})

	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d: %s", resp.StatusCode, string(body))
	}

	var errResp map[string]string
	json.Unmarshal(body, &errResp)
	if errResp["error"] != "X-Project header or project query param required" {
		t.Fatalf("unexpected error: %q", errResp["error"])
	}
}

func TestRetryFailedHandler_MissingWorkflow(t *testing.T) {
	dbDir := t.TempDir()
	dbPath := filepath.Join(dbDir, "test.db")
	database, err := db.OpenPath(dbPath)
	if err != nil {
		t.Fatalf("failed to init DB: %v", err)
	}
	database.Close()

	seedProject(t, dbPath, "proj")
	baseURL := startAPIServer(t, dbPath)

	resp, body := doRetryFailed(t, baseURL, "TICK-1", "proj", map[string]string{
		"workflow":   "",
		"session_id": "sess-1",
	})

	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d: %s", resp.StatusCode, string(body))
	}

	var errResp map[string]string
	json.Unmarshal(body, &errResp)
	if errResp["error"] != "workflow name is required" {
		t.Fatalf("unexpected error: %q", errResp["error"])
	}
}

func TestRetryFailedHandler_MissingSessionID(t *testing.T) {
	dbDir := t.TempDir()
	dbPath := filepath.Join(dbDir, "test.db")
	database, err := db.OpenPath(dbPath)
	if err != nil {
		t.Fatalf("failed to init DB: %v", err)
	}
	database.Close()

	seedProject(t, dbPath, "proj")
	baseURL := startAPIServer(t, dbPath)

	resp, body := doRetryFailed(t, baseURL, "TICK-1", "proj", map[string]string{
		"workflow":   "test",
		"session_id": "",
	})

	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d: %s", resp.StatusCode, string(body))
	}

	var errResp map[string]string
	json.Unmarshal(body, &errResp)
	if errResp["error"] != "session_id is required" {
		t.Fatalf("unexpected error: %q", errResp["error"])
	}
}

func TestRetryFailedHandler_WorkflowNotFound(t *testing.T) {
	dbDir := t.TempDir()
	dbPath := filepath.Join(dbDir, "test.db")
	database, err := db.OpenPath(dbPath)
	if err != nil {
		t.Fatalf("failed to init DB: %v", err)
	}
	database.Close()

	seedProject(t, dbPath, "proj")
	baseURL := startAPIServer(t, dbPath)

	resp, body := doRetryFailed(t, baseURL, "TICK-1", "proj", map[string]string{
		"workflow":   "nonexistent",
		"session_id": "sess-1",
	})

	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d: %s", resp.StatusCode, string(body))
	}
}

func TestRetryFailedHandler_WorkflowNotFailed(t *testing.T) {
	dbDir := t.TempDir()
	dbPath := filepath.Join(dbDir, "test.db")
	database, err := db.OpenPath(dbPath)
	if err != nil {
		t.Fatalf("failed to init DB: %v", err)
	}
	database.Close()

	seedProject(t, dbPath, "proj")
	seedWorkflowDef(t, dbPath, "proj")
	seedTicketAndWorkflow(t, dbPath, "proj", "TICK-1")
	baseURL := startAPIServer(t, dbPath)

	// Workflow is active, not failed
	resp, body := doRetryFailed(t, baseURL, "TICK-1", "proj", map[string]string{
		"workflow":   "test",
		"session_id": "sess-1",
	})

	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d: %s", resp.StatusCode, string(body))
	}

	var errResp map[string]string
	json.Unmarshal(body, &errResp)
	if errResp["error"] == "" {
		t.Fatal("expected non-empty error message")
	}
}

func TestRetryFailedHandler_HappyPath(t *testing.T) {
	dbDir := t.TempDir()
	dbPath := filepath.Join(dbDir, "test.db")
	database, err := db.OpenPath(dbPath)
	if err != nil {
		t.Fatalf("failed to init DB: %v", err)
	}
	database.Close()

	seedProjectWithRoot(t, dbPath, "proj")
	seedWorkflowDef(t, dbPath, "proj")
	seedTicketAndWorkflow(t, dbPath, "proj", "TICK-1")

	// Mark workflow as failed and create failed agent session
	database, _ = db.Open(dbPath)
	pool := db.WrapAsPool(database)

	wfiRepo := repo.NewWorkflowInstanceRepo(pool)
	wi, _ := wfiRepo.GetByTicketAndWorkflow("proj", "TICK-1", "test")
	wfiRepo.UpdateStatus(wi.ID, model.WorkflowInstanceFailed)

	asRepo := repo.NewAgentSessionRepo(database)
	session := &model.AgentSession{
		ID:                 "sess-retry-1",
		ProjectID:          "proj",
		TicketID:           "TICK-1",
		WorkflowInstanceID: wi.ID,
		Phase:              "analyzer",
		AgentType:          "analyzer",
		Status:             model.AgentSessionFailed,
		Result:             sql.NullString{String: "fail", Valid: true},
	}
	asRepo.Create(session)
	database.Close()

	baseURL := startAPIServer(t, dbPath)

	resp, body := doRetryFailed(t, baseURL, "TICK-1", "proj", map[string]string{
		"workflow":   "test",
		"session_id": "sess-retry-1",
	})

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", resp.StatusCode, string(body))
	}

	var result map[string]string
	json.Unmarshal(body, &result)
	if result["status"] != "retrying" {
		t.Fatalf("expected status=retrying, got %q", result["status"])
	}

	// Verify workflow status reset to active
	database, _ = db.Open(dbPath)
	pool = db.WrapAsPool(database)
	wfiRepo = repo.NewWorkflowInstanceRepo(pool)
	wi, _ = wfiRepo.Get(wi.ID)
	database.Close()

	if wi.Status != model.WorkflowInstanceActive {
		t.Errorf("expected status=active, got %s", wi.Status)
	}
	if wi.RetryCount != 1 {
		t.Errorf("expected retry_count=1, got %d", wi.RetryCount)
	}
}

func TestRetryFailedHandler_InvalidJSON(t *testing.T) {
	dbDir := t.TempDir()
	dbPath := filepath.Join(dbDir, "test.db")
	database, err := db.OpenPath(dbPath)
	if err != nil {
		t.Fatalf("failed to init DB: %v", err)
	}
	database.Close()

	seedProject(t, dbPath, "proj")
	baseURL := startAPIServer(t, dbPath)

	req, _ := http.NewRequest("POST", baseURL+"/api/v1/tickets/TICK-1/workflow/retry-failed",
		bytes.NewBufferString("{invalid json"))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Project", "proj")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusBadRequest {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("expected 400, got %d: %s", resp.StatusCode, string(body))
	}
}

func TestRetryFailedHandler_EmptyBody(t *testing.T) {
	dbDir := t.TempDir()
	dbPath := filepath.Join(dbDir, "test.db")
	database, err := db.OpenPath(dbPath)
	if err != nil {
		t.Fatalf("failed to init DB: %v", err)
	}
	database.Close()

	seedProject(t, dbPath, "proj")
	baseURL := startAPIServer(t, dbPath)

	req, _ := http.NewRequest("POST", baseURL+"/api/v1/tickets/TICK-1/workflow/retry-failed",
		bytes.NewBufferString(""))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Project", "proj")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusBadRequest {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("expected 400, got %d: %s", resp.StatusCode, string(body))
	}
}

func TestRetryFailedProjectHandler_HappyPath(t *testing.T) {
	dbDir := t.TempDir()
	dbPath := filepath.Join(dbDir, "test.db")
	database, err := db.OpenPath(dbPath)
	if err != nil {
		t.Fatalf("failed to init DB: %v", err)
	}
	database.Close()

	seedProjectWithRoot(t, dbPath, "proj")
	seedWorkflowDef(t, dbPath, "proj")

	// Create project-scoped workflow
	database, _ = db.Open(dbPath)
	pool := db.WrapAsPool(database)

	// Update workflow to project scope
	_, err = database.Exec(`UPDATE workflows SET scope_type = 'project' WHERE project_id = ? AND id = ?`,
		"proj", "test")
	if err != nil {
		t.Fatalf("failed to update workflow scope: %v", err)
	}

	wfSvc := service.NewWorkflowService(pool)
	wi, err := wfSvc.InitProjectWorkflow("proj", &types.ProjectWorkflowRunRequest{
		Workflow: "test",
	})
	if err != nil {
		t.Fatalf("failed to init project workflow: %v", err)
	}

	wfiRepo := repo.NewWorkflowInstanceRepo(pool)
	wfiRepo.UpdateStatus(wi.ID, model.WorkflowInstanceFailed)

	asRepo := repo.NewAgentSessionRepo(database)
	session := &model.AgentSession{
		ID:                 "sess-proj-retry-1",
		ProjectID:          "proj",
		TicketID:           "",
		WorkflowInstanceID: wi.ID,
		Phase:              "analyzer",
		AgentType:          "analyzer",
		Status:             model.AgentSessionFailed,
		Result:             sql.NullString{String: "fail", Valid: true},
	}
	asRepo.Create(session)
	database.Close()

	baseURL := startAPIServer(t, dbPath)

	resp, body := doRetryFailedProject(t, baseURL, "proj", map[string]string{
		"workflow":    "test",
		"session_id":  "sess-proj-retry-1",
		"instance_id": wi.ID,
	})

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", resp.StatusCode, string(body))
	}

	var result map[string]string
	json.Unmarshal(body, &result)
	if result["status"] != "retrying" {
		t.Fatalf("expected status=retrying, got %q", result["status"])
	}

	// Verify workflow status reset to active
	database, _ = db.Open(dbPath)
	pool = db.WrapAsPool(database)
	wfiRepo = repo.NewWorkflowInstanceRepo(pool)
	wi, _ = wfiRepo.Get(wi.ID)
	database.Close()

	if wi.Status != model.WorkflowInstanceActive {
		t.Errorf("expected status=active, got %s", wi.Status)
	}
	if wi.RetryCount != 1 {
		t.Errorf("expected retry_count=1, got %d", wi.RetryCount)
	}
}

func TestRetryFailedProjectHandler_MissingProject(t *testing.T) {
	dbDir := t.TempDir()
	dbPath := filepath.Join(dbDir, "test.db")
	database, err := db.OpenPath(dbPath)
	if err != nil {
		t.Fatalf("failed to init DB: %v", err)
	}
	database.Close()

	baseURL := startAPIServer(t, dbPath)

	resp, body := doRetryFailedProject(t, baseURL, "", map[string]string{
		"workflow":   "test",
		"session_id": "sess-1",
	})

	// Empty project ID results in malformed URL path, router returns 404
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("expected 404, got %d: %s", resp.StatusCode, string(body))
	}
}

func TestRetryFailedProjectHandler_MissingWorkflow(t *testing.T) {
	dbDir := t.TempDir()
	dbPath := filepath.Join(dbDir, "test.db")
	database, err := db.OpenPath(dbPath)
	if err != nil {
		t.Fatalf("failed to init DB: %v", err)
	}
	database.Close()

	seedProject(t, dbPath, "proj")
	baseURL := startAPIServer(t, dbPath)

	resp, body := doRetryFailedProject(t, baseURL, "proj", map[string]string{
		"workflow":   "",
		"session_id": "sess-1",
	})

	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d: %s", resp.StatusCode, string(body))
	}
}

func TestRetryFailedProjectHandler_MissingSessionID(t *testing.T) {
	dbDir := t.TempDir()
	dbPath := filepath.Join(dbDir, "test.db")
	database, err := db.OpenPath(dbPath)
	if err != nil {
		t.Fatalf("failed to init DB: %v", err)
	}
	database.Close()

	seedProject(t, dbPath, "proj")
	baseURL := startAPIServer(t, dbPath)

	resp, body := doRetryFailedProject(t, baseURL, "proj", map[string]string{
		"workflow":   "test",
		"session_id": "",
	})

	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d: %s", resp.StatusCode, string(body))
	}
}
