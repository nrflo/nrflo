package api

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os/exec"
	"path/filepath"
	"testing"

	"be/internal/clock"
	"be/internal/db"
)

// skipIfJqMissingAPI skips the test if jq is not installed on the host.
func skipIfJqMissingAPI(t *testing.T) {
	t.Helper()
	if _, err := exec.LookPath("jq"); err != nil {
		t.Skip("jq not installed — skipping safety hook check test")
	}
}

// newSafetyHookCheckServer creates a minimal Server for safety hook check tests.
func newSafetyHookCheckServer(t *testing.T) *Server {
	t.Helper()
	dbPath := filepath.Join(t.TempDir(), "safety_hook_check_test.db")
	if err := apiCopyTemplateDB(dbPath); err != nil {
		t.Fatalf("copy template DB: %v", err)
	}
	pool, err := db.OpenPoolExisting(dbPath, db.DefaultPoolConfig())
	if err != nil {
		t.Fatalf("failed to create pool: %v", err)
	}
	t.Cleanup(func() { pool.Close() })
	return &Server{pool: pool, clock: clock.Real()}
}

type safetyHookCheckResponseBody struct {
	Allowed bool   `json:"allowed"`
	Reason  string `json:"reason"`
}

func decodeSafetyHookCheckResponse(t *testing.T, rr *httptest.ResponseRecorder) safetyHookCheckResponseBody {
	t.Helper()
	var result safetyHookCheckResponseBody
	if err := json.NewDecoder(rr.Body).Decode(&result); err != nil {
		t.Fatalf("decode safetyHookCheckResponse: %v (body: %s)", err, rr.Body.String())
	}
	return result
}

func postSafetyHookCheck(t *testing.T, s *Server, body interface{}) *httptest.ResponseRecorder {
	t.Helper()
	b, err := json.Marshal(body)
	if err != nil {
		t.Fatalf("json.Marshal request body: %v", err)
	}
	req := httptest.NewRequest(http.MethodPost, "/api/v1/safety-hook/check", bytes.NewReader(b))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()
	s.handleCheckSafetyHook(rr, req)
	return rr
}

func TestHandleCheckSafetyHook_MissingCommand(t *testing.T) {
	s := newSafetyHookCheckServer(t)
	body := map[string]interface{}{
		"config": map[string]interface{}{
			"enabled":   true,
			"allow_git": true,
		},
		// command intentionally omitted
	}
	rr := postSafetyHookCheck(t, s, body)
	if rr.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400; body: %s", rr.Code, rr.Body.String())
	}
}

// TestHandleCheckSafetyHook_EmptyConfig verifies that a zero-value config
// (all false, no patterns) is accepted — it produces a valid safety hook script
// that allows most commands (only hardcoded rm patterns are blocked).
func TestHandleCheckSafetyHook_EmptyConfig(t *testing.T) {
	skipIfJqMissingAPI(t)
	s := newSafetyHookCheckServer(t)
	body := map[string]interface{}{
		"command": "ls -la",
		// config intentionally omitted — zero-value SafetyHookConfig{}
	}
	rr := postSafetyHookCheck(t, s, body)
	if rr.Code != http.StatusOK {
		t.Errorf("status = %d, want 200; body: %s", rr.Code, rr.Body.String())
	}
}

func TestHandleCheckSafetyHook_InvalidJSON(t *testing.T) {
	s := newSafetyHookCheckServer(t)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/safety-hook/check", bytes.NewBufferString("not json"))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()
	s.handleCheckSafetyHook(rr, req)
	if rr.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400; body: %s", rr.Code, rr.Body.String())
	}
}

