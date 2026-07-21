package consoleui

import (
	"fmt"
	"strings"

	"charm.land/lipgloss/v2"
)

// invokeView renders the /invoke flow's chrome for the current phase: an
// args-phase field prompt, or a confirm-phase one-liner, both in the
// approvalBox style (mirrors approvalView, view.go:70).
func (m *model) invokeView() string {
	var line string
	switch m.invoke.phase {
	case invokePhaseArgs:
		line = m.invokeArgLine()
	case invokePhaseConfirm:
		line = m.invokeConfirmLine()
	default:
		return ""
	}
	return approvalBox.BorderForeground(accent).Width(max(1, m.width-2)).Render(line)
}

func (m *model) invokeArgLine() string {
	if m.invoke.index < 0 || m.invoke.index >= len(m.invoke.fields) {
		return ""
	}
	field := m.invoke.fields[m.invoke.index]
	parts := []string{field.Type}
	if field.Required {
		parts = append(parts, "required")
	}
	if field.Type == "object" {
		parts = append(parts, "JSON")
	}
	label := fmt.Sprintf("%s (%s):", field.Name, strings.Join(parts, ", "))
	return lipgloss.NewStyle().Bold(true).Foreground(accent).Render(label) +
		"  " + mutedStyle.Render("enter accept · esc cancel")
}

func (m *model) invokeConfirmLine() string {
	toggle := "off"
	if m.invoke.inform {
		toggle = "on"
	}
	return lipgloss.NewStyle().Bold(true).Foreground(warn).Render("run "+m.invoke.tool+"?") +
		"  " + lipgloss.NewStyle().Bold(true).Render(fmt.Sprintf("[y] run · [i] toggle inform (%s) · [esc] cancel", toggle))
}
