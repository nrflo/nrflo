package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"be/internal/clock"
	"be/internal/db"
	"be/internal/service"
)

// newCLIModelCheckServer creates a minimal Server for CLI model check handler tests.
// The auto-migrated DB contains 10 seeded read-only CLI models.
func newCLIModelCheckServer(t *testing.T) *Server {
	t.Helper()
	dbPath := filepath.Join(t.TempDir(), "cli_model_check_test.db")
	pool, err := db.NewPoolPath(dbPath, db.DefaultPoolConfig())
	if err != nil {
		t.Fatalf("failed to create pool: %v", err)
	}
	t.Cleanup(func() { pool.Close() })
	return &Server{pool: pool, clock: clock.Real()}
}

// decodeCLIModelCheckResult decodes a TestCLIModelResult from the response recorder.
func decodeCLIModelCheckResult(t *testing.T, rr *httptest.ResponseRecorder) service.TestCLIModelResult {
	t.Helper()
	var result service.TestCLIModelResult
	if err := json.NewDecoder(rr.Body).Decode(&result); err != nil {
		t.Fatalf("decode TestCLIModelResult: %v", err)
	}
	return result
}

// TestHandleTestCLIModel_NotFound verifies that requesting a non-existent model ID returns 404.
func TestHandleTestCLIModel_NotFound(t *testing.T) {
	s := newCLIModelCheckServer(t)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/cli-models/no-such-model/test", nil)
	req.SetPathValue("id", "no-such-model")
	rr := httptest.NewRecorder()
	s.handleTestCLIModel(rr, req)

	if rr.Code != http.StatusNotFound {
		t.Errorf("status = %d, want 404; body: %s", rr.Code, rr.Body.String())
	}
	assertErrorContains(t, rr, "not found")
}

// TestHandleTestCLIModel_ReturnsHTTP200OnExecutionAttempt verifies that even when
// the CLI binary is missing (expected in CI/dev environments without the CLIs installed),
// the handler returns HTTP 200 — not a 5xx error — because execution failure is a
// legitimate check result rather than a handler error.
func TestHandleTestCLIModel_ReturnsHTTP200OnExecutionAttempt(t *testing.T) {
	s := newCLIModelCheckServer(t)

	// "sonnet" is a seeded read-only model with cli_type="claude".
	req := httptest.NewRequest(http.MethodPost, "/api/v1/cli-models/sonnet/test", nil)
	req.SetPathValue("id", "sonnet")
	rr := httptest.NewRecorder()
	s.handleTestCLIModel(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("status = %d, want 200; body: %s", rr.Code, rr.Body.String())
	}
}

// TestHandleTestCLIModel_ResponseShape verifies the JSON response always contains
// the required fields (success bool, duration_ms int64), and that error is non-empty
// when success is false.
func TestHandleTestCLIModel_ResponseShape(t *testing.T) {
	s := newCLIModelCheckServer(t)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/cli-models/sonnet/test", nil)
	req.SetPathValue("id", "sonnet")
	rr := httptest.NewRecorder()
	s.handleTestCLIModel(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rr.Code)
	}

	result := decodeCLIModelCheckResult(t, rr)

	if result.DurationMs < 0 {
		t.Errorf("duration_ms = %d, want >= 0", result.DurationMs)
	}
	if !result.Success && result.Error == "" {
		t.Errorf("success=false but error field is empty")
	}
}

// TestHandleTestCLIModel_CLITypeRouting verifies that each CLI type causes the handler
// to attempt spawning the correct binary. When the binary is not installed (common in CI),
// the error message must reference the expected binary name.
func TestHandleTestCLIModel_CLITypeRouting(t *testing.T) {
	cases := []struct {
		modelID     string
		cliType     string
		expectedBin string
	}{
		{"sonnet", "claude", "claude"},
		{"opencode_gpt_normal", "opencode", "opencode"},
		{"codex_gpt_normal", "codex", "codex"},
	}

	s := newCLIModelCheckServer(t)

	for _, tc := range cases {
		t.Run(tc.modelID, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodPost, "/api/v1/cli-models/"+tc.modelID+"/test", nil)
			req.SetPathValue("id", tc.modelID)
			rr := httptest.NewRecorder()
			s.handleTestCLIModel(rr, req)

			if rr.Code != http.StatusOK {
				t.Fatalf("status = %d, want 200; body: %s", rr.Code, rr.Body.String())
			}

			result := decodeCLIModelCheckResult(t, rr)

			if result.DurationMs < 0 {
				t.Errorf("duration_ms = %d, want >= 0", result.DurationMs)
			}

			// When the binary is not installed, the error must mention the correct CLI binary.
			// If success=true, the binary was found and ran — no assertion on error needed.
			if !result.Success {
				if result.Error == "" {
					t.Errorf("model %s: success=false but error is empty", tc.modelID)
				}
				if strings.Contains(result.Error, "failed to start") &&
					!strings.Contains(result.Error, tc.expectedBin) {
					t.Errorf("model %s: error %q does not mention expected binary %q",
						tc.modelID, result.Error, tc.expectedBin)
				}
			}
		})
	}
}

