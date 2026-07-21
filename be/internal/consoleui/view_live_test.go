package consoleui

import (
	"strings"
	"testing"

	"charm.land/bubbles/v2/spinner"
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
	if strings.Contains(m.liveRegionView(), "working…") {
		t.Errorf("idle liveRegionView() contains working indicator, want none")
	}

	m.status = "running"
	if !strings.Contains(m.liveRegionView(), "working…") {
		t.Errorf("running liveRegionView() = %q, want it to contain working indicator", m.liveRegionView())
	}
}

// TestLiveRegionView_DeltasAndThinkingRendered verifies in-flight deltas and
// the thinking line both appear in the composed live region.
func TestLiveRegionView_DeltasAndThinkingRendered(t *testing.T) {
	m := liveTestModel(80, 24)
	m.deltaOrder = []string{"answer"}
	m.deltas["answer"] = "partial reply text"
	m.thinking = "considering options"

	out := m.liveRegionView()
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

	out := m.liveRegionView()
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
	if out := m.liveRegionView(); out != "" {
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

	out := m.liveRegionView()
	gotLines := strings.Count(out, "\n") + 1
	maxLines := max(1, height-2)
	if gotLines > maxLines {
		t.Errorf("liveRegionView() line count = %d, want tail-clipped to <= %d", gotLines, maxLines)
	}
	// The tail must be kept (last line), not the head.
	if !strings.HasSuffix(out, "line") {
		t.Errorf("liveRegionView() tail = %q, want it to end with the last delta line", out)
	}
}
