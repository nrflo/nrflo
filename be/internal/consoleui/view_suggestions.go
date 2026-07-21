package consoleui

import (
	"fmt"
	"strings"

	"charm.land/lipgloss/v2"
)

// maxDetailLines caps the word-wrapped full-description block rendered when
// ctrl+o toggles details open (header line + up to maxDetailLines-1 body
// lines, the last marked truncated when the wrapped text overflows).
const maxDetailLines = 6

// clampInt clamps v into [0, hi], returning 0 when hi < 0 (empty range).
func clampInt(v, hi int) int {
	if hi < 0 {
		return 0
	}
	if v < 0 {
		return 0
	}
	if v > hi {
		return hi
	}
	return v
}

// suggestionWindowSize returns how many rows the "/" dropdown renders for
// total matches: all of them when they fit within maxSuggestionRows,
// otherwise one fewer to reserve a row for the "N/total" indicator line.
func suggestionWindowSize(total int) int {
	if total <= maxSuggestionRows {
		return total
	}
	return maxSuggestionRows - 1
}

// suggestionWindow computes the [start, end) slice of matches to render so
// the selected row stays visible: it centers the window on selected,
// clamping so the window never runs past [0, total). Guarantees
// start <= selected < end when total > 0 and size > 0.
func suggestionWindow(total, selected, size int) (int, int) {
	if size <= 0 || total <= 0 {
		return 0, 0
	}
	if total <= size {
		return 0, total
	}
	selected = clampInt(selected, total-1)
	start := selected - size/2
	start = clampInt(start, total-size)
	return start, start + size
}

// suggestionRows returns the rendered row count for n matches, capped at
// maxSuggestionRows (used by chromeRows without re-rendering the box). When
// n overflows the cap, the reserved indicator line already accounted for by
// suggestionWindowSize is included in the returned total.
func suggestionRows(n int) int {
	if n <= 0 {
		return 0
	}
	if n > maxSuggestionRows {
		n = maxSuggestionRows
	}
	return n
}

// detailLines renders the ctrl+o full-description block: a header line
// ("/name") followed by word-wrapped description lines, capped at
// maxDetailLines total (the last line is truncated with an ellipsis when the
// wrap overflows the cap). Pure function — no *model, no terminal — for unit
// testing.
func detailLines(name, description string, width int) []string {
	width = max(1, width)
	lines := []string{truncate("/"+name, width)}
	if description != "" {
		wrapped := lipgloss.NewStyle().Width(width).Render(description)
		lines = append(lines, strings.Split(wrapped, "\n")...)
	}
	if len(lines) > maxDetailLines {
		lines = lines[:maxDetailLines]
		lines[maxDetailLines-1] = truncate(lines[maxDetailLines-1], width)
	}
	return lines
}

// suggestionView renders the bordered "/" skill-suggestion box above the
// composer: a scrolling window of matches that follows the selected row
// (every row truncated to one terminal line so suggestionRows/chromeRows
// accounting stays exact), an overflow position indicator, and an optional
// ctrl+o full-description block.
func (m *model) suggestionView() string {
	matches := m.suggestionMatches()
	total := len(matches)
	selected := clampInt(m.skillIndex, total-1)
	inner := max(1, m.width-6)

	start, end := suggestionWindow(total, selected, suggestionWindowSize(total))
	rows := make([]string, 0, end-start+2)
	for i := start; i < end; i++ {
		skill := matches[i]
		line := "/" + skill.Name
		if skill.Description != "" {
			line += " — " + skill.Description
		}
		line = truncate(line, inner)
		if i == selected {
			rows = append(rows, lipgloss.NewStyle().Bold(true).Foreground(accent).Render(line))
		} else {
			rows = append(rows, mutedStyle.Render(line))
		}
	}
	if total > maxSuggestionRows {
		indicator := fmt.Sprintf(" %d/%d · ctrl+o details", selected+1, total)
		rows = append(rows, mutedStyle.Render(truncate(indicator, inner)))
	}
	if m.skillDetails && total > 0 {
		for _, line := range detailLines(matches[selected].Name, matches[selected].Description, inner) {
			rows = append(rows, mutedStyle.Render(line))
		}
	}
	return approvalBox.BorderForeground(accent).Width(max(1, m.width-2)).Render(strings.Join(rows, "\n"))
}
