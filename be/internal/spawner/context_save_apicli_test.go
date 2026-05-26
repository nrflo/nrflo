package spawner

// Tests for the apiViaCLI branch of shouldUseAgentSave. An api-via-cli session
// runs inside a PTY whose backend returns SupportsResume()=true, but we must
// never attempt --resume on a transient api-via-cli PTY process.

import (
	"testing"

	"be/internal/clock"
)

// TestShouldUseAgentSave_APIViaCLI_ForcesAgentSave verifies that
// proc.apiViaCLI=true forces the agent-save path even when the backend
// supports resume (cli_interactive/PTY). Contrast with
// TestShouldUseAgentSave_ClaudeUsesResume which tests the same backend
// but with apiViaCLI=false.
func TestShouldUseAgentSave_APIViaCLI_ForcesAgentSave(t *testing.T) {
	t.Parallel()
	s := New(Config{ContextSaveViaAgent: false, Clock: clock.Real()})
	proc := &processInfo{
		modelID:   "claude:sonnet",
		apiViaCLI: true,
		backend:   fakeBackend{name: "cli_interactive", supportsResume: true},
	}
	if !s.shouldUseAgentSave(proc) {
		t.Error("apiViaCLI=true must force agent save even when backend.SupportsResume()=true")
	}
}

// TestShouldUseAgentSave_APIViaCLI_GlobalSettingAlsoForces verifies that
// the global ContextSaveViaAgent=true flag still forces agent save for
// api-via-cli sessions (it is checked first).
func TestShouldUseAgentSave_APIViaCLI_GlobalSettingAlsoForces(t *testing.T) {
	t.Parallel()
	s := New(Config{ContextSaveViaAgent: true, Clock: clock.Real()})
	proc := &processInfo{
		modelID:   "claude:sonnet",
		apiViaCLI: true,
		backend:   fakeBackend{name: "cli_interactive", supportsResume: true},
	}
	if !s.shouldUseAgentSave(proc) {
		t.Error("global ContextSaveViaAgent=true must force agent save")
	}
}
