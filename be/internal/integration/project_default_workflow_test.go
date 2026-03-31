package integration

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"path/filepath"
	"testing"
)

// TestCreateProjectWithDefaultWorkflowFieldIgnored verifies that sending
// default_workflow in the create body is silently ignored (field removed from API).
func TestCreateProjectWithDefaultWorkflowFieldIgnored(t *testing.T) {
	dbDir := t.TempDir()
	dbPath := filepath.Join(dbDir, "test.db")

	if err := copyTemplateDB(dbPath); err != nil {
		t.Fatalf("failed to copy template DB: %v", err)
	}

	baseURL := startAPIServer(t, dbPath)

	// Include default_workflow — should be ignored, not rejected
	body := `{"id":"proj-dw-ignored","name":"DW Ignored","default_workflow":"feature"}`
	req, _ := http.NewRequest("POST", baseURL+"/api/v1/projects", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusCreated {
		respBody, _ := io.ReadAll(resp.Body)
		t.Fatalf("expected 201, got %d: %s", resp.StatusCode, string(respBody))
	}

	var result map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	if result["id"] != "proj-dw-ignored" {
		t.Fatalf("expected id 'proj-dw-ignored', got %v", result["id"])
	}
}

// TestGetProjectResponseOmitsDefaultWorkflow verifies that the GET /projects/:id
// response does not include a default_workflow key.
func TestGetProjectResponseOmitsDefaultWorkflow(t *testing.T) {
	dbDir := t.TempDir()
	dbPath := filepath.Join(dbDir, "test.db")

	if err := copyTemplateDB(dbPath); err != nil {
		t.Fatalf("failed to copy template DB: %v", err)
	}

	baseURL := startAPIServer(t, dbPath)

	// Create a project
	createBody := `{"id":"proj-get-no-dw","name":"Get No DW"}`
	req, _ := http.NewRequest("POST", baseURL+"/api/v1/projects", bytes.NewBufferString(createBody))
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("create failed: %v", err)
	}
	resp.Body.Close()

	// GET the project
	req, _ = http.NewRequest("GET", baseURL+"/api/v1/projects/proj-get-no-dw", nil)
	resp, err = http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("get request failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(resp.Body)
		t.Fatalf("expected 200, got %d: %s", resp.StatusCode, string(respBody))
	}

	var result map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	if _, ok := result["default_workflow"]; ok {
		t.Fatalf("response must not contain 'default_workflow' key, got value: %v", result["default_workflow"])
	}
}

// TestListProjectsResponseOmitsDefaultWorkflow verifies that GET /projects list
// entries do not include a default_workflow key.
func TestListProjectsResponseOmitsDefaultWorkflow(t *testing.T) {
	dbDir := t.TempDir()
	dbPath := filepath.Join(dbDir, "test.db")

	if err := copyTemplateDB(dbPath); err != nil {
		t.Fatalf("failed to copy template DB: %v", err)
	}

	baseURL := startAPIServer(t, dbPath)

	// Create a project
	createBody := `{"id":"proj-list-no-dw","name":"List No DW"}`
	req, _ := http.NewRequest("POST", baseURL+"/api/v1/projects", bytes.NewBufferString(createBody))
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("create failed: %v", err)
	}
	resp.Body.Close()

	// List projects
	req, _ = http.NewRequest("GET", baseURL+"/api/v1/projects", nil)
	resp, err = http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("list request failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(resp.Body)
		t.Fatalf("expected 200, got %d: %s", resp.StatusCode, string(respBody))
	}

	var listResult struct {
		Projects []map[string]interface{} `json:"projects"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&listResult); err != nil {
		t.Fatalf("failed to decode list response: %v", err)
	}

	for _, proj := range listResult.Projects {
		if _, ok := proj["default_workflow"]; ok {
			t.Fatalf("project %v must not contain 'default_workflow' key", proj["id"])
		}
	}
}

// TestUpdateProjectWithDefaultWorkflowFieldIgnored verifies that PATCH with
// default_workflow in the body is silently ignored and returns 200.
func TestUpdateProjectWithDefaultWorkflowFieldIgnored(t *testing.T) {
	dbDir := t.TempDir()
	dbPath := filepath.Join(dbDir, "test.db")

	if err := copyTemplateDB(dbPath); err != nil {
		t.Fatalf("failed to copy template DB: %v", err)
	}

	baseURL := startAPIServer(t, dbPath)

	// Create a project
	createBody := `{"id":"proj-patch-dw","name":"Patch DW"}`
	req, _ := http.NewRequest("POST", baseURL+"/api/v1/projects", bytes.NewBufferString(createBody))
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("create request failed: %v", err)
	}
	resp.Body.Close()

	// PATCH with default_workflow — should be silently ignored, not rejected
	updateBody := `{"name":"Patch DW Updated","default_workflow":"bugfix"}`
	req, _ = http.NewRequest("PATCH", baseURL+"/api/v1/projects/proj-patch-dw", bytes.NewBufferString(updateBody))
	req.Header.Set("Content-Type", "application/json")
	resp, err = http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("update request failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(resp.Body)
		t.Fatalf("expected 200, got %d: %s", resp.StatusCode, string(respBody))
	}

	var result map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	// name should have been updated
	if result["name"] != "Patch DW Updated" {
		t.Fatalf("expected name 'Patch DW Updated', got %v", result["name"])
	}

	// default_workflow must not appear in the response
	if _, ok := result["default_workflow"]; ok {
		t.Fatalf("response must not contain 'default_workflow' key, got value: %v", result["default_workflow"])
	}
}
