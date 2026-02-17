package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

func TestHandleGetAgentManual_Success(t *testing.T) {
	dir := t.TempDir()
	content := "# Agent Manual\n\nThis is the agent manual content.\n\n## Section\n\nSome details."
	if err := os.WriteFile(filepath.Join(dir, "agent_manual.md"), []byte(content), 0644); err != nil {
		t.Fatalf("failed to write agent_manual.md: %v", err)
	}

	server := &Server{dataPath: dir}
	req := httptest.NewRequest(http.MethodGet, "/api/v1/docs/agent-manual", nil)
	rr := httptest.NewRecorder()

	server.handleGetAgentManual(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d: %s", rr.Code, rr.Body.String())
	}

	var resp map[string]interface{}
	if err := json.NewDecoder(rr.Body).Decode(&resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	got, ok := resp["content"].(string)
	if !ok {
		t.Fatalf("content field missing or not a string, got: %v", resp["content"])
	}
	if got != content {
		t.Errorf("content mismatch\ngot:  %q\nwant: %q", got, content)
	}

	title, ok := resp["title"].(string)
	if !ok {
		t.Fatalf("title field missing or not a string, got: %v", resp["title"])
	}
	if title != "Agent Documentation" {
		t.Errorf("title = %q, want %q", title, "Agent Documentation")
	}
}

func TestHandleGetAgentManual_FileNotFound(t *testing.T) {
	dir := t.TempDir() // empty — no agent_manual.md

	server := &Server{dataPath: dir}
	req := httptest.NewRequest(http.MethodGet, "/api/v1/docs/agent-manual", nil)
	rr := httptest.NewRecorder()

	server.handleGetAgentManual(rr, req)

	if rr.Code != http.StatusNotFound {
		t.Errorf("expected status 404, got %d: %s", rr.Code, rr.Body.String())
	}

	var resp map[string]interface{}
	if err := json.NewDecoder(rr.Body).Decode(&resp); err != nil {
		t.Fatalf("failed to decode error response: %v", err)
	}

	errMsg, ok := resp["error"].(string)
	if !ok {
		t.Fatalf("error field missing or not a string, got: %v", resp["error"])
	}
	if errMsg == "" {
		t.Error("error message should not be empty")
	}
}

func TestHandleGetAgentManual_ContentType(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "agent_manual.md"), []byte("content"), 0644); err != nil {
		t.Fatalf("failed to write agent_manual.md: %v", err)
	}

	server := &Server{dataPath: dir}
	req := httptest.NewRequest(http.MethodGet, "/api/v1/docs/agent-manual", nil)
	rr := httptest.NewRecorder()

	server.handleGetAgentManual(rr, req)

	ct := rr.Header().Get("Content-Type")
	if ct != "application/json" {
		t.Errorf("Content-Type = %q, want %q", ct, "application/json")
	}
}

func TestHandleGetAgentManual_EmptyFile(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "agent_manual.md"), []byte(""), 0644); err != nil {
		t.Fatalf("failed to write agent_manual.md: %v", err)
	}

	server := &Server{dataPath: dir}
	req := httptest.NewRequest(http.MethodGet, "/api/v1/docs/agent-manual", nil)
	rr := httptest.NewRecorder()

	server.handleGetAgentManual(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("expected status 200 for empty file, got %d", rr.Code)
	}

	var resp map[string]interface{}
	if err := json.NewDecoder(rr.Body).Decode(&resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	got, ok := resp["content"].(string)
	if !ok {
		t.Fatalf("content field missing or not a string")
	}
	if got != "" {
		t.Errorf("content = %q, want empty string", got)
	}
}
