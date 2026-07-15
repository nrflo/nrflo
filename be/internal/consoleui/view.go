package consoleui

import (
	"fmt"
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/glamour/v2"
	"charm.land/lipgloss/v2"
)

var (
	accent      = lipgloss.Color("63")
	muted       = lipgloss.Color("244")
	good        = lipgloss.Color("42")
	warn        = lipgloss.Color("214")
	bad         = lipgloss.Color("196")
	headerStyle = lipgloss.NewStyle().Bold(true).Foreground(accent)
	mutedStyle  = lipgloss.NewStyle().Foreground(muted)
	userStyle   = lipgloss.NewStyle().Foreground(lipgloss.Color("117")).Bold(true)
	errorStyle  = lipgloss.NewStyle().Foreground(bad)
	approvalBox = lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(warn).Padding(0, 1)
	composerBox = lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(accent).Padding(0, 1)
)

func (m *model) View() tea.View {
	if !m.ready {
		view := tea.NewView("Starting nrflo console…")
		view.AltScreen = true
		return view
	}
	sections := []string{m.header(), m.viewport.View()}
	if len(m.approvals) > 0 {
		sections = append(sections, m.approvalView())
	}
	sections = append(sections, composerBox.Width(max(1, m.width-2)).Render(m.input.View()), m.footer())
	view := tea.NewView(lipgloss.JoinVertical(lipgloss.Left, sections...))
	view.AltScreen = true
	view.MouseMode = tea.MouseModeCellMotion
	view.WindowTitle = "nrflo console"
	return view
}

func (m *model) header() string {
	connection := lipgloss.NewStyle().Foreground(bad).Render("offline")
	if m.connected {
		connection = lipgloss.NewStyle().Foreground(good).Render("connected")
	}
	contextText := ""
	if m.detail.ContextLeft != nil {
		contextText = fmt.Sprintf("  context %d%%", *m.detail.ContextLeft)
	}
	modelName := m.detail.Model
	if modelName == "" {
		modelName = "default"
	}
	return headerStyle.Render(" nrflo") + mutedStyle.Render(fmt.Sprintf("  %s / %s  %s  %s%s", m.detail.Engine, modelName, m.detail.ProjectID, connection, contextText))
}

func (m *model) footer() string {
	help := "enter send · shift+enter/ctrl+j newline · ctrl+d quit"
	if m.status == "running" {
		help = "working… · ctrl+c interrupt · ctrl+d quit"
	}
	if m.lastErr != "" {
		return errorStyle.Render(" " + truncate(m.lastErr, max(20, m.width-2)))
	}
	return mutedStyle.Render(" " + help)
}

func (m *model) approvalView() string {
	a := m.approvals[0]
	detail := a.Command
	if detail == "" {
		detail = a.Reason
	}
	text := lipgloss.NewStyle().Bold(true).Foreground(warn).Render("Approval required") +
		"  " + truncate(detail, max(20, m.width-28)) + "  " +
		lipgloss.NewStyle().Bold(true).Render("[y] allow  [n] deny")
	return approvalBox.Width(max(1, m.width-2)).Render(text)
}

func (m *model) resize(width, height int) {
	m.width, m.height, m.ready = width, height, true
	m.input.SetWidth(max(10, width-6))
	approvalHeight := 0
	if len(m.approvals) > 0 {
		approvalHeight = 3
	}
	m.viewport.SetWidth(max(1, width))
	m.viewport.SetHeight(max(1, height-8-approvalHeight))
	m.refreshTranscript()
}

func (m *model) refreshTranscript() {
	if !m.ready {
		return
	}
	atBottom := m.viewport.AtBottom()
	contentWidth := max(20, m.width-4)
	if m.historyDirty || m.renderedWidth != contentWidth {
		history := make([]string, 0, len(m.messages))
		for _, message := range m.messages {
			history = append(history, renderMessage(message, contentWidth))
		}
		m.renderedHistory = strings.Join(history, "\n\n")
		m.renderedWidth = contentWidth
		m.historyDirty = false
	}
	parts := make([]string, 0, len(m.deltas)+2)
	if m.renderedHistory != "" {
		parts = append(parts, m.renderedHistory)
	}
	for _, id := range m.deltaOrder {
		if text := m.deltas[id]; text != "" {
			parts = append(parts, headerStyle.Render("assistant")+"\n"+text)
		}
	}
	if m.thinking != "" {
		parts = append(parts, mutedStyle.Italic(true).Render("thinking · "+m.thinking))
	}
	m.viewport.SetContent(strings.Join(parts, "\n\n"))
	if atBottom || len(m.messages) <= 1 {
		m.viewport.GotoBottom()
	}
}

func renderMessage(message Message, width int) string {
	switch message.Category {
	case "user_input":
		return userStyle.Render("you") + "\n" + message.Content
	case "tool", "tool_use", "tool_result":
		return mutedStyle.Render("tool · " + message.Content)
	case "thinking":
		return mutedStyle.Italic(true).Render("thinking · " + message.Content)
	default:
		renderer, err := glamour.NewTermRenderer(glamour.WithStandardStyle("dark"), glamour.WithWordWrap(width))
		if err == nil {
			if rendered, renderErr := renderer.Render(message.Content); renderErr == nil {
				return headerStyle.Render("assistant") + "\n" + strings.TrimSpace(rendered)
			}
		}
		return headerStyle.Render("assistant") + "\n" + message.Content
	}
}

func truncate(value string, width int) string {
	value = strings.ReplaceAll(value, "\n", " ")
	if lipgloss.Width(value) <= width {
		return value
	}
	return lipgloss.NewStyle().MaxWidth(max(1, width-1)).Render(value) + "…"
}
