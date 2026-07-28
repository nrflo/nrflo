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
	bgText := ""
	if m.bgRunning > 0 {
		bgText = fmt.Sprintf("  bg:%d", m.bgRunning)
	}
	allowedText := ""
	if len(m.detail.SessionApprovals) > 0 {
		allowedText = "  always:" + strings.Join(m.detail.SessionApprovals, ",")
	}
	yoloText := ""
	if m.detail.Yolo {
		yoloText = "  " + lipgloss.NewStyle().Foreground(bad).Render("YOLO")
	}
	modelName := m.detail.Model
	if modelName == "" {
		modelName = "default"
	}
	bar := headerStyle.Render(" nrflo") + mutedStyle.Render(fmt.Sprintf("  %s / %s  %s  %s%s%s%s%s", m.detail.Engine, modelName, m.detail.ProjectID, connection, contextText, costText, bgText, allowedText)) + yoloText
	if m.width <= 0 {
		return bar
	}
	// Single physical row always: a wrapped status bar breaks the chrome
	// height math (clampChrome counts "\n" lines, terminals count cells).
	return truncate(bar, max(10, m.width-1))
}
