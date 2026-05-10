package notify

import (
	"fmt"
	"strings"

	"be/internal/ws"
)

// NotificationBaseURL is the URL prefix used for ticket / project-workflow links
// embedded in notification bodies. Overridable in tests or via wiring.
var NotificationBaseURL = "http://localhost:6587"

// renderSlack builds a Slack mrkdwn message for the given event.
func renderSlack(eventType string, data map[string]interface{}) string {
	header := buildHeader(eventType, data, slackLink, func(s string) string { return s })
	details := eventDetails(data)

	if eventType == ws.EventOrchestrationCompleted {
		if summary, ok := data["workflow_final_result"].(string); ok && summary != "" {
			block := renderSummaryBlock(summary, func(s string) string { return s })
			if details != "" {
				return header + "\n" + block + "\n" + details
			}
			return header + "\n" + block
		}
	}

	if details != "" {
		return header + "\n" + details
	}
	return header
}

// renderTelegram builds a Telegram MarkdownV2 message for the given event.
func renderTelegram(eventType string, data map[string]interface{}) string {
	header := buildHeader(eventType, data, telegramLink, escapeTelegramV2)
	details := escapeTelegramV2(eventDetails(data))

	if eventType == ws.EventOrchestrationCompleted {
		if summary, ok := data["workflow_final_result"].(string); ok && summary != "" {
			block := renderSummaryBlock(summary, escapeTelegramV2)
			if details != "" {
				return header + "\n" + block + "\n" + details
			}
			return header + "\n" + block
		}
	}

	if details != "" {
		return header + "\n" + details
	}
	return header
}

// buildHeader produces the first line: "*nrflo* — <event-type> <scope-link>".
// linkFn formats a (text, url) pair into the platform-specific markup; escape
// is applied to non-link text segments.
func buildHeader(eventType string, data map[string]interface{}, linkFn func(text, url string) string, escape func(string) string) string {
	scope := scopeLink(data, linkFn, escape)
	header := "*nrflo* — " + escape(eventType)
	if scope != "" {
		header = header + " " + scope
	}
	return header
}

// scopeLink returns the markup for the ticket or project-workflow link, or "".
// For ticket-scoped events, links to /tickets/<id>; for project-scoped, links
// to /project-workflows?instance_id=<id> when instance_id is present.
func scopeLink(data map[string]interface{}, linkFn func(text, url string) string, escape func(string) string) string {
	if ticketID, _ := data["ticket_id"].(string); ticketID != "" {
		url := fmt.Sprintf("%s/tickets/%s", NotificationBaseURL, ticketID)
		return linkFn(escape(ticketID), url)
	}
	if instanceID, _ := data["instance_id"].(string); instanceID != "" {
		url := fmt.Sprintf("%s/project-workflows?instance_id=%s", NotificationBaseURL, instanceID)
		return linkFn(escape("project"), url)
	}
	return ""
}

// slackLink renders <url|text> for Slack mrkdwn.
func slackLink(text, url string) string {
	return fmt.Sprintf("<%s|%s>", url, text)
}

// telegramLink renders [text](url) for Telegram MarkdownV2. Inside the URL, only
// '\' and ')' need escaping. The text is expected to be already escaped.
func telegramLink(text, url string) string {
	r := strings.NewReplacer(`\`, `\\`, `)`, `\)`)
	return fmt.Sprintf("[%s](%s)", text, r.Replace(url))
}

// renderSummaryBlock truncates, optionally escapes, and formats a summary as > -prefixed lines.
func renderSummaryBlock(summary string, escape func(string) string) string {
	truncated := truncateRunes(summary, 1500)
	escaped := escape(truncated)
	lines := strings.Split(escaped, "\n")
	var b strings.Builder
	for i, line := range lines {
		if i > 0 {
			b.WriteString("\n")
		}
		b.WriteString("> ")
		b.WriteString(line)
	}
	return b.String()
}

// truncateRunes truncates s to at most max runes, appending "…" when truncated.
func truncateRunes(s string, max int) string {
	runes := []rune(s)
	if len(runes) <= max {
		return s
	}
	return string(runes[:max]) + "…"
}

// eventDetails extracts extra detail fields into a readable string.
func eventDetails(data map[string]interface{}) string {
	var parts []string
	if v, ok := data["agent_type"].(string); ok && v != "" {
		parts = append(parts, "agent: "+v)
	}
	if v, ok := data["workflow"].(string); ok && v != "" {
		parts = append(parts, "workflow: "+v)
	}
	if v, ok := data["reason"].(string); ok && v != "" {
		parts = append(parts, "reason: "+v)
	}
	if v, ok := data["instance_id"].(string); ok && v != "" {
		parts = append(parts, "instance: "+v)
	}
	return strings.Join(parts, " | ")
}

// escapeTelegramV2 escapes MarkdownV2 special characters.
func escapeTelegramV2(s string) string {
	// Characters that must be escaped in MarkdownV2 (outside entity-level)
	special := `_[]()~>#+-=|{}.!`
	var b strings.Builder
	for _, c := range s {
		if strings.ContainsRune(special, c) {
			b.WriteRune('\\')
		}
		b.WriteRune(c)
	}
	return b.String()
}
