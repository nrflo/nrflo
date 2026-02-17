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

func TestUpdateProjectToggleUseDockerIsolation(t *testing.T) {
	dbDir := t.TempDir()
	dbPath := filepath.Join(dbDir, "test.db")

	database, err := db.OpenPath(dbPath)
	if err != nil {
		t.Fatalf("failed to init DB: %v", err)
	}
	database.Close()

	baseURL := startAPIServer(t, dbPath)

	// Create project without use_docker_isolation (defaults to false)
	createBody := `{"id":"toggle-docker-proj","name":"Toggle Docker Project"}`
	req, _ := http.NewRequest("POST", baseURL+"/api/v1/projects", bytes.NewBufferString(createBody))
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("create request failed: %v", err)
	}
	resp.Body.Close()

	// Update to set use_docker_isolation=true
	updateBody := `{"use_docker_isolation":true}`
	req, _ = http.NewRequest("PATCH", baseURL+"/api/v1/projects/toggle-docker-proj", bytes.NewBufferString(updateBody))
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

	useDockerIsolation, ok := result["use_docker_isolation"]
	if !ok {
		t.Fatalf("expected use_docker_isolation field in response")
	}

	if useDockerIsolation != true {
		t.Fatalf("expected use_docker_isolation true, got %v", useDockerIsolation)
	}

	// Verify in DB
	database, err = db.Open(dbPath)
	if err != nil {
		t.Fatalf("failed to reopen DB: %v", err)
	}
	defer database.Close()

	var dbValue int
	err = database.QueryRow("SELECT use_docker_isolation FROM projects WHERE id = ?", "toggle-docker-proj").
		Scan(&dbValue)
	if err != nil {
		t.Fatalf("failed to query project: %v", err)
	}

	if dbValue != 1 {
		t.Fatalf("expected use_docker_isolation to be 1 in DB, got %d", dbValue)
	}

	// Now toggle it back to false
	updateBody2 := `{"use_docker_isolation":false}`
	req, _ = http.NewRequest("PATCH", baseURL+"/api/v1/projects/toggle-docker-proj", bytes.NewBufferString(updateBody2))
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

	useDockerIsolation2, ok := result2["use_docker_isolation"]
	if !ok {
		t.Fatalf("expected use_docker_isolation field in response 2")
	}

	if useDockerIsolation2 != false {
		t.Fatalf("expected use_docker_isolation false, got %v", useDockerIsolation2)
	}

	// Verify in DB again
	var dbValue2 int
	err = database.QueryRow("SELECT use_docker_isolation FROM projects WHERE id = ?", "toggle-docker-proj").
		Scan(&dbValue2)
	if err != nil {
		t.Fatalf("failed to query project after second update: %v", err)
	}

	if dbValue2 != 0 {
		t.Fatalf("expected use_docker_isolation to be 0 in DB, got %d", dbValue2)
	}
}

func TestUpdateProjectWithoutUseDockerIsolationDoesNotReset(t *testing.T) {
	dbDir := t.TempDir()
	dbPath := filepath.Join(dbDir, "test.db")

	database, err := db.OpenPath(dbPath)
	if err != nil {
		t.Fatalf("failed to init DB: %v", err)
	}
	database.Close()

	baseURL := startAPIServer(t, dbPath)

	// Create project with use_docker_isolation=true
	createBody := `{"id":"no-reset-docker-proj","name":"No Reset Docker Project","use_docker_isolation":true}`
	req, _ := http.NewRequest("POST", baseURL+"/api/v1/projects", bytes.NewBufferString(createBody))
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("create request failed: %v", err)
	}
	resp.Body.Close()

	// Update other field without touching use_docker_isolation
	updateBody := `{"name":"Updated Docker Name"}`
	req, _ = http.NewRequest("PATCH", baseURL+"/api/v1/projects/no-reset-docker-proj", bytes.NewBufferString(updateBody))
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

	// Verify use_docker_isolation is still true (not reset)
	useDockerIsolation, ok := result["use_docker_isolation"]
	if !ok {
		t.Fatalf("expected use_docker_isolation field in response")
	}

	if useDockerIsolation != true {
		t.Fatalf("expected use_docker_isolation to remain true, got %v", useDockerIsolation)
	}

	// Verify in DB
	database, err = db.Open(dbPath)
	if err != nil {
		t.Fatalf("failed to reopen DB: %v", err)
	}
	defer database.Close()

	var dbValue int
	err = database.QueryRow("SELECT use_docker_isolation FROM projects WHERE id = ?", "no-reset-docker-proj").
		Scan(&dbValue)
	if err != nil {
		t.Fatalf("failed to query project: %v", err)
	}

	if dbValue != 1 {
		t.Fatalf("expected use_docker_isolation to remain 1 in DB, got %d", dbValue)
	}
}

