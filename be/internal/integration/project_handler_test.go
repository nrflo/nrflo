package integration

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"path/filepath"
	"testing"

	"be/internal/db"
	"be/internal/model"
)

func TestCreateProjectWithDefaultBranch(t *testing.T) {
	dbDir := t.TempDir()
	dbPath := filepath.Join(dbDir, "test.db")

	database, err := db.OpenPath(dbPath)
	if err != nil {
		t.Fatalf("failed to init DB: %v", err)
	}
	database.Close()

	baseURL := startAPIServer(t, dbPath)

	body := `{"id":"proj-with-branch","name":"Project with Branch","default_branch":"develop"}`
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

	var project model.Project
	if err := json.NewDecoder(resp.Body).Decode(&project); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	if project.ID != "proj-with-branch" {
		t.Fatalf("expected ID 'proj-with-branch', got %q", project.ID)
	}

	// Verify default_branch is set by checking the JSON directly
	database, err = db.Open(dbPath)
	if err != nil {
		t.Fatalf("failed to reopen DB: %v", err)
	}
	defer database.Close()

	var defaultBranch *string
	var rootPath *string
	var defaultWorkflow *string

	err = database.QueryRow("SELECT default_branch, root_path, default_workflow FROM projects WHERE id = ?", "proj-with-branch").
		Scan(&defaultBranch, &rootPath, &defaultWorkflow)
	if err != nil {
		t.Fatalf("failed to query project: %v", err)
	}

	if defaultBranch == nil {
		t.Fatalf("expected default_branch to be set, got nil")
	}
	if *defaultBranch != "develop" {
		t.Fatalf("expected default_branch 'develop', got %q", *defaultBranch)
	}
}

func TestCreateProjectWithoutDefaultBranch(t *testing.T) {
	dbDir := t.TempDir()
	dbPath := filepath.Join(dbDir, "test.db")

	database, err := db.OpenPath(dbPath)
	if err != nil {
		t.Fatalf("failed to init DB: %v", err)
	}
	database.Close()

	baseURL := startAPIServer(t, dbPath)

	body := `{"id":"proj-no-branch","name":"Project without Branch"}`
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

	var project model.Project
	if err := json.NewDecoder(resp.Body).Decode(&project); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	// Verify default_branch is NULL in DB
	database, err = db.Open(dbPath)
	if err != nil {
		t.Fatalf("failed to reopen DB: %v", err)
	}
	defer database.Close()

	var defaultBranch *string
	err = database.QueryRow("SELECT default_branch FROM projects WHERE id = ?", "proj-no-branch").
		Scan(&defaultBranch)
	if err != nil {
		t.Fatalf("failed to query project: %v", err)
	}

	if defaultBranch != nil {
		t.Fatalf("expected default_branch to be nil, got %q", *defaultBranch)
	}
}

func TestGetProjectReturnsDefaultBranch(t *testing.T) {
	dbDir := t.TempDir()
	dbPath := filepath.Join(dbDir, "test.db")

	database, err := db.OpenPath(dbPath)
	if err != nil {
		t.Fatalf("failed to init DB: %v", err)
	}
	database.Close()

	baseURL := startAPIServer(t, dbPath)

	// Create project with default_branch
	createBody := `{"id":"get-proj","name":"Get Project","default_branch":"main"}`
	req, _ := http.NewRequest("POST", baseURL+"/api/v1/projects", bytes.NewBufferString(createBody))
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("create request failed: %v", err)
	}
	resp.Body.Close()

	// GET the project
	req, _ = http.NewRequest("GET", baseURL+"/api/v1/projects/get-proj", nil)
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

	defaultBranch, ok := result["default_branch"]
	if !ok {
		t.Fatalf("expected default_branch field in response")
	}

	if defaultBranch != "main" {
		t.Fatalf("expected default_branch 'main', got %v", defaultBranch)
	}
}

func TestGetProjectReturnsNullDefaultBranch(t *testing.T) {
	dbDir := t.TempDir()
	dbPath := filepath.Join(dbDir, "test.db")

	database, err := db.OpenPath(dbPath)
	if err != nil {
		t.Fatalf("failed to init DB: %v", err)
	}
	database.Close()

	baseURL := startAPIServer(t, dbPath)

	// Create project without default_branch
	createBody := `{"id":"get-proj-null","name":"Get Project Null"}`
	req, _ := http.NewRequest("POST", baseURL+"/api/v1/projects", bytes.NewBufferString(createBody))
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("create request failed: %v", err)
	}
	resp.Body.Close()

	// GET the project
	req, _ = http.NewRequest("GET", baseURL+"/api/v1/projects/get-proj-null", nil)
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

	defaultBranch, ok := result["default_branch"]
	if !ok {
		t.Fatalf("expected default_branch field in response")
	}

	if defaultBranch != nil {
		t.Fatalf("expected default_branch null, got %v", defaultBranch)
	}
}

