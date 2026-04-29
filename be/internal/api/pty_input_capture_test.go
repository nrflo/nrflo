package api

import (
	"strings"
	"testing"
)

// TestSanitizeInput covers sanitizeInput: plain text, ANSI stripping, control byte
// conversion, and combined inputs.
func TestSanitizeInput(t *testing.T) {
	cases := []struct {
		name string
		in   []byte
		want string
	}{
		// Plain text
		{"empty", []byte{}, ""},
		{"plain ascii", []byte("hello"), "hello"},
		{"newline preserved", []byte("a\nb"), "a\nb"},
		{"tab preserved", []byte("a\tb"), "a\tb"},
		{"cr preserved", []byte("a\rb"), "a\rb"},
		// ANSI/VT stripping
		{"CSI color", []byte("\x1b[31mred\x1b[0m"), "red"},
		{"CSI cursor up", []byte("\x1b[1A"), ""},
		{"OSC title sequence", []byte("\x1b]0;title\x07"), ""},
		{"2-char ESC", []byte("\x1bM"), ""},
		{"ANSI + text + newline", []byte("\x1b[31mred\x1b[0m\n"), "red\n"},
		{"CSI ? sequence", []byte("\x1b[?25h"), ""},
		// Control byte conversion
		{"ctrl-c", []byte{0x03}, "^C"},
		{"ctrl-d", []byte{0x04}, "^D"},
		{"bell 0x07", []byte{0x07}, "␇"},
		{"del backspace 0x7f", []byte{0x7f}, "^?"},
		{"ctrl-a 0x01", []byte{0x01}, "^A"},
		{"ctrl-z 0x1a", []byte{0x1a}, "^Z"},
		{"ctrl-b 0x02", []byte{0x02}, "^B"},
		// Combined
		{"ctrl-c then newline", []byte{0x03, '\n'}, "^C\n"},
		{"ANSI wrapping ctrl-c", []byte("\x1b[1m\x03\x1b[0m"), "^C"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := sanitizeInput(tc.in)
			if got != tc.want {
				t.Errorf("sanitizeInput(%q) = %q, want %q", tc.in, got, tc.want)
			}
		})
	}
}

// TestSplitLines covers splitLines for various line-ending patterns.
func TestSplitLines(t *testing.T) {
	cases := []struct {
		name    string
		in      []byte
		wantN   int
		wantRem string
		wantLines []string // if non-nil, check each line
	}{
		{"nil input", nil, 0, "", nil},
		{"no newline", []byte("partial"), 0, "partial", nil},
		{"LF only", []byte("hello\n"), 1, "", []string{"hello"}},
		{"CR only", []byte("hello\r"), 1, "", []string{"hello"}},
		{"CRLF counts as one", []byte("a\r\nb\n"), 2, "", []string{"a", "b"}},
		{"LFCR counts as one", []byte("a\n\rb\n"), 2, "", []string{"a", "b"}},
		{"empty line LF", []byte("\n"), 1, "", []string{""}},
		{"empty line CR", []byte("\r"), 1, "", []string{""}},
		{"remainder after split", []byte("line1\nrem"), 1, "rem", []string{"line1"}},
		{"multi-line with remainder", []byte("a\nb\nrem"), 2, "rem", []string{"a", "b"}},
		{"three lines", []byte("x\ny\nz\n"), 3, "", []string{"x", "y", "z"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			lines, rem := splitLines(tc.in)
			if len(lines) != tc.wantN {
				t.Errorf("splitLines(%q): got %d lines, want %d; lines=%v", tc.in, len(lines), tc.wantN, lines)
				return
			}
			if string(rem) != tc.wantRem {
				t.Errorf("splitLines(%q): remainder = %q, want %q", tc.in, rem, tc.wantRem)
			}
			if tc.wantLines != nil {
				for i, want := range tc.wantLines {
					if i >= len(lines) {
						break
					}
					if string(lines[i]) != want {
						t.Errorf("splitLines(%q): lines[%d] = %q, want %q", tc.in, i, lines[i], want)
					}
				}
			}
		})
	}
}

// TestSplitLines_IndependentCopies verifies that returned lines are copies —
// mutating the input buffer does not change the returned slices.
func TestSplitLines_IndependentCopies(t *testing.T) {
	in := []byte("hello\nworld\n")
	lines, _ := splitLines(in)
	if len(lines) != 2 {
		t.Fatalf("expected 2 lines, got %d", len(lines))
	}
	for i := range in {
		in[i] = 'x'
	}
	if string(lines[0]) != "hello" {
		t.Errorf("lines[0] was mutated: got %q", lines[0])
	}
	if string(lines[1]) != "world" {
		t.Errorf("lines[1] was mutated: got %q", lines[1])
	}
}

// TestCapEntry covers the truncation behaviour of capEntry.
func TestCapEntry(t *testing.T) {
	t.Run("short text unchanged", func(t *testing.T) {
		got := capEntry("hello")
		if got != "hello" {
			t.Errorf("capEntry(short) = %q, want hello", got)
		}
	})

	t.Run("empty unchanged", func(t *testing.T) {
		got := capEntry("")
		if got != "" {
			t.Errorf("capEntry('') = %q, want empty", got)
		}
	})

	t.Run("at limit unchanged", func(t *testing.T) {
		text := strings.Repeat("a", maxInputEntryBytes)
		got := capEntry(text)
		if len(got) != maxInputEntryBytes {
			t.Errorf("capEntry(exact limit): len = %d, want %d", len(got), maxInputEntryBytes)
		}
	})

	t.Run("over limit truncated with ellipsis", func(t *testing.T) {
		text := strings.Repeat("b", maxInputEntryBytes+100)
		got := capEntry(text)
		if !strings.HasSuffix(got, "…") {
			t.Errorf("capEntry(over limit) should end with …")
		}
		// First maxInputEntryBytes chars must be preserved.
		if !strings.HasPrefix(got, strings.Repeat("b", maxInputEntryBytes)) {
			t.Errorf("capEntry(over limit) should preserve first %d chars", maxInputEntryBytes)
		}
	})
}

// TestShouldRecord verifies shouldRecord returns false for whitespace-only input
// and true for any input containing visible characters.
func TestShouldRecord(t *testing.T) {
	falsy := []string{"", " ", "\t", "\n", "   \t\n  ", "\r\n"}
	for _, s := range falsy {
		if shouldRecord(s) {
			t.Errorf("shouldRecord(%q) = true, want false", s)
		}
	}

	truthy := []string{"a", "hello", " hi ", "^C", "0", "x y z", "\n hello\n"}
	for _, s := range truthy {
		if !shouldRecord(s) {
			t.Errorf("shouldRecord(%q) = false, want true", s)
		}
	}
}
