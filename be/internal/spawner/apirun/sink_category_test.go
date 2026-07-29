package apirun

import (
	"encoding/json"
	"strings"
	"testing"
)

// TestToolCategory verifies the canonical ToolCategory (apirun owns it per
// Rule 6; spawner.ToolCategory delegates here).
func TestToolCategory(t *testing.T) {
	cases := []struct {
		name    string
		wantCat string
	}{
		{"Task", "subagent"},
		{"Agent", "subagent"},
		{"Skill", "skill"},
		{"Bash", "tool"},
		{"Grep", "tool"},
		{"findings_add", "tool"},
		{"", "tool"},
		// Launcher tools → subagent.
		{"delegate", "subagent"},
		{"consult", "subagent"},
		{"dynamic_workflow", "subagent"},
		{"run_subworkflow", "subagent"},
		// Poller tools stay "tool" — categorizing them subagent would emit a
		// bogus subagent marker per poll tick.
		{"get_delegation", "tool"},
		{"get_subworkflow", "tool"},
		// MCP bridge prefixes are stripped before matching.
		{"mcp__nrflo__delegate", "subagent"},
		{"nrflo/delegate", "subagent"},
		{"mcp__nrflo__consult", "subagent"},
		{"mcp__nrflo__get_delegation", "tool"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := ToolCategory(tc.name)
			if got != tc.wantCat {
				t.Errorf("ToolCategory(%q) = %q, want %q", tc.name, got, tc.wantCat)
			}
		})
	}
}

// TestRunnerSink_ToolUseStartStop_SubagentAndSkillCategories verifies that Task
// and Agent tools produce "subagent" category and Skill produces "skill".
func TestRunnerSink_ToolUseStartStop_SubagentAndSkillCategories(t *testing.T) {
	cases := []struct {
		toolName string
		wantCat  string
	}{
		{"Task", "subagent"},
		{"Agent", "subagent"},
		{"Skill", "skill"},
	}
	for _, tc := range cases {
		t.Run(tc.toolName, func(t *testing.T) {
			sink := &recordingSink{}
			rs := newRunnerSink(sink, false, nil)
			t.Cleanup(rs.close)

			rs.OnToolUseStart("id-1", tc.toolName)
			rs.OnToolUseStop("id-1", json.RawMessage(`{}`))

			calls := sink.Calls()
			if len(calls) != 1 {
				t.Fatalf("Calls = %d, want 1; got %+v", len(calls), calls)
			}
			if calls[0].category != tc.wantCat {
				t.Errorf("category = %q, want %q", calls[0].category, tc.wantCat)
			}
			if !strings.Contains(calls[0].content, "["+tc.toolName+"]") {
				t.Errorf("content = %q, want [%s] prefix", calls[0].content, tc.toolName)
			}
		})
	}
}
