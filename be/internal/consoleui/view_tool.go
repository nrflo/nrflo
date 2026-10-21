package consoleui

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/charmbracelet/x/ansi"
)

const toolPreviewRows = 12
const toolPreviewBytes = 16 * 1024

func isToolMessage(category string) bool {
	switch category {
	case "tool", "tool_use", "tool_result", "subagent":
		return true
	}
	return false
}

func toolInput(message Message) map[string]json.RawMessage {
	var payload struct {
		Input map[string]json.RawMessage `json:"input"`
	}
	_ = json.Unmarshal([]byte(message.Payload), &payload)
	return payload.Input
}

func inputText(input map[string]json.RawMessage, keys ...string) string {
	for _, key := range keys {
		var value string
		if json.Unmarshal(input[key], &value) == nil && value != "" {
			return value
		}
	}
	return ""
}

func renderToolMessage(message Message, width int) string {
	message.Content = previewText(message.Content)
	if strings.TrimSpace(message.Content) == "" {
		return ""
	}
	name, rest := splitToolRow(message.Content)
	if name == "" {
		return toolPreview(renderStyledText(toolTextStyle, rest), width)
	}
	input := toolInput(message)
	label := toolLabelStyle(name).Render("[" + name + "]")
	var body string
	switch strings.ToLower(name) {
	case "bash", "shell", "exec_command":
		command := inputText(input, "command", "cmd")
		output := ""
		if command != "" {
			if strings.HasPrefix(rest, command) {
				output = strings.TrimPrefix(rest, command)
			}
		} else {
			command, output, _ = strings.Cut(rest, "\n")
			if output != "" {
				output = "\n" + output
			}
		}
		if strings.HasPrefix(rest, "→") || message.Category == "tool_result" {
			body = renderStyledText(toolTextStyle, rest)
		} else {
			body = highlightSyntax(command, "command.sh", toolTextStyle) + renderStyledText(toolTextStyle, output)
		}
	case "edit", "edit_file", "write", "multiedit", "apply_patch", "write_file":
		body = renderEditPreview(input, rest)
	default:
		body = renderStyledText(toolTextStyle, rest)
	}
	if body == "" {
		return toolPreview(label, width)
	}
	// Commands wrap under their header; subsequent output/diff lines are indented.
	return toolPreview(label+" "+body, width)
}

func toolPreview(content string, width int) string {
	width = max(1, width)
	content = strings.ToValidUTF8(content, "�")
	indent := min(2, width-1)
	lines := strings.Split(fitWidth(content, width-indent), "\n")
	hidden := max(0, len(lines)-toolPreviewRows)
	if hidden > 0 {
		lines = lines[:toolPreviewRows]
	}
	for i := 1; i < len(lines); i++ {
		lines[i] = strings.Repeat(" ", indent) + lines[i]
	}
	if hidden > 0 {
		marker := fmt.Sprintf("… %d more lines", hidden)
		lines = append(lines, mutedStyle.Render(ansi.Truncate(marker, width, "…")))
	}
	return strings.Join(lines, "\n")
}

func previewText(text string) string {
	text = ansi.Strip(text)
	if len(text) > toolPreviewBytes {
		text = strings.ToValidUTF8(text[:toolPreviewBytes], "") + "\n… preview truncated"
	}
	return text
}
