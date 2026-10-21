package consoleui

import (
	"strings"

	"charm.land/lipgloss/v2"
)

// renderMessage uses the full transcript width; timing is shown once per turn.
func renderMessage(message Message, width int) string {
	return renderMessageBody(message, width)
}

// renderMessageBody is the single transcript renderer: preRender reduces the
// row's content where raw text isn't human prose (notice envelopes collapse
// to one line, tools render bounded blocks), then fitWidth
// applies the shared wrap/clip invariant before role and label colors.
func renderMessageBody(message Message, width int) string {
	if isToolMessage(message.Category) {
		return renderToolMessage(message, width)
	}
	content := preRender(message)
	if content == "" {
		return ""
	}
	content = fitWidth(content, width)
	return highlightMessageLabel(message.Category, content)
}

// preRender maps a message's content to its display text before the shared
// wrap/clip/style pass:
//
//   - system_notice rows are server-internal and never print (printNewMessages
//     skips the empty result; does not consume the watermark oddly).
//   - task_notification / system_turn envelopes collapse to their one-line
//     summary (the model-facing instruction tail is useless to a human).
//   - everything else (user_input, thinking, assistant) passes through as-is.
func preRender(message Message) string {
	switch message.Category {
	case "system_notice":
		return ""
	case "task_notification":
		return collapseTaskNotification(message.Content)
	case "system_turn":
		return collapseServerNotice(message.Content)

	default:
		// Trim outer whitespace: a whitespace-only row must still render ""
		// (printNewMessages skips empty renders), matching the previous
		// glamour pipeline's TrimSpace on its rendered document.
		return strings.TrimSpace(message.Content)
	}
}

// splitToolRow splits a "[Name] rest" row into its bracketed name and trimmed
// remainder. Rows without a leading "[" (the "Name: err" error-row shape from
// spawner/tool_format.go) have no name.
func splitToolRow(content string) (name, rest string) {
	if !strings.HasPrefix(content, "[") {
		return "", content
	}
	idx := strings.Index(content, "]")
	if idx < 0 {
		return "", content
	}
	return content[1:idx], strings.TrimSpace(content[idx+1:])
}

// styleFor returns the single role color for a transcript row: user light
// blue, assistant near-white, tools/callbacks muted cyan, thinking dim gray. Role is
// conveyed by color alone — no "you"/"assistant" header rows.
func styleFor(category string) lipgloss.Style {
	switch category {
	case "user_input":
		return userStyle
	case "turn_summary":
		return mutedStyle
	case "thinking":
		return mutedStyle.Italic(true)
	case "tool", "tool_use", "tool_result", "subagent", "task_notification", "system_turn":
		return toolTextStyle
	default:
		return assistantStyle
	}
}

// truncate collapses newlines and clips value to width, appending the
// ellipsis marker when cut (chrome/footer/status-bar helper).
func truncate(value string, width int) string {
	value = strings.ReplaceAll(value, "\n", " ")
	if lipgloss.Width(value) <= width {
		return value
	}
	return lipgloss.NewStyle().MaxWidth(max(1, width-1)).Render(value) + "…"
}
