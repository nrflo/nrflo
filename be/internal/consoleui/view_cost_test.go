package consoleui

import (
	"strings"
	"testing"
)

// TestHeader_CostEstimate verifies the header renders a "~$X.XX" segment
// only when CostEstimate is known, and omits it entirely when nil.
func TestHeader_CostEstimate(t *testing.T) {
	t.Run("present", func(t *testing.T) {
		cost := 3.5
		m := &model{detail: ChatDetail{Engine: "codex", ProjectID: "p1", CostEstimate: &cost}}
		if got := m.header(); !strings.Contains(got, "~$3.50") {
			t.Errorf("header() = %q, want it to contain ~$3.50", got)
		}
	})
	t.Run("absent", func(t *testing.T) {
		m := &model{detail: ChatDetail{Engine: "codex", ProjectID: "p1"}}
		if got := m.header(); strings.Contains(got, "~$") {
			t.Errorf("header() = %q, want no cost segment when CostEstimate is nil", got)
		}
	})
}
