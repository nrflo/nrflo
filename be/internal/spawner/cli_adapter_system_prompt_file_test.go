package spawner

import (
	"strings"
	"testing"
)

// TestClaudeAdapter_BuildCommand_SystemPromptFile verifies that BuildCommand
// emits --append-system-prompt-file <path> exactly when SystemPromptFile != "".
func TestClaudeAdapter_BuildCommand_SystemPromptFile(t *testing.T) {
	adapter := &ClaudeAdapter{}

	opts := SpawnOptions{
		Model:            "sonnet",
		SessionID:        "sess-1",
		WorkDir:          "/tmp",
		SystemPromptFile: "/tmp/nrflo/foo.md",
	}

	cmd := adapter.BuildCommand(opts)
	args := strings.Join(cmd.Args, " ")

	if !strings.Contains(args, "--append-system-prompt-file") {
		t.Errorf("BuildCommand with SystemPromptFile missing --append-system-prompt-file: %s", args)
	}
	if !strings.Contains(args, "/tmp/nrflo/foo.md") {
		t.Errorf("BuildCommand args missing SystemPromptFile path: %s", args)
	}
}

// TestClaudeAdapter_BuildCommand_NoSystemPromptFile verifies that BuildCommand
// does NOT emit --append-system-prompt-file when SystemPromptFile is empty.
func TestClaudeAdapter_BuildCommand_NoSystemPromptFile(t *testing.T) {
	adapter := &ClaudeAdapter{}

	opts := SpawnOptions{
		Model:     "sonnet",
		SessionID: "sess-1",
		WorkDir:   "/tmp",
	}

	cmd := adapter.BuildCommand(opts)
	args := strings.Join(cmd.Args, " ")

	if strings.Contains(args, "--append-system-prompt-file") {
		t.Errorf("BuildCommand with empty SystemPromptFile should not emit --append-system-prompt-file: %s", args)
	}
}

// TestClaudeAdapter_BuildResumeCommand_SystemPromptFile verifies that
// BuildResumeCommand emits --append-system-prompt-file when SystemPromptFile != "".
func TestClaudeAdapter_BuildResumeCommand_SystemPromptFile(t *testing.T) {
	adapter := &ClaudeAdapter{}

	opts := ResumeOptions{
		SessionID:        "sess-resume",
		Prompt:           "Continue",
		WorkDir:          "/tmp",
		SystemPromptFile: "/tmp/nrflo/foo.md",
	}

	cmd := adapter.BuildResumeCommand(opts)
	args := strings.Join(cmd.Args, " ")

	if !strings.Contains(args, "--append-system-prompt-file") {
		t.Errorf("BuildResumeCommand with SystemPromptFile missing --append-system-prompt-file: %s", args)
	}
	if !strings.Contains(args, "/tmp/nrflo/foo.md") {
		t.Errorf("BuildResumeCommand args missing SystemPromptFile path: %s", args)
	}
}

// TestClaudeAdapter_SupportsSystemPromptFile verifies ClaudeAdapter capability.
func TestClaudeAdapter_SupportsSystemPromptFile(t *testing.T) {
	adapter := &ClaudeAdapter{}
	if !adapter.SupportsSystemPromptFile() {
		t.Error("ClaudeAdapter.SupportsSystemPromptFile() should return true")
	}
}

// TestOpencodeAdapter_BuildCommand_IgnoresSystemPromptFile verifies that
// OpencodeAdapter never emits --append-system-prompt-file even when set.
func TestOpencodeAdapter_BuildCommand_IgnoresSystemPromptFile(t *testing.T) {
	adapter := &OpencodeAdapter{}

	opts := SpawnOptions{
		Model:            "sonnet",
		WorkDir:          "/tmp",
		SystemPromptFile: "/tmp/nrflo/foo.md",
	}

	cmd := adapter.BuildCommand(opts)
	args := strings.Join(cmd.Args, " ")

	if strings.Contains(args, "--append-system-prompt-file") {
		t.Errorf("OpencodeAdapter.BuildCommand should not emit --append-system-prompt-file: %s", args)
	}
}

// TestOpencodeAdapter_SupportsSystemPromptFile verifies capability is false.
func TestOpencodeAdapter_SupportsSystemPromptFile(t *testing.T) {
	adapter := &OpencodeAdapter{}
	if adapter.SupportsSystemPromptFile() {
		t.Error("OpencodeAdapter.SupportsSystemPromptFile() should return false")
	}
}

// TestCodexAdapter_BuildCommand_IgnoresSystemPromptFile verifies that
// CodexAdapter never emits --append-system-prompt-file even when set.
func TestCodexAdapter_BuildCommand_IgnoresSystemPromptFile(t *testing.T) {
	adapter := &CodexAdapter{}

	opts := SpawnOptions{
		Model:            "codex_gpt_high",
		WorkDir:          "/tmp",
		SystemPromptFile: "/tmp/nrflo/foo.md",
	}

	cmd := adapter.BuildCommand(opts)
	args := strings.Join(cmd.Args, " ")

	if strings.Contains(args, "--append-system-prompt-file") {
		t.Errorf("CodexAdapter.BuildCommand should not emit --append-system-prompt-file: %s", args)
	}
}
