package api

import (
	"testing"
)

// TestAvailableAgentTools_BuiltinsAndMandatory verifies the builtin tools are
// returned with the baseline tools (agent_* lifecycle group plus findings_add)
// flagged mandatory. projectID="" skips the python lookup, so no DB is required.
func TestAvailableAgentTools_BuiltinsAndMandatory(t *testing.T) {
	s := &Server{}
	tools := s.availableAgentTools("")

	byName := make(map[string]availableTool, len(tools))
	for _, tl := range tools {
		byName[tl.Name] = tl
	}

	finished, ok := byName["agent_finished"]
	if !ok {
		t.Fatalf("agent_finished not in available tools")
	}
	if finished.Source != "builtin" {
		t.Errorf("agent_finished source = %q, want builtin", finished.Source)
	}
	if !finished.Mandatory {
		t.Errorf("agent_finished should be mandatory (lifecycle)")
	}

	if add, ok := byName["findings_add"]; !ok {
		t.Errorf("findings_add not in available tools")
	} else if !add.Mandatory {
		t.Errorf("findings_add should be mandatory (baseline)")
	}
}

// TestToolsCSVWarnings verifies warn-only validation: patterns matching no known
// tool warn; "*", exact, and glob matches do not.
func TestToolsCSVWarnings(t *testing.T) {
	available := []availableTool{
		{Name: "findings_add"},
		{Name: "findings_get"},
		{Name: "agent_fail"},
	}

	cases := []struct {
		csv      string
		wantWarn int
	}{
		{"*", 0},
		{"findings_add", 0},
		{"findings_*", 0},
		{"agent_*", 0},
		{"findings.*", 1},  // dotted form matches nothing
		{"bogus_tool", 1},  // unknown exact name
		{"findings_add,nope", 1},
		{"", 0},
	}
	for _, tc := range cases {
		got := toolsCSVWarnings(tc.csv, available)
		if len(got) != tc.wantWarn {
			t.Errorf("toolsCSVWarnings(%q) = %v (%d), want %d warnings", tc.csv, got, len(got), tc.wantWarn)
		}
	}
}
