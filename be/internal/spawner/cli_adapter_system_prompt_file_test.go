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

// ── --system-prompt-file (SystemPromptOverrideFile) tests ─────────────────────

// TestClaudeAdapter_BuildInteractiveCommand_BothFiles verifies that when both
// SystemPromptOverrideFile and SystemPromptFile are set, Claude emits
// --system-prompt-file followed by --append-system-prompt-file.
func TestClaudeAdapter_BuildInteractiveCommand_BothFiles(t *testing.T) {
	t.Parallel()
	adapter := &ClaudeAdapter{}

	opts := InteractiveSpawnOptions{
		Model:                    "sonnet",
		SessionID:                "sess-both",
		WorkDir:                  "/tmp",
		SystemPromptOverrideFile: "/tmp/nrflo/override.md",
		SystemPromptFile:         "/tmp/nrflo/suffix.md",
	}

	cmdArgs := adapter.BuildInteractiveCommand(opts).Args
	argsStr := strings.Join(cmdArgs, " ")

	// Find positions of the flags as elements
	overridePos, appendPos := -1, -1
	for i, a := range cmdArgs {
		if a == "--system-prompt-file" {
			overridePos = i
		}
		if a == "--append-system-prompt-file" {
			appendPos = i
		}
	}
	if overridePos == -1 {
		t.Errorf("BuildInteractiveCommand missing --system-prompt-file: %s", argsStr)
	}
	if appendPos == -1 {
		t.Errorf("BuildInteractiveCommand missing --append-system-prompt-file: %s", argsStr)
	}
	if !strings.Contains(argsStr, "/tmp/nrflo/override.md") {
		t.Errorf("BuildInteractiveCommand args missing override path: %s", argsStr)
	}
	if !strings.Contains(argsStr, "/tmp/nrflo/suffix.md") {
		t.Errorf("BuildInteractiveCommand args missing suffix path: %s", argsStr)
	}
	// Override must precede suffix
	if overridePos >= 0 && appendPos >= 0 && overridePos >= appendPos {
		t.Errorf("--system-prompt-file (pos=%d) should precede --append-system-prompt-file (pos=%d): %v", overridePos, appendPos, cmdArgs)
	}
}

// TestClaudeAdapter_BuildInteractiveCommand_OverrideFileOnly verifies that
// with SystemPromptOverrideFile set but SystemPromptFile empty, only
// --system-prompt-file is emitted (no --append-system-prompt-file).
func TestClaudeAdapter_BuildInteractiveCommand_OverrideFileOnly(t *testing.T) {
	t.Parallel()
	adapter := &ClaudeAdapter{}

	opts := InteractiveSpawnOptions{
		Model:                    "sonnet",
		SessionID:                "sess-override-only",
		WorkDir:                  "/tmp",
		SystemPromptOverrideFile: "/tmp/nrflo/override.md",
	}

	cmdArgs := adapter.BuildInteractiveCommand(opts).Args
	foundOverride, foundAppend := false, false
	for _, a := range cmdArgs {
		if a == "--system-prompt-file" {
			foundOverride = true
		}
		if a == "--append-system-prompt-file" {
			foundAppend = true
		}
	}
	if !foundOverride {
		t.Errorf("BuildInteractiveCommand missing --system-prompt-file: %v", cmdArgs)
	}
	if foundAppend {
		t.Errorf("BuildInteractiveCommand should not emit --append-system-prompt-file when SystemPromptFile is empty: %v", cmdArgs)
	}
}

// TestClaudeAdapter_BuildInteractiveCommand_OverrideFileEmpty verifies that
// with SystemPromptOverrideFile empty, --system-prompt-file is not emitted as a
// standalone flag (--append-system-prompt-file still appears when SystemPromptFile set).
func TestClaudeAdapter_BuildInteractiveCommand_OverrideFileEmpty(t *testing.T) {
	t.Parallel()
	adapter := &ClaudeAdapter{}

	opts := InteractiveSpawnOptions{
		Model:            "sonnet",
		SessionID:        "sess-no-override",
		WorkDir:          "/tmp",
		SystemPromptFile: "/tmp/nrflo/suffix.md",
	}

	cmdArgs := adapter.BuildInteractiveCommand(opts).Args
	for i, a := range cmdArgs {
		if a == "--system-prompt-file" {
			t.Errorf("BuildInteractiveCommand should not emit --system-prompt-file when override is empty, found at index %d; args: %v", i, cmdArgs)
		}
	}

	argsStr := strings.Join(cmdArgs, " ")
	if !strings.Contains(argsStr, "--append-system-prompt-file") {
		t.Errorf("BuildInteractiveCommand should still emit --append-system-prompt-file: %s", argsStr)
	}
}

// TestNonClaudeAdapters_IgnoreSystemPromptOverrideFile verifies that codex
// never emits --system-prompt-file even when SystemPromptOverrideFile is set.
func TestNonClaudeAdapters_IgnoreSystemPromptOverrideFile(t *testing.T) {
	t.Parallel()
	overrideOpts := InteractiveSpawnOptions{
		WorkDir:                  "/tmp",
		SystemPromptOverrideFile: "/tmp/nrflo/override.md",
		SystemPromptFile:         "/tmp/nrflo/suffix.md",
	}
	adapters := []struct {
		name    string
		adapter CLIAdapter
		model   string
	}{
		{"codex", &CodexAdapter{}, "codex_gpt_high"},
	}
	for _, tt := range adapters {
		t.Run(tt.name, func(t *testing.T) {
			opts := overrideOpts
			opts.Model = tt.model
			for i, a := range tt.adapter.BuildInteractiveCommand(opts).Args {
				if a == "--system-prompt-file" {
					t.Errorf("%s should not emit --system-prompt-file at index %d", tt.name, i)
				}
			}
		})
	}
}
