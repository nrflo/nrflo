package consoleui

import (
	"fmt"
	"reflect"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
)

// pinTestModel builds a ready *model literal sufficient to drive the real
// Update() path (WindowSizeMsg, historyMsg, syncMsg) without a running
// terminal or client.
func pinTestModel(t *testing.T) *model {
	t.Helper()
	m := &model{
		deltas: map[string]string{},
		input:  newTestComposer(t),
		detail: ChatDetail{SessionID: "s1", Engine: "claude", Model: "opus"},
		status: "idle",
	}
	return m
}

// printlnBodies recursively executes cmd (unwrapping tea.BatchMsg and the
// unexported tea.sequenceMsg) and returns the message bodies of every
// tea.Println command found, in traversal order, reading the unexported
// printLineMessage.messageBody field via reflection since the type itself is
// unexported by the tea package.
func printlnBodies(t *testing.T, cmd tea.Cmd) []string {
	t.Helper()
	if cmd == nil {
		return nil
	}
	msg := cmd()
	if msg == nil {
		return nil
	}
	if batch, ok := msg.(tea.BatchMsg); ok {
		var out []string
		for _, sub := range batch {
			out = append(out, printlnBodies(t, sub)...)
		}
		return out
	}
	if fmt.Sprintf("%T", msg) == "tea.sequenceMsg" {
		v := reflect.ValueOf(msg)
		var out []string
		for i := 0; i < v.Len(); i++ {
			if sub, ok := v.Index(i).Interface().(tea.Cmd); ok {
				out = append(out, printlnBodies(t, sub)...)
			}
		}
		return out
	}
	if fmt.Sprintf("%T", msg) != "tea.printLineMessage" {
		return nil
	}
	v := reflect.ValueOf(msg)
	return []string{v.Field(0).String()}
}

func longMessagePage(n int) MessagePage {
	messages := make([]Message, n)
	for i := range messages {
		messages[i] = Message{
			Category: "assistant",
			Content:  strings.Repeat(fmt.Sprintf("paragraph %d word word word word word. ", i), 6),
		}
	}
	return MessagePage{Messages: messages, Total: n}
}

// TestUpdate_HistoryMsgPrintsAndDocksToBottom drives WindowSizeMsg then a
// short historyMsg page through the real Update path and verifies the
// returned commands are tea.Println (native scrollback), and that View()
// still docks: it never exceeds the terminal height, and the footer lands
// on the last line.
func TestUpdate_HistoryMsgPrintsAndDocksToBottom(t *testing.T) {
	const width, height = 80, 24
	m := pinTestModel(t)

	next, cmd := m.Update(tea.WindowSizeMsg{Width: width, Height: height})
	m = next.(*model)
	_ = printlnBodies(t, cmd) // initial (empty) page

	page := MessagePage{
		Messages: []Message{
			{Category: "user_input", Content: "hello"},
			{Category: "assistant", Content: "hi there, how can I help?"},
		},
		Total: 2,
	}
	next, cmd = m.Update(historyMsg{page: page})
	m = next.(*model)

	// Bodies must arrive in transcript order (tea.Sequence, not Batch): the
	// user line first, the assistant reply after it.
	bodies := printlnBodies(t, cmd)
	joined := strings.Join(bodies, "\n")
	helloIdx := strings.Index(joined, "hello")
	replyIdx := strings.Index(joined, "hi there")
	if helloIdx == -1 || replyIdx == -1 {
		t.Fatalf("printlnBodies missing message content, got %#v", bodies)
	}
	if helloIdx >= replyIdx {
		t.Errorf("user line (idx %d) not printed before assistant reply (idx %d)", helloIdx, replyIdx)
	}
	if m.printedLines <= 0 {
		t.Errorf("printedLines = %d, want > 0 after printing a page", m.printedLines)
	}

	content := m.View().Content
	if got := lipgloss.Height(content); got > height {
		t.Errorf("lipgloss.Height(View().Content) = %d, want <= %d", got, height)
	}
	lines := strings.Split(content, "\n")
	last := lines[len(lines)-1]
	if !strings.Contains(last, "enter send") {
		t.Errorf("last line = %q, want it to contain footer help", last)
	}
}

