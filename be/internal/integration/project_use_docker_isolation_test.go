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

func TestCreateProjectWithUseDockerIsolation(t *testing.T) {
	dbDir := t.TempDir()
	dbPath := filepath.Join(dbDir, "test.db")

	database, err := db.OpenPath(dbPath)
	if err != nil {
		t.Fatalf("failed to init DB: %v", err)
	}
	database.Close()

	baseURL := startAPIServer(t, dbPath)

	body := `{"id":"proj-with-docker","name":"Project with Docker","use_docker_isolation":true}`
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

	if project.ID != "proj-with-docker" {
		t.Fatalf("expected ID 'proj-with-docker', got %q", project.ID)
	}

	if !project.UseDockerIsolation {
		t.Fatalf("expected UseDockerIsolation to be true, got false")
	}

	// Verify use_docker_isolation is 1 in DB
	database, err = db.Open(dbPath)
	if err != nil {
		t.Fatalf("failed to reopen DB: %v", err)
	}
	defer database.Close()

	var useDockerIsolation int
	err = database.QueryRow("SELECT use_docker_isolation FROM projects WHERE id = ?", "proj-with-docker").
		Scan(&useDockerIsolation)
	if err != nil {
		t.Fatalf("failed to query project: %v", err)
	}

	if useDockerIsolation != 1 {
		t.Fatalf("expected use_docker_isolation to be 1, got %d", useDockerIsolation)
	}
}

func TestCreateProjectWithoutUseDockerIsolation(t *testing.T) {
	dbDir := t.TempDir()
	dbPath := filepath.Join(dbDir, "test.db")

	database, err := db.OpenPath(dbPath)
	if err != nil {
		t.Fatalf("failed to init DB: %v", err)
	}
	database.Close()

	baseURL := startAPIServer(t, dbPath)

	body := `{"id":"proj-no-docker","name":"Project without Docker"}`
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

	if project.UseDockerIsolation {
		t.Fatalf("expected UseDockerIsolation to be false (default), got true")
	}

	// Verify use_docker_isolation is 0 in DB (default)
	database, err = db.Open(dbPath)
	if err != nil {
		t.Fatalf("failed to reopen DB: %v", err)
	}
	defer database.Close()

	var useDockerIsolation int
	err = database.QueryRow("SELECT use_docker_isolation FROM projects WHERE id = ?", "proj-no-docker").
		Scan(&useDockerIsolation)
	if err != nil {
		t.Fatalf("failed to query project: %v", err)
	}

	if useDockerIsolation != 0 {
		t.Fatalf("expected use_docker_isolation to be 0 (default), got %d", useDockerIsolation)
	}
}

func TestCreateProjectWithUseDockerIsolationFalse(t *testing.T) {
	dbDir := t.TempDir()
	dbPath := filepath.Join(dbDir, "test.db")

	database, err := db.OpenPath(dbPath)
	if err != nil {
		t.Fatalf("failed to init DB: %v", err)
	}
	database.Close()

	baseURL := startAPIServer(t, dbPath)

	body := `{"id":"proj-explicit-docker-false","name":"Project Explicit Docker False","use_docker_isolation":false}`
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

	if project.UseDockerIsolation {
		t.Fatalf("expected UseDockerIsolation to be false, got true")
	}

	// Verify use_docker_isolation is 0 in DB
	database, err = db.Open(dbPath)
	if err != nil {
		t.Fatalf("failed to reopen DB: %v", err)
	}
	defer database.Close()

	var useDockerIsolation int
	err = database.QueryRow("SELECT use_docker_isolation FROM projects WHERE id = ?", "proj-explicit-docker-false").
		Scan(&useDockerIsolation)
	if err != nil {
		t.Fatalf("failed to query project: %v", err)
	}

	if useDockerIsolation != 0 {
		t.Fatalf("expected use_docker_isolation to be 0, got %d", useDockerIsolation)
	}
}

func TestGetProjectReturnsUseDockerIsolation(t *testing.T) {
	dbDir := t.TempDir()
	dbPath := filepath.Join(dbDir, "test.db")

	database, err := db.OpenPath(dbPath)
	if err != nil {
		t.Fatalf("failed to init DB: %v", err)
	}
	database.Close()

	baseURL := startAPIServer(t, dbPath)

	// Create project with use_docker_isolation=true
	createBody := `{"id":"get-proj-docker","name":"Get Project Docker","use_docker_isolation":true}`
	req, _ := http.NewRequest("POST", baseURL+"/api/v1/projects", bytes.NewBufferString(createBody))
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("create request failed: %v", err)
	}
	resp.Body.Close()

	// GET the project
	req, _ = http.NewRequest("GET", baseURL+"/api/v1/projects/get-proj-docker", nil)
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

	useDockerIsolation, ok := result["use_docker_isolation"]
	if !ok {
		t.Fatalf("expected use_docker_isolation field in response")
	}

	if useDockerIsolation != true {
		t.Fatalf("expected use_docker_isolation true, got %v", useDockerIsolation)
	}
}

