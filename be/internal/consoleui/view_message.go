package consoleui

import (
	"strconv"
	"strings"
	"time"

	"charm.land/glamour/v2"
	glamouransi "charm.land/glamour/v2/ansi"
	"charm.land/glamour/v2/styles"
	"charm.land/lipgloss/v2"
)

// assistantGlamour is DarkStyleConfig with the document color lifted to match
// assistantStyle, so glamour body text doesn't read as the same gray as tool
// rows.
var assistantGlamour = func() glamouransi.StyleConfig {
	cfg := styles.DarkStyleConfig
	color := "254"
	cfg.Document.Color = &color
	return cfg
}()

// timePrefixWidth is the printed width of "HH:MM:SS " — the muted local-time
// prefix on every transcript row.
const timePrefixWidth = 9

// timePrefix formats a message's persisted created_at as a local "HH:MM:SS "
// prefix; "" (no prefix) when the timestamp is absent or unparseable.
func timePrefix(createdAt string) string {
	t, err := time.Parse(time.RFC3339Nano, createdAt)
	if err != nil {
		return ""
	}
	return t.Local().Format("15:04:05") + " "
}

// renderMessage renders a transcript row for printing: the body (rendered at
// width minus the timestamp column) prefixed on its first line with the muted
// local created_at time, continuation lines indented to align. Rows without a
// parseable timestamp — and terminals too narrow for the column — render the
// body alone at full width. Every line stays tab-free and within width:
// bubbletea's insertAbove row math desyncs otherwise (ansi.StringWidth counts
// "\t" as 0 while the terminal advances to the next tab stop) and ghost rows
// of the live frame leak into native scrollback.
func renderMessage(message Message, width int) string {
	prefix := timePrefix(message.CreatedAt)
	if prefix == "" || width-timePrefixWidth < 20 {
		return renderMessageBody(message, width)
	}
	body := renderMessageBody(message, width-timePrefixWidth)
	if body == "" {
		return ""
	}
	pad := strings.Repeat(" ", timePrefixWidth)
	lines := strings.Split(body, "\n")
	for i := range lines {
		if i == 0 {
			lines[i] = mutedStyle.Render(prefix) + lines[i]
		} else if lines[i] != "" {
			lines[i] = pad + lines[i]
		}
	}
	return strings.Join(lines, "\n")
}

func renderMessageBody(message Message, width int) string {
	switch message.Category {
	case "user_input":
		return userStyle.Render(fitWidth(message.Content, width))
	case "tool", "tool_use", "tool_result", "subagent":
		return mutedStyle.Render(toolCard(message.Content, width))
	case "thinking":
		return mutedStyle.Italic(true).Render(fitWidth("thinking · "+message.Content, width))
	case "system_notice":
		return ""
	case "task_notification":
		return mutedStyle.Render(fitWidth(collapseTaskNotification(message.Content), width))
	case "system_turn":
		return mutedStyle.Render(fitWidth(collapseServerNotice(message.Content), width))
	default:
		renderer, err := glamour.NewTermRenderer(glamour.WithStyles(assistantGlamour), glamour.WithWordWrap(width))
		if err == nil {
			if rendered, renderErr := renderer.Render(message.Content); renderErr == nil {
				return fitWidth(strings.TrimSpace(rendered), width)
			}
		}
		return assistantStyle.Render(fitWidth(message.Content, width))
	}
}

func truncate(value string, width int) string {
	value = strings.ReplaceAll(value, "\n", " ")
	if lipgloss.Width(value) <= width {
		return value
	}
	return lipgloss.NewStyle().MaxWidth(max(1, width-1)).Render(value) + "…"
}

// itoa is a tiny non-negative int → string helper for test fixtures.
func itoa(n int) string {
	return strconv.Itoa(n)
}
