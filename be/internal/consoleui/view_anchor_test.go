package consoleui

import (
	"strings"
	"testing"

	"charm.land/bubbles/v2/spinner"
	"charm.land/lipgloss/v2"
)

// anchorTestModel builds a ready *model literal with a real composer, minimal
// enough to exercise View()'s bottom-anchoring without a running terminal.
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

// TestView_EmptyScrollbackAnchorsChromeToBottom verifies that with no printed
// scrollback, the view fills the full terminal height and the footer help
// text lands on the last line, with a blank line pinned at the top (AC#1).
func TestView_EmptyScrollbackAnchorsChromeToBottom(t *testing.T) {
	const height = 20
	m := anchorTestModel(t, 80, height)

	content := m.View().Content
	if got := lipgloss.Height(content); got != height {
		t.Errorf("lipgloss.Height(View().Content) = %d, want %d", got, height)
	}

	lines := strings.Split(content, "\n")
	if len(lines) != height {
		t.Fatalf("len(lines) = %d, want %d", len(lines), height)
	}
	if strings.TrimSpace(lines[0]) != "" {
		t.Errorf("first line = %q, want blank (chrome pinned to bottom)", lines[0])
	}
	last := lines[len(lines)-1]
	if !strings.Contains(last, "enter send") {
		t.Errorf("last line = %q, want it to contain footer help %q", last, "enter send")
	}
}

// TestView_ResizeAnchorsToNewHeight verifies that resizing the terminal with
// no printed scrollback yet still produces a full-height view at the new
// height, with the footer on the last line (AC#2).
func TestView_ResizeAnchorsToNewHeight(t *testing.T) {
	m := anchorTestModel(t, 80, 24)

	const newHeight = 30
	m.resize(80, newHeight)

	content := m.View().Content
	if got := lipgloss.Height(content); got != newHeight {
		t.Errorf("after resize: lipgloss.Height(View().Content) = %d, want %d", got, newHeight)
	}
	lines := strings.Split(content, "\n")
	last := lines[len(lines)-1]
	if !strings.Contains(last, "enter send") {
		t.Errorf("after resize: last line = %q, want it to contain footer help", last)
	}
}

// TestPrintNewMessages_NoNewRowsLeavesPrintedLinesAlone verifies that when
// there is nothing new to print, printedLines is left untouched.
func TestPrintNewMessages_NoNewRowsLeavesPrintedLinesAlone(t *testing.T) {
	m := printTestModel(2, "")
	m.printedLines = 7
	page := MessagePage{Messages: messagesOf("a", "b"), Total: 2}

	m.printNewMessages(page)

	if m.printedLines != 7 {
		t.Errorf("printedLines = %d, want unchanged at 7", m.printedLines)
	}
}
