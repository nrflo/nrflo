package spawner

import (
	"strings"
	"testing"

	"be/internal/model"
)

// TestClaudeAdapter_NativeToolsAllowlist verifies a non-empty NativeToolsCSV
// is emitted verbatim as --tools <csv>.
func TestClaudeAdapter_NativeToolsAllowlist(t *testing.T) {
	t.Parallel()
	adapter := &ClaudeAdapter{}

	opts := InteractiveSpawnOptions{
		SessionID:      "sess-native",
		Model:          "claude-sonnet",
		WorkDir:        "/tmp",
		NativeToolsCSV: "Read,Grep",
	}
	cmdArgs := adapter.BuildInteractiveCommand(opts).Args

	pos := findArgElement(cmdArgs, "--tools")
	if pos == -1 {
		t.Fatalf("BuildInteractiveCommand missing --tools: %v", cmdArgs)
	}
	if pos+1 >= len(cmdArgs) || cmdArgs[pos+1] != "Read,Grep" {
		t.Errorf("--tools value = %q, want %q: %v", cmdArgs[pos+1], "Read,Grep", cmdArgs)
	}
}

// TestClaudeAdapter_NativeToolsNoneSentinel verifies the model.NativeToolsNone
// sentinel maps to --tools "" (an empty argv element — the CLI's way of
// disabling every built-in tool), not --tools "none".
func TestClaudeAdapter_NativeToolsNoneSentinel(t *testing.T) {
	t.Parallel()
	adapter := &ClaudeAdapter{}

	opts := InteractiveSpawnOptions{
		SessionID:      "sess-none",
		Model:          "claude-sonnet",
		WorkDir:        "/tmp",
		NativeToolsCSV: model.NativeToolsNone,
	}
	cmdArgs := adapter.BuildInteractiveCommand(opts).Args

	pos := findArgElement(cmdArgs, "--tools")
	if pos == -1 {
		t.Fatalf("BuildInteractiveCommand missing --tools for sentinel: %v", cmdArgs)
	}
	if pos+1 >= len(cmdArgs) || cmdArgs[pos+1] != "" {
		t.Errorf("--tools value = %q, want empty string: %v", cmdArgs[pos+1], cmdArgs)
	}
}

// TestClaudeAdapter_NativeToolsEmptyOmitsFlag verifies an empty NativeToolsCSV
// (unrestricted, the default) emits no --tools flag at all.
func TestClaudeAdapter_NativeToolsEmptyOmitsFlag(t *testing.T) {
	t.Parallel()
	adapter := &ClaudeAdapter{}

	opts := InteractiveSpawnOptions{
		SessionID: "sess-empty",
		Model:     "claude-sonnet",
		WorkDir:   "/tmp",
	}
	cmdArgs := adapter.BuildInteractiveCommand(opts).Args

	if pos := findArgElement(cmdArgs, "--tools"); pos != -1 {
		t.Errorf("BuildInteractiveCommand with empty NativeToolsCSV must not emit --tools: %v", cmdArgs)
	}
}

// TestClaudeAdapter_NativeToolsKeepsDenyPairLast verifies the restriction flag
// coexists with the always-appended --disallowedTools deny pair and that the
// deny pair stays last (variadic-swallow guard).
func TestClaudeAdapter_NativeToolsKeepsDenyPairLast(t *testing.T) {
	t.Parallel()
	adapter := &ClaudeAdapter{}

	opts := InteractiveSpawnOptions{
		SessionID:       "sess-order",
		Model:           "claude-sonnet",
		WorkDir:         "/tmp",
		NativeToolsCSV:  model.NativeToolsNone,
		MCPConfigJSON:   testMCPConfigJSON,
		AllowedToolsCSV: "mcp__nrflo__*",
	}
	cmdArgs := adapter.BuildInteractiveCommand(opts).Args

	denyPos := findArgElement(cmdArgs, "--disallowedTools")
	if denyPos == -1 || denyPos+1 >= len(cmdArgs) {
		t.Fatalf("missing --disallowedTools or its value: %v", cmdArgs)
	}
	if denyPos+2 < len(cmdArgs) && !strings.HasPrefix(cmdArgs[denyPos+2], "-") {
		t.Errorf("--disallowedTools must stay the last flag pair, got trailing %q: %v", cmdArgs[denyPos+2], cmdArgs)
	}
	if toolsPos := findArgElement(cmdArgs, "--tools"); toolsPos == -1 || toolsPos > denyPos {
		t.Errorf("--tools must precede --disallowedTools: %v", cmdArgs)
	}
}