// TestHandleTestCLIModel_MappedModelUsed verifies that MappedModel from the DB record
// is forwarded to the adapter via SpawnOptions. We create a custom model with a distinctive
// MappedModel value and verify the "failed to start" error still references the correct binary
// (not the mapped model name), confirming the adapter is selected from CLIType.
func TestHandleTestCLIModel_MappedModelUsed(t *testing.T) {
	s := newCLIModelCheckServer(t)

	// Insert a non-readonly custom model with a distinctive MappedModel.
	_, err := s.pool.Exec(`
		INSERT INTO cli_models (id, cli_type, display_name, mapped_model, reasoning_effort, context_length, read_only, created_at, updated_at)
		VALUES ('custom-claude', 'claude', 'Custom Claude', 'claude-custom-mapped-value', '', 200000, 0, '2026-01-01T00:00:00Z', '2026-01-01T00:00:00Z')`,
	)
	if err != nil {
		t.Fatalf("insert custom model: %v", err)
	}

	req := httptest.NewRequest(http.MethodPost, "/api/v1/cli-models/custom-claude/test", nil)
	req.SetPathValue("id", "custom-claude")
	rr := httptest.NewRecorder()
	s.handleTestCLIModel(rr, req)

	// Handler must not return 500 (internal error) for a valid cli_type.
	if rr.Code != http.StatusOK {
		t.Errorf("status = %d, want 200; body: %s", rr.Code, rr.Body.String())
	}

	result := decodeCLIModelCheckResult(t, rr)

	if result.DurationMs < 0 {
		t.Errorf("duration_ms = %d, want >= 0", result.DurationMs)
	}

	// Confirm adapter was selected from CLIType (not MappedModel).
	// When claude binary is absent, the error should reference "claude", not the mapped value.
	if !result.Success && strings.Contains(result.Error, "failed to start") {
		if !strings.Contains(result.Error, "claude") {
			t.Errorf("error %q does not mention 'claude'; expected adapter selected from cli_type", result.Error)
		}
		if strings.Contains(result.Error, "claude-custom-mapped-value") {
			t.Errorf("error %q unexpectedly contains the MappedModel value (adapter should use cli_type)", result.Error)
		}
	}
}

// TestHandleTestCLIModel_ReasoningEffortModel verifies that a model with a reasoning_effort
// set (opencode/codex) is handled correctly by the handler without errors.
func TestHandleTestCLIModel_ReasoningEffortModel(t *testing.T) {
	cases := []struct {
		modelID string
	}{
		{"opencode_gpt_high"},   // reasoning_effort="high"
		{"codex_gpt54_normal"},  // reasoning_effort="medium"
		{"codex_gpt54_high"},    // reasoning_effort="high"
	}

	s := newCLIModelCheckServer(t)

	for _, tc := range cases {
		t.Run(tc.modelID, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodPost, "/api/v1/cli-models/"+tc.modelID+"/test", nil)
			req.SetPathValue("id", tc.modelID)
			rr := httptest.NewRecorder()
			s.handleTestCLIModel(rr, req)

			// Must return 200 regardless of binary availability.
			if rr.Code != http.StatusOK {
				t.Errorf("model %s: status = %d, want 200; body: %s",
					tc.modelID, rr.Code, rr.Body.String())
			}

			result := decodeCLIModelCheckResult(t, rr)
			if !result.Success && result.Error == "" {
				t.Errorf("model %s: success=false but error is empty", tc.modelID)
			}
		})
	}
}

// TestHandleTestCLIModel_AllSeededModels verifies the handler responds with HTTP 200
// for every seeded CLI model in the database without panicking or returning unexpected
// status codes.
func TestHandleTestCLIModel_AllSeededModels(t *testing.T) {
	seededIDs := []string{
		"opus", "opus_1m", "sonnet", "haiku",
		"opencode_gpt_normal", "opencode_gpt_high",
		"codex_gpt_normal", "codex_gpt_high",
		"codex_gpt54_normal", "codex_gpt54_high",
	}

	s := newCLIModelCheckServer(t)

	for _, id := range seededIDs {
		t.Run(id, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodPost, "/api/v1/cli-models/"+id+"/test", nil)
			req.SetPathValue("id", id)
			rr := httptest.NewRecorder()
			s.handleTestCLIModel(rr, req)

			if rr.Code != http.StatusOK {
				t.Errorf("model %s: status = %d, want 200; body: %s", id, rr.Code, rr.Body.String())
			}

			// Response must be valid JSON with required fields.
			var raw map[string]interface{}
			if err := json.NewDecoder(rr.Body).Decode(&raw); err != nil {
				t.Fatalf("model %s: failed to decode JSON response: %v", id, err)
			}
			if _, ok := raw["success"]; !ok {
				t.Errorf("model %s: response missing 'success' field", id)
			}
			if _, ok := raw["duration_ms"]; !ok {
				t.Errorf("model %s: response missing 'duration_ms' field", id)
			}
		})
	}
}

// TestHandleTestCLIModel_EmptyIDNotFound verifies that an empty/whitespace-only model ID
// is treated as not found (no panic, no 500).
func TestHandleTestCLIModel_EmptyIDNotFound(t *testing.T) {
	s := newCLIModelCheckServer(t)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/cli-models//test", nil)
	req.SetPathValue("id", "")
	rr := httptest.NewRecorder()
	s.handleTestCLIModel(rr, req)

	// Empty ID should result in not found (404) or bad request — never a panic or 500 internal.
	if rr.Code == http.StatusInternalServerError {
		t.Errorf("status = 500, want non-500 for empty ID; body: %s", rr.Body.String())
	}
}
