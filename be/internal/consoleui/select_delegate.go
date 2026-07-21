package consoleui

import (
	"fmt"
	"io"

	"charm.land/bubbles/v2/list"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
)

// selectedPrefixStyle marks the highlighted row in the compact picker; the
// rest of the emphasis comes from headerStyle/mutedStyle (view.go).
var selectedPrefixStyle = lipgloss.NewStyle().Foreground(accent).Bold(true)

// compactDelegate is a single-line list.ItemDelegate for the console
// resume/brand/model/mode/effort picker: every level of the drill-down
// reuses one list.Model, so this delegate applies uniformly at every depth.
// Height 1 + Spacing 0 is what makes it compact (list.DefaultDelegate is
// Height 2 + Spacing 1).
type compactDelegate struct{}

func (compactDelegate) Height() int  { return 1 }
func (compactDelegate) Spacing() int { return 0 }

func (compactDelegate) Update(tea.Msg, *list.Model) tea.Cmd { return nil }

func (compactDelegate) Render(w io.Writer, m list.Model, index int, item list.Item) {
	di, ok := item.(list.DefaultItem)
	if !ok {
		return
	}
	if m.Width() <= 0 {
		return
	}
	selected := index == m.Index() && m.FilterState() != list.Filtering
	fmt.Fprint(w, compactRow(di.Title(), di.Description(), m.Width(), selected)) //nolint:errcheck
}

// compactRow composes one picker row — "<name>  <muted description>" — onto
// a single line, truncated to width via truncate (view.go). selected renders
// the row with the accent selection prefix/highlight used elsewhere in the
// picker; unselected rows dim the description via mutedStyle.
func compactRow(title, detail string, width int, selected bool) string {
	prefix := "  "
	if selected {
		prefix = selectedPrefixStyle.Render("> ")
	}
	name := title
	if selected {
		name = headerStyle.Render(name)
	}
	line := name
	if detail != "" {
		line = name + "  " + mutedStyle.Render(detail)
	}
	budget := width - lipgloss.Width(prefix)
	return prefix + truncate(line, max(1, budget))
}
