package spawner

import (
	"strings"
	"testing"
)

const testMCPConfigJSON = `{"mcpServers":{"nrflo":{"command":"nrflo","args":["agent","mcp"]}}}`

// findArgElement returns the index of the first exact-match element in args, or -1.
func findArgElement(args []string, s string) int {
	for i, a := range args {
		if a == s {
			return i
		}
	}
	return -1
}

// TestClaudeAdapter_BuildInteractiveCommand_AllMCPToolFlags verifies that when
// NativeToolsCSV, MCPConfigJSON, and AllowedToolsCSV are all set the adapter
// emits --tools <csv>, --mcp-config <json>, --strict-mcp-config, and
// --allowedTools <csv> as discrete argv elements.
func TestClaudeAdapter_BuildInteractiveCommand_AllMCPToolFlags(t *testing.T) {
	t.Parallel()
	adapter := &ClaudeAdapter{}

	opts := InteractiveSpawnOptions{
		SessionID:       "sess-mcp-all",
		Model:           "claude-sonnet",
		WorkDir:         "/tmp",
		NativeToolsCSV:  "Read",
		MCPConfigJSON:   testMCPConfigJSON,
		AllowedToolsCSV: "mcp__nrflo__* Read",
	}

	cmdArgs := adapter.BuildInteractiveCommand(opts).Args
	argsStr := strings.Join(cmdArgs, " ")

	// --tools followed by the value as adjacent elements
	toolsPos := findArgElement(cmdArgs, "--tools")
	if toolsPos == -1 {
		t.Fatalf("BuildInteractiveCommand missing --tools: %s", argsStr)
	}
	if toolsPos+1 >= len(cmdArgs) || cmdArgs[toolsPos+1] != "Read" {
		t.Errorf("--tools value should be %q, got %q: %v", "Read", cmdArgs[toolsPos+1], cmdArgs)
	}

	// --mcp-config followed by the JSON as a single element
	mcpPos := findArgElement(cmdArgs, "--mcp-config")
	if mcpPos == -1 {
		t.Fatalf("BuildInteractiveCommand missing --mcp-config: %s", argsStr)
	}
	if mcpPos+1 >= len(cmdArgs) || cmdArgs[mcpPos+1] != testMCPConfigJSON {
		t.Errorf("--mcp-config value = %q, want %q", cmdArgs[mcpPos+1], testMCPConfigJSON)
	}

	// --strict-mcp-config is present
	if findArgElement(cmdArgs, "--strict-mcp-config") == -1 {
		t.Errorf("BuildInteractiveCommand missing --strict-mcp-config: %s", argsStr)
	}

	// --allowedTools followed by the value as a single element (value contains space)
	allowedPos := findArgElement(cmdArgs, "--allowedTools")
	if allowedPos == -1 {
		t.Fatalf("BuildInteractiveCommand missing --allowedTools: %s", argsStr)
	}
	if allowedPos+1 >= len(cmdArgs) || cmdArgs[allowedPos+1] != "mcp__nrflo__* Read" {
		t.Errorf("--allowedTools value = %q, want %q", cmdArgs[allowedPos+1], "mcp__nrflo__* Read")
	}
}

// TestClaudeAdapter_BuildInteractiveCommand_AllMCPToolFlagsAbsent is a regression
// guard: when all three fields are empty the command is byte-identical to the
// baseline (no --tools, --mcp-config, --strict-mcp-config, or --allowedTools).
func TestClaudeAdapter_BuildInteractiveCommand_AllMCPToolFlagsAbsent(t *testing.T) {
	t.Parallel()
	adapter := &ClaudeAdapter{}

	opts := InteractiveSpawnOptions{
		SessionID: "sess-baseline",
		Model:     "claude-sonnet",
		WorkDir:   "/tmp",
		// NativeToolsCSV, MCPConfigJSON, AllowedToolsCSV all empty
	}

	cmdArgs := adapter.BuildInteractiveCommand(opts).Args
	argsStr := strings.Join(cmdArgs, " ")

	for _, forbidden := range []string{"--tools", "--mcp-config", "--strict-mcp-config", "--allowedTools"} {
		if findArgElement(cmdArgs, forbidden) != -1 {
			t.Errorf("BuildInteractiveCommand with empty MCP/tool fields must not emit %q: %s", forbidden, argsStr)
		}
	}
}

