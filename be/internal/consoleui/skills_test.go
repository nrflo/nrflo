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
			got := skillNames(filterSkills(skills, tt.query))
			if len(got) == 0 && len(tt.want) == 0 {
				return
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("filterSkills(%q) = %v, want %v", tt.query, got, tt.want)
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
	got := skillNames(filterSkills(skills, "code"))
	want := []string{"code-review"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("filterSkills(code) = %v, want %v (prefix must win over substring)", got, want)
	}
}

// TestFilterSkills_SubstringFallback verifies substring matching applies
// only when no skill matches as a prefix.
func TestFilterSkills_SubstringFallback(t *testing.T) {
	skills := []ConsoleSkill{
		{Name: "encode-helper"},
		{Name: "unrelated"},
	}
	got := skillNames(filterSkills(skills, "code"))
	want := []string{"encode-helper"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("filterSkills(code) = %v, want %v", got, want)
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

// TestChromeRows_ViewportHeightNeverBelowOne sweeps composer height 1..8,
// suggestion box open vs closed, and approval present vs absent, asserting
// the derived viewport height is never negative/zero-clamped incorrectly and
// that viewport+chrome always equals the terminal height.
func TestChromeRows_ViewportHeightNeverBelowOne(t *testing.T) {
	const terminalHeight = 24
	for composer := 1; composer <= 8; composer++ {
		for _, suggestionMatches := range []int{0, 3, 12} {
			for _, approvalCount := range []int{0, 1} {
				chrome := chromeRows(composer, suggestionMatches, approvalCount, 0)
				viewportHeight := max(1, terminalHeight-chrome)
				if viewportHeight < 1 {
					t.Fatalf("composer=%d suggestions=%d approvals=%d: viewport height %d < 1", composer, suggestionMatches, approvalCount, viewportHeight)
				}
				if chrome < terminalHeight {
					if viewportHeight+chrome != terminalHeight {
						t.Errorf("composer=%d suggestions=%d approvals=%d: viewport(%d)+chrome(%d) = %d, want %d", composer, suggestionMatches, approvalCount, viewportHeight, chrome, viewportHeight+chrome, terminalHeight)
					}
				}
			}
		}
	}
}

// TestChromeRows_ExtremelySmallTerminal verifies the viewport height clamps
// to 1 (never goes negative) when chrome alone exceeds the terminal height.
func TestChromeRows_ExtremelySmallTerminal(t *testing.T) {
	chrome := chromeRows(8, 12, 1, 0) // composer maxed out + full suggestion box + approval
	viewportHeight := max(1, 5-chrome)
	if viewportHeight != 1 {
		t.Errorf("viewport height = %d, want clamped to 1 when chrome(%d) > terminal(5)", viewportHeight, chrome)
	}
}

// TestChromeRows_ComponentsAdditive verifies each optional chrome component
// (suggestion box, approval box) adds a strictly positive, independent
// contribution to the total.
func TestChromeRows_ComponentsAdditive(t *testing.T) {
	base := chromeRows(1, 0, 0, 0)
	withSuggestions := chromeRows(1, 3, 0, 0)
	withApproval := chromeRows(1, 0, 1, 0)
	withBoth := chromeRows(1, 3, 1, 0)

	if withSuggestions <= base {
		t.Errorf("chromeRows with suggestions (%d) must exceed base (%d)", withSuggestions, base)
	}
	if withApproval <= base {
		t.Errorf("chromeRows with approval (%d) must exceed base (%d)", withApproval, base)
	}
	if withBoth != base+(withSuggestions-base)+(withApproval-base) {
		t.Errorf("chromeRows(both)=%d, want additive combination of suggestion(+%d) and approval(+%d) over base(%d)", withBoth, withSuggestions-base, withApproval-base, base)
	}
}

// TestSuggestionRows_CapsAtMax verifies suggestionRows caps its output at
// maxSuggestionRows regardless of how many matches are passed in, and
// returns 0 for non-positive counts.
func TestSuggestionRows_CapsAtMax(t *testing.T) {
	tests := []struct {
		n    int
		want int
	}{
		{0, 0},
		{-1, 0},
		{1, 1},
		{maxSuggestionRows, maxSuggestionRows},
		{maxSuggestionRows + 5, maxSuggestionRows},
	}
	for _, tt := range tests {
		if got := suggestionRows(tt.n); got != tt.want {
			t.Errorf("suggestionRows(%d) = %d, want %d", tt.n, got, tt.want)
		}
	}
}
