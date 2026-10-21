package consoleui

import (
	"strings"

	"charm.land/lipgloss/v2"
)

func toolLabelStyle(name string) lipgloss.Style {
	color := "111"
	switch strings.ToLower(name) {
	case "bash", "shell", "exec_command", "write_stdin":
		color = "214"
	case "read", "read_file", "glob", "grep", "search", "list_dir":
		color = "81"
	case "edit", "write", "multiedit", "apply_patch", "write_file":
		color = "114"
	case "skill":
		color = "213"
	case "task", "agent", "delegate", "spawn_agent":
		color = "141"
	}
	return lipgloss.NewStyle().Foreground(lipgloss.Color(color)).Bold(true)
}

func highlightToolLabel(content, prefix string, bodyStyle lipgloss.Style) string {
	if !strings.HasPrefix(content, prefix+"[") {
		return bodyStyle.Render(content)
	}
	name, _ := splitToolRow(content[len(prefix):])
	if name == "" {
		return bodyStyle.Render(content)
	}
	end := len(prefix) + len(name) + 2
	return bodyStyle.Render(content[:len(prefix)]) + toolLabelStyle(name).Render(content[len(prefix):end]) + bodyStyle.Render(content[end:])
}

func highlightMessageLabel(category, content string) string {
	bodyStyle := styleFor(category)
	switch category {
	case "tool", "tool_use", "tool_result", "subagent":
		return highlightToolLabel(content, "", bodyStyle)
	case "task_notification", "system_turn":
		label, _, _ := strings.Cut(content, " · ")
		return toolLabelStyle("").Render(label) + bodyStyle.Render(content[len(label):])
	default:
		return bodyStyle.Render(content)
	}
}
