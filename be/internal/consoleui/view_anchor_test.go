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

// TestView_ShortHistoryPadsToHeight verifies that with a small printedLines
// count (rows already emitted to native scrollback, below the chrome
// budget), View().Content is padded to exactly fill the remaining terminal
// rows (m.height - m.printedLines) so printedLines+View() together fill the
// screen, with the leftover slack rendered as blank padding above the frame.
func TestView_ShortHistoryPadsToHeight(t *testing.T) {
	const height = 20
	m := anchorTestModel(t, 80, height)
	m.printedLines = 3

	chromeSections := []string{
		composerBox.Width(max(1, m.width-2)).Render(m.input.View()),
		m.statusBar(),
		m.footer(),
	}
	chrome := lipgloss.JoinVertical(lipgloss.Left, chromeSections...)
	chromeHeight := lipgloss.Height(chrome)
	if m.printedLines >= height-chromeHeight {
		t.Fatalf("test setup invalid: printedLines=%d must be < height-chromeHeight=%d", m.printedLines, height-chromeHeight)
	}
	wantContentHeight := height - m.printedLines
	wantPadding := wantContentHeight - chromeHeight

	content := m.View().Content
	if got := lipgloss.Height(content); got != wantContentHeight {
		t.Errorf("lipgloss.Height(View().Content) = %d, want %d (height=%d printedLines=%d)", got, wantContentHeight, height, m.printedLines)
	}

	lines := strings.Split(content, "\n")
	gotPadding := 0
	for _, line := range lines {
		if strings.TrimSpace(line) == "" {
			gotPadding++
			continue
		}
		break
	}
	if gotPadding != wantPadding {
		t.Errorf("leading blank lines = %d, want %d (height=%d printedLines=%d chromeHeight=%d)",
			gotPadding, wantPadding, height, m.printedLines, chromeHeight)
	}
}

// TestView_FullScrollbackYieldsNoPadding verifies that once printedLines has
// filled (or exceeded) the terminal height, padding clamps to zero and the
// view height equals the chrome height alone — the previously-passing
// full-scrollback behavior is unchanged (AC#3).
func TestView_FullScrollbackYieldsNoPadding(t *testing.T) {
	const height = 20
	m := anchorTestModel(t, 80, height)
	m.printedLines = height

	chromeSections := []string{
		composerBox.Width(max(1, m.width-2)).Render(m.input.View()),
		m.statusBar(),
		m.footer(),
	}
	chrome := lipgloss.JoinVertical(lipgloss.Left, chromeSections...)
	chromeHeight := lipgloss.Height(chrome)

	content := m.View().Content
	if got := lipgloss.Height(content); got != chromeHeight {
		t.Errorf("lipgloss.Height(View().Content) = %d, want %d (chrome height, no padding)", got, chromeHeight)
	}

	// Also verify a printedLines count well beyond the height still clamps
	// to zero padding rather than going negative.
	m.printedLines = height * 10
	content = m.View().Content
	if got := lipgloss.Height(content); got != chromeHeight {
		t.Errorf("with printedLines >> height: lipgloss.Height(View().Content) = %d, want %d", got, chromeHeight)
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

// TestPrintNewMessages_IncrementsPrintedLinesByRenderedHeight verifies
// printNewMessages advances m.printedLines by the summed lipgloss.Height of
// each rendered row, matching how printedTotal advances in the same loop.
func TestPrintNewMessages_IncrementsPrintedLinesByRenderedHeight(t *testing.T) {
	m := printTestModel(0, "")
	page := MessagePage{
		Messages: []Message{
			{Category: "user_input", Content: "hello"},
			{Category: "tool", Content: "some tool output"},
		},
		Total: 2,
	}

	wantLines := 0
	width := m.contentWidth()
	for _, msg := range page.Messages {
		wantLines += lipgloss.Height(renderMessage(msg, width))
	}

	m.printNewMessages(page)

	if m.printedLines != wantLines {
		t.Errorf("printedLines = %d, want %d (summed rendered heights)", m.printedLines, wantLines)
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
