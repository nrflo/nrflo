package consoleui

import (
	"strings"
	"testing"

	"charm.land/bubbles/v2/textarea"
	"charm.land/lipgloss/v2"
)

// newTestComposer builds a textarea.Model mirroring the configuration in
// Run() (be/internal/consoleui/model.go), including the Prompt="" line that
// zeroes the per-line "┃ " prompt glyph.
func newTestComposer(t *testing.T) textarea.Model {
	t.Helper()
	input := textarea.New()
	input.Placeholder = "Ask nrflo…"
	input.ShowLineNumbers = false
	input.Prompt = ""
	input.MinHeight = 1
	input.MaxHeight = 8
	input.DynamicHeight = true
	input.SetHeight(1)
	input.CharLimit = 64 * 1024
	input.Focus()
	return input
}

// TestComposer_NoPromptGlyph verifies the composer textarea renders without
// the default "┃" prompt glyph, so lines start flush with the box padding.
func TestComposer_NoPromptGlyph(t *testing.T) {
	input := newTestComposer(t)
	input.SetWidth(74)
	if strings.Contains(input.View(), "┃") {
		t.Errorf("input.View() = %q, want no ┃ prompt glyph", input.View())
	}
}

// TestComposer_WidthMatchesSetWidth verifies the composer renders at exactly
// the width passed to SetWidth, with no gap or overflow introduced by the
// removed prompt reservation, for both a single-line and a grown draft.
func TestComposer_WidthMatchesSetWidth(t *testing.T) {
	const width = 80 // composerBox width - 6 == textarea SetWidth argument in view.go
	w := max(10, width-6)

	t.Run("single line", func(t *testing.T) {
		input := newTestComposer(t)
		input.SetWidth(w)
		if got := lipgloss.Width(input.View()); got != w {
			t.Errorf("lipgloss.Width(input.View()) = %d, want %d", got, w)
		}
	})

	t.Run("grown to 8 rows", func(t *testing.T) {
		input := newTestComposer(t)
		input.SetWidth(w)
		input.SetValue("line1\nline2\nline3\nline4\nline5\nline6\nline7\nline8")
		for _, line := range strings.Split(input.View(), "\n") {
			if got := lipgloss.Width(line); got != w {
				t.Errorf("lipgloss.Width(line) = %d for %q, want %d", got, line, w)
			}
		}
	})
}
