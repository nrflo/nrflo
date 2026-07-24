package consoleui

import (
	"strings"
	"testing"
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
