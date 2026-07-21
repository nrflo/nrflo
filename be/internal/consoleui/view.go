package consoleui

import (
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
	sections := []string{m.viewport.View()}
	if len(m.approvals) > 0 {
		sections = append(sections, m.approvalView())
	}
	composer := m.input.View()
	normalMode := !m.searchMode && !m.copyMode
	if m.searchMode {
		composer = m.search.View()
	} else if m.copyMode {
		composer = "copy mode · navigate with arrows/j/k · y copies visible text · ctrl+p loads older"
	}
	if normalMode && m.suggestionsOpen() {
		sections = append(sections, m.suggestionView())
	}
	if normalMode && m.invoke.active {
		sections = append(sections, m.invokeView())
	}
	sections = append(sections, composerBox.Width(max(1, m.width-2)).Render(composer), m.statusBar(), m.footer())
	view := tea.NewView(lipgloss.JoinVertical(lipgloss.Left, sections...))
	view.AltScreen = true
	view.WindowTitle = "nrflo console"
	return view
}

func (m *model) footer() string {
	help := "enter send · ctrl+f search · ctrl+g copy mode · ctrl+d detach · ctrl+x close · pgup/pgdn scroll"
	if m.status == "running" {
		help = "working… · ctrl+c interrupt · ctrl+d detach · ctrl+x close"
	}
	if m.lastErr != "" {
		return errorStyle.Render(" " + truncate(m.lastErr, max(20, m.width-2)))
	}
	if m.notice != "" {
		return mutedStyle.Render(" " + m.notice)
	}
	if m.searchStatus != "" {
		return mutedStyle.Render(" " + m.searchStatus + " · F3/Shift+F3 next/previous")
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
		lipgloss.NewStyle().Bold(true).Render("[y] allow  [a] always  [n] deny")
	return approvalBox.Width(max(1, m.width-2)).Render(text)
}

func (m *model) resize(width, height int) {
	m.width, m.height, m.ready = width, height, true
	m.input.SetWidth(max(10, width-6))
	m.relayout()
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
			key := renderCacheKey(message, contentWidth)
			rendered, ok := m.renderCache[key]
			if !ok {
				rendered = renderMessage(message, contentWidth)
				m.renderCache[key] = rendered
			}
			history = append(history, rendered)
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
	if m.status == "running" {
		parts = append(parts, m.spin.View()+mutedStyle.Render(" working…"))
	}
	m.viewport.SetContent(strings.Join(parts, "\n\n"))
	if m.searchStatus != "" {
		m.applySearch()
	}
	if atBottom || len(m.messages) <= 1 {
		m.viewport.GotoBottom()
	}
}

func renderMessage(message Message, width int) string {
	switch message.Category {
	case "user_input":
		return userStyle.Render("you") + "\n" + wrapToWidth(message.Content, width)
	case "tool", "tool_use", "tool_result":
		return mutedStyle.Render(wrapToWidth("tool · "+prettyToolContent(message.Content), width))
	case "thinking":
		return mutedStyle.Italic(true).Render(wrapToWidth("thinking · "+message.Content, width))
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
