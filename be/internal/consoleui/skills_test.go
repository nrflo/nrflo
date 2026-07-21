package consoleui

import (
	"reflect"
	"testing"
)

func skillNames(skills []ConsoleSkill) []string {
	names := make([]string, len(skills))
	for i, s := range skills {
		names[i] = s.Name
	}
	return names
}

func TestFilterSkills(t *testing.T) {
	skills := []ConsoleSkill{
		{Name: "deploy", Description: "deploy the app"},
		{Name: "debug", Description: "debug tools"},
		{Name: "codebase-exploration", Description: "explore code"},
	}

	tests := []struct {
		name  string
		query string
		want  []string
	}{
		{"empty query returns all", "", []string{"deploy", "debug", "codebase-exploration"}},
		{"exact match", "deploy", []string{"deploy"}},
		{"prefix match returns only prefix rows", "de", []string{"deploy", "debug"}},
		{"case-insensitive", "DE", []string{"deploy", "debug"}},
		{"no match returns empty", "zzz", []string{}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := skillNames(filterByName(skills, tt.query, func(s ConsoleSkill) string { return s.Name }))
			if len(got) == 0 && len(tt.want) == 0 {
				return
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("filterByName(%q) = %v, want %v", tt.query, got, tt.want)
			}
		})
	}
}

// TestFilterSkills_PrefixBeatsSubstring verifies that when both a prefix
// match and a substring-only match exist, only the prefix matches are
// returned (substring rows are excluded, not appended).
func TestFilterSkills_PrefixBeatsSubstring(t *testing.T) {
	skills := []ConsoleSkill{
		{Name: "encode-helper"}, // substring match only: "code" appears mid-name
		{Name: "code-review"},   // prefix match: starts with "code"
	}
	got := skillNames(filterByName(skills, "code", func(s ConsoleSkill) string { return s.Name }))
	want := []string{"code-review"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("filterByName(code) = %v, want %v (prefix must win over substring)", got, want)
	}
}

// TestFilterSkills_SubstringFallback verifies substring matching applies
// only when no skill matches as a prefix.
func TestFilterSkills_SubstringFallback(t *testing.T) {
	skills := []ConsoleSkill{
		{Name: "encode-helper"},
		{Name: "unrelated"},
	}
	got := skillNames(filterByName(skills, "code", func(s ConsoleSkill) string { return s.Name }))
	want := []string{"encode-helper"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("filterByName(code) = %v, want %v", got, want)
	}
}

func TestSlashQuery(t *testing.T) {
	tests := []struct {
		value     string
		wantQuery string
		wantOK    bool
	}{
		{"/", "", true},
		{"/de", "de", true},
		{"/de foo", "", false},
		{"/de\nx", "", false},
		{"hi", "", false},
		{"", "", false},
	}
	for _, tt := range tests {
		t.Run(tt.value, func(t *testing.T) {
			query, ok := slashQuery(tt.value)
			if ok != tt.wantOK || query != tt.wantQuery {
				t.Errorf("slashQuery(%q) = (%q, %v), want (%q, %v)", tt.value, query, ok, tt.wantQuery, tt.wantOK)
			}
		})
	}
}
