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

func TestCreateProjectWithUseGitWorktrees(t *testing.T) {
	dbDir := t.TempDir()
	dbPath := filepath.Join(dbDir, "test.db")

	if err := copyTemplateDB(dbPath); err != nil {
		t.Fatalf("failed to copy template DB: %v", err)
	}

	baseURL := startAPIServer(t, dbPath)

	body := `{"id":"proj-with-worktrees","name":"Project with Worktrees","use_git_worktrees":true}`
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

	if project.ID != "proj-with-worktrees" {
		t.Fatalf("expected ID 'proj-with-worktrees', got %q", project.ID)
	}

	if !project.UseGitWorktrees {
		t.Fatalf("expected UseGitWorktrees to be true, got false")
	}

	// Verify use_git_worktrees is 1 in DB
	database, err := db.Open(dbPath)
	if err != nil {
		t.Fatalf("failed to reopen DB: %v", err)
	}
	defer database.Close()

	var useGitWorktrees int
	err = database.QueryRow("SELECT use_git_worktrees FROM projects WHERE id = ?", "proj-with-worktrees").
		Scan(&useGitWorktrees)
	if err != nil {
		t.Fatalf("failed to query project: %v", err)
	}

	if useGitWorktrees != 1 {
		t.Fatalf("expected use_git_worktrees to be 1, got %d", useGitWorktrees)
	}
}

func TestCreateProjectWithoutUseGitWorktrees(t *testing.T) {
	dbDir := t.TempDir()
	dbPath := filepath.Join(dbDir, "test.db")

	if err := copyTemplateDB(dbPath); err != nil {
		t.Fatalf("failed to copy template DB: %v", err)
	}

	baseURL := startAPIServer(t, dbPath)

	body := `{"id":"proj-no-worktrees","name":"Project without Worktrees"}`
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

	if project.UseGitWorktrees {
		t.Fatalf("expected UseGitWorktrees to be false (default), got true")
	}

	// Verify use_git_worktrees is 0 in DB (default)
	database, err := db.Open(dbPath)
	if err != nil {
		t.Fatalf("failed to reopen DB: %v", err)
	}
	defer database.Close()

	var useGitWorktrees int
	err = database.QueryRow("SELECT use_git_worktrees FROM projects WHERE id = ?", "proj-no-worktrees").
		Scan(&useGitWorktrees)
	if err != nil {
		t.Fatalf("failed to query project: %v", err)
	}

	if useGitWorktrees != 0 {
		t.Fatalf("expected use_git_worktrees to be 0 (default), got %d", useGitWorktrees)
	}
}

func TestCreateProjectWithUseGitWorktreesFalse(t *testing.T) {
	dbDir := t.TempDir()
	dbPath := filepath.Join(dbDir, "test.db")

	if err := copyTemplateDB(dbPath); err != nil {
		t.Fatalf("failed to copy template DB: %v", err)
	}

	baseURL := startAPIServer(t, dbPath)

	body := `{"id":"proj-explicit-false","name":"Project Explicit False","use_git_worktrees":false}`
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

	if project.UseGitWorktrees {
		t.Fatalf("expected UseGitWorktrees to be false, got true")
	}

	// Verify use_git_worktrees is 0 in DB
	database, err := db.Open(dbPath)
	if err != nil {
		t.Fatalf("failed to reopen DB: %v", err)
	}
	defer database.Close()

	var useGitWorktrees int
	err = database.QueryRow("SELECT use_git_worktrees FROM projects WHERE id = ?", "proj-explicit-false").
		Scan(&useGitWorktrees)
	if err != nil {
		t.Fatalf("failed to query project: %v", err)
	}

	if useGitWorktrees != 0 {
		t.Fatalf("expected use_git_worktrees to be 0, got %d", useGitWorktrees)
	}
}

