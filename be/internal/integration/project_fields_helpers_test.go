package integration

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"path/filepath"
	"testing"
)

// projectFieldsServer spins up a fresh API server backed by a copy of the
// template DB and returns the base URL + client.
func projectFieldsServer(t *testing.T) (string, *http.Client) {
	t.Helper()
	baseURL, client, _ := projectFieldsServerWithDB(t)
	return baseURL, client
}

// projectFieldsServerWithDB is like projectFieldsServer but also returns the
// db path for tests that assert raw column values.
func projectFieldsServerWithDB(t *testing.T) (string, *http.Client, string) {
	t.Helper()
	dbDir := t.TempDir()
	dbPath := filepath.Join(dbDir, "test.db")
	if err := copyTemplateDB(dbPath); err != nil {
		t.Fatalf("failed to copy template DB: %v", err)
	}
	baseURL, client := startAPIServer(t, dbPath)
	return baseURL, client, dbPath
}

func createProjectJSON(t *testing.T, client *http.Client, baseURL, body string) {
	t.Helper()
	req, _ := http.NewRequest("POST", baseURL+"/api/v1/projects", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("create request failed: %v", err)
	}
	if resp.StatusCode != http.StatusCreated {
		respBody, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		t.Fatalf("create: expected 201, got %d: %s", resp.StatusCode, string(respBody))
	}
	resp.Body.Close()
}

func getProjectMap(t *testing.T, client *http.Client, baseURL, id string) map[string]interface{} {
	t.Helper()
	req, _ := http.NewRequest("GET", baseURL+"/api/v1/projects/"+id, nil)
	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("get request failed: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(resp.Body)
		t.Fatalf("get: expected 200, got %d: %s", resp.StatusCode, string(respBody))
	}
	var result map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		t.Fatalf("decode get response: %v", err)
	}
	return result
}

func listProjectMaps(t *testing.T, client *http.Client, baseURL string) []map[string]interface{} {
	t.Helper()
	req, _ := http.NewRequest("GET", baseURL+"/api/v1/projects", nil)
	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("list request failed: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(resp.Body)
		t.Fatalf("list: expected 200, got %d: %s", resp.StatusCode, string(respBody))
	}
	var result struct {
		Projects []map[string]interface{} `json:"projects"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		t.Fatalf("decode list response: %v", err)
	}
	return result.Projects
}

func patchProjectMap(t *testing.T, client *http.Client, baseURL, id, body string) map[string]interface{} {
	t.Helper()
	req, _ := http.NewRequest("PATCH", baseURL+"/api/v1/projects/"+id, bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("update request failed: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(resp.Body)
		t.Fatalf("update: expected 200, got %d: %s", resp.StatusCode, string(respBody))
	}
	var result map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		t.Fatalf("decode update response: %v", err)
	}
	return result
}
