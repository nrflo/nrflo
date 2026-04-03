package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os/exec"
	"path/filepath"
	"testing"

	"be/internal/clock"
	"be/internal/db"
	"be/internal/service"
	"be/internal/spawner"
)

// mockCLIAdapter returns a CLIAdapter that runs "echo ok" instead of a real CLI binary.
type mockCLIAdapter struct {
	name string
}

func (m *mockCLIAdapter) Name() string                                    { return m.name }
func (m *mockCLIAdapter) BuildCommand(_ spawner.SpawnOptions) *exec.Cmd   { return exec.Command("echo", "ok") }
func (m *mockCLIAdapter) MapModel(model string) string                    { return model }
func (m *mockCLIAdapter) SupportsSessionID() bool                         { return false }
func (m *mockCLIAdapter) SupportsSystemPromptFile() bool                  { return false }
func (m *mockCLIAdapter) SupportsResume() bool                            { return false }
func (m *mockCLIAdapter) UsesStdinPrompt() bool                           { return false }
func (m *mockCLIAdapter) BuildResumeCommand(_ spawner.ResumeOptions) *exec.Cmd { return nil }

func mockGetCLIAdapter(cliType string) (spawner.CLIAdapter, error) {
	return &mockCLIAdapter{name: cliType}, nil
}

// newCLIModelCheckServer creates a minimal Server with mocked CLI adapter for tests.
func newCLIModelCheckServer(t *testing.T) *Server {
	t.Helper()
	dbPath := filepath.Join(t.TempDir(), "cli_model_check_test.db")
	pool, err := db.NewPoolPath(dbPath, db.DefaultPoolConfig())
	if err != nil {
		t.Fatalf("failed to create pool: %v", err)
	}
	t.Cleanup(func() { pool.Close() })
	return &Server{pool: pool, clock: clock.Real(), cliAdapterFunc: mockGetCLIAdapter}
}

func decodeCLIModelCheckResult(t *testing.T, rr *httptest.ResponseRecorder) service.TestCLIModelResult {
	t.Helper()
	var result service.TestCLIModelResult
	if err := json.NewDecoder(rr.Body).Decode(&result); err != nil {
		t.Fatalf("decode TestCLIModelResult: %v", err)
	}
	return result
}

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

func TestHandleTestCLIModel_ReturnsHTTP200OnSuccess(t *testing.T) {
	s := newCLIModelCheckServer(t)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/cli-models/sonnet/test", nil)
	req.SetPathValue("id", "sonnet")
	rr := httptest.NewRecorder()
	s.handleTestCLIModel(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("status = %d, want 200; body: %s", rr.Code, rr.Body.String())
	}
	result := decodeCLIModelCheckResult(t, rr)
	if !result.Success {
		t.Errorf("expected success=true, got error: %s", result.Error)
	}
}

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
}

func TestHandleTestCLIModel_CLITypeRouting(t *testing.T) {
	cases := []struct {
		modelID string
		cliType string
	}{
		{"sonnet", "claude"},
		{"opencode_gpt_normal", "opencode"},
		{"codex_gpt_normal", "codex"},
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
			if !result.Success {
				t.Errorf("model %s: expected success=true, got error: %s", tc.modelID, result.Error)
			}
		})
	}
}

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
			var raw map[string]interface{}
			if err := json.NewDecoder(rr.Body).Decode(&raw); err != nil {
				t.Fatalf("model %s: failed to decode JSON: %v", id, err)
			}
			if _, ok := raw["success"]; !ok {
				t.Errorf("model %s: missing 'success' field", id)
			}
			if _, ok := raw["duration_ms"]; !ok {
				t.Errorf("model %s: missing 'duration_ms' field", id)
			}
		})
	}
}

func TestHandleTestCLIModel_EmptyIDNotFound(t *testing.T) {
	s := newCLIModelCheckServer(t)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/cli-models//test", nil)
	req.SetPathValue("id", "")
	rr := httptest.NewRecorder()
	s.handleTestCLIModel(rr, req)

	if rr.Code == http.StatusInternalServerError {
		t.Errorf("status = 500, want non-500 for empty ID; body: %s", rr.Body.String())
	}
}

func TestHandleTestCLIModel_FailedStart(t *testing.T) {
	s := newCLIModelCheckServer(t)
	// Override adapter to return a command that will fail to start
	s.cliAdapterFunc = func(cliType string) (spawner.CLIAdapter, error) {
		return &failStartAdapter{name: cliType}, nil
	}

	req := httptest.NewRequest(http.MethodPost, "/api/v1/cli-models/sonnet/test", nil)
	req.SetPathValue("id", "sonnet")
	rr := httptest.NewRecorder()
	s.handleTestCLIModel(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rr.Code)
	}
	result := decodeCLIModelCheckResult(t, rr)
	if result.Success {
		t.Error("expected success=false for nonexistent binary")
	}
	if result.Error == "" {
		t.Error("expected non-empty error")
	}
}

// failStartAdapter returns a command for a nonexistent binary.
type failStartAdapter struct{ name string }

func (a *failStartAdapter) Name() string                                    { return a.name }
func (a *failStartAdapter) BuildCommand(_ spawner.SpawnOptions) *exec.Cmd   { return exec.Command("__nrflow_nonexistent_binary__") }
func (a *failStartAdapter) MapModel(model string) string                    { return model }
func (a *failStartAdapter) SupportsSessionID() bool                         { return false }
func (a *failStartAdapter) SupportsSystemPromptFile() bool                  { return false }
func (a *failStartAdapter) SupportsResume() bool                            { return false }
func (a *failStartAdapter) UsesStdinPrompt() bool                           { return false }
func (a *failStartAdapter) BuildResumeCommand(_ spawner.ResumeOptions) *exec.Cmd { return nil }