// TestUpdate_LongGlamourPageNeverExceedsHeight verifies that a long page of
// glamour-rendered assistant messages (many physical rows once wrapped)
// still keeps View() within the terminal height — the vanish guard: once
// printedLines exceeds the height, padding clamps to zero rather than
// pushing the frame off-screen.
func TestUpdate_LongGlamourPageNeverExceedsHeight(t *testing.T) {
	const width, height = 80, 24
	m := pinTestModel(t)

	next, _ := m.Update(tea.WindowSizeMsg{Width: width, Height: height})
	m = next.(*model)

	next, cmd := m.Update(syncMsg{detail: m.detail, page: longMessagePage(20)})
	m = next.(*model)

	// Chunked printing: at least one Println per message, more once a
	// message's rendered rows exceed the chunk headroom.
	bodies := printlnBodies(t, cmd)
	if len(bodies) < 20 {
		t.Fatalf("printlnBodies = %d, want >= 20 tea.Println commands", len(bodies))
	}
	if m.printedLines <= height {
		t.Fatalf("test setup invalid: printedLines=%d must exceed height=%d to exercise the vanish guard", m.printedLines, height)
	}

	content := m.View().Content
	if got := lipgloss.Height(content); got > height {
		t.Errorf("lipgloss.Height(View().Content) = %d, want <= %d (vanish guard)", got, height)
	}
	lines := strings.Split(content, "\n")
	last := lines[len(lines)-1]
	if !strings.Contains(last, "enter send") {
		t.Errorf("last line = %q, want it to contain footer help", last)
	}
}

// TestUpdate_ResizeAfterPrintsRedocksToNewHeight verifies that after
// printing a page and then resizing, View() still docks at the new
// dimensions: printedLines is recomputed from the retained tail buffer
// rather than drifting, so the frame never overflows and the footer still
// lands on the last line.
func TestUpdate_ResizeAfterPrintsRedocksToNewHeight(t *testing.T) {
	const width, height = 80, 24
	m := pinTestModel(t)

	next, _ := m.Update(tea.WindowSizeMsg{Width: width, Height: height})
	m = next.(*model)
	next, _ = m.Update(historyMsg{page: longMessagePage(6)})
	m = next.(*model)

	const newWidth, newHeight = 50, 15
	next, _ = m.Update(tea.WindowSizeMsg{Width: newWidth, Height: newHeight})
	m = next.(*model)

	if m.width != newWidth || m.height != newHeight {
		t.Fatalf("after resize: width=%d height=%d, want %d/%d", m.width, m.height, newWidth, newHeight)
	}

	content := m.View().Content
	if got := lipgloss.Height(content); got > newHeight {
		t.Errorf("after resize: lipgloss.Height(View().Content) = %d, want <= %d", got, newHeight)
	}
	lines := strings.Split(content, "\n")
	last := lines[len(lines)-1]
	if !strings.Contains(last, "enter send") {
		t.Errorf("after resize: last line = %q, want it to contain footer help", last)
	}
}

// TestUpdate_ShrinkThenGrowStillDocksAtBottom verifies that a shrink doesn't
// over-evict printedTail history: the eviction bound must stay pinned to the
// max terminal height ever seen, not the current (shrunk) height, so that
// growing back keeps enough retained tail rows to recompute printedLines
// >= the regrown height. Otherwise the pin (target = m.height-printedLines)
// injects spurious blank padding above the composer after regrowing — the
// float-up symptom, reachable via a shrink-then-grow sequence.
func TestUpdate_ShrinkThenGrowStillDocksAtBottom(t *testing.T) {
	const width, tallHeight, shortHeight = 80, 40, 10
	m := pinTestModel(t)

	next, _ := m.Update(tea.WindowSizeMsg{Width: width, Height: tallHeight})
	m = next.(*model)
	next, _ = m.Update(historyMsg{page: longMessagePage(20)})
	m = next.(*model)
	next, _ = m.Update(tea.WindowSizeMsg{Width: width, Height: shortHeight})
	m = next.(*model)
	next, _ = m.Update(tea.WindowSizeMsg{Width: width, Height: tallHeight})
	m = next.(*model)

	if m.printedLines < tallHeight {
		t.Errorf("after shrink-then-grow: printedLines=%d, want >= tallHeight=%d — an undercount here means the pin injects phantom blank padding above the composer", m.printedLines, tallHeight)
	}

	content := m.View().Content
	if got := lipgloss.Height(content); got > tallHeight {
		t.Errorf("after shrink-then-grow: lipgloss.Height(View().Content) = %d, want <= %d", got, tallHeight)
	}
	lines := strings.Split(content, "\n")
	last := lines[len(lines)-1]
	if !strings.Contains(last, "enter send") {
		t.Errorf("after shrink-then-grow: last line = %q, want it to contain footer help", last)
	}
}

