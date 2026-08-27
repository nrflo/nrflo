package consoleui

import (
	"fmt"
	"strings"

	"charm.land/lipgloss/v2"
)

// clampInt clamps v into [0, hi], returning 0 when hi < 0.
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

// suggestionView occupies the status bar's existing row. Completion must not
// grow the inline frame: shrinking a multi-row popup leaves stale frame-band
// padding between the transcript and composer after submission.
func (m *model) suggestionView() string {
	matches := m.suggestionMatches()
	selected := clampInt(m.skillIndex, len(matches)-1)
	if len(matches) == 0 {
		return ""
	}

	position := fmt.Sprintf(" %d/%d", selected+1, len(matches))
	width := max(1, m.width)
	if lipgloss.Width(position)+2 >= width {
		return mutedStyle.Render(truncate(position, width))
	}
	available := width - lipgloss.Width(position) - 1
	start, end := suggestionRange(matches, selected, available)
	items := make([]string, 0, end-start)
	for i := start; i < end; i++ {
		name := truncate("/"+matches[i].Name, available)
		if i == selected {
			items = append(items, headerStyle.Render(name))
		} else {
			items = append(items, mutedStyle.Render(name))
		}
	}
	return " " + strings.Join(items, "  ") + mutedStyle.Render(position)
}

// suggestionRange returns the largest contiguous name-only window around the
// selection that fits in width. The position marker communicates omitted
// matches while Up/Down keeps the selected item in view.
func suggestionRange(matches []suggestionItem, selected, width int) (int, int) {
	selected = clampInt(selected, len(matches)-1)
	if len(matches) == 0 || width < 1 {
		return 0, 0
	}
	start, end := selected, selected+1
	used := lipgloss.Width("/" + matches[selected].Name)
	for {
		grew := false
		if end < len(matches) {
			cost := 2 + lipgloss.Width("/"+matches[end].Name)
			if used+cost <= width {
				used += cost
				end++
				grew = true
			}
		}
		if start > 0 {
			cost := 2 + lipgloss.Width("/"+matches[start-1].Name)
			if used+cost <= width {
				used += cost
				start--
				grew = true
			}
		}
		if !grew {
			break
		}
	}
	return start, end
}
