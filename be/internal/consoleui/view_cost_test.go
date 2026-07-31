package consoleui

import (
	"strings"
	"testing"
)

// TestStatusBar_CostEstimate verifies statusBar() renders a "~$X.XX" segment
// only when CostEstimate is known, and omits it entirely when nil.
func TestStatusBar_CostEstimate(t *testing.T) {
	t.Run("present", func(t *testing.T) {
		cost := 3.5
		m := &model{detail: ChatDetail{Engine: "codex", ProjectID: "p1", CostEstimate: &cost}}
		if got := m.statusBar(); !strings.Contains(got, "~$3.50") {
			t.Errorf("statusBar() = %q, want it to contain ~$3.50", got)
		}
	})
	t.Run("absent", func(t *testing.T) {
		m := &model{detail: ChatDetail{Engine: "codex", ProjectID: "p1"}}
		if got := m.statusBar(); strings.Contains(got, "~$") {
			t.Errorf("statusBar() = %q, want no cost segment when CostEstimate is nil", got)
		}
	})
}

// TestStatusBar_ConnectionAndProject verifies statusBar() reflects the
// connection state and always shows the project id.
func TestStatusBar_ConnectionAndProject(t *testing.T) {
	t.Run("offline", func(t *testing.T) {
		m := &model{detail: ChatDetail{Engine: "codex", ProjectID: "p1"}, connected: false}
		if got := m.statusBar(); !strings.Contains(got, "○") || !strings.Contains(got, "p1") {
			t.Errorf("statusBar() = %q, want ○ + project id p1", got)
		}
	})
	t.Run("connected", func(t *testing.T) {
		m := &model{detail: ChatDetail{Engine: "codex", ProjectID: "p1"}, connected: true}
		if got := m.statusBar(); !strings.Contains(got, "●") || strings.Contains(got, "○") {
			t.Errorf("statusBar() = %q, want ● without ○", got)
		}
	})
}

// TestStatusBar_AlwaysList verifies statusBar() surfaces the always-allowed
// tool list only when SessionApprovals is non-empty.
func TestStatusBar_AlwaysList(t *testing.T) {
	t.Run("present", func(t *testing.T) {
		m := &model{detail: ChatDetail{Engine: "codex", ProjectID: "p1", SessionApprovals: []string{"bash", "read"}}}
		got := m.statusBar()
		if !strings.Contains(got, "always:bash,read") {
			t.Errorf("statusBar() = %q, want always:bash,read", got)
		}
	})
	t.Run("absent", func(t *testing.T) {
		m := &model{detail: ChatDetail{Engine: "codex", ProjectID: "p1"}}
		if got := m.statusBar(); strings.Contains(got, "always:") {
			t.Errorf("statusBar() = %q, want no always-list segment", got)
		}
	})
}