func TestUpdateProjectMultipleFieldsIncludingUseDockerIsolation(t *testing.T) {
	dbDir := t.TempDir()
	dbPath := filepath.Join(dbDir, "test.db")

	database, err := db.OpenPath(dbPath)
	if err != nil {
		t.Fatalf("failed to init DB: %v", err)
	}
	database.Close()

	baseURL := startAPIServer(t, dbPath)

	// Create project with some fields
	createBody := `{"id":"multi-docker-proj","name":"Multi Docker Project","default_branch":"main"}`
	req, _ := http.NewRequest("POST", baseURL+"/api/v1/projects", bytes.NewBufferString(createBody))
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("create request failed: %v", err)
	}
	resp.Body.Close()

	// Update multiple fields including use_docker_isolation
	updateBody := `{"name":"Updated Docker Multi","default_branch":"develop","use_docker_isolation":true}`
	req, _ = http.NewRequest("PATCH", baseURL+"/api/v1/projects/multi-docker-proj", bytes.NewBufferString(updateBody))
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

	if result["name"] != "Updated Docker Multi" {
		t.Errorf("expected name 'Updated Docker Multi', got %v", result["name"])
	}
	if result["default_branch"] != "develop" {
		t.Errorf("expected default_branch 'develop', got %v", result["default_branch"])
	}
	if result["use_docker_isolation"] != true {
		t.Errorf("expected use_docker_isolation true, got %v", result["use_docker_isolation"])
	}
}

func TestProjectUseDockerIsolationEndToEnd(t *testing.T) {
	dbDir := t.TempDir()
	dbPath := filepath.Join(dbDir, "test.db")

	database, err := db.OpenPath(dbPath)
	if err != nil {
		t.Fatalf("failed to init DB: %v", err)
	}
	database.Close()

	baseURL := startAPIServer(t, dbPath)

	// Step 1: Create project with use_docker_isolation=true
	createBody := `{"id":"e2e-docker","name":"E2E Docker","use_docker_isolation":true,"default_branch":"main"}`
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
	req, _ = http.NewRequest("GET", baseURL+"/api/v1/projects/e2e-docker", nil)
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

	if getResult["use_docker_isolation"] != true {
		t.Fatalf("get: expected use_docker_isolation true, got %v", getResult["use_docker_isolation"])
	}
	if getResult["default_branch"] != "main" {
		t.Fatalf("get: expected default_branch 'main', got %v", getResult["default_branch"])
	}

	// Step 3: Update use_docker_isolation to false
	updateBody := `{"use_docker_isolation":false}`
	req, _ = http.NewRequest("PATCH", baseURL+"/api/v1/projects/e2e-docker", bytes.NewBufferString(updateBody))
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
	req, _ = http.NewRequest("GET", baseURL+"/api/v1/projects/e2e-docker", nil)
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

	if get2Result["use_docker_isolation"] != false {
		t.Fatalf("get2: expected use_docker_isolation false, got %v", get2Result["use_docker_isolation"])
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
		if proj["id"] == "e2e-docker" {
			found = true
			if proj["use_docker_isolation"] != false {
				t.Fatalf("list: expected use_docker_isolation false, got %v", proj["use_docker_isolation"])
			}
			break
		}
	}

	if !found {
		t.Fatalf("e2e-docker not found in list response")
	}
}