func TestGetProjectReturnsUseGitWorktrees(t *testing.T) {
	dbDir := t.TempDir()
	dbPath := filepath.Join(dbDir, "test.db")

	if err := copyTemplateDB(dbPath); err != nil {
		t.Fatalf("failed to copy template DB: %v", err)
	}

	baseURL := startAPIServer(t, dbPath)

	// Create project with use_git_worktrees=true
	createBody := `{"id":"get-proj-worktrees","name":"Get Project Worktrees","use_git_worktrees":true}`
	req, _ := http.NewRequest("POST", baseURL+"/api/v1/projects", bytes.NewBufferString(createBody))
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("create request failed: %v", err)
	}
	resp.Body.Close()

	// GET the project
	req, _ = http.NewRequest("GET", baseURL+"/api/v1/projects/get-proj-worktrees", nil)
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

	useGitWorktrees, ok := result["use_git_worktrees"]
	if !ok {
		t.Fatalf("expected use_git_worktrees field in response")
	}

	if useGitWorktrees != true {
		t.Fatalf("expected use_git_worktrees true, got %v", useGitWorktrees)
	}
}

func TestGetProjectReturnsUseGitWorktreesFalse(t *testing.T) {
	dbDir := t.TempDir()
	dbPath := filepath.Join(dbDir, "test.db")

	if err := copyTemplateDB(dbPath); err != nil {
		t.Fatalf("failed to copy template DB: %v", err)
	}

	baseURL := startAPIServer(t, dbPath)

	// Create project without use_git_worktrees (defaults to false)
	createBody := `{"id":"get-proj-no-worktrees","name":"Get Project No Worktrees"}`
	req, _ := http.NewRequest("POST", baseURL+"/api/v1/projects", bytes.NewBufferString(createBody))
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("create request failed: %v", err)
	}
	resp.Body.Close()

	// GET the project
	req, _ = http.NewRequest("GET", baseURL+"/api/v1/projects/get-proj-no-worktrees", nil)
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

	useGitWorktrees, ok := result["use_git_worktrees"]
	if !ok {
		t.Fatalf("expected use_git_worktrees field in response")
	}

	if useGitWorktrees != false {
		t.Fatalf("expected use_git_worktrees false, got %v", useGitWorktrees)
	}
}

func TestListProjectsReturnsUseGitWorktrees(t *testing.T) {
	dbDir := t.TempDir()
	dbPath := filepath.Join(dbDir, "test.db")

	if err := copyTemplateDB(dbPath); err != nil {
		t.Fatalf("failed to copy template DB: %v", err)
	}

	baseURL := startAPIServer(t, dbPath)

	// Create project with use_git_worktrees=true
	createBody1 := `{"id":"list-proj-worktrees-1","name":"List Project Worktrees 1","use_git_worktrees":true}`
	req, _ := http.NewRequest("POST", baseURL+"/api/v1/projects", bytes.NewBufferString(createBody1))
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("create request 1 failed: %v", err)
	}
	resp.Body.Close()

	// Create project with use_git_worktrees=false (default)
	createBody2 := `{"id":"list-proj-worktrees-2","name":"List Project Worktrees 2"}`
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

	// Find our projects and verify use_git_worktrees
	var proj1Found, proj2Found bool
	for _, proj := range result.Projects {
		id, ok := proj["id"].(string)
		if !ok {
			continue
		}

		if id == "list-proj-worktrees-1" {
			proj1Found = true
			useGitWorktrees, ok := proj["use_git_worktrees"]
			if !ok {
				t.Errorf("expected use_git_worktrees field for list-proj-worktrees-1")
			}
			if useGitWorktrees != true {
				t.Errorf("expected use_git_worktrees true for list-proj-worktrees-1, got %v", useGitWorktrees)
			}
		} else if id == "list-proj-worktrees-2" {
			proj2Found = true
			useGitWorktrees, ok := proj["use_git_worktrees"]
			if !ok {
				t.Errorf("expected use_git_worktrees field for list-proj-worktrees-2")
			}
			if useGitWorktrees != false {
				t.Errorf("expected use_git_worktrees false for list-proj-worktrees-2, got %v", useGitWorktrees)
			}
		}
	}

	if !proj1Found {
		t.Errorf("list-proj-worktrees-1 not found in list response")
	}
	if !proj2Found {
		t.Errorf("list-proj-worktrees-2 not found in list response")
	}
}
