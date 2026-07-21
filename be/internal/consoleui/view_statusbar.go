package consoleui

import (
	"fmt"
	"strings"

	"charm.land/lipgloss/v2"
)

// statusBar renders the bottom status line (relocated from the former
// top header()): engine/model, project id, connection, context%, cost
// estimate, and the session's always-allowed tool list.
func (m *model) statusBar() string {
	connection := lipgloss.NewStyle().Foreground(bad).Render("offline")
	if m.connected {
		connection = lipgloss.NewStyle().Foreground(good).Render("connected")
	}
	contextText := ""
	if m.detail.ContextLeft != nil {
		contextText = fmt.Sprintf("  context %d%%", *m.detail.ContextLeft)
	}
	costText := ""
	if m.detail.CostEstimate != nil {
		costText = fmt.Sprintf("  ~$%.2f", *m.detail.CostEstimate)
	}
	allowedText := ""
	if len(m.detail.SessionApprovals) > 0 {
		allowedText = "  always:" + strings.Join(m.detail.SessionApprovals, ",")
	}
	modelName := m.detail.Model
	if modelName == "" {
		modelName = "default"
	}
	return headerStyle.Render(" nrflo") + mutedStyle.Render(fmt.Sprintf("  %s / %s  %s  %s%s%s%s", m.detail.Engine, modelName, m.detail.ProjectID, connection, contextText, costText, allowedText))
}

// suggestionView renders the bordered "/" skill-suggestion box above the
// composer, highlighting the currently selected row.
func (m *model) suggestionView() string {
	matches := m.suggestionMatches()
	if len(matches) > maxSuggestionRows {
		matches = matches[:maxSuggestionRows]
	}
	active := m.skillIndex
	if active < 0 || active >= len(matches) {
		active = 0
	}
	rows := make([]string, 0, len(matches))
	for i, skill := range matches {
		line := "/" + skill.Name
		if skill.Description != "" {
			line += " — " + skill.Description
		}
		if i == active {
			line = lipgloss.NewStyle().Bold(true).Foreground(accent).Render(line)
		} else {
			line = mutedStyle.Render(line)
		}
		rows = append(rows, line)
	}
	return approvalBox.BorderForeground(accent).Width(max(1, m.width-2)).Render(strings.Join(rows, "\n"))
}

// suggestionRows returns the rendered row count for n matches, capped at
// maxSuggestionRows (used by chromeRows without re-rendering the box).
func suggestionRows(n int) int {
	if n <= 0 {
		return 0
	}
	if n > maxSuggestionRows {
		n = maxSuggestionRows
	}
	return n
}

// composerContentRows returns the composer's current content height: the
// textarea's dynamic height in normal mode, or a fixed single row for the
// search/copy-mode placeholder text.
func (m *model) composerContentRows() int {
	if m.searchMode || m.copyMode {
		return 1
	}
	return m.input.Height()
}

// chromeRows totals the fixed (non-viewport) rows the layout reserves:
// composer box (content + 2 border rows), the "/" suggestion box when open
// (content + 2 border rows), the approval box when present, plus the
// status bar and footer lines.
func chromeRows(composerContent, suggestionMatches, approvalCount int) int {
	rows := composerContent + 2 // composerBox border top/bottom
	if suggestionMatches > 0 {
		rows += suggestionRows(suggestionMatches) + 2 // suggestion box border
	}
	if approvalCount > 0 {
		rows += 3 // approvalBox
	}
	rows += 2 // statusBar + footer
	return rows
}

// relayout resizes the viewport from the actual composer/suggestion/approval
// chrome instead of a fixed magic constant, then re-renders the transcript.
func (m *model) relayout() {
	suggestionMatches := 0
	if !m.searchMode && !m.copyMode && m.suggestionsOpen() {
		suggestionMatches = len(m.suggestionMatches())
	}
	chrome := chromeRows(m.composerContentRows(), suggestionMatches, len(m.approvals))
	m.viewport.SetWidth(max(1, m.width))
	m.viewport.SetHeight(max(1, m.height-chrome))
	m.refreshTranscript()
}
