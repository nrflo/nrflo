package consoleui

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
)

// TestHandleKey_PlainArrowFallsThrough verifies plain up/down always fall
// through (handled=false) to composer/dropdown routing, unconditionally
// (no singleLineComposer/approvals guard), leaving the viewport untouched.
func TestHandleKey_PlainArrowFallsThrough(t *testing.T) {
	for _, code := range []rune{tea.KeyUp, tea.KeyDown} {
		m := newScrollTestModel()
		m.input.SetValue("draft")
		before := m.input.Value()
		if !m.viewport.AtBottom() {
			t.Fatal("viewport should start at bottom")
		}

		_, handled := m.handleKey(tea.KeyPressMsg{Code: code})
		if handled {
			t.Errorf("handleKey(%v) handled = true, want false", code)
		}
		if !m.viewport.AtBottom() {
			t.Errorf("handleKey(%v) scrolled the viewport, want untouched", code)
		}
		if m.input.Value() != before {
			t.Errorf("handleKey(%v) mutated composer input: got %q, want %q", code, m.input.Value(), before)
		}
	}
}

// TestHandleKey_ShiftArrowScrollsViewport verifies shift+up/shift+down are
// intercepted by the new case (handled=true, unconditionally, even with a
// multi-line composer draft, proving there is no line-break guard), actually
// scroll the transcript viewport (via direct ScrollUp/ScrollDown calls, since
// viewport.Update's DefaultKeyMap never matches shift-modified arrows), and
// never mutate the composer.
func TestHandleKey_ShiftArrowScrollsViewport(t *testing.T) {
	m := newScrollTestModel()
	m.input.SetValue("a\nb")
	before := m.input.Value()
	if !m.viewport.AtBottom() {
		t.Fatal("viewport should start at bottom")
	}

	cmd, handled := m.handleKey(tea.KeyPressMsg{Code: tea.KeyUp, Mod: tea.ModShift})
	_ = cmd
	if !handled {
		t.Fatal("handleKey(shift+up) handled = false, want true")
	}
	if m.viewport.AtBottom() {
		t.Error("handleKey(shift+up) did not scroll the viewport")
	}
	if m.input.Value() != before {
		t.Errorf("handleKey(shift+up) mutated composer input: got %q, want %q", m.input.Value(), before)
	}

	for !m.viewport.AtBottom() {
		if _, handled := m.handleKey(tea.KeyPressMsg{Code: tea.KeyDown, Mod: tea.ModShift}); !handled {
			t.Fatal("handleKey(shift+down) handled = false, want true")
		}
	}
}

// TestHandleKey_ArrowWithDropdownOpenNavigatesSuggestions verifies the
// suggestion dropdown interceptor wins over plain arrows: up/down move
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

// TestNormalizeCopyText verifies NBSP removal, per-line trailing-whitespace
// trimming, and preservation of interior single spaces.
func TestNormalizeCopyText(t *testing.T) {
	input := "a b\nline with trailing   \nc"
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