func TestListProjectsReturnsDefaultBranch(t *testing.T) {
	dbDir := t.TempDir()
	dbPath := filepath.Join(dbDir, "test.db")

	database, err := db.OpenPath(dbPath)
	if err != nil {
		t.Fatalf("failed to init DB: %v", err)
	}
	database.Close()

	baseURL := startAPIServer(t, dbPath)

	// Create project with default_branch
	createBody1 := `{"id":"list-proj-1","name":"List Project 1","default_branch":"main"}`
	req, _ := http.NewRequest("POST", baseURL+"/api/v1/projects", bytes.NewBufferString(createBody1))
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("create request 1 failed: %v", err)
	}
	resp.Body.Close()

	// Create project without default_branch
	createBody2 := `{"id":"list-proj-2","name":"List Project 2"}`
	req, _ = http.NewRequest("POST", baseURL+"/api/v1/projects", bytes.NewBufferString(createBody2))
	req.Header.Set("Content-Type", "application/json")
	resp, err = http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("create request 2 failed: %v", err)
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

	var result struct {
		Projects []map[string]interface{} `json:"projects"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	if len(result.Projects) < 2 {
		t.Fatalf("expected at least 2 projects, got %d", len(result.Projects))
	}

	// Find our projects and verify default_branch
	var proj1Found, proj2Found bool
	for _, proj := range result.Projects {
		id, ok := proj["id"].(string)
		if !ok {
			continue
		}

		if id == "list-proj-1" {
			proj1Found = true
			defaultBranch, ok := proj["default_branch"]
			if !ok {
				t.Fatalf("expected default_branch field for list-proj-1")
			}
			if defaultBranch != "main" {
				t.Fatalf("expected default_branch 'main' for list-proj-1, got %v", defaultBranch)
			}
		} else if id == "list-proj-2" {
			proj2Found = true
			defaultBranch, ok := proj["default_branch"]
			if !ok {
				t.Fatalf("expected default_branch field for list-proj-2")
			}
			if defaultBranch != nil {
				t.Fatalf("expected default_branch null for list-proj-2, got %v", defaultBranch)
			}
		}
	}

	if !proj1Found {
		t.Fatalf("list-proj-1 not found in list response")
	}
	if !proj2Found {
		t.Fatalf("list-proj-2 not found in list response")
	}
}

func TestUpdateProjectSetDefaultBranch(t *testing.T) {
	dbDir := t.TempDir()
	dbPath := filepath.Join(dbDir, "test.db")

	database, err := db.OpenPath(dbPath)
	if err != nil {
		t.Fatalf("failed to init DB: %v", err)
	}
	database.Close()

	baseURL := startAPIServer(t, dbPath)

	// Create project without default_branch
	createBody := `{"id":"update-proj","name":"Update Project"}`
	req, _ := http.NewRequest("POST", baseURL+"/api/v1/projects", bytes.NewBufferString(createBody))
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("create request failed: %v", err)
	}
	resp.Body.Close()

	// Update to set default_branch
	updateBody := `{"default_branch":"feature-branch"}`
	req, _ = http.NewRequest("PATCH", baseURL+"/api/v1/projects/update-proj", bytes.NewBufferString(updateBody))
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

	defaultBranch, ok := result["default_branch"]
	if !ok {
		t.Fatalf("expected default_branch field in response")
	}

	if defaultBranch != "feature-branch" {
		t.Fatalf("expected default_branch 'feature-branch', got %v", defaultBranch)
	}
}

func TestUpdateProjectClearDefaultBranch(t *testing.T) {
	dbDir := t.TempDir()
	dbPath := filepath.Join(dbDir, "test.db")

	database, err := db.OpenPath(dbPath)
	if err != nil {
		t.Fatalf("failed to init DB: %v", err)
	}
	database.Close()

	baseURL := startAPIServer(t, dbPath)

	// Create project with default_branch
	createBody := `{"id":"clear-proj","name":"Clear Project","default_branch":"main"}`
	req, _ := http.NewRequest("POST", baseURL+"/api/v1/projects", bytes.NewBufferString(createBody))
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("create request failed: %v", err)
	}
	resp.Body.Close()

	// Update to clear default_branch (empty string)
	updateBody := `{"default_branch":""}`
	req, _ = http.NewRequest("PATCH", baseURL+"/api/v1/projects/clear-proj", bytes.NewBufferString(updateBody))
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

	// Verify default_branch is empty string in DB (stored as empty, not NULL)
	database, err = db.Open(dbPath)
	if err != nil {
		t.Fatalf("failed to reopen DB: %v", err)
	}
	defer database.Close()

	var defaultBranch *string
	err = database.QueryRow("SELECT default_branch FROM projects WHERE id = ?", "clear-proj").
		Scan(&defaultBranch)
	if err != nil {
		t.Fatalf("failed to query project: %v", err)
	}

	// Empty string in update should result in empty string stored
	if defaultBranch == nil {
		t.Fatalf("expected default_branch to be empty string, got nil")
	}
	if *defaultBranch != "" {
		t.Fatalf("expected default_branch to be empty string, got %q", *defaultBranch)
	}
}

func TestUpdateProjectMultipleFields(t *testing.T) {
	dbDir := t.TempDir()
	dbPath := filepath.Join(dbDir, "test.db")

	database, err := db.OpenPath(dbPath)
	if err != nil {
		t.Fatalf("failed to init DB: %v", err)
	}
	database.Close()

	baseURL := startAPIServer(t, dbPath)

	// Create project with some fields
	createBody := `{"id":"multi-proj","name":"Multi Project","default_branch":"main"}`
	req, _ := http.NewRequest("POST", baseURL+"/api/v1/projects", bytes.NewBufferString(createBody))
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("create request failed: %v", err)
	}
	resp.Body.Close()

	// Update multiple fields including default_branch
	updateBody := `{"name":"Updated Multi","default_branch":"develop","default_workflow":"feature"}`
	req, _ = http.NewRequest("PATCH", baseURL+"/api/v1/projects/multi-proj", bytes.NewBufferString(updateBody))
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

	if result["name"] != "Updated Multi" {
		t.Fatalf("expected name 'Updated Multi', got %v", result["name"])
	}
	if result["default_branch"] != "develop" {
		t.Fatalf("expected default_branch 'develop', got %v", result["default_branch"])
	}
	if result["default_workflow"] != "feature" {
		t.Fatalf("expected default_workflow 'feature', got %v", result["default_workflow"])
	}
}

func TestProjectDefaultBranchEndToEnd(t *testing.T) {
	dbDir := t.TempDir()
	dbPath := filepath.Join(dbDir, "test.db")

	database, err := db.OpenPath(dbPath)
	if err != nil {
		t.Fatalf("failed to init DB: %v", err)
	}
	database.Close()

	baseURL := startAPIServer(t, dbPath)

	// Step 1: Create project with default_branch
	createBody := `{"id":"e2e-proj","name":"E2E Project","default_branch":"main","root_path":"/tmp/test"}`
	req, _ := http.NewRequest("POST", baseURL+"/api/v1/projects", bytes.NewBufferString(createBody))
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("create request failed: %v", err)
	}
	if resp.StatusCode != http.StatusCreated {
		respBody, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		t.Fatalf("create: expected 201, got %d: %s", resp.StatusCode, string(respBody))
	}
	resp.Body.Close()

	// Step 2: GET project and verify all fields
	req, _ = http.NewRequest("GET", baseURL+"/api/v1/projects/e2e-proj", nil)
	resp, err = http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("get request failed: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		t.Fatalf("get: expected 200, got %d: %s", resp.StatusCode, string(respBody))
	}
	var getResult map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&getResult); err != nil {
		t.Fatalf("failed to decode get response: %v", err)
	}
	resp.Body.Close()

	if getResult["default_branch"] != "main" {
		t.Fatalf("get: expected default_branch 'main', got %v", getResult["default_branch"])
	}
	if getResult["root_path"] != "/tmp/test" {
		t.Fatalf("get: expected root_path '/tmp/test', got %v", getResult["root_path"])
	}

	// Step 3: Update default_branch
	updateBody := `{"default_branch":"develop"}`
	req, _ = http.NewRequest("PATCH", baseURL+"/api/v1/projects/e2e-proj", bytes.NewBufferString(updateBody))
	req.Header.Set("Content-Type", "application/json")
	resp, err = http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("update request failed: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		t.Fatalf("update: expected 200, got %d: %s", resp.StatusCode, string(respBody))
	}
	resp.Body.Close()

	// Step 4: GET again and verify update persisted
	req, _ = http.NewRequest("GET", baseURL+"/api/v1/projects/e2e-proj", nil)
	resp, err = http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("get2 request failed: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		t.Fatalf("get2: expected 200, got %d: %s", resp.StatusCode, string(respBody))
	}
	var get2Result map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&get2Result); err != nil {
		t.Fatalf("failed to decode get2 response: %v", err)
	}
	resp.Body.Close()

	if get2Result["default_branch"] != "develop" {
		t.Fatalf("get2: expected default_branch 'develop', got %v", get2Result["default_branch"])
	}

	// Step 5: List projects and verify it appears correctly
	req, _ = http.NewRequest("GET", baseURL+"/api/v1/projects", nil)
	resp, err = http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("list request failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(resp.Body)
		t.Fatalf("list: expected 200, got %d: %s", resp.StatusCode, string(respBody))
	}

	var listResult struct {
		Projects []map[string]interface{} `json:"projects"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&listResult); err != nil {
		t.Fatalf("failed to decode list response: %v", err)
	}

	var found bool
	for _, proj := range listResult.Projects {
		if proj["id"] == "e2e-proj" {
			found = true
			if proj["default_branch"] != "develop" {
				t.Fatalf("list: expected default_branch 'develop', got %v", proj["default_branch"])
			}
			break
		}
	}

	if !found {
		t.Fatalf("e2e-proj not found in list response")
	}
}