// TestClaudeAdapter_BuildInteractiveCommand_MCPConfigAlone verifies that when
// only MCPConfigJSON is set, both --mcp-config and --strict-mcp-config appear
// but --tools and --allowedTools do not.
func TestClaudeAdapter_BuildInteractiveCommand_MCPConfigAlone(t *testing.T) {
	t.Parallel()
	adapter := &ClaudeAdapter{}

	opts := InteractiveSpawnOptions{
		SessionID:     "sess-mcp-only",
		Model:         "claude-sonnet",
		WorkDir:       "/tmp",
		MCPConfigJSON: testMCPConfigJSON,
	}

	cmdArgs := adapter.BuildInteractiveCommand(opts).Args
	argsStr := strings.Join(cmdArgs, " ")

	if findArgElement(cmdArgs, "--mcp-config") == -1 {
		t.Errorf("BuildInteractiveCommand with MCPConfigJSON missing --mcp-config: %s", argsStr)
	}
	if findArgElement(cmdArgs, "--strict-mcp-config") == -1 {
		t.Errorf("BuildInteractiveCommand with MCPConfigJSON missing --strict-mcp-config: %s", argsStr)
	}
	if findArgElement(cmdArgs, "--tools") != -1 {
		t.Errorf("BuildInteractiveCommand with MCPConfigJSON alone must not emit --tools: %s", argsStr)
	}
	if findArgElement(cmdArgs, "--allowedTools") != -1 {
		t.Errorf("BuildInteractiveCommand with MCPConfigJSON alone must not emit --allowedTools: %s", argsStr)
	}
}

// TestClaudeAdapter_BuildInteractiveCommand_NativeToolsCSVAlone verifies that
// only --tools is emitted (no --mcp-config/--strict-mcp-config/--allowedTools).
func TestClaudeAdapter_BuildInteractiveCommand_NativeToolsCSVAlone(t *testing.T) {
	t.Parallel()
	adapter := &ClaudeAdapter{}

	opts := InteractiveSpawnOptions{
		SessionID:      "sess-tools-only",
		Model:          "claude-sonnet",
		WorkDir:        "/tmp",
		NativeToolsCSV: "Read,Write,Edit",
	}

	cmdArgs := adapter.BuildInteractiveCommand(opts).Args
	argsStr := strings.Join(cmdArgs, " ")

	toolsPos := findArgElement(cmdArgs, "--tools")
	if toolsPos == -1 {
		t.Fatalf("BuildInteractiveCommand with NativeToolsCSV missing --tools: %s", argsStr)
	}
	if toolsPos+1 >= len(cmdArgs) || cmdArgs[toolsPos+1] != "Read,Write,Edit" {
		t.Errorf("--tools value = %q, want %q", cmdArgs[toolsPos+1], "Read,Write,Edit")
	}
	for _, absent := range []string{"--mcp-config", "--strict-mcp-config", "--allowedTools"} {
		if findArgElement(cmdArgs, absent) != -1 {
			t.Errorf("BuildInteractiveCommand with NativeToolsCSV alone must not emit %q: %s", absent, argsStr)
		}
	}
}

// TestClaudeAdapter_BuildInteractiveCommand_AllowedToolsCSVAlone verifies that
// only --allowedTools is emitted (no --tools/--mcp-config/--strict-mcp-config).
func TestClaudeAdapter_BuildInteractiveCommand_AllowedToolsCSVAlone(t *testing.T) {
	t.Parallel()
	adapter := &ClaudeAdapter{}

	opts := InteractiveSpawnOptions{
		SessionID:       "sess-allowed-only",
		Model:           "claude-sonnet",
		WorkDir:         "/tmp",
		AllowedToolsCSV: "mcp__nrflo__*",
	}

	cmdArgs := adapter.BuildInteractiveCommand(opts).Args
	argsStr := strings.Join(cmdArgs, " ")

	allowedPos := findArgElement(cmdArgs, "--allowedTools")
	if allowedPos == -1 {
		t.Fatalf("BuildInteractiveCommand with AllowedToolsCSV missing --allowedTools: %s", argsStr)
	}
	if allowedPos+1 >= len(cmdArgs) || cmdArgs[allowedPos+1] != "mcp__nrflo__*" {
		t.Errorf("--allowedTools value = %q, want %q", cmdArgs[allowedPos+1], "mcp__nrflo__*")
	}
	for _, absent := range []string{"--tools", "--mcp-config", "--strict-mcp-config"} {
		if findArgElement(cmdArgs, absent) != -1 {
			t.Errorf("BuildInteractiveCommand with AllowedToolsCSV alone must not emit %q: %s", absent, argsStr)
		}
	}
}

