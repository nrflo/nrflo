package consoleui

import (
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
)

// TestStatusBar_YoloBadge verifies the status bar shows a "YOLO" badge only
// when detail.Yolo is true.
func TestStatusBar_YoloBadge(t *testing.T) {
	m := &model{detail: ChatDetail{Engine: "codex", ProjectID: "proj-1"}}

	if got := m.statusBar(); strings.Contains(got, "YOLO") {
		t.Errorf("statusBar() with Yolo=false = %q, want no YOLO badge", got)
	}

	m.detail.Yolo = true
	if got := m.statusBar(); !strings.Contains(got, "YOLO") {
		t.Errorf("statusBar() with Yolo=true = %q, want a YOLO badge", got)
	}
}

// TestStatusBar_TruncatesToSingleRow verifies a status bar longer than the
// terminal width is truncated to one physical row: a wrapped bar breaks the
// chrome height math and can leak into scrollback via tea.Println inserts.
func TestStatusBar_TruncatesToSingleRow(t *testing.T) {
	m := &model{detail: ChatDetail{
		Engine: "claude", Model: "claude-fable-5", ProjectID: "a-rather-long-project-identifier",
		SessionApprovals: []string{"Bash", "Read", "Edit", "WebFetch", "WebSearch", "NotebookEdit"},
	}}
	m.width = 40
	if got := ansi.StringWidth(m.statusBar()); got > 40 {
		t.Errorf("statusBar() display width = %d, want <= terminal width 40", got)
	}
	if bar := m.statusBar(); strings.Contains(bar, "\n") {
		t.Errorf("statusBar() = %q, want a single line", bar)
	}
}
