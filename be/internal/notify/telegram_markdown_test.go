package notify

import (
	"strings"
	"testing"
)

func TestEscapeTelegramV2_FullSpecialSet(t *testing.T) {
	cases := []struct {
		name  string
		char  string
		input string
		want  string
	}{
		{"backslash", `\`, `a\b`, `a\\b`},
		{"asterisk", "*", "a*b", `a\*b`},
		{"backtick", "`", "a`b", "a\\`b"},
		{"underscore", "_", "a_b", `a\_b`},
		{"open_bracket", "[", "a[b", `a\[b`},
		{"close_bracket", "]", "a]b", `a\]b`},
		{"open_paren", "(", "a(b", `a\(b`},
		{"close_paren", ")", "a)b", `a\)b`},
		{"tilde", "~", "a~b", `a\~b`},
		{"gt", ">", "a>b", `a\>b`},
		{"hash", "#", "a#b", `a\#b`},
		{"plus", "+", "a+b", `a\+b`},
		{"minus", "-", "a-b", `a\-b`},
		{"equals", "=", "a=b", `a\=b`},
		{"pipe", "|", "a|b", `a\|b`},
		{"open_brace", "{", "a{b", `a\{b`},
		{"close_brace", "}", "a}b", `a\}b`},
		{"dot", ".", "a.b", `a\.b`},
		{"bang", "!", "a!b", `a\!b`},
		{"no_special", "x", "normaltext", "normaltext"},
		{"empty", "", "", ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := escapeTelegramV2(tc.input)
			if got != tc.want {
				t.Errorf("escapeTelegramV2(%q) = %q, want %q", tc.input, got, tc.want)
			}
		})
	}
}

func TestEscapeTelegramV2_BackslashEscapesFirst(t *testing.T) {
	// A literal backslash immediately followed by a special char must not be
	// mistaken for an existing escape sequence: each rune is escaped
	// independently, left to right.
	got := escapeTelegramV2(`\*`)
	want := `\\\*`
	if got != want {
		t.Errorf("escapeTelegramV2(%q) = %q, want %q", `\*`, got, want)
	}
}

func TestStripMarkdownV2_RoundTrip(t *testing.T) {
	cases := []struct {
		name  string
		input string
	}{
		{"plain", "normal text no specials"},
		{"underscore", "my_project_name"},
		{"asterisk_bold_marker", "**Done** with *task*"},
		{"backtick_code", "run `make test` now"},
		{"backslash_trailing", `trailing backslash \`},
		{"mixed_specials", `_~>#+-=|{}.!*` + "`"},
		{"cjk", "完成した タスク 🎉"},
		{"emoji_and_specials", "done! 🚀 *bold* and `code`"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			escaped := escapeTelegramV2(tc.input)
			got := stripMarkdownV2(escaped)
			if got != tc.input {
				t.Errorf("stripMarkdownV2(escapeTelegramV2(%q)) = %q, want original %q", tc.input, got, tc.input)
			}
		})
	}
}

func TestStripMarkdownV2_UnwrapsLink(t *testing.T) {
	got := stripMarkdownV2(`[T\-42](http://localhost:6587/tickets/T-42)`)
	want := `T-42 (http://localhost:6587/tickets/T-42)`
	if got != want {
		t.Errorf("stripMarkdownV2(link) = %q, want %q", got, want)
	}
}

func TestStripMarkdownV2_TrailingLoneBackslashUnchanged(t *testing.T) {
	// A trailing backslash with no following char to escape must not panic
	// or be silently dropped.
	got := stripMarkdownV2(`abc\`)
	want := `abc\`
	if got != want {
		t.Errorf("stripMarkdownV2(trailing backslash) = %q, want %q", got, want)
	}
}

func TestUtf16Len_CJKAndEmoji(t *testing.T) {
	// Sanity check backing the round-trip cases above: CJK is 1 unit/rune,
	// astral emoji is 2 units/rune (surrogate pair).
	if got := utf16Len("完"); got != 1 {
		t.Errorf("utf16Len(CJK) = %d, want 1", got)
	}
	if got := utf16Len("🎉"); got != 2 {
		t.Errorf("utf16Len(emoji) = %d, want 2", got)
	}
}

func TestIsEntityParseError(t *testing.T) {
	cases := []struct {
		name string
		desc string
		want bool
	}{
		{"exact", "Bad Request: can't parse entities: Can't find end of Bold entity at byte offset 42", true},
		{"case_insensitive", "BAD REQUEST: CAN'T PARSE ENTITIES", true},
		{"mixed_case", "Can'T Parse Entities: unexpected token", true},
		{"chat_not_found", "Bad Request: chat not found", false},
		{"flood_control", "Too Many Requests: retry after 30", false},
		{"bot_blocked", "Forbidden: bot was blocked by the user", false},
		{"empty", "", false},
		{"unrelated", "Bad Request: message is too long", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := isEntityParseError(tc.desc)
			if got != tc.want {
				t.Errorf("isEntityParseError(%q) = %v, want %v", tc.desc, got, tc.want)
			}
		})
	}
}

func TestEscapeTelegramV2_FuzzyNastyStrings(t *testing.T) {
	nasty := []string{
		"a\\b*c`d_e[f]g(h)i~j>k#l+m-n=o|p{q}r.s!t",
		strings.Repeat("*", 20),
		strings.Repeat("\\", 20),
		"",
		"\\",
		"*",
		"`",
	}
	for _, s := range nasty {
		escaped := escapeTelegramV2(s)
		got := stripMarkdownV2(escaped)
		if got != s {
			t.Errorf("round-trip failed for %q: escaped=%q stripped=%q", s, escaped, got)
		}
	}
}
