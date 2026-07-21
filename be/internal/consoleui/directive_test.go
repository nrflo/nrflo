package consoleui

import (
	"reflect"
	"testing"
)

func suggestionNames(items []suggestionItem) []string {
	names := make([]string, len(items))
	for i, it := range items {
		names[i] = it.Name
	}
	return names
}

// TestSkillSuggestions_BareSlashPrependsDirective verifies the bare "/"
// query returns the invoke directive first, followed by project skills in
// original order.
func TestSkillSuggestions_BareSlashPrependsDirective(t *testing.T) {
	skills := []ConsoleSkill{
		{Name: "deploy", Description: "deploy the app"},
		{Name: "debug", Description: "debug tools"},
	}
	got := skillSuggestions(skills, "")
	want := []string{"invoke", "deploy", "debug"}
	if !reflect.DeepEqual(suggestionNames(got), want) {
		t.Fatalf("skillSuggestions(skills, \"\") names = %v, want %v", suggestionNames(got), want)
	}
	if got[0] != invokeDirective {
		t.Errorf("skillSuggestions(skills, \"\")[0] = %+v, want invokeDirective %+v", got[0], invokeDirective)
	}
}

// TestSkillSuggestions_Filter covers matching and non-matching queries
// against the combined [invoke, ...skills] list.
func TestSkillSuggestions_Filter(t *testing.T) {
	skills := []ConsoleSkill{
		{Name: "deploy", Description: "deploy the app"},
		{Name: "debug", Description: "debug tools"},
	}

	tests := []struct {
		name  string
		query string
		want  []string
	}{
		{"inv matches invoke directive", "inv", []string{"invoke"}},
		{"invoke matches invoke directive exactly", "invoke", []string{"invoke"}},
		{"non-matching query drops directive, prefix wins", "de", []string{"deploy", "debug"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := suggestionNames(skillSuggestions(skills, tt.query))
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("skillSuggestions(skills, %q) = %v, want %v", tt.query, got, tt.want)
			}
		})
	}
}

// TestSkillSuggestions_DedupRealSkillNamedInvoke verifies a project skill
// literally named "invoke" (any case) is deduped: exactly one "invoke" row
// appears, and it is the directive (first, with the directive description).
func TestSkillSuggestions_DedupRealSkillNamedInvoke(t *testing.T) {
	skills := []ConsoleSkill{
		{Name: "Invoke", Description: "a real skill also called invoke"},
		{Name: "deploy", Description: "deploy the app"},
	}
	got := skillSuggestions(skills, "")

	count := 0
	for _, it := range got {
		if it.Name == invokeDirectiveName {
			count++
		}
	}
	if count != 1 {
		t.Fatalf("skillSuggestions dedup: got %d rows named %q, want exactly 1", count, invokeDirectiveName)
	}
	if got[0].Name != invokeDirectiveName || got[0].Description != invokeDirective.Description {
		t.Errorf("skillSuggestions dedup: first row = %+v, want the directive row %+v", got[0], invokeDirective)
	}
}

// TestSuggestionsOpen_BareSlashWithNoSkills verifies the directive alone is
// enough to open the dropdown even when the project has zero skills — the
// core discoverability fix.
func TestSuggestionsOpen_BareSlashWithNoSkills(t *testing.T) {
	m := invokeTestModel("/", nil, nil, false)
	if !m.suggestionsOpen() {
		t.Errorf("suggestionsOpen() = false for bare \"/\" with zero skills, want true (directive alone opens dropdown)")
	}
	matches := m.suggestionMatches()
	if len(matches) != 1 || matches[0].Name != invokeDirectiveName {
		t.Errorf("suggestionMatches() = %v, want single invoke directive row", matches)
	}
}

// TestHandleSuggestionKey_DirectiveCompletion verifies selecting the
// directive row (skillIndex 0 on bare "/") completes to "/invoke " via
// enterInvokeDirective, and that a subsequent invoke-mode query resolves to
// suggestionKindTools.
func TestHandleSuggestionKey_DirectiveCompletion(t *testing.T) {
	m := invokeTestModel("/", nil, nil, false)
	m.skillIndex = 0
	if handled := m.handleSuggestionKey("enter"); !handled {
		t.Fatalf("handleSuggestionKey(enter) = false, want true")
	}
	if got := m.input.Value(); got != "/invoke " {
		t.Errorf("input value after directive completion = %q, want %q", got, "/invoke ")
	}
	if kind := m.suggestionKind(); kind != suggestionKindTools {
		t.Errorf("suggestionKind() after \"/invoke \" = %v, want suggestionKindTools", kind)
	}
}

// TestHandleSuggestionKey_RealSkillCompletion verifies selecting a real
// skill row (not the directive) completes to "/<skill> " via completeSkill,
// distinct from the directive's fixed "/invoke " completion.
func TestHandleSuggestionKey_RealSkillCompletion(t *testing.T) {
	skills := []ConsoleSkill{
		{Name: "deploy", Description: "deploy the app"},
		{Name: "debug", Description: "debug tools"},
	}
	m := invokeTestModel("/", skills, nil, false)
	// matches: [invoke, deploy, debug] -> index 1 is "deploy".
	m.skillIndex = 1
	if handled := m.handleSuggestionKey("tab"); !handled {
		t.Fatalf("handleSuggestionKey(tab) = false, want true")
	}
	if got := m.input.Value(); got != "/deploy " {
		t.Errorf("input value after skill completion = %q, want %q", got, "/deploy ")
	}
}