func TestGetProjectReturnsUseDockerIsolationFalse(t *testing.T) {
	dbDir := t.TempDir()
	dbPath := filepath.Join(dbDir, "test.db")

	database, err := db.OpenPath(dbPath)
	if err != nil {
		t.Fatalf("failed to init DB: %v", err)
	}
	database.Close()

	baseURL := startAPIServer(t, dbPath)

	// Create project without use_docker_isolation (defaults to false)
	createBody := `{"id":"get-proj-no-docker","name":"Get Project No Docker"}`
	req, _ := http.NewRequest("POST", baseURL+"/api/v1/projects", bytes.NewBufferString(createBody))
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("create request failed: %v", err)
	}
	resp.Body.Close()

	// GET the project
	req, _ = http.NewRequest("GET", baseURL+"/api/v1/projects/get-proj-no-docker", nil)
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

	useDockerIsolation, ok := result["use_docker_isolation"]
	if !ok {
		t.Fatalf("expected use_docker_isolation field in response")
	}

	if useDockerIsolation != false {
		t.Fatalf("expected use_docker_isolation false, got %v", useDockerIsolation)
	}
}

func TestListProjectsReturnsUseDockerIsolation(t *testing.T) {
	dbDir := t.TempDir()
	dbPath := filepath.Join(dbDir, "test.db")

	database, err := db.OpenPath(dbPath)
	if err != nil {
		t.Fatalf("failed to init DB: %v", err)
	}
	database.Close()

	baseURL := startAPIServer(t, dbPath)

	// Create project with use_docker_isolation=true
	createBody1 := `{"id":"list-proj-docker-1","name":"List Project Docker 1","use_docker_isolation":true}`
	req, _ := http.NewRequest("POST", baseURL+"/api/v1/projects", bytes.NewBufferString(createBody1))
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("create request 1 failed: %v", err)
	}
	resp.Body.Close()

	// Create project with use_docker_isolation=false (default)
	createBody2 := `{"id":"list-proj-docker-2","name":"List Project Docker 2"}`
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

	// Find our projects and verify use_docker_isolation
	var proj1Found, proj2Found bool
	for _, proj := range result.Projects {
		id, ok := proj["id"].(string)
		if !ok {
			continue
		}

		if id == "list-proj-docker-1" {
			proj1Found = true
			useDockerIsolation, ok := proj["use_docker_isolation"]
			if !ok {
				t.Errorf("expected use_docker_isolation field for list-proj-docker-1")
			}
			if useDockerIsolation != true {
				t.Errorf("expected use_docker_isolation true for list-proj-docker-1, got %v", useDockerIsolation)
			}
		} else if id == "list-proj-docker-2" {
			proj2Found = true
			useDockerIsolation, ok := proj["use_docker_isolation"]
			if !ok {
				t.Errorf("expected use_docker_isolation field for list-proj-docker-2")
			}
			if useDockerIsolation != false {
				t.Errorf("expected use_docker_isolation false for list-proj-docker-2, got %v", useDockerIsolation)
			}
		}
	}

	if !proj1Found {
		t.Errorf("list-proj-docker-1 not found in list response")
	}
	if !proj2Found {
		t.Errorf("list-proj-docker-2 not found in list response")
	}
}

func TestMigrationExistingProjectsHaveUseDockerIsolationFalse(t *testing.T) {
	dbDir := t.TempDir()
	dbPath := filepath.Join(dbDir, "test.db")

	database, err := db.OpenPath(dbPath)
	if err != nil {
		t.Fatalf("failed to init DB: %v", err)
	}

	// Directly query a new database to verify the migration sets default to 0
	var defaultValue int
	err = database.QueryRow("SELECT use_docker_isolation FROM projects LIMIT 1").Scan(&defaultValue)
	if err != nil && err.Error() != "sql: no rows in result set" {
		// It's OK if there are no rows, we just want to verify the column exists with default 0
		database.Close()
		t.Fatalf("unexpected error querying use_docker_isolation: %v", err)
	}
	database.Close()

	// Create a project via API and verify it defaults to false
	baseURL := startAPIServer(t, dbPath)

	createBody := `{"id":"migration-docker-test","name":"Migration Docker Test"}`
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

	if result["use_docker_isolation"] != false {
		t.Fatalf("expected use_docker_isolation to default to false, got %v", result["use_docker_isolation"])
	}
}
