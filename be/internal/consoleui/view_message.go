package consoleui

import (
	"strings"
	"time"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
)

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

// renderMessage renders a transcript row for printing: the body rendered by
// ONE pipeline — category-specific pre-render (collapse/cap), then
// expandTabs → ANSI word-wrap → clip, then role color — prefixed on its first
// line with the muted local created_at time, continuation lines indented to
// align. Rows without a parseable timestamp — and terminals too narrow for
// the column — render the body alone at full width. Every line stays tab-free
// and within width: bubbletea's insertAbove row math desyncs otherwise
// (ansi.StringWidth counts "\t" as 0 while the terminal advances to the next
// tab stop) and ghost rows of the live frame leak into native scrollback.
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

// renderMessageBody is the single transcript renderer: preRender reduces the
// row's content where raw text isn't human prose (notice envelopes collapse
// to one line, tool-family rows collapse to ONE clipped line), then fitWidth
// applies the shared wrap/clip invariant and styleFor colors by role alone.
func renderMessageBody(message Message, width int) string {
	content := preRender(message, width)
	if content == "" {
		return ""
	}
	return styleFor(message.Category).Render(fitWidth(content, width))
}

// preRender maps a message's content to its display text before the shared
// wrap/clip/style pass:
//
//   - system_notice rows are server-internal and never print (printNewMessages
//     skips the empty result; does not consume the watermark oddly).
//   - task_notification / system_turn envelopes collapse to their one-line
//     summary (the model-facing instruction tail is useless to a human).
//   - tool-family rows collapse to ONE line: the "tool · [Name]" head plus the
//     payload's first line, ellipsis-marked when more followed — the full
//     payload lives in the web UI's tool card; scrollback only needs the gist.
//   - everything else (user_input, thinking, assistant) passes through as-is.
func preRender(message Message, width int) string {
	switch message.Category {
	case "system_notice":
		return ""
	case "task_notification":
		return collapseTaskNotification(message.Content)
	case "system_turn":
		return collapseServerNotice(message.Content)
	case "tool", "tool_use", "tool_result", "subagent":
		name, rest := splitToolRow(message.Content)
		var b strings.Builder
		b.WriteString(toolRowPrefix)
		if name != "" {
			b.WriteString("[" + name + "]")
		}
		if rest = headLine(rest); rest != "" {
			b.WriteString(" ")
			b.WriteString(rest)
		}
		// Tabs are expanded BEFORE the width clip: ansi.StringWidth counts a
		// tab as zero, so an unexpanded tab measures short here and then
		// widens under fitWidth's expandTabs, wrapping the row onto extra
		// physical lines — exactly what the one-line collapse exists to stop.
		return clipOneLine(expandTabs(b.String()), width)
	default:
		// Trim outer whitespace: a whitespace-only row must still render ""
		// (printNewMessages skips empty renders), matching the previous
		// glamour pipeline's TrimSpace on its rendered document.
		return strings.TrimSpace(message.Content)
	}
}

// toolRowPrefix is the single prefix shared by every tool-family transcript
// row (tool, tool_use, tool_result, subagent) so they read as one kind of
// row.
const toolRowPrefix = "tool · "

// headLine collapses a tool payload to its first line: leading blank lines
// are dropped, everything after the first "\n" is cut, and the cut is marked
// with forceEllipsis so a multi-line payload is visibly truncated rather than
// silently shortened. The width cap itself still comes from the shared
// fitWidth pass.
func headLine(body string) string {
	body = strings.TrimLeft(body, " \t\r\n")
	if i := strings.IndexByte(body, '\n'); i >= 0 {
		// fitWidth runs after preRender, so use a generous width here and let
		// the shared clip enforce the real terminal width on this line.
		return forceEllipsis(strings.TrimRight(body[:i], " \t\r"), maxInlineHeadWidth)
	}
	return strings.TrimRight(body, " \t\r")
}

// maxInlineHeadWidth bounds the ellipsis-marked head line before the shared
// wrap/clip pass; generous, since fitWidth does the final clipping.
const maxInlineHeadWidth = 4096

// forceEllipsis rewrites a clipped line's tail to end in an ellipsis while
// keeping it within width.
func forceEllipsis(line string, width int) string {
	width = max(1, width)
	if width == 1 {
		return "…"
	}
	return lipgloss.NewStyle().MaxWidth(width-1).Render(line) + "…"
}

// clipOneLine hard-clips s to width with a trailing '…' when cut — the
// tool-row counterpart of fitWidth's word-wrap: one logical row must stay ONE
// physical row, so overflow is clipped rather than wrapped.
func clipOneLine(s string, width int) string {
	width = max(1, width)
	if ansi.StringWidth(s) <= width {
		return s
	}
	return forceEllipsis(ansi.Truncate(s, width-1, ""), width)
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
// blue, assistant near-white, tools/thinking/notices dim gray. Role is
// conveyed by color alone — no "you"/"assistant" header rows.
func styleFor(category string) lipgloss.Style {
	switch category {
	case "user_input":
		return userStyle
	case "thinking":
		return mutedStyle.Italic(true)
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
