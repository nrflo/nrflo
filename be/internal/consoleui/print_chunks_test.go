package consoleui

import (
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
)

// TestSplitChunks verifies rendered content splits into "\n"-delimited groups
// of at most maxRows lines, preserving order and content.
func TestSplitChunks(t *testing.T) {
	tests := []struct {
		name    string
		lines   int
		maxRows int
		want    []int // lines per chunk
	}{
		{name: "fits in one chunk", lines: 3, maxRows: 10, want: []int{3}},
		{name: "exact multiple", lines: 6, maxRows: 3, want: []int{3, 3}},
		{name: "remainder chunk", lines: 7, maxRows: 3, want: []int{3, 3, 1}},
		{name: "single row chunks", lines: 3, maxRows: 1, want: []int{1, 1, 1}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			lines := make([]string, tt.lines)
			for i := range lines {
				lines[i] = strings.Repeat("x", i+1)
			}
			rendered := strings.Join(lines, "\n")
			chunks := splitChunks(rendered, tt.maxRows)
			if len(chunks) != len(tt.want) {
				t.Fatalf("splitChunks returned %d chunks, want %d", len(chunks), len(tt.want))
			}
			for i, chunk := range chunks {
				if got := strings.Count(chunk, "\n") + 1; got != tt.want[i] {
					t.Errorf("chunk[%d] has %d lines, want %d", i, got, tt.want[i])
				}
			}
			if rejoined := strings.Join(chunks, "\n"); rejoined != rendered {
				t.Errorf("rejoined chunks = %q, want original %q", rejoined, rendered)
			}
		})
	}
}

// TestMaxPrintRows verifies the chunk bound leaves headroom for the live
// region and worst-case chrome, flooring at one row on short terminals.
func TestMaxPrintRows(t *testing.T) {
	tests := []struct{ height, want int }{
		{40, 40 - liveRegionCap - chromeAllowance},
		{24, 1}, // 24-12-12=0 floors to 1
		{10, 1},
	}
	for _, tt := range tests {
		m := &model{}
		m.height = tt.height
		if got := m.maxPrintRows(); got != tt.want {
			t.Errorf("maxPrintRows() at height=%d = %d, want %d", tt.height, got, tt.want)
		}
	}
}

// TestRenderMessage_NeverEmitsTabsOrOverWideLines verifies every category's
// rendered output is tab-free and hard-wrapped within the content width:
// tabs count as zero-width in ansi.StringWidth but advance to the next tab
// stop on a real terminal, and over-wide lines both desync bubbletea's
// insertAbove row accounting and leak frame rows into scrollback.
func TestRenderMessage_NeverEmitsTabsOrOverWideLines(t *testing.T) {
	const width = 40
	long := strings.Repeat("verylongtoken", 20)
	messages := []Message{
		{Category: "user_input", Content: "a\tb\n" + long},
		{Category: "tool", Content: "[Bash] sqlite3\theader\tcolumn " + long},
		{Category: "thinking", Content: "pondering\ttabs " + long},
		{Category: "assistant", Content: "para\n\n```python\n\tindented = 1\n" + long + "\n```\n"},
	}
	for _, message := range messages {
		t.Run(message.Category, func(t *testing.T) {
			rendered := renderMessage(message, width)
			if strings.Contains(rendered, "\t") {
				t.Errorf("renderMessage(%s) output contains a literal tab", message.Category)
			}
			for i, line := range strings.Split(rendered, "\n") {
				if lw := ansi.StringWidth(line); lw > width {
					t.Errorf("renderMessage(%s) line %d width = %d, want <= %d", message.Category, i, lw, width)
				}
			}
		})
	}
}