// TestClaudeAdapter_BuildInteractiveCommand_AllowedToolsValueWithSpace verifies
// that an AllowedToolsCSV value containing a space (e.g. "mcp__nrflo__* Read")
// is passed as a single argv element, not split.
func TestClaudeAdapter_BuildInteractiveCommand_AllowedToolsValueWithSpace(t *testing.T) {
	t.Parallel()
	adapter := &ClaudeAdapter{}

	wantValue := "mcp__nrflo__* Read"
	opts := InteractiveSpawnOptions{
		SessionID:       "sess-allowed-space",
		Model:           "claude-sonnet",
		WorkDir:         "/tmp",
		AllowedToolsCSV: wantValue,
	}

	cmdArgs := adapter.BuildInteractiveCommand(opts).Args
	allowedPos := findArgElement(cmdArgs, "--allowedTools")
	if allowedPos == -1 {
		t.Fatalf("BuildInteractiveCommand missing --allowedTools: %v", cmdArgs)
	}
	if allowedPos+1 >= len(cmdArgs) {
		t.Fatalf("--allowedTools has no following element: %v", cmdArgs)
	}
	got := cmdArgs[allowedPos+1]
	if got != wantValue {
		t.Errorf("--allowedTools value = %q, want %q (must be single argv element, not split)", got, wantValue)
	}
}

// TestClaudeAdapter_BuildInteractiveCommand_StrictMCPConfigAfterValue verifies
// that --strict-mcp-config immediately follows the --mcp-config value.
func TestClaudeAdapter_BuildInteractiveCommand_StrictMCPConfigAfterValue(t *testing.T) {
	t.Parallel()
	adapter := &ClaudeAdapter{}

	opts := InteractiveSpawnOptions{
		SessionID:     "sess-mcp-order",
		Model:         "claude-sonnet",
		WorkDir:       "/tmp",
		MCPConfigJSON: testMCPConfigJSON,
	}

	cmdArgs := adapter.BuildInteractiveCommand(opts).Args

	mcpPos := findArgElement(cmdArgs, "--mcp-config")
	strictPos := findArgElement(cmdArgs, "--strict-mcp-config")
	if mcpPos == -1 || strictPos == -1 {
		t.Fatalf("missing --mcp-config (pos=%d) or --strict-mcp-config (pos=%d): %v", mcpPos, strictPos, cmdArgs)
	}
	// --mcp-config <value> --strict-mcp-config means strict is at mcpPos+2
	if strictPos != mcpPos+2 {
		t.Errorf("--strict-mcp-config should be at index %d (mcpPos+2), got %d: %v", mcpPos+2, strictPos, cmdArgs)
	}
}

// TestCodexAdapter_BuildInteractiveCommand_IgnoresMCPToolFields verifies that
// CodexAdapter never emits --tools/--mcp-config/--strict-mcp-config/--allowedTools
// even when the fields are set, keeping adapter divergence in the implementation.
func TestCodexAdapter_BuildInteractiveCommand_IgnoresMCPToolFields(t *testing.T) {
	t.Parallel()
	adapter := &CodexAdapter{}

	opts := InteractiveSpawnOptions{
		Model:           "gpt-5.3-codex",
		WorkDir:         "/tmp",
		NativeToolsCSV:  "Read",
		MCPConfigJSON:   testMCPConfigJSON,
		AllowedToolsCSV: "mcp__nrflo__* Read",
	}

	cmdArgs := adapter.BuildInteractiveCommand(opts).Args
	argsStr := strings.Join(cmdArgs, " ")

	for _, absent := range []string{"--tools", "--mcp-config", "--strict-mcp-config", "--allowedTools"} {
		if findArgElement(cmdArgs, absent) != -1 {
			t.Errorf("CodexAdapter must not emit %q: %s", absent, argsStr)
		}
	}
}
