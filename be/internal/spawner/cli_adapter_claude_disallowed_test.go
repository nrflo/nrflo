package spawner

import (
	"strings"
	"testing"
)

// TestClaudeAdapter_DisallowsNativeOrchestration verifies --disallowedTools
// is emitted with the exact deny value, as discrete adjacent argv elements.
func TestClaudeAdapter_DisallowsNativeOrchestration(t *testing.T) {
	t.Parallel()
	adapter := &ClaudeAdapter{}

	opts := InteractiveSpawnOptions{
		SessionID: "sess-deny",
		Model:     "claude-sonnet",
		WorkDir:   "/tmp",
	}
	cmdArgs := adapter.BuildInteractiveCommand(opts).Args

	pos := findArgElement(cmdArgs, "--disallowedTools")
	if pos == -1 {
		t.Fatalf("BuildInteractiveCommand missing --disallowedTools: %v", cmdArgs)
	}
	if pos+1 >= len(cmdArgs) || cmdArgs[pos+1] != "Agent Task Workflow SendMessage" {
		t.Errorf("--disallowedTools value = %q, want %q: %v", cmdArgs[pos+1], "Agent Task Workflow SendMessage", cmdArgs)
	}
}

// TestClaudeAdapter_DisallowedToolsAlwaysPresent is the real regression guard:
// it proves no managed spawn path (cli_interactive, api-via-cli, observer,
// context-save-resume) can produce a claude argv without the deny, since all
// of them build InteractiveSpawnOptions and none can leave it more zero-valued
// than SessionID/Model alone.
func TestClaudeAdapter_DisallowedToolsAlwaysPresent(t *testing.T) {
	t.Parallel()
	adapter := &ClaudeAdapter{}

	opts := InteractiveSpawnOptions{
		SessionID: "sess-zero",
		Model:     "claude-sonnet",
	}
	cmdArgs := adapter.BuildInteractiveCommand(opts).Args

	pos := findArgElement(cmdArgs, "--disallowedTools")
	if pos == -1 {
		t.Fatalf("BuildInteractiveCommand with zero-valued opts missing --disallowedTools: %v", cmdArgs)
	}
	if pos+1 >= len(cmdArgs) || cmdArgs[pos+1] != claudeDisallowedNativeTools {
		t.Errorf("--disallowedTools value = %q, want %q: %v", cmdArgs[pos+1], claudeDisallowedNativeTools, cmdArgs)
	}
}

// TestClaudeAdapter_DisallowedToolsDoesNotDenyNrfloOrCodingTools asserts the
// deny value contains neither mcp__nrflo__ nor any ordinary coding tool, and
// that --allowedTools mcp__nrflo__* still appears alongside the deny when
// AllowedToolsCSV is set — i.e. no behavior change for single-agent flows.
func TestClaudeAdapter_DisallowedToolsDoesNotDenyNrfloOrCodingTools(t *testing.T) {
	t.Parallel()
	adapter := &ClaudeAdapter{}

	opts := InteractiveSpawnOptions{
		SessionID:       "sess-allowed",
		Model:           "claude-sonnet",
		WorkDir:         "/tmp",
		AllowedToolsCSV: "mcp__nrflo__*",
	}
	cmdArgs := adapter.BuildInteractiveCommand(opts).Args

	denyPos := findArgElement(cmdArgs, "--disallowedTools")
	if denyPos == -1 || denyPos+1 >= len(cmdArgs) {
		t.Fatalf("BuildInteractiveCommand missing --disallowedTools: %v", cmdArgs)
	}
	denyValue := cmdArgs[denyPos+1]

	if strings.Contains(denyValue, "mcp__nrflo__") {
		t.Errorf("deny value must not contain mcp__nrflo__: %q", denyValue)
	}
	for _, codingTool := range []string{"Bash", "Edit", "Read", "Write", "Grep", "Glob"} {
		for _, denied := range strings.Fields(denyValue) {
			if denied == codingTool {
				t.Errorf("deny value must not deny coding tool %q: %q", codingTool, denyValue)
			}
		}
	}

	allowedPos := findArgElement(cmdArgs, "--allowedTools")
	if allowedPos == -1 {
		t.Fatalf("BuildInteractiveCommand with AllowedToolsCSV missing --allowedTools: %v", cmdArgs)
	}
	if allowedPos+1 >= len(cmdArgs) || cmdArgs[allowedPos+1] != "mcp__nrflo__*" {
		t.Errorf("--allowedTools value = %q, want %q: %v", cmdArgs[allowedPos+1], "mcp__nrflo__*", cmdArgs)
	}
}

// TestClaudeAdapter_DisallowedToolsNotFollowedByPositional guards the
// variadic-swallow gotcha: `--disallowedTools <tools...>` is variadic and
// greedily consumes following POSITIONAL argv elements until the next
// `-`-prefixed flag. Claude never receives a positional prompt today
// (DeliversPromptInline() is false), but this test protects against a future
// contributor adding one after the deny pair.
func TestClaudeAdapter_DisallowedToolsNotFollowedByPositional(t *testing.T) {
	t.Parallel()
	adapter := &ClaudeAdapter{}

	opts := InteractiveSpawnOptions{
		SessionID:       "sess-tail",
		Model:           "claude-sonnet",
		WorkDir:         "/tmp",
		NativeToolsCSV:  "Read",
		MCPConfigJSON:   testMCPConfigJSON,
		AllowedToolsCSV: "mcp__nrflo__*",
	}
	cmdArgs := adapter.BuildInteractiveCommand(opts).Args

	pos := findArgElement(cmdArgs, "--disallowedTools")
	if pos == -1 || pos+1 >= len(cmdArgs) {
		t.Fatalf("BuildInteractiveCommand missing --disallowedTools or its value: %v", cmdArgs)
	}

	// Nothing may follow the deny value, or the next element must itself be a flag.
	if pos+2 < len(cmdArgs) {
		next := cmdArgs[pos+2]
		if !strings.HasPrefix(next, "-") {
			t.Errorf("element after --disallowedTools value must be absent or start with '-' (would be swallowed by the variadic flag), got %q: %v", next, cmdArgs)
		}
	}
}