// TestView_TinyTerminalDropsOptionalChromeKeepsMandatoryTail verifies that
// when stacked optional chrome (an approval prompt) plus the mandatory
// composer/status/footer tail together exceed the terminal height,
// clampChrome drops the optional section first (front-to-back) and keeps
// the mandatory tail — including the composer's content line — fully
// intact, with View()'s height clamped to exactly m.height.
func TestView_TinyTerminalDropsOptionalChromeKeepsMandatoryTail(t *testing.T) {
	m := pinTestModel(t)
	m.resize(80, 24)
	m.approvals = []Approval{{ID: "a1", Command: "make test"}}

	mandatorySections := []string{
		composerBox.Width(max(1, m.width-2)).Render(m.input.View()),
		m.statusBar(),
		m.footer(),
	}
	mandatoryChrome := lipgloss.JoinVertical(lipgloss.Left, mandatorySections...)
	mandatoryHeight := lipgloss.Height(mandatoryChrome)

	fullSections := append([]string{m.approvalView()}, mandatorySections...)
	fullHeight := lipgloss.Height(lipgloss.JoinVertical(lipgloss.Left, fullSections...))
	if fullHeight <= mandatoryHeight {
		t.Fatalf("test setup invalid: approval section adds no height (full=%d mandatory=%d)", fullHeight, mandatoryHeight)
	}

	// Shrink to fit the mandatory tail exactly but not the optional approval
	// section on top of it.
	m.resize(80, mandatoryHeight)

	content := m.View().Content
	if got := lipgloss.Height(content); got != mandatoryHeight {
		t.Errorf("lipgloss.Height(View().Content) = %d, want %d (clamped to terminal height)", got, mandatoryHeight)
	}
	if content != mandatoryChrome {
		t.Errorf("View().Content = %q, want the mandatory tail alone %q (approval section dropped)", content, mandatoryChrome)
	}
	if !strings.Contains(content, "nrflo…") {
		t.Errorf("View().Content = %q, want the composer's placeholder content line still visible", content)
	}
}

// TestView_ExtremeTinyTerminalDropsComposerKeepsFooterVisible verifies that
// when the terminal is too short even for the mandatory composer section,
// clampChrome keeps dropping front sections (composer, then status) until
// what remains fits, View() pads the slack with blank lines to still fill
// exactly m.height (the same bottom-docking pad as a normal frame), and the
// footer — the last section — is never dropped and stays on the last line.
func TestView_ExtremeTinyTerminalDropsComposerKeepsFooterVisible(t *testing.T) {
	m := pinTestModel(t)
	m.resize(80, 24)

	const tinyHeight = 1
	m.resize(80, tinyHeight)

	content := m.View().Content
	if got := lipgloss.Height(content); got != tinyHeight {
		t.Errorf("lipgloss.Height(View().Content) = %d, want %d (clamped to terminal height)", got, tinyHeight)
	}
	if strings.Contains(content, "╭") || strings.Contains(content, "nrflo…") {
		t.Errorf("View().Content = %q, want the composer box dropped at h=%d", content, tinyHeight)
	}
	if !strings.Contains(content, "enter send") {
		t.Errorf("View().Content = %q, want the footer still visible on the single remaining line", content)
	}
}
