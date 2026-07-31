package consoleui

import "testing"

func TestCompactPath(t *testing.T) {
	home := "/Users/anderfred"
	cases := []struct {
		name string
		path string
		home string
		want string
	}{
		{"short home path fits under cap", "/Users/anderfred/nrflo/be", home, "~/nrflo/be"},
		{"exact home", "/Users/anderfred", home, "~"},
		{"non-home short path", "/opt/app", home, "/opt/app"},
		{"root", "/", home, "/"},
		{"blank home leaves path untouched", "/Users/anderfred/nrflo", "", "/Users/anderfred/nrflo"},
		{
			"long home path elides middle keeping first and last two segments",
			"/Users/anderfred/projects/2026/some-very-long-project-name-example/deeply/nested/be",
			home,
			"~/projects/…/nested/be",
		},
		{
			"long non-home path elides middle",
			"/Users/foo/bar/baz/qux/quux/corge/grault",
			home,
			"/Users/…/corge/grault",
		},
		{
			"too few segments to elide falls back to rune truncation",
			"~/aVeryLongSingleSegmentNameThatByItselfExceedsTheBudgetEasily",
			home,
			"~/aVeryLongSingleSegmentNam…",
		},
		{
			"non-ascii segments stay rune-safe",
			"/Users/anderfred/projects/日本語のディレクトリ名前がとても長い場合のテストです/deeply/nested/be",
			home,
			"~/projects/…/nested/be",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := compactPath(tc.path, tc.home); got != tc.want {
				t.Errorf("compactPath(%q, %q) = %q, want %q", tc.path, tc.home, got, tc.want)
			}
		})
	}
}

func TestCompactPath_NeverExceedsCap(t *testing.T) {
	home := "/Users/anderfred"
	paths := []string{
		"/Users/anderfred/projects/2026/some-very-long-project-name-example/deeply/nested/be",
		"~/aVeryLongSingleSegmentNameThatByItselfExceedsTheBudgetEasily",
		"/a/b/c/d/e/f/g/h/i/j/k/l/m/n/o/p",
	}
	for _, p := range paths {
		got := compactPath(p, home)
		if n := runeLen(got); n > cwdMaxRunes {
			t.Errorf("compactPath(%q) = %q (%d runes), want <= %d", p, got, n, cwdMaxRunes)
		}
	}
}

func runeLen(s string) int {
	return len([]rune(s))
}
