package consoleui

import (
	"strings"
	"testing"

	"charm.land/bubbles/v2/spinner"
	"charm.land/lipgloss/v2"
)

// anchorTestModel builds a ready *model literal with a real composer, minimal
// enough to exercise View() without a running terminal.
func anchorTestModel(t *testing.T, width, height int) *model {
	t.Helper()
	m := &model{
		deltas: map[string]string{},
		spin:   spinner.New(spinner.WithSpinner(spinner.MiniDot)),
		input:  newTestComposer(t),
		detail: ChatDetail{SessionID: "s1", Engine: "claude", Model: "opus"},
	}
	m.resize(width, height)
	m.status = "idle"
	return m
}

// TestView_FreshFrameHasNoPadding verifies a fresh frame (no shrink seen yet)
// carries no band padding and never fills the terminal: bottom-anchoring
// comes from the terminal cursor position (clearScreenSeq parks it on the
// bottom row), and insertAbove needs free rows above the frame to insert
// printed content into — a full-height frame desyncs the renderer one row
// per insert.
func TestView_FreshFrameHasNoPadding(t *testing.T) {
	const height = 20
	m := anchorTestModel(t, 80, height)

	content := m.View().Content
	if got := lipgloss.Height(content); got >= height {
		t.Errorf("lipgloss.Height(View().Content) = %d, want frame-only (< %d)", got, height)
	}
	lines := strings.Split(content, "\n")
	if strings.TrimSpace(lines[0]) == "" {
		t.Errorf("first line = %q, want frame content, not padding", lines[0])
	}
	last := lines[len(lines)-1]
	if !strings.Contains(last, "ctrl+t graph") {
		t.Errorf("last line = %q, want it to contain footer help %q", last, "ctrl+t graph")
	}
}

// TestClearScreenSeq_ParksCursorOnBottomRow guards the startup contract the
// no-padding View depends on: the pre-program clear must end by moving the
// cursor to the terminal's bottom row so the inline region starts
// bottom-anchored.
func TestClearScreenSeq_ParksCursorOnBottomRow(t *testing.T) {
	if !strings.HasSuffix(clearScreenSeq, "\x1b[999;1H") {
		t.Errorf("clearScreenSeq = %q, want it to end with a bottom-row cursor park", clearScreenSeq)
	}
}
