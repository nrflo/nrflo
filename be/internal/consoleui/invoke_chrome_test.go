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

// TestInvokeChromeRows verifies invokeChromeRows returns 0 when inactive and
// a fixed 3 rows for both the args and confirm phases (content + 2 border
// rows, mirroring approvalBox).
func TestInvokeChromeRows(t *testing.T) {
	m := &model{invoke: invokeState{active: false}}
	if got := m.invokeChromeRows(); got != 0 {
		t.Errorf("invokeChromeRows() inactive = %d, want 0", got)
	}

	m = &model{invoke: invokeState{active: true, phase: invokePhaseArgs, fields: []argField{{Name: "a"}}}}
	if got := m.invokeChromeRows(); got != 3 {
		t.Errorf("invokeChromeRows() args phase = %d, want 3", got)
	}

	m = &model{invoke: invokeState{active: true, phase: invokePhaseConfirm}}
	if got := m.invokeChromeRows(); got != 3 {
		t.Errorf("invokeChromeRows() confirm phase = %d, want 3", got)
	}
}

// TestChromeRows_InvokeRowsAdditive mirrors
// TestChromeRows_ComponentsAdditive: the invokeRows argument adds exactly its
// value on top of the base, independent of the other chrome components.
func TestChromeRows_InvokeRowsAdditive(t *testing.T) {
	base := chromeRows(1, 0, 0, 0, 0)
	withInvoke := chromeRows(1, 0, 0, 0, 3)
	if withInvoke != base+3 {
		t.Errorf("chromeRows with invokeRows=3 = %d, want base(%d)+3 = %d", withInvoke, base, base+3)
	}

	withAll := chromeRows(1, 3, 1, 2, 3)
	withSuggestions := chromeRows(1, 3, 0, 0, 0)
	withApproval := chromeRows(1, 0, 1, 0, 0)
	withDetails := chromeRows(1, 3, 0, 2, 0) - withSuggestions
	wantAll := base + (withSuggestions - base) + (withApproval - base) + withDetails + 3
	if withAll != wantAll {
		t.Errorf("chromeRows(all components) = %d, want additive combination = %d", withAll, wantAll)
	}
}

// TestChromeRows_ViewportHeightNeverBelowOne_WithInvoke sweeps invokeRows
// alongside the existing composer/suggestion/approval sweep, verifying the
// viewport height still floors at 1 and the additive identity holds.
func TestChromeRows_ViewportHeightNeverBelowOne_WithInvoke(t *testing.T) {
	const terminalHeight = 24
	for _, invokeRows := range []int{0, 3} {
		chrome := chromeRows(1, 0, 0, 0, invokeRows)
		viewportHeight := max(1, terminalHeight-chrome)
		if viewportHeight < 1 {
			t.Fatalf("invokeRows=%d: viewport height %d < 1", invokeRows, viewportHeight)
		}
		if viewportHeight+chrome != terminalHeight {
			t.Errorf("invokeRows=%d: viewport(%d)+chrome(%d) = %d, want %d", invokeRows, viewportHeight, chrome, viewportHeight+chrome, terminalHeight)
		}
	}
}
