package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestHandleGetAgentManual_Success(t *testing.T) {
	server := &Server{}
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

	content, ok := resp["content"].(string)
	if !ok || content == "" {
		t.Fatal("content field missing or empty")
	}

	title, ok := resp["title"].(string)
	if !ok || title != "Agent Documentation" {
		t.Errorf("title = %q, want %q", title, "Agent Documentation")
	}
}

func TestHandleGetAgentManual_ContentType(t *testing.T) {
	server := &Server{}
	req := httptest.NewRequest(http.MethodGet, "/api/v1/docs/agent-manual", nil)
	rr := httptest.NewRecorder()

	server.handleGetAgentManual(rr, req)

	ct := rr.Header().Get("Content-Type")
	if ct != "application/json" {
		t.Errorf("Content-Type = %q, want %q", ct, "application/json")
	}
}
