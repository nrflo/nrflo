package consoleui

import (
	"strings"
	"testing"

	"charm.land/bubbles/v2/spinner"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
)

// liveTestModel builds a ready *model literal sufficient to exercise
// liveRegionView() without a running terminal.
func liveTestModel(width, height int) *model {
	m := &model{
		deltas: map[string]string{},
		spin:   spinner.New(spinner.WithSpinner(spinner.MiniDot)),
	}
	m.width, m.height, m.ready = width, height, true
	return m
}

// TestLiveRegionView_RunningShowsSpinner verifies the working spinner line
// renders only while a turn is running, not when idle.
func TestLiveRegionView_RunningShowsSpinner(t *testing.T) {
	m := liveTestModel(80, 24)
	m.status = "idle"
	if strings.Contains(m.liveRegionView(m.height), "working…") {
		t.Errorf("idle liveRegionView() contains working indicator, want none")
	}

	m.status = "running"
	if !strings.Contains(m.liveRegionView(m.height), "working…") {
		t.Errorf("running liveRegionView() = %q, want it to contain working indicator", m.liveRegionView(m.height))
	}
}

// TestLiveRegionView_DeltasAndThinkingRendered verifies in-flight deltas and
// the thinking line both appear in the composed live region.
func TestLiveRegionView_DeltasAndThinkingRendered(t *testing.T) {
	m := liveTestModel(80, 24)
	m.deltaOrder = []string{"answer"}
	m.deltas["answer"] = "partial reply text"
	m.thinking = "considering options"

	out := m.liveRegionView(m.height)
	if !strings.Contains(out, "partial reply text") {
		t.Errorf("liveRegionView() missing delta content: %q", out)
	}
	if !strings.Contains(out, "considering options") {
		t.Errorf("liveRegionView() missing thinking content: %q", out)
	}
}

// TestLiveRegionView_PendingUserAboveDeltas verifies the optimistic pending
// user line renders before (above) in-flight assistant deltas.
func TestLiveRegionView_PendingUserAboveDeltas(t *testing.T) {
	m := liveTestModel(80, 24)
	m.pendingUser = "what is the status"
	m.deltaOrder = []string{"answer"}
	m.deltas["answer"] = "working on it"

	out := m.liveRegionView(m.height)
	userIdx := strings.Index(out, "what is the status")
	deltaIdx := strings.Index(out, "working on it")
	if userIdx == -1 || deltaIdx == -1 {
		t.Fatalf("liveRegionView() missing pendingUser or delta content: %q", out)
	}
	if userIdx >= deltaIdx {
		t.Errorf("pendingUser line (idx %d) not above delta content (idx %d)", userIdx, deltaIdx)
	}
}

// TestLiveRegionView_EmptyWhenNothingLive verifies an idle model with no
// pending user line, deltas, or thinking renders an empty live region.
func TestLiveRegionView_EmptyWhenNothingLive(t *testing.T) {
	m := liveTestModel(80, 24)
	m.status = "idle"
	if out := m.liveRegionView(m.height); out != "" {
		t.Errorf("liveRegionView() = %q, want empty", out)
	}
}

// TestLiveRegionView_TailBoundedToHeight verifies a delta buffer taller than
// the terminal is clipped to the tail (~m.height-2 lines), never growing the
// live region unbounded.
func TestLiveRegionView_TailBoundedToHeight(t *testing.T) {
	const height = 10
	m := liveTestModel(80, height)
	m.deltaOrder = []string{"answer"}
	// Each delta line is rendered as "header\ntext"; force many wrapped lines
	// via embedded newlines in the delta text so the composed content exceeds
	// the terminal height.
	lines := make([]string, 50)
	for i := range lines {
		lines[i] = "line"
	}
	m.deltas["answer"] = strings.Join(lines, "\n")

	maxLines := max(1, height-2)
	out := m.liveRegionView(maxLines)
	gotLines := strings.Count(out, "\n") + 1
	if gotLines > maxLines {
		t.Errorf("liveRegionView() line count = %d, want tail-clipped to <= %d", gotLines, maxLines)
	}
	// The tail must be kept (last line), not the head.
	if !strings.HasSuffix(ansi.Strip(out), "line") {
		t.Errorf("liveRegionView() tail = %q, want it to end with the last delta line", out)
	}
}

// TestLiveRegionView_MaxLinesBelowOneIsEmpty verifies a zero/negative budget
// (e.g. when chrome alone already fills the terminal) renders nothing rather
// than panicking or producing a negative-length slice.
func TestLiveRegionView_MaxLinesBelowOneIsEmpty(t *testing.T) {
	m := liveTestModel(80, 24)
	m.deltaOrder = []string{"answer"}
	m.deltas["answer"] = "some content"
	m.thinking = "thinking"

	if out := m.liveRegionView(0); out != "" {
		t.Errorf("liveRegionView(0) = %q, want empty", out)
	}
	if out := m.liveRegionView(-5); out != "" {
		t.Errorf("liveRegionView(-5) = %q, want empty", out)
	}
}

