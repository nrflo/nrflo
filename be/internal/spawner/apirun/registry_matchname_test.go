package apirun

import "testing"

// TestMatchName verifies MatchName semantics: wildcard, prefix glob, exact, and no-match.
func TestMatchName(t *testing.T) {
	cases := []struct {
		pattern string
		name    string
		want    bool
	}{
		// Wildcard: matches everything.
		{"*", "anything", true},
		{"*", "findings_add", true},
		{"*", "", true},

		// Prefix glob: matches names that start with the prefix.
		{"git_*", "git_commit", true},
		{"git_*", "git_push", true},
		{"git_*", "git_", true},
		{"git_*", "commit", false},
		{"git_*", "xgit_commit", false},
		{"git_*", "", false},

		// Exact match: case-sensitive equality.
		{"alpha", "alpha", true},
		{"findings_add", "findings_add", true},
		{"alpha", "beta", false},
		{"alpha", "alpha_extra", false},
		{"alpha", "ALPHA", false},
		{"alpha", "", false},
	}

	for _, tc := range cases {
		got := MatchName(tc.pattern, tc.name)
		if got != tc.want {
			t.Errorf("MatchName(%q, %q) = %v, want %v", tc.pattern, tc.name, got, tc.want)
		}
	}
}
