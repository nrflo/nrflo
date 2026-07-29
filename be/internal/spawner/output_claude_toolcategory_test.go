package spawner

import "testing"

// TestToolCategory verifies ToolCategory (which delegates to the canonical
// apirun.ToolCategory, Rule 6) through the CLI-hook entry point. Split out of
// output_claude_test.go to stay under the filesize ratchet.
func TestToolCategory(t *testing.T) {
	t.Parallel()
	tests := []struct {
		toolName string
		want     string
	}{
		{"Task", "subagent"},
		{"Agent", "subagent"},
		{"Skill", "skill"},
		{"Bash", "tool"},
		{"Read", "tool"},
		{"Write", "tool"},
		{"Edit", "tool"},
		{"Glob", "tool"},
		{"Grep", "tool"},
		{"WebFetch", "tool"},
		{"WebSearch", "tool"},
		{"", "tool"},
		{"Unknown", "tool"},
		{"TodoWrite", "tool"},
		// Launcher tools → subagent (spawner.ToolCategory delegates to
		// apirun.ToolCategory, Rule 6 — asserted here through the CLI-hook
		// entry point too).
		{"delegate", "subagent"},
		{"consult", "subagent"},
		{"dynamic_workflow", "subagent"},
		{"run_subworkflow", "subagent"},
		// Poller tools stay "tool".
		{"get_delegation", "tool"},
		{"get_subworkflow", "tool"},
		// MCP bridge prefixes are stripped before matching.
		{"mcp__nrflo__delegate", "subagent"},
		{"nrflo/delegate", "subagent"},
	}

	for _, tt := range tests {
		t.Run(tt.toolName, func(t *testing.T) {
			got := ToolCategory(tt.toolName)
			if got != tt.want {
				t.Errorf("ToolCategory(%q) = %q, want %q", tt.toolName, got, tt.want)
			}
		})
	}
}
