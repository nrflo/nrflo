package consoleui

import (
	"strings"
	"testing"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
)

// TestWrapToWidth_ShortUnchanged verifies text already within width passes
// through unchanged.
func TestWrapToWidth_ShortUnchanged(t *testing.T) {
	got := wrapToWidth("hello world", 40)
	if got != "hello world" {
		t.Errorf("wrapToWidth(short, 40) = %q, want unchanged %q", got, "hello world")
	}
}

// TestWrapToWidth_LongSentence verifies a long sentence wraps to lines each
// within width, without splitting words mid-word where avoidable.
func TestWrapToWidth_LongSentence(t *testing.T) {
	const width = 20
	sentence := strings.Repeat("the quick brown fox jumps over the lazy dog ", 4)
	got := wrapToWidth(sentence, width)
	for i, line := range strings.Split(got, "\n") {
		if w := lipgloss.Width(ansi.Strip(line)); w > width {
			t.Errorf("line %d width = %d, want <= %d (line %q)", i, w, width, line)
		}
	}
	// Words present in the input (space-separated tokens) should still be
	// present as whole tokens somewhere in the output, i.e. no mid-word
	// splitting introduced spurious partial tokens for words shorter than
	// width.
	for _, word := range strings.Fields(sentence) {
		if !strings.Contains(got, word) {
			t.Errorf("wrapped output missing whole word %q", word)
		}
	}
}

// TestWrapToWidth_LongToken verifies a single unbroken long token is hard
// wrapped: every line <= width and no runes are lost.
func TestWrapToWidth_LongToken(t *testing.T) {
	const width = 20
	token := strings.Repeat("x", 5000)
	got := wrapToWidth(token, width)
	totalRunes := 0
	for i, line := range strings.Split(got, "\n") {
		stripped := ansi.Strip(line)
		if w := lipgloss.Width(stripped); w > width {
			t.Errorf("line %d width = %d, want <= %d", i, w, width)
		}
		totalRunes += len([]rune(stripped))
	}
	if totalRunes != 5000 {
		t.Errorf("total non-newline runes = %d, want 5000", totalRunes)
	}
}

// TestWrapToWidth_PreservesNewlines verifies embedded newlines are preserved
// (output has at least as many lines as the input).
func TestWrapToWidth_PreservesNewlines(t *testing.T) {
	input := "line one\nline two\nline three"
	got := wrapToWidth(input, 40)
	wantMinLines := strings.Count(input, "\n") + 1
	gotLines := strings.Count(got, "\n") + 1
	if gotLines < wantMinLines {
		t.Errorf("wrapToWidth line count = %d, want >= %d (input line count)", gotLines, wantMinLines)
	}
}

// TestWrapToWidth_ZeroOrNegativeWidth verifies no panic and the input text
// content is preserved (modulo wrapping) at width 0/negative.
func TestWrapToWidth_ZeroOrNegativeWidth(t *testing.T) {
	for _, width := range []int{0, -1, -100} {
		func() {
			defer func() {
				if r := recover(); r != nil {
					t.Fatalf("wrapToWidth(s, %d) panicked: %v", width, r)
				}
			}()
			got := wrapToWidth("hello world", width)
			if got == "" {
				t.Errorf("wrapToWidth(s, %d) = empty, want non-empty", width)
			}
		}()
	}
}

func TestRenderMessage_ToolLongJSONWrapped(t *testing.T) {
	const width = 30
	msg := Message{Category: "tool", Content: "t → " + strings.Repeat("x", 5000)}
	out := renderMessage(msg, width)
	for i, line := range strings.Split(out, "\n") {
		stripped := ansi.Strip(line)
		if w := lipgloss.Width(stripped); w > width {
			t.Errorf("renderMessage tool line %d width = %d, want <= %d", i, w, width)
		}
	}
}
