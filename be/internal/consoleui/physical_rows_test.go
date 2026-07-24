package consoleui

import (
	"strings"
	"testing"

	"charm.land/lipgloss/v2"
)

// TestPhysicalRows_MatchesInsertAboveFormula verifies physicalRows mirrors
// bubbletea v2's cursedRenderer.insertAbove counting: one row per
// "\n"-delimited line plus one extra row per full multiple of width a
// line's display width consumes, counted at the full terminal width (not
// the narrower content width the text was wrapped to). For messages whose
// widest line fits within width, this must equal lipgloss.Height.
func TestPhysicalRows_MatchesInsertAboveFormula(t *testing.T) {
	tests := []struct {
		name           string
		rendered       string
		width          int
		want           int
		equalsLipgloss bool
	}{
		{
			name:           "single short line no wrap",
			rendered:       "assistant",
			width:          80,
			want:           1,
			equalsLipgloss: true,
		},
		{
			name:           "multi-line fits within width",
			rendered:       "assistant\nhello there",
			width:          80,
			want:           2,
			equalsLipgloss: true,
		},
		{
			name:           "wide code-block line wraps beyond content width but full terminal width matters",
			rendered:       "assistant\n" + strings.Repeat("x", 200),
			width:          80,
			want:           2 + 200/80, // 2 lines + 2 extra wrap rows for the 200-wide line
			equalsLipgloss: false,
		},
		{
			name:           "line exactly a multiple of width",
			rendered:       strings.Repeat("y", 240),
			width:          80,
			want:           1 + 240/80,
			equalsLipgloss: false,
		},
		{
			name:           "guards non-positive width",
			rendered:       "ab",
			width:          0,
			want:           1 + 2, // width floored to 1, so 2 extra rows for a 2-wide line
			equalsLipgloss: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := physicalRows(tt.rendered, tt.width)
			if got != tt.want {
				t.Errorf("physicalRows(%q, %d) = %d, want %d", tt.rendered, tt.width, got, tt.want)
			}
			height := lipgloss.Height(tt.rendered)
			if tt.equalsLipgloss && got != height {
				t.Errorf("physicalRows = %d, want equal to lipgloss.Height = %d for a message whose lines fit", got, height)
			}
			if !tt.equalsLipgloss && got == height {
				t.Errorf("physicalRows = %d, want it to diverge from lipgloss.Height = %d for a wrapping line", got, height)
			}
		})
	}
}
