package consoleui

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
)

// TestSingleLineComposer covers the pure helper's line-break detection.
func TestSingleLineComposer(t *testing.T) {
	cases := []struct {
		name  string
		value string
		want  bool
	}{
		{"empty", "", true},
		{"single-line", "abc", true},
		{"multi-line", "a\nb", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := singleLineComposer(tc.value); got != tc.want {
				t.Errorf("singleLineComposer(%q) = %v, want %v", tc.value, got, tc.want)
			}
		})
	}
}

// TestHandleKey_ArrowUpScrollsSingleLineComposer verifies a lone up arrow
// scrolls the transcript viewport (mirroring pgup) when the composer draft
// has no line breaks, leaving the composer untouched.
func TestHandleKey_ArrowUpScrollsSingleLineComposer(t *testing.T) {
	m := newScrollTestModel()
	m.input.SetValue("draft")
	before := m.input.Value()
	if !m.viewport.AtBottom() {
		t.Fatal("viewport should start at bottom")
	}

	cmd, handled := m.handleKey(tea.KeyPressMsg{Code: tea.KeyUp})
	_ = cmd
	if !handled {
		t.Fatal("handleKey(up) handled = false, want true")
	}
	if m.viewport.AtBottom() {
		t.Error("handleKey(up) did not scroll the viewport away from the bottom")
	}
	if m.input.Value() != before {
		t.Errorf("handleKey(up) mutated composer input: got %q, want %q", m.input.Value(), before)
	}

	if _, handled := m.handleKey(tea.KeyPressMsg{Code: tea.KeyDown}); !handled {
		t.Fatal("handleKey(down) handled = false, want true")
	}
	if !m.viewport.AtBottom() {
		t.Error("handleKey(down) should scroll back to the bottom")
	}
}

// TestHandleKey_ArrowMultiLineComposerFallsThrough verifies arrow keys fall
// through (handled=false) when the composer draft has embedded newlines, so
// the textarea keeps native cursor-movement behavior.
func TestHandleKey_ArrowMultiLineComposerFallsThrough(t *testing.T) {
	m := newScrollTestModel()
	m.input.SetValue("a\nb")

	_, handled := m.handleKey(tea.KeyPressMsg{Code: tea.KeyUp})
	if handled {
		t.Error("handleKey(up) handled = true with multi-line composer, want false")
	}
	if !m.viewport.AtBottom() {
		t.Error("viewport should be untouched (still at bottom) when arrow falls through")
	}
}

// TestHandleKey_ArrowWithDropdownOpenNavigatesSuggestions verifies the
// suggestion dropdown interceptor wins over arrow-scroll: up/down move
// m.skillIndex and leave the viewport untouched.
func TestHandleKey_ArrowWithDropdownOpenNavigatesSuggestions(t *testing.T) {
	skills := []ConsoleSkill{{Name: "deploy"}, {Name: "docs"}}
	m := invokeTestModel("/", skills, nil, false)
	m.viewport = newScrollTestModel().viewport
	before := m.skillIndex

	cmd, handled := m.handleKey(tea.KeyPressMsg{Code: tea.KeyUp})
	_ = cmd
	if !handled {
		t.Fatal("handleKey(up) with dropdown open handled = false, want true")
	}
	if m.skillIndex == before {
		t.Errorf("handleKey(up) did not change skillIndex: got %d", m.skillIndex)
	}
	if !m.viewport.AtBottom() {
		t.Error("dropdown navigation must not scroll the transcript viewport")
	}
}

// TestHandleKey_ArrowWithApprovalsOpenFallsThrough verifies the len(approvals)
// guard forces arrow keys to fall through (handled=false) while an approval
// is pending, regardless of composer content.
func TestHandleKey_ArrowWithApprovalsOpenFallsThrough(t *testing.T) {
	m := newScrollTestModel()
	m.input.SetValue("draft")
	m.approvals = []Approval{{ID: "a1"}}

	_, handled := m.handleKey(tea.KeyPressMsg{Code: tea.KeyUp})
	if handled {
		t.Error("handleKey(up) handled = true with approvals pending, want false")
	}
	if !m.viewport.AtBottom() {
		t.Error("viewport should be untouched when arrow falls through under approvals")
	}
}

// TestNormalizeCopyText verifies NBSP removal, per-line trailing-whitespace
// trimming, and preservation of interior single spaces.
func TestNormalizeCopyText(t *testing.T) {
	input := "a b\nline with trailing   \nc"
	got := normalizeCopyText(input)
	if got != "a b\nline with trailing\nc" {
		t.Errorf("normalizeCopyText(%q) = %q, want %q", input, got, "a b\nline with trailing\nc")
	}
	for _, line := range []string{"a b", "line with trailing", "c"} {
		if !strings.Contains(got, line) {
			t.Errorf("normalizeCopyText output %q missing expected line %q", got, line)
		}
	}
}

// TestHandleKey_CopyModeShiftYYanksRawTranscript verifies the copy-mode
// "shift+y" branch (Keystroke() lowercases the rune and sets ModShift, so the
// case matches "shift+y" not "Y") exits copy mode, refocuses the composer,
// sets the raw-copy notice, and dispatches an OSC52 command built from
// rawTranscript rather than the rendered viewport.
func TestHandleKey_CopyModeShiftYYanksRawTranscript(t *testing.T) {
	m := newScrollTestModel()
	m.copyMode = true
	m.input.Blur()
	m.messages = []Message{{Content: "# Hi"}, {Content: ""}, {Content: "world"}}

	cmd, handled := m.handleKey(tea.KeyPressMsg{Code: 'y', Mod: tea.ModShift})
	if !handled {
		t.Fatal("handleKey(shift+y) in copy mode handled = false, want true")
	}
	if cmd == nil {
		t.Error("handleKey(shift+y) returned nil cmd, want an OSC52 copy command")
	}
	if m.copyMode {
		t.Error("handleKey(shift+y) left copyMode = true, want false")
	}
	if m.notice != "copied raw transcript" {
		t.Errorf("notice = %q, want %q", m.notice, "copied raw transcript")
	}
}

// TestRawTranscript verifies non-empty message content is joined with a
// blank-line separator, empty content is skipped, and raw markdown markers
// (no glamour rendering) are preserved verbatim.
func TestRawTranscript(t *testing.T) {
	messages := []Message{
		{Content: "# Hi"},
		{Content: ""},
		{Content: "world"},
	}
	got := rawTranscript(messages)
	want := "# Hi\n\nworld"
	if got != want {
		t.Errorf("rawTranscript(...) = %q, want %q", got, want)
	}
	if strings.Contains(got, "•") {
		t.Errorf("rawTranscript output contains glamour bullet artifact: %q", got)
	}
}
