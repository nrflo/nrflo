package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

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
	if err := apiCopyTemplateDB(dbPath); err != nil {
		t.Fatalf("copy template DB: %v", err)
	}
	pool, err := db.OpenPoolExisting(dbPath, db.DefaultPoolConfig())
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
		{"opencode_gpt54", "opencode"},
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
		"opus_4_6", "opus_4_6_1m", "opus_4_7", "opus_4_7_1m", "sonnet", "haiku",
		"opencode_minimax_m25_free", "opencode_qwen36_plus_free", "opencode_gpt54",
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
func (a *failStartAdapter) BuildCommand(_ spawner.SpawnOptions) *exec.Cmd   { return exec.Command("__nrflo_nonexistent_binary__") }
func (a *failStartAdapter) MapModel(model string) string                    { return model }
func (a *failStartAdapter) SupportsSessionID() bool                         { return false }
func (a *failStartAdapter) SupportsSystemPromptFile() bool                  { return false }
func (a *failStartAdapter) SupportsResume() bool                            { return false }
func (a *failStartAdapter) UsesStdinPrompt() bool                           { return false }
func (a *failStartAdapter) BuildResumeCommand(_ spawner.ResumeOptions) *exec.Cmd { return nil }

// hangingAdapter returns a command that runs indefinitely (sleep 999).
// Used to test the timeout/kill path without waiting 40s: caller provides a
// short-lived request context so the handler's derived context expires quickly.
type hangingAdapter struct{ name string }

func (a *hangingAdapter) Name() string                                    { return a.name }
func (a *hangingAdapter) BuildCommand(_ spawner.SpawnOptions) *exec.Cmd   { return exec.Command("sleep", "999") }
func (a *hangingAdapter) MapModel(model string) string                    { return model }
func (a *hangingAdapter) SupportsSessionID() bool                         { return false }
func (a *hangingAdapter) SupportsSystemPromptFile() bool                  { return false }
func (a *hangingAdapter) SupportsResume() bool                            { return false }
func (a *hangingAdapter) UsesStdinPrompt() bool                           { return false }
func (a *hangingAdapter) BuildResumeCommand(_ spawner.ResumeOptions) *exec.Cmd { return nil }

// TestHandleTestCLIModel_TimeoutMessage exercises the timeout code path:
// the response must be success=false with an error containing "40s", and the
// process-group kill must unblock cmd.Wait() so the handler returns promptly.
func TestHandleTestCLIModel_TimeoutMessage(t *testing.T) {
	s := newCLIModelCheckServer(t)
	s.cliAdapterFunc = func(cliType string) (spawner.CLIAdapter, error) {
		return &hangingAdapter{name: cliType}, nil
	}

	// A 100ms parent context expires long before the 40s handler timeout, but
	// gives cmd.Start() enough time to fork sleep(1).  SIGKILL then terminates
	// the process group immediately so cmd.Wait() unblocks and the test stays
	// well under the 5s single-test budget.
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	req := httptest.NewRequest(http.MethodPost, "/api/v1/cli-models/sonnet/test", nil).WithContext(ctx)
	req.SetPathValue("id", "sonnet")
	rr := httptest.NewRecorder()
	s.handleTestCLIModel(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body: %s", rr.Code, rr.Body.String())
	}
	result := decodeCLIModelCheckResult(t, rr)
	if result.Success {
		t.Error("expected success=false for timeout, got success=true")
	}
	if !strings.Contains(result.Error, "40s") {
		t.Errorf("timeout error %q does not mention '40s'", result.Error)
	}
	if result.DurationMs < 0 {
		t.Errorf("duration_ms = %d, want >= 0", result.DurationMs)
	}
}
