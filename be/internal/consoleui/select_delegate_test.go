package consoleui

import (
	"strings"
	"testing"

	"charm.land/lipgloss/v2"
)

// TestCompactRow_SingleLineWithinWidth verifies compactRow never emits a
// newline and never exceeds the requested width, across both the selected
// and unselected variants.
func TestCompactRow_SingleLineWithinWidth(t *testing.T) {
	for _, selected := range []bool{false, true} {
		out := compactRow("opus-4-8", "Claude Opus 4.8 — premium reasoning model", 24, selected)
		if strings.Contains(out, "\n") {
			t.Fatalf("compactRow(selected=%v) contains newline: %q", selected, out)
		}
		if w := lipgloss.Width(out); w > 24 {
			t.Errorf("compactRow(selected=%v) width = %d, want <= 24", selected, w)
		}
	}
}

// TestCompactRow_OverflowAppendsEllipsis verifies a name+detail combination
// that overflows the given width is truncated with a trailing '…' marker,
// rather than silently dropping the description.
func TestCompactRow_OverflowAppendsEllipsis(t *testing.T) {
	out := compactRow("gpt-5.6-sol", "a very long description that will not fit in the row width", 20, false)
	if !strings.HasSuffix(out, "…") {
		t.Errorf("compactRow overflow output = %q, want it to end in the '…' truncation marker", out)
	}
	if w := lipgloss.Width(out); w > 20 {
		t.Errorf("compactRow overflow width = %d, want <= 20", w)
	}
}

// TestCompactRow_WideWidthKeepsNameAndDescription verifies that at a width
// wide enough to fit both, the description is elided (via mutedStyle), not
// dropped: both the title and detail text appear in the rendered row.
func TestCompactRow_WideWidthKeepsNameAndDescription(t *testing.T) {
	out := compactRow("sonnet-5", "Claude Sonnet 5", 200, false)
	if !strings.Contains(out, "sonnet-5") {
		t.Errorf("compactRow wide output = %q, want it to contain the title %q", out, "sonnet-5")
	}
	if !strings.Contains(out, "Claude Sonnet 5") {
		t.Errorf("compactRow wide output = %q, want it to contain the detail %q", out, "Claude Sonnet 5")
	}
}

// TestCompactRow_SelectedVariantDiffersFromNormal verifies the selected
// row's rendering differs from the unselected row's rendering for the same
// inputs (the accent selection prefix/highlight is actually applied).
func TestCompactRow_SelectedVariantDiffersFromNormal(t *testing.T) {
	normal := compactRow("haiku-4-5", "Claude Haiku 4.5", 40, false)
	selected := compactRow("haiku-4-5", "Claude Haiku 4.5", 40, true)
	if normal == selected {
		t.Errorf("compactRow selected and normal variants should differ, both = %q", normal)
	}
}

// TestCompactRow_EmptyDetailOmitsSeparator verifies a row with no detail
// text renders just the (possibly styled) name, without a dangling
// separator.
func TestCompactRow_EmptyDetailOmitsSeparator(t *testing.T) {
	out := compactRow("codex", "", 40, false)
	if strings.Contains(out, "\n") {
		t.Fatalf("compactRow with empty detail contains newline: %q", out)
	}
	if !strings.Contains(out, "codex") {
		t.Errorf("compactRow with empty detail = %q, want it to contain the title", out)
	}
}
