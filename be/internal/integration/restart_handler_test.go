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
	"be/internal/service"
	"be/internal/types"
)

func doRestart(t *testing.T, baseURL, ticketID, project string, body map[string]string) (*http.Response, []byte) {
	t.Helper()
	bodyJSON, _ := json.Marshal(body)
	req, _ := http.NewRequest("POST", baseURL+"/api/v1/tickets/"+ticketID+"/workflow/restart",
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

func TestRestartHandler_MissingProject(t *testing.T) {
	dbDir := t.TempDir()
	dbPath := filepath.Join(dbDir, "test.db")
	if err := copyTemplateDB(dbPath); err != nil {
		t.Fatalf("failed to copy template DB: %v", err)
	}

	baseURL := startAPIServer(t, dbPath)

	resp, body := doRestart(t, baseURL, "TICK-1", "", map[string]string{
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

func TestRestartHandler_MissingWorkflow(t *testing.T) {
	dbDir := t.TempDir()
	dbPath := filepath.Join(dbDir, "test.db")
	if err := copyTemplateDB(dbPath); err != nil {
		t.Fatalf("failed to copy template DB: %v", err)
	}

	seedProject(t, dbPath, "proj")
	baseURL := startAPIServer(t, dbPath)

	resp, body := doRestart(t, baseURL, "TICK-1", "proj", map[string]string{
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

func TestRestartHandler_MissingSessionID(t *testing.T) {
	dbDir := t.TempDir()
	dbPath := filepath.Join(dbDir, "test.db")
	if err := copyTemplateDB(dbPath); err != nil {
		t.Fatalf("failed to copy template DB: %v", err)
	}

	seedProject(t, dbPath, "proj")
	baseURL := startAPIServer(t, dbPath)

	resp, body := doRestart(t, baseURL, "TICK-1", "proj", map[string]string{
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

func TestRestartHandler_WorkflowNotFound(t *testing.T) {
	dbDir := t.TempDir()
	dbPath := filepath.Join(dbDir, "test.db")
	if err := copyTemplateDB(dbPath); err != nil {
		t.Fatalf("failed to copy template DB: %v", err)
	}

	seedProject(t, dbPath, "proj")
	baseURL := startAPIServer(t, dbPath)

	// No workflow initialized — should get 404
	resp, body := doRestart(t, baseURL, "TICK-1", "proj", map[string]string{
		"workflow":   "nonexistent",
		"session_id": "sess-1",
	})

	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("expected 404, got %d: %s", resp.StatusCode, string(body))
	}
}

func TestRestartHandler_NoRunningOrchestration(t *testing.T) {
	dbDir := t.TempDir()
	dbPath := filepath.Join(dbDir, "test.db")
	if err := copyTemplateDB(dbPath); err != nil {
		t.Fatalf("failed to copy template DB: %v", err)
	}

	seedProject(t, dbPath, "proj")
	seedWorkflowDef(t, dbPath, "proj")
	seedTicketAndWorkflow(t, dbPath, "proj", "TICK-1")
	baseURL := startAPIServer(t, dbPath)

	// Workflow exists but no orchestration running
	resp, body := doRestart(t, baseURL, "TICK-1", "proj", map[string]string{
		"workflow":   "test",
		"session_id": "sess-1",
	})

	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("expected 404, got %d: %s", resp.StatusCode, string(body))
	}

	var errResp map[string]string
	json.Unmarshal(body, &errResp)
	if errResp["error"] == "" {
		t.Fatal("expected non-empty error message")
	}
}

func TestRestartHandler_InvalidJSON(t *testing.T) {
	dbDir := t.TempDir()
	dbPath := filepath.Join(dbDir, "test.db")
	if err := copyTemplateDB(dbPath); err != nil {
		t.Fatalf("failed to copy template DB: %v", err)
	}

	seedProject(t, dbPath, "proj")
	baseURL := startAPIServer(t, dbPath)

	req, _ := http.NewRequest("POST", baseURL+"/api/v1/tickets/TICK-1/workflow/restart",
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

func TestRestartHandler_EmptyBody(t *testing.T) {
	dbDir := t.TempDir()
	dbPath := filepath.Join(dbDir, "test.db")
	if err := copyTemplateDB(dbPath); err != nil {
		t.Fatalf("failed to copy template DB: %v", err)
	}

	seedProject(t, dbPath, "proj")
	baseURL := startAPIServer(t, dbPath)

	req, _ := http.NewRequest("POST", baseURL+"/api/v1/tickets/TICK-1/workflow/restart",
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

// seedWorkflowDef creates a "test" workflow definition with agent definitions in the DB.
func seedWorkflowDef(t *testing.T, dbPath, projectID string) {
	t.Helper()
	database, err := db.Open(dbPath)
	if err != nil {
		t.Fatalf("failed to open DB: %v", err)
	}
	defer database.Close()

	pool := db.WrapAsPool(database)
	wfSvc := service.NewWorkflowService(pool, clock.Real())
	_, err = wfSvc.CreateWorkflowDef(projectID, &types.WorkflowDefCreateRequest{
		ID:          "test",
		Description: "Test workflow",
	})
	if err != nil {
		t.Fatalf("failed to seed workflow def: %v", err)
	}

	adSvc := service.NewAgentDefinitionService(pool, clock.Real(), service.NewCLIModelService(pool, clock.Real()))
	for _, ad := range []types.AgentDefCreateRequest{
		{ID: "analyzer", Prompt: "analyze", Layer: 0},
		{ID: "builder", Prompt: "build", Layer: 1},
	} {
		if _, err := adSvc.CreateAgentDef(projectID, "test", &ad); err != nil {
			t.Fatalf("failed to seed agent def %s: %v", ad.ID, err)
		}
	}
}

// seedTicketAndWorkflow creates a ticket and initializes the "test" workflow.
func seedTicketAndWorkflow(t *testing.T, dbPath, projectID, ticketID string) {
	t.Helper()
	database, err := db.Open(dbPath)
	if err != nil {
		t.Fatalf("failed to open DB: %v", err)
	}
	defer database.Close()

	pool := db.WrapAsPool(database)
	ticketSvc := service.NewTicketService(pool, clock.Real())
	_, err = ticketSvc.Create(projectID, &types.TicketCreateRequest{
		ID:    ticketID,
		Title: "Test ticket",
	})
	if err != nil {
		t.Fatalf("failed to seed ticket: %v", err)
	}

	wfSvc := service.NewWorkflowService(pool, clock.Real())
	_, err = wfSvc.Init(projectID, ticketID, &types.WorkflowInitRequest{
		Workflow: "test",
	})
	if err != nil {
		t.Fatalf("failed to init workflow: %v", err)
	}
}
