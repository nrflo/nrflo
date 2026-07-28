package consoleui

import (
	"strings"
	"testing"
	"time"
)

func TestWorkingSuffixAndElapsed(t *testing.T) {
	if got := workingSuffix(runningTool{}, time.Minute); got != "" {
		t.Errorf("workingSuffix(no tool) = %q, want empty", got)
	}
	tool := runningTool{Detail: "[Bash] make test"}
	if got := workingSuffix(tool, 42*time.Second); got != " · [Bash] make test · 42s" {
		t.Errorf("workingSuffix = %q", got)
	}
	tests := []struct {
		d    time.Duration
		want string
	}{
		{-5 * time.Second, "0s"},
		{42 * time.Second, "42s"},
		{3*time.Minute + 10*time.Second, "3m10s"},
		{time.Hour + 5*time.Minute, "1h05m"},
	}
	for _, tt := range tests {
		if got := formatElapsed(tt.d); got != tt.want {
			t.Errorf("formatElapsed(%v) = %q, want %q", tt.d, got, tt.want)
		}
	}
}

// TestApplyStream_ToolStartedFinished verifies tool_started sets the running
// tool line (falling back to the tool name when detail is empty), and that
// tool_finished, turn idle, and error all clear it.
func TestApplyStream_ToolStartedFinished(t *testing.T) {
	m := &model{detail: ChatDetail{SessionID: "s1"}, deltas: map[string]string{}}

	m.applyStream(streamUpdate{Events: []Event{event("console_chat.tool_started", "s1", map[string]any{"tool": "Bash", "detail": "[Bash] make test"})}})
	if m.tool.Detail != "[Bash] make test" || m.tool.Since.IsZero() {
		t.Fatalf("tool after tool_started = %+v, want detail + non-zero Since", m.tool)
	}

	m.applyStream(streamUpdate{Events: []Event{event("console_chat.tool_finished", "s1", map[string]any{"tool": "Bash"})}})
	if m.tool.Detail != "" {
		t.Errorf("tool after tool_finished = %+v, want cleared", m.tool)
	}

	m.applyStream(streamUpdate{Events: []Event{event("console_chat.tool_started", "s1", map[string]any{"tool": "WebFetch"})}})
	if m.tool.Detail != "[WebFetch]" {
		t.Errorf("tool detail fallback = %q, want [WebFetch]", m.tool.Detail)
	}
	m.applyStream(streamUpdate{Events: []Event{event("console_chat.turn", "s1", map[string]any{"state": "idle"})}})
	if m.tool.Detail != "" {
		t.Errorf("tool after idle = %+v, want cleared", m.tool)
	}

	m.applyStream(streamUpdate{Events: []Event{
		event("console_chat.tool_started", "s1", map[string]any{"tool": "Bash"}),
		event("console_chat.error", "s1", map[string]any{"text": "boom"}),
	}})
	if m.tool.Detail != "" {
		t.Errorf("tool after error = %+v, want cleared", m.tool)
	}
}

// TestLiveRegionView_RunningToolOnSpinnerLine verifies the in-flight tool
// detail renders on the working line while a turn runs.
func TestLiveRegionView_RunningToolOnSpinnerLine(t *testing.T) {
	m := liveTestModel(80, 24)
	m.status = "running"
	m.tool = runningTool{Detail: "[Bash] make test", Since: time.Now()}
	out := m.liveRegionView(m.height)
	if !strings.Contains(out, "[Bash] make test") {
		t.Errorf("liveRegionView() = %q, want it to contain the running tool detail", out)
	}
}

func TestBgRelevant(t *testing.T) {
	if bgRelevant([]Event{{Type: "console_chat.delta"}, {Type: "messages.updated"}}) {
		t.Error("bgRelevant = true for unrelated events, want false")
	}
	if !bgRelevant([]Event{{Type: "workflow.updated"}}) {
		t.Error("bgRelevant = false for workflow.updated, want true")
	}
	if !bgRelevant([]Event{{Type: "orchestration.completed"}}) {
		t.Error("bgRelevant = false for orchestration.completed, want true")
	}
}

// TestStatusBar_BgCounter verifies the status bar shows bg:N only when
// active workflow instances exist for the project.
func TestStatusBar_BgCounter(t *testing.T) {
	m := &model{detail: ChatDetail{Engine: "claude", ProjectID: "p1"}}
	if got := m.statusBar(); strings.Contains(got, "bg:") {
		t.Errorf("statusBar() with no bg work = %q, want no bg segment", got)
	}
	m.bgRunning = 3
	if got := m.statusBar(); !strings.Contains(got, "bg:3") {
		t.Errorf("statusBar() with bgRunning=3 = %q, want bg:3", got)
	}
}