// TestLiveRegionView_CappedRegardlessOfBudget verifies the live region never
// exceeds liveRegionCap rows even when the terminal budget is far larger:
// tea.Println inserts scroll against the frame currently on screen, so a tall
// live region leaves no headroom and its rows leak into native scrollback.
func TestLiveRegionView_CappedRegardlessOfBudget(t *testing.T) {
	m := liveTestModel(80, 60)
	m.deltaOrder = []string{"answer"}
	lines := make([]string, 100)
	for i := range lines {
		lines[i] = "line"
	}
	m.deltas["answer"] = strings.Join(lines, "\n")

	out := m.liveRegionView(50)
	if gotLines := strings.Count(out, "\n") + 1; gotLines > liveRegionCap {
		t.Errorf("liveRegionView(50) line count = %d, want <= liveRegionCap %d", gotLines, liveRegionCap)
	}
	if !strings.HasSuffix(ansi.Strip(out), "line") {
		t.Errorf("liveRegionView() tail = %q, want it to end with the last delta line", out)
	}
}

// TestLiveBand_HoldsHeightUntilPrintReleases verifies the live section's
// height ratchets (liveBand): clearing content keeps the band as blank rows —
// a frame shrink not paired with an insert would float the chrome — and a
// print releases exactly its own row count from the band.
func TestLiveBand_HoldsHeightUntilPrintReleases(t *testing.T) {
	m := liveTestModel(80, 40)
	m.ready = true
	m.deltaOrder = []string{"a"}
	m.deltas["a"] = "one\ntwo\nthree"

	if out := m.liveRegionView(20); strings.Count(out, "\n")+1 != 3 || m.liveBand != 3 {
		t.Fatalf("live region = %q (band %d), want 3 content rows and band 3", out, m.liveBand)
	}

	m.deltas = map[string]string{}
	m.deltaOrder = nil
	out := m.liveRegionView(20)
	lines := strings.Split(out, "\n")
	if len(lines) != 3 {
		t.Fatalf("band after clear = %d rows, want 3 blank rows held", len(lines))
	}
	for i, line := range lines {
		if strings.TrimSpace(ansi.Strip(line)) != "" {
			t.Errorf("band line %d = %q, want blank", i, line)
		}
	}

	// A 1-row print releases 1 row of band.
	cmd := m.printNewMessages(MessagePage{Messages: []Message{{Category: "user_input", Content: "hi"}}, Total: 1})
	if cmd == nil {
		t.Fatal("printNewMessages returned nil cmd")
	}
	if m.liveBand != 2 {
		t.Errorf("liveBand after 1-row print = %d, want 2", m.liveBand)
	}
}

// TestView_NeverExceedsHeight verifies the total rendered view (live region +
// chrome) never exceeds m.height, even with a delta buffer far taller than
// the terminal, at a small terminal height.
func TestView_NeverExceedsHeight(t *testing.T) {
	const height = 10
	m := &model{
		deltas: map[string]string{},
		spin:   spinner.New(spinner.WithSpinner(spinner.MiniDot)),
		input:  newTestComposer(t),
		detail: ChatDetail{SessionID: "s1", Engine: "claude", Model: "opus"},
	}
	m.resize(80, height)
	m.status = "running"

	lines := make([]string, 200)
	for i := range lines {
		lines[i] = "line"
	}
	m.deltaOrder = []string{"answer"}
	m.deltas["answer"] = strings.Join(lines, "\n")

	if got := lipgloss.Height(m.View().Content); got > m.height {
		t.Errorf("lipgloss.Height(m.View().Content) = %d, want <= %d", got, m.height)
	}
}

// TestView_NeverExceedsHeight_WorstCaseChrome verifies the total rendered
// view still fits within m.height when approval, suggestion-dropdown, and
// invoke chrome are all active simultaneously alongside a tall delta buffer —
// the live region must shrink enough to absorb all of it.
func TestView_NeverExceedsHeight_WorstCaseChrome(t *testing.T) {
	const height = 12
	m := &model{
		deltas: map[string]string{},
		spin:   spinner.New(spinner.WithSpinner(spinner.MiniDot)),
		input:  newTestComposer(t),
		detail: ChatDetail{SessionID: "s1", Engine: "claude", Model: "opus"},
	}
	m.resize(80, height)
	m.status = "running"

	lines := make([]string, 200)
	for i := range lines {
		lines[i] = "line"
	}
	m.deltaOrder = []string{"answer"}
	m.deltas["answer"] = strings.Join(lines, "\n")

	m.approvals = []Approval{{ID: "a1", Command: "make test"}}

	m.skills = []ConsoleSkill{{Name: "deploy"}}
	m.input.SetValue("/de")
	if !m.suggestionsOpen() {
		t.Fatalf("suggestionsOpen() = false, want true (test setup invalid)")
	}

	m.invoke = invokeState{active: true, tool: "deploy", phase: invokePhaseConfirm, inform: true}

	if got := lipgloss.Height(m.View().Content); got > m.height {
		t.Errorf("worst-case chrome: lipgloss.Height(m.View().Content) = %d, want <= %d", got, m.height)
	}
}