func TestHandleCheckSafetyHook_AllowedCommand(t *testing.T) {
	skipIfJqMissingAPI(t)
	s := newSafetyHookCheckServer(t)
	body := map[string]interface{}{
		"config": map[string]interface{}{
			"enabled":            true,
			"allow_git":          true,
			"rm_rf_allowed_paths": []string{"/tmp"},
			"dangerous_patterns": []string{},
		},
		"command": "ls -la",
	}
	rr := postSafetyHookCheck(t, s, body)
	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body: %s", rr.Code, rr.Body.String())
	}
	result := decodeSafetyHookCheckResponse(t, rr)
	if !result.Allowed {
		t.Errorf("expected allowed=true for 'ls -la', got blocked: %s", result.Reason)
	}
}

func TestHandleCheckSafetyHook_BlockedCommand(t *testing.T) {
	skipIfJqMissingAPI(t)
	s := newSafetyHookCheckServer(t)
	body := map[string]interface{}{
		"config": map[string]interface{}{
			"enabled":            true,
			"allow_git":          true,
			"dangerous_patterns": []string{"DROP TABLE"},
		},
		"command": "DROP TABLE users",
	}
	rr := postSafetyHookCheck(t, s, body)
	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body: %s", rr.Code, rr.Body.String())
	}
	result := decodeSafetyHookCheckResponse(t, rr)
	if result.Allowed {
		t.Error("expected allowed=false for dangerous command, got allowed=true")
	}
	if result.Reason == "" {
		t.Error("expected non-empty reason for blocked command")
	}
}

func TestHandleCheckSafetyHook_BlockedGitOp(t *testing.T) {
	skipIfJqMissingAPI(t)
	s := newSafetyHookCheckServer(t)
	body := map[string]interface{}{
		"config": map[string]interface{}{
			"enabled":            true,
			"allow_git":          false,
			"rm_rf_allowed_paths": []string{"/tmp"},
		},
		"command": "git push origin main",
	}
	rr := postSafetyHookCheck(t, s, body)
	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body: %s", rr.Code, rr.Body.String())
	}
	result := decodeSafetyHookCheckResponse(t, rr)
	if result.Allowed {
		t.Error("expected allowed=false for git push when allow_git=false, got allowed=true")
	}
	if result.Reason == "" {
		t.Error("expected non-empty reason for blocked git op")
	}
}

func TestHandleCheckSafetyHook_ResponseShape(t *testing.T) {
	skipIfJqMissingAPI(t)
	s := newSafetyHookCheckServer(t)
	body := map[string]interface{}{
		"config": map[string]interface{}{
			"enabled":            true,
			"allow_git":          true,
			"rm_rf_allowed_paths": []string{"/tmp"},
		},
		"command": "echo hello",
	}
	rr := postSafetyHookCheck(t, s, body)
	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body: %s", rr.Code, rr.Body.String())
	}
	// Verify raw JSON contains required fields
	var raw map[string]interface{}
	if err := json.NewDecoder(rr.Body).Decode(&raw); err != nil {
		t.Fatalf("decode JSON: %v", err)
	}
	if _, ok := raw["allowed"]; !ok {
		t.Error("response missing 'allowed' field")
	}
	if _, ok := raw["reason"]; !ok {
		t.Error("response missing 'reason' field")
	}
}

func TestHandleCheckSafetyHook_AllowedReasonEmpty(t *testing.T) {
	skipIfJqMissingAPI(t)
	s := newSafetyHookCheckServer(t)
	body := map[string]interface{}{
		"config": map[string]interface{}{
			"enabled":            true,
			"allow_git":          true,
			"rm_rf_allowed_paths": []string{"/tmp"},
		},
		"command": "cat README.md",
	}
	rr := postSafetyHookCheck(t, s, body)
	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body: %s", rr.Code, rr.Body.String())
	}
	result := decodeSafetyHookCheckResponse(t, rr)
	if !result.Allowed {
		t.Errorf("expected allowed=true for safe command, got blocked: %s", result.Reason)
	}
	if result.Reason != "" {
		t.Errorf("expected empty reason for allowed command, got: %s", result.Reason)
	}
}
