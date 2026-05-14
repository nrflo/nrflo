package spawner

import (
	"strings"
	"testing"
)

// TestClaudeAdapter_BuildInteractiveCommand_SystemPromptFile verifies that
// BuildInteractiveCommand emits --append-system-prompt-file <path> when
// SystemPromptFile is set.
func TestClaudeAdapter_BuildInteractiveCommand_SystemPromptFile(t *testing.T) {
	t.Parallel()
	adapter := &ClaudeAdapter{}

	opts := InteractiveSpawnOptions{
		Model:            "sonnet",
		SessionID:        "sess-1",
		WorkDir:          "/tmp",
		SystemPromptFile: "/tmp/nrflo/foo.md",
	}

	args := strings.Join(adapter.BuildInteractiveCommand(opts).Args, " ")

	if !strings.Contains(args, "--append-system-prompt-file") {
		t.Errorf("BuildInteractiveCommand with SystemPromptFile missing --append-system-prompt-file: %s", args)
	}
	if !strings.Contains(args, "/tmp/nrflo/foo.md") {
		t.Errorf("BuildInteractiveCommand args missing SystemPromptFile path: %s", args)
	}
}

// TestClaudeAdapter_BuildInteractiveCommand_NoSystemPromptFile verifies that
// BuildInteractiveCommand does NOT emit --append-system-prompt-file when empty.
func TestClaudeAdapter_BuildInteractiveCommand_NoSystemPromptFile(t *testing.T) {
	t.Parallel()
	adapter := &ClaudeAdapter{}

	opts := InteractiveSpawnOptions{
		Model:     "sonnet",
		SessionID: "sess-1",
		WorkDir:   "/tmp",
	}

	args := strings.Join(adapter.BuildInteractiveCommand(opts).Args, " ")

	if strings.Contains(args, "--append-system-prompt-file") {
		t.Errorf("BuildInteractiveCommand with empty SystemPromptFile should not emit --append-system-prompt-file: %s", args)
	}
}

// TestClaudeAdapter_ResumeWithSystemPromptFile verifies that resuming a session
// via BuildInteractiveCommand (ResumeSessionID set) still includes
// --append-system-prompt-file when SystemPromptFile is non-empty.
func TestClaudeAdapter_ResumeWithSystemPromptFile(t *testing.T) {
	t.Parallel()
	adapter := &ClaudeAdapter{}

	opts := InteractiveSpawnOptions{
		SessionID:        "sess-new",
		ResumeSessionID:  "sess-resume",
		Model:            "sonnet",
		WorkDir:          "/tmp",
		SystemPromptFile: "/tmp/nrflo/foo.md",
	}

	args := strings.Join(adapter.BuildInteractiveCommand(opts).Args, " ")

	if !strings.Contains(args, "--append-system-prompt-file") {
		t.Errorf("BuildInteractiveCommand (resume) with SystemPromptFile missing --append-system-prompt-file: %s", args)
	}
	if !strings.Contains(args, "/tmp/nrflo/foo.md") {
		t.Errorf("BuildInteractiveCommand (resume) args missing SystemPromptFile path: %s", args)
	}
}

// TestClaudeAdapter_SupportsSystemPromptFile verifies ClaudeAdapter capability.
func TestClaudeAdapter_SupportsSystemPromptFile(t *testing.T) {
	t.Parallel()
	adapter := &ClaudeAdapter{}
	if !adapter.SupportsSystemPromptFile() {
		t.Error("ClaudeAdapter.SupportsSystemPromptFile() should return true")
	}
}

// TestOpencodeAdapter_BuildInteractiveCommand_IgnoresSystemPromptFile verifies
// OpencodeAdapter never emits --append-system-prompt-file even when set.
func TestOpencodeAdapter_BuildInteractiveCommand_IgnoresSystemPromptFile(t *testing.T) {
	t.Parallel()
	adapter := &OpencodeAdapter{}

	opts := InteractiveSpawnOptions{
		Model:            "opencode_minimax_m25_free",
		WorkDir:          "/tmp",
		SystemPromptFile: "/tmp/nrflo/foo.md",
	}

	args := strings.Join(adapter.BuildInteractiveCommand(opts).Args, " ")

	if strings.Contains(args, "--append-system-prompt-file") {
		t.Errorf("OpencodeAdapter.BuildInteractiveCommand should not emit --append-system-prompt-file: %s", args)
	}
}

// TestOpencodeAdapter_SupportsSystemPromptFile verifies capability is false.
func TestOpencodeAdapter_SupportsSystemPromptFile(t *testing.T) {
	t.Parallel()
	adapter := &OpencodeAdapter{}
	if adapter.SupportsSystemPromptFile() {
		t.Error("OpencodeAdapter.SupportsSystemPromptFile() should return false")
	}
}

// TestCodexAdapter_BuildInteractiveCommand_IgnoresSystemPromptFile verifies
// CodexAdapter never emits --append-system-prompt-file even when set.
func TestCodexAdapter_BuildInteractiveCommand_IgnoresSystemPromptFile(t *testing.T) {
	t.Parallel()
	adapter := &CodexAdapter{}

	opts := InteractiveSpawnOptions{
		Model:            "codex_gpt_high",
		WorkDir:          "/tmp",
		SystemPromptFile: "/tmp/nrflo/foo.md",
	}

	args := strings.Join(adapter.BuildInteractiveCommand(opts).Args, " ")

	if strings.Contains(args, "--append-system-prompt-file") {
		t.Errorf("CodexAdapter.BuildInteractiveCommand should not emit --append-system-prompt-file: %s", args)
	}
}
