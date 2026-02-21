package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"be/internal/usagelimits"
)

func newUsageLimitsServer() *Server {
	return &Server{usageLimitsCache: usagelimits.NewCache(nil, nil)}
}

func TestHandleGetUsageLimits_NilCache(t *testing.T) {
	server := newUsageLimitsServer()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/usage-limits", nil)
	rr := httptest.NewRecorder()

	server.handleGetUsageLimits(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("status = %d, want 200", rr.Code)
	}

	var resp map[string]interface{}
	if err := json.NewDecoder(rr.Body).Decode(&resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	claude, ok := resp["claude"].(map[string]interface{})
	if !ok {
		t.Fatalf("claude field missing or wrong type: %v", resp["claude"])
	}
	if claude["available"] != false {
		t.Errorf("claude.available = %v, want false", claude["available"])
	}

	codex, ok := resp["codex"].(map[string]interface{})
	if !ok {
		t.Fatalf("codex field missing or wrong type: %v", resp["codex"])
	}
	if codex["available"] != false {
		t.Errorf("codex.available = %v, want false", codex["available"])
	}

	if val, exists := resp["fetched_at"]; !exists || val != nil {
		t.Errorf("fetched_at = %v, want nil", val)
	}
}

func TestHandleGetUsageLimits_WithCachedData(t *testing.T) {
	server := newUsageLimitsServer()

	now := time.Now().UTC().Truncate(time.Second)
	data := &usagelimits.UsageLimits{
		Claude: usagelimits.ToolUsage{
			Available: true,
			Session:   &usagelimits.UsageMetric{UsedPct: 45.2, ResetsAt: "in 2h"},
			Weekly:    &usagelimits.UsageMetric{UsedPct: 12.5, ResetsAt: "Monday"},
		},
		Codex: usagelimits.ToolUsage{
			Available: false,
			Error:     "codex not installed",
		},
		FetchedAt: now,
	}
	server.usageLimitsCache.Set(data)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/usage-limits", nil)
	rr := httptest.NewRecorder()

	server.handleGetUsageLimits(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("status = %d, want 200", rr.Code)
	}

	var resp map[string]interface{}
	if err := json.NewDecoder(rr.Body).Decode(&resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	claude, ok := resp["claude"].(map[string]interface{})
	if !ok {
		t.Fatalf("claude field missing or wrong type: %v", resp["claude"])
	}
	if claude["available"] != true {
		t.Errorf("claude.available = %v, want true", claude["available"])
	}

	session, ok := claude["session"].(map[string]interface{})
	if !ok {
		t.Fatalf("claude.session missing or wrong type: %v", claude["session"])
	}
	if session["used_pct"] != 45.2 {
		t.Errorf("claude.session.used_pct = %v, want 45.2", session["used_pct"])
	}
	if session["resets_at"] != "in 2h" {
		t.Errorf("claude.session.resets_at = %v, want 'in 2h'", session["resets_at"])
	}

	codex, ok := resp["codex"].(map[string]interface{})
	if !ok {
		t.Fatalf("codex field missing or wrong type: %v", resp["codex"])
	}
	if codex["available"] != false {
		t.Errorf("codex.available = %v, want false", codex["available"])
	}
	if codex["error"] != "codex not installed" {
		t.Errorf("codex.error = %v, want 'codex not installed'", codex["error"])
	}

	if resp["fetched_at"] == nil {
		t.Error("fetched_at should not be nil when cache is populated")
	}
}

func TestHandleGetUsageLimits_ContentType(t *testing.T) {
	server := newUsageLimitsServer()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/usage-limits", nil)
	rr := httptest.NewRecorder()

	server.handleGetUsageLimits(rr, req)

	ct := rr.Header().Get("Content-Type")
	if ct != "application/json" {
		t.Errorf("Content-Type = %q, want %q", ct, "application/json")
	}
}

func TestHandleGetUsageLimits_AllFieldsPresent(t *testing.T) {
	// Verify all required JSON fields are present in nil-cache response.
	server := newUsageLimitsServer()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/usage-limits", nil)
	rr := httptest.NewRecorder()

	server.handleGetUsageLimits(rr, req)

	var resp map[string]interface{}
	if err := json.NewDecoder(rr.Body).Decode(&resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	for _, key := range []string{"claude", "codex", "fetched_at"} {
		if _, exists := resp[key]; !exists {
			t.Errorf("response missing required key %q", key)
		}
	}
}

func TestHandleGetUsageLimits_ToolUnavailable(t *testing.T) {
	// When a tool is not available, its session and weekly fields should be nil.
	server := newUsageLimitsServer()
	server.usageLimitsCache.Set(&usagelimits.UsageLimits{
		Claude: usagelimits.ToolUsage{Available: false},
		Codex:  usagelimits.ToolUsage{Available: false},
	})

	req := httptest.NewRequest(http.MethodGet, "/api/v1/usage-limits", nil)
	rr := httptest.NewRecorder()

	server.handleGetUsageLimits(rr, req)

	var resp map[string]interface{}
	if err := json.NewDecoder(rr.Body).Decode(&resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	claude := resp["claude"].(map[string]interface{})
	if claude["available"] != false {
		t.Errorf("claude.available = %v, want false", claude["available"])
	}
	if claude["session"] != nil {
		t.Errorf("claude.session = %v, want nil", claude["session"])
	}
	if claude["weekly"] != nil {
		t.Errorf("claude.weekly = %v, want nil", claude["weekly"])
	}
}

func TestHandleGetUsageLimits_ParseErrorReflectedInResponse(t *testing.T) {
	// When a tool is available but parsing failed, error field should be present.
	server := newUsageLimitsServer()
	server.usageLimitsCache.Set(&usagelimits.UsageLimits{
		Claude: usagelimits.ToolUsage{
			Available: true,
			Error:     "failed to parse /usage output",
		},
	})

	req := httptest.NewRequest(http.MethodGet, "/api/v1/usage-limits", nil)
	rr := httptest.NewRecorder()

	server.handleGetUsageLimits(rr, req)

	var resp map[string]interface{}
	if err := json.NewDecoder(rr.Body).Decode(&resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	claude := resp["claude"].(map[string]interface{})
	if claude["available"] != true {
		t.Errorf("claude.available = %v, want true", claude["available"])
	}
	if claude["error"] != "failed to parse /usage output" {
		t.Errorf("claude.error = %v, want error string", claude["error"])
	}
}
