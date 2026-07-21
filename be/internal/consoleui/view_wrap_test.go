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

// TestPrettyToolContent_JSONObject verifies an object right-hand side is
// pretty-printed while the left side and delimiter are preserved.
func TestPrettyToolContent_JSONObject(t *testing.T) {
	got := prettyToolContent(`foo → {"a":1,"b":2}`)
	if !strings.HasPrefix(got, "foo → ") {
		t.Fatalf("prettyToolContent object result missing left+delimiter prefix: %q", got)
	}
	if !strings.Contains(got, "\n") {
		t.Errorf("prettyToolContent object result should be multi-line indented JSON: %q", got)
	}
	if !strings.Contains(got, `"a": 1`) || !strings.Contains(got, `"b": 2`) {
		t.Errorf("prettyToolContent object result missing indented fields: %q", got)
	}
}

// TestPrettyToolContent_JSONArray verifies an array right-hand side is
// pretty-printed (indented).
func TestPrettyToolContent_JSONArray(t *testing.T) {
	got := prettyToolContent("foo → [1,2,3]")
	if !strings.HasPrefix(got, "foo → ") {
		t.Fatalf("prettyToolContent array result missing left+delimiter prefix: %q", got)
	}
	if !strings.Contains(got, "\n") {
		t.Errorf("prettyToolContent array result should be multi-line indented JSON: %q", got)
	}
	for _, want := range []string{"1", "2", "3"} {
		if !strings.Contains(got, want) {
			t.Errorf("prettyToolContent array result missing element %q: %q", want, got)
		}
	}
}

// TestPrettyToolContent_NonJSONRightSide verifies a non-JSON right side with
// the delimiter present passes through unchanged.
func TestPrettyToolContent_NonJSONRightSide(t *testing.T) {
	input := "foo → done"
	if got := prettyToolContent(input); got != input {
		t.Errorf("prettyToolContent(%q) = %q, want unchanged", input, got)
	}
}

// TestPrettyToolContent_NoDelimiter verifies content without the " → "
// delimiter passes through unchanged.
func TestPrettyToolContent_NoDelimiter(t *testing.T) {
	input := "plain text no delimiter"
	if got := prettyToolContent(input); got != input {
		t.Errorf("prettyToolContent(%q) = %q, want unchanged", input, got)
	}
}

// TestRenderMessage_ToolLongJSONWrapped is an integration-lite check that
// renderMessage wraps a very long tool result to the given width end to end.
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
