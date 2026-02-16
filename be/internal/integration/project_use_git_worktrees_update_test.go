package integration

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"path/filepath"
	"testing"

	"be/internal/db"
)

func TestUpdateProjectToggleUseGitWorktrees(t *testing.T) {
	dbDir := t.TempDir()
	dbPath := filepath.Join(dbDir, "test.db")

	database, err := db.OpenPath(dbPath)
	if err != nil {
		t.Fatalf("failed to init DB: %v", err)
	}
	database.Close()

	baseURL := startAPIServer(t, dbPath)

	// Create project without use_git_worktrees (defaults to false)
	createBody := `{"id":"toggle-proj","name":"Toggle Project"}`
	req, _ := http.NewRequest("POST", baseURL+"/api/v1/projects", bytes.NewBufferString(createBody))
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("create request failed: %v", err)
	}
	resp.Body.Close()

	// Update to set use_git_worktrees=true
	updateBody := `{"use_git_worktrees":true}`
	req, _ = http.NewRequest("PATCH", baseURL+"/api/v1/projects/toggle-proj", bytes.NewBufferString(updateBody))
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

	useGitWorktrees, ok := result["use_git_worktrees"]
	if !ok {
		t.Fatalf("expected use_git_worktrees field in response")
	}

	if useGitWorktrees != true {
		t.Fatalf("expected use_git_worktrees true, got %v", useGitWorktrees)
	}

	// Verify in DB
	database, err = db.Open(dbPath)
	if err != nil {
		t.Fatalf("failed to reopen DB: %v", err)
	}
	defer database.Close()

	var dbValue int
	err = database.QueryRow("SELECT use_git_worktrees FROM projects WHERE id = ?", "toggle-proj").
		Scan(&dbValue)
	if err != nil {
		t.Fatalf("failed to query project: %v", err)
	}

	if dbValue != 1 {
		t.Fatalf("expected use_git_worktrees to be 1 in DB, got %d", dbValue)
	}

	// Now toggle it back to false
	updateBody2 := `{"use_git_worktrees":false}`
	req, _ = http.NewRequest("PATCH", baseURL+"/api/v1/projects/toggle-proj", bytes.NewBufferString(updateBody2))
	req.Header.Set("Content-Type", "application/json")
	resp2, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("update request 2 failed: %v", err)
	}
	defer resp2.Body.Close()

	if resp2.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(resp2.Body)
		t.Fatalf("expected 200, got %d: %s", resp2.StatusCode, string(respBody))
	}

	var result2 map[string]interface{}
	if err := json.NewDecoder(resp2.Body).Decode(&result2); err != nil {
		t.Fatalf("failed to decode response 2: %v", err)
	}

	useGitWorktrees2, ok := result2["use_git_worktrees"]
	if !ok {
		t.Fatalf("expected use_git_worktrees field in response 2")
	}

	if useGitWorktrees2 != false {
		t.Fatalf("expected use_git_worktrees false, got %v", useGitWorktrees2)
	}

	// Verify in DB again
	var dbValue2 int
	err = database.QueryRow("SELECT use_git_worktrees FROM projects WHERE id = ?", "toggle-proj").
		Scan(&dbValue2)
	if err != nil {
		t.Fatalf("failed to query project after second update: %v", err)
	}

	if dbValue2 != 0 {
		t.Fatalf("expected use_git_worktrees to be 0 in DB, got %d", dbValue2)
	}
}

func TestUpdateProjectWithoutUseGitWorktreesDoesNotReset(t *testing.T) {
	dbDir := t.TempDir()
	dbPath := filepath.Join(dbDir, "test.db")

	database, err := db.OpenPath(dbPath)
	if err != nil {
		t.Fatalf("failed to init DB: %v", err)
	}
	database.Close()

	baseURL := startAPIServer(t, dbPath)

	// Create project with use_git_worktrees=true
	createBody := `{"id":"no-reset-proj","name":"No Reset Project","use_git_worktrees":true}`
	req, _ := http.NewRequest("POST", baseURL+"/api/v1/projects", bytes.NewBufferString(createBody))
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("create request failed: %v", err)
	}
	resp.Body.Close()

	// Update other field without touching use_git_worktrees
	updateBody := `{"name":"Updated Name"}`
	req, _ = http.NewRequest("PATCH", baseURL+"/api/v1/projects/no-reset-proj", bytes.NewBufferString(updateBody))
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

	// Verify use_git_worktrees is still true (not reset)
	useGitWorktrees, ok := result["use_git_worktrees"]
	if !ok {
		t.Fatalf("expected use_git_worktrees field in response")
	}

	if useGitWorktrees != true {
		t.Fatalf("expected use_git_worktrees to remain true, got %v", useGitWorktrees)
	}

	// Verify in DB
	database, err = db.Open(dbPath)
	if err != nil {
		t.Fatalf("failed to reopen DB: %v", err)
	}
	defer database.Close()

	var dbValue int
	err = database.QueryRow("SELECT use_git_worktrees FROM projects WHERE id = ?", "no-reset-proj").
		Scan(&dbValue)
	if err != nil {
		t.Fatalf("failed to query project: %v", err)
	}

	if dbValue != 1 {
		t.Fatalf("expected use_git_worktrees to remain 1 in DB, got %d", dbValue)
	}
}

