package integration

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"path/filepath"
	"testing"

)

func doTakeControl(t *testing.T, baseURL, ticketID, project string, body map[string]string) (*http.Response, []byte) {
	t.Helper()
	bodyJSON, _ := json.Marshal(body)
	req, _ := http.NewRequest("POST", baseURL+"/api/v1/tickets/"+ticketID+"/workflow/take-control",
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

func doExitInteractive(t *testing.T, baseURL, ticketID, project string, body map[string]string) (*http.Response, []byte) {
	t.Helper()
	bodyJSON, _ := json.Marshal(body)
	req, _ := http.NewRequest("POST", baseURL+"/api/v1/tickets/"+ticketID+"/workflow/exit-interactive",
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

// TestTakeControlHandler_MissingProject verifies 400 when X-Project header is missing.
func TestTakeControlHandler_MissingProject(t *testing.T) {
	dbDir := t.TempDir()
	dbPath := filepath.Join(dbDir, "test.db")
	if err := copyTemplateDB(dbPath); err != nil {
		t.Fatalf("failed to copy template DB: %v", err)
	}

	baseURL := startAPIServer(t, dbPath)

	resp, body := doTakeControl(t, baseURL, "TICK-1", "", map[string]string{
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

// TestTakeControlHandler_MissingWorkflow verifies 400 when workflow is not in body.
func TestTakeControlHandler_MissingWorkflow(t *testing.T) {
	dbDir := t.TempDir()
	dbPath := filepath.Join(dbDir, "test.db")
	if err := copyTemplateDB(dbPath); err != nil {
		t.Fatalf("failed to copy template DB: %v", err)
	}

	seedProject(t, dbPath, "proj")
	baseURL := startAPIServer(t, dbPath)

	resp, body := doTakeControl(t, baseURL, "TICK-1", "proj", map[string]string{
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

// TestTakeControlHandler_MissingSessionID verifies 400 when session_id is missing.
func TestTakeControlHandler_MissingSessionID(t *testing.T) {
	dbDir := t.TempDir()
	dbPath := filepath.Join(dbDir, "test.db")
	if err := copyTemplateDB(dbPath); err != nil {
		t.Fatalf("failed to copy template DB: %v", err)
	}

	seedProject(t, dbPath, "proj")
	baseURL := startAPIServer(t, dbPath)

	resp, body := doTakeControl(t, baseURL, "TICK-1", "proj", map[string]string{
		"workflow": "test",
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

// TestTakeControlHandler_WorkflowNotFound verifies 404 when no workflow instance exists.
func TestTakeControlHandler_WorkflowNotFound(t *testing.T) {
	dbDir := t.TempDir()
	dbPath := filepath.Join(dbDir, "test.db")
	if err := copyTemplateDB(dbPath); err != nil {
		t.Fatalf("failed to copy template DB: %v", err)
	}

	seedProject(t, dbPath, "proj")
	baseURL := startAPIServer(t, dbPath)

	resp, body := doTakeControl(t, baseURL, "TICK-NONE", "proj", map[string]string{
		"workflow":   "nonexistent",
		"session_id": "sess-1",
	})

	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("expected 404, got %d: %s", resp.StatusCode, string(body))
	}
}

// TestTakeControlHandler_NoRunningOrchestration verifies 404 when workflow exists
// but no orchestration is running.
func TestTakeControlHandler_NoRunningOrchestration(t *testing.T) {
	dbDir := t.TempDir()
	dbPath := filepath.Join(dbDir, "test.db")
	if err := copyTemplateDB(dbPath); err != nil {
		t.Fatalf("failed to copy template DB: %v", err)
	}

	seedProject(t, dbPath, "proj")
	seedWorkflowDef(t, dbPath, "proj")
	seedTicketAndWorkflow(t, dbPath, "proj", "TICK-1")
	baseURL := startAPIServer(t, dbPath)

	resp, body := doTakeControl(t, baseURL, "TICK-1", "proj", map[string]string{
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

// TestTakeControlHandler_InvalidBody verifies 400 for malformed JSON.
func TestTakeControlHandler_InvalidBody(t *testing.T) {
	dbDir := t.TempDir()
	dbPath := filepath.Join(dbDir, "test.db")
	if err := copyTemplateDB(dbPath); err != nil {
		t.Fatalf("failed to copy template DB: %v", err)
	}

	seedProject(t, dbPath, "proj")
	baseURL := startAPIServer(t, dbPath)

	req, _ := http.NewRequest("POST", baseURL+"/api/v1/tickets/TICK-1/workflow/take-control",
		bytes.NewBufferString("{not valid json"))
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

// TestExitInteractiveHandler_MissingProject verifies 400 when X-Project is missing.
func TestExitInteractiveHandler_MissingProject(t *testing.T) {
	dbDir := t.TempDir()
	dbPath := filepath.Join(dbDir, "test.db")
	if err := copyTemplateDB(dbPath); err != nil {
		t.Fatalf("failed to copy template DB: %v", err)
	}

	baseURL := startAPIServer(t, dbPath)

	resp, body := doExitInteractive(t, baseURL, "TICK-1", "", map[string]string{
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

// TestExitInteractiveHandler_MissingWorkflow verifies 400 when workflow is missing.
func TestExitInteractiveHandler_MissingWorkflow(t *testing.T) {
	dbDir := t.TempDir()
	dbPath := filepath.Join(dbDir, "test.db")
	if err := copyTemplateDB(dbPath); err != nil {
		t.Fatalf("failed to copy template DB: %v", err)
	}

	seedProject(t, dbPath, "proj")
	baseURL := startAPIServer(t, dbPath)

	resp, body := doExitInteractive(t, baseURL, "TICK-1", "proj", map[string]string{
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

// TestExitInteractiveHandler_MissingSessionID verifies 400 when session_id is missing.
func TestExitInteractiveHandler_MissingSessionID(t *testing.T) {
	dbDir := t.TempDir()
	dbPath := filepath.Join(dbDir, "test.db")
	if err := copyTemplateDB(dbPath); err != nil {
		t.Fatalf("failed to copy template DB: %v", err)
	}

	seedProject(t, dbPath, "proj")
	baseURL := startAPIServer(t, dbPath)

	resp, body := doExitInteractive(t, baseURL, "TICK-1", "proj", map[string]string{
		"workflow": "test",
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

// TestExitInteractiveHandler_SessionNotFound verifies 400 when the session does not exist
// in the DB (UpdateStatusToInteractiveCompleted returns not-found error which propagates
// as a constraint error due to the missing migration).
func TestExitInteractiveHandler_SessionNotFound(t *testing.T) {
	dbDir := t.TempDir()
	dbPath := filepath.Join(dbDir, "test.db")
	if err := copyTemplateDB(dbPath); err != nil {
		t.Fatalf("failed to copy template DB: %v", err)
	}

	seedProject(t, dbPath, "proj")
	baseURL := startAPIServer(t, dbPath)

	resp, body := doExitInteractive(t, baseURL, "TICK-1", "proj", map[string]string{
		"workflow":   "test",
		"session_id": "does-not-exist",
	})

	// When the session doesn't exist UpdateStatusToInteractiveCompleted returns
	// an error (either "not found" or a DB constraint error), so expect 400.
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d: %s", resp.StatusCode, string(body))
	}
}

// TestTakeControlProjectHandler_MissingWorkflow verifies 400 for project-scoped route.
func TestTakeControlProjectHandler_MissingWorkflow(t *testing.T) {
	dbDir := t.TempDir()
	dbPath := filepath.Join(dbDir, "test.db")
	if err := copyTemplateDB(dbPath); err != nil {
		t.Fatalf("failed to copy template DB: %v", err)
	}

	baseURL := startAPIServer(t, dbPath)

	bodyJSON, _ := json.Marshal(map[string]string{"session_id": "s1", "instance_id": "i1"})
	req, _ := http.NewRequest("POST", baseURL+"/api/v1/projects/proj-1/workflow/take-control",
		bytes.NewBuffer(bodyJSON))
	req.Header.Set("Content-Type", "application/json")

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

// TestTakeControlProjectHandler_NoRunningOrchestration verifies 404 when instance
// not found for project-scoped take-control.
func TestTakeControlProjectHandler_NoRunningOrchestration(t *testing.T) {
	dbDir := t.TempDir()
	dbPath := filepath.Join(dbDir, "test.db")
	if err := copyTemplateDB(dbPath); err != nil {
		t.Fatalf("failed to copy template DB: %v", err)
	}

	baseURL := startAPIServer(t, dbPath)

	bodyJSON, _ := json.Marshal(map[string]string{
		"workflow":    "test",
		"session_id":  "sess-1",
		"instance_id": "nonexistent-instance",
	})
	req, _ := http.NewRequest("POST", baseURL+"/api/v1/projects/proj-1/workflow/take-control",
		bytes.NewBuffer(bodyJSON))
	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusNotFound {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("expected 404, got %d: %s", resp.StatusCode, string(body))
	}
}

// TestExitInteractiveProjectHandler_SessionNotFound verifies 400 for project-scoped
// exit-interactive when session does not exist.
func TestExitInteractiveProjectHandler_SessionNotFound(t *testing.T) {
	dbDir := t.TempDir()
	dbPath := filepath.Join(dbDir, "test.db")
	if err := copyTemplateDB(dbPath); err != nil {
		t.Fatalf("failed to copy template DB: %v", err)
	}

	baseURL := startAPIServer(t, dbPath)

	bodyJSON, _ := json.Marshal(map[string]string{
		"workflow":   "test",
		"session_id": "no-such-session",
	})
	req, _ := http.NewRequest("POST", baseURL+"/api/v1/projects/proj-1/workflow/exit-interactive",
		bytes.NewBuffer(bodyJSON))
	req.Header.Set("Content-Type", "application/json")

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
