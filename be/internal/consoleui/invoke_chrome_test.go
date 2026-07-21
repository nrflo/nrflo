package consoleui

import (
	"testing"

	"charm.land/bubbles/v2/textarea"
)

// invokeTestModel builds a *model literal (no terminal) with the given
// composer value, skills, tools, and invoke-active flag, sufficient to
// exercise suggestionKind()/suggestionMatches()/suggestionsOpen().
func invokeTestModel(value string, skills []ConsoleSkill, tools []ConsoleTool, invokeActive bool) *model {
	input := textarea.New()
	input.SetValue(value)
	return &model{
		input:  input,
		skills: skills,
		tools:  tools,
		invoke: invokeState{active: invokeActive},
	}
}

// TestSuggestionKind_Dispatch covers the three-way dispatch: "/x" -> skills,
// "/invoke tick" -> tools, and none while an invoke flow is already active.
func TestSuggestionKind_Dispatch(t *testing.T) {
	skills := []ConsoleSkill{{Name: "deploy"}}
	tools := []ConsoleTool{{Name: "ticket_list"}}

	m := invokeTestModel("/de", skills, tools, false)
	if kind := m.suggestionKind(); kind != suggestionKindSkills {
		t.Errorf("suggestionKind(%q) = %v, want suggestionKindSkills", "/de", kind)
	}

	m = invokeTestModel("/invoke tick", skills, tools, false)
	if kind := m.suggestionKind(); kind != suggestionKindTools {
		t.Errorf("suggestionKind(%q) = %v, want suggestionKindTools", "/invoke tick", kind)
	}

	m = invokeTestModel("/invoke tick", skills, tools, true)
	if kind := m.suggestionKind(); kind != suggestionKindNone {
		t.Errorf("suggestionKind while invoke.active = %v, want suggestionKindNone", kind)
	}

	m = invokeTestModel("plain text", skills, tools, false)
	if kind := m.suggestionKind(); kind != suggestionKindNone {
		t.Errorf("suggestionKind(%q) = %v, want suggestionKindNone", "plain text", kind)
	}
}

// TestSuggestionMatches_ToolsMode verifies suggestionMatches sources rows
// from the tool catalogue (name/description) while an "/invoke " query is
// active, applying the same filter semantics as skills.
func TestSuggestionMatches_ToolsMode(t *testing.T) {
	tools := []ConsoleTool{
		{Name: "ticket_list", Description: "list tickets"},
		{Name: "ticket_create", Description: "create a ticket"},
		{Name: "findings_add", Description: "add a finding"},
	}
	m := invokeTestModel("/invoke ticket", nil, tools, false)
	got := m.suggestionMatches()
	want := []string{"ticket_list", "ticket_create"}
	if len(got) != len(want) {
		t.Fatalf("suggestionMatches() = %v, want names %v", got, want)
	}
	for i, item := range got {
		if item.Name != want[i] {
			t.Errorf("suggestionMatches()[%d].Name = %q, want %q", i, item.Name, want[i])
		}
	}
}

// TestSuggestionsOpen_FalseWhileInvokeActive verifies the dropdown closes
// once an invoke flow has taken over the composer, even if a stale directive
// query would otherwise match.
func TestSuggestionsOpen_FalseWhileInvokeActive(t *testing.T) {
	tools := []ConsoleTool{{Name: "ticket_list"}}
	m := invokeTestModel("/invoke ticket", nil, tools, true)
	if m.suggestionsOpen() {
		t.Errorf("suggestionsOpen() = true while invoke.active, want false")
	}
}