func TestUpdateProjectMultipleFieldsIncludingUseGitWorktrees(t *testing.T) {
	dbDir := t.TempDir()
	dbPath := filepath.Join(dbDir, "test.db")

	database, err := db.OpenPath(dbPath)
	if err != nil {
		t.Fatalf("failed to init DB: %v", err)
	}
	database.Close()

	baseURL := startAPIServer(t, dbPath)

	// Create project with some fields
	createBody := `{"id":"multi-field-proj","name":"Multi Field Project","default_branch":"main"}`
	req, _ := http.NewRequest("POST", baseURL+"/api/v1/projects", bytes.NewBufferString(createBody))
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("create request failed: %v", err)
	}
	resp.Body.Close()

	// Update multiple fields including use_git_worktrees
	updateBody := `{"name":"Updated Multi","default_branch":"develop","use_git_worktrees":true}`
	req, _ = http.NewRequest("PATCH", baseURL+"/api/v1/projects/multi-field-proj", bytes.NewBufferString(updateBody))
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
		t.Errorf("expected name 'Updated Multi', got %v", result["name"])
	}
	if result["default_branch"] != "develop" {
		t.Errorf("expected default_branch 'develop', got %v", result["default_branch"])
	}
	if result["use_git_worktrees"] != true {
		t.Errorf("expected use_git_worktrees true, got %v", result["use_git_worktrees"])
	}
}

func TestProjectUseGitWorktreesEndToEnd(t *testing.T) {
	dbDir := t.TempDir()
	dbPath := filepath.Join(dbDir, "test.db")

	database, err := db.OpenPath(dbPath)
	if err != nil {
		t.Fatalf("failed to init DB: %v", err)
	}
	database.Close()

	baseURL := startAPIServer(t, dbPath)

	// Step 1: Create project with use_git_worktrees=true
	createBody := `{"id":"e2e-worktrees","name":"E2E Worktrees","use_git_worktrees":true,"default_branch":"main"}`
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
	req, _ = http.NewRequest("GET", baseURL+"/api/v1/projects/e2e-worktrees", nil)
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

	if getResult["use_git_worktrees"] != true {
		t.Fatalf("get: expected use_git_worktrees true, got %v", getResult["use_git_worktrees"])
	}
	if getResult["default_branch"] != "main" {
		t.Fatalf("get: expected default_branch 'main', got %v", getResult["default_branch"])
	}

	// Step 3: Update use_git_worktrees to false
	updateBody := `{"use_git_worktrees":false}`
	req, _ = http.NewRequest("PATCH", baseURL+"/api/v1/projects/e2e-worktrees", bytes.NewBufferString(updateBody))
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
	req, _ = http.NewRequest("GET", baseURL+"/api/v1/projects/e2e-worktrees", nil)
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

	if get2Result["use_git_worktrees"] != false {
		t.Fatalf("get2: expected use_git_worktrees false, got %v", get2Result["use_git_worktrees"])
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
		if proj["id"] == "e2e-worktrees" {
			found = true
			if proj["use_git_worktrees"] != false {
				t.Fatalf("list: expected use_git_worktrees false, got %v", proj["use_git_worktrees"])
			}
			break
		}
	}

	if !found {
		t.Fatalf("e2e-worktrees not found in list response")
	}
}

func TestMigrationExistingProjectsHaveUseGitWorktreesFalse(t *testing.T) {
	dbDir := t.TempDir()
	dbPath := filepath.Join(dbDir, "test.db")

	database, err := db.OpenPath(dbPath)
	if err != nil {
		t.Fatalf("failed to init DB: %v", err)
	}

	// Directly query a new database to verify the migration sets default to 0
	var defaultValue int
	err = database.QueryRow("SELECT use_git_worktrees FROM projects LIMIT 1").Scan(&defaultValue)
	if err != nil && err.Error() != "sql: no rows in result set" {
		// It's OK if there are no rows, we just want to verify the column exists with default 0
		database.Close()
		t.Fatalf("unexpected error querying use_git_worktrees: %v", err)
	}
	database.Close()

	// Create a project via API and verify it defaults to false
	baseURL := startAPIServer(t, dbPath)

	createBody := `{"id":"migration-test","name":"Migration Test"}`
	req, _ := http.NewRequest("POST", baseURL+"/api/v1/projects", bytes.NewBufferString(createBody))
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("create request failed: %v", err)
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

	if result["use_git_worktrees"] != false {
		t.Fatalf("expected use_git_worktrees to default to false, got %v", result["use_git_worktrees"])
	}
}
