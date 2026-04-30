package notify

import (
	"fmt"
	"strings"
)

// renderSlack builds a Slack mrkdwn message for the given event.
func renderSlack(eventType string, data map[string]interface{}) string {
	label := eventLabel(eventType, data)
	details := eventDetails(data)
	if details != "" {
		return fmt.Sprintf("*nrflo* — %s\n%s", label, details)
	}
	return fmt.Sprintf("*nrflo* — %s", label)
}

// renderTelegram builds a Telegram MarkdownV2 message for the given event.
func renderTelegram(eventType string, data map[string]interface{}) string {
	label := escapeTelegramV2(eventLabel(eventType, data))
	details := escapeTelegramV2(eventDetails(data))
	if details != "" {
		return fmt.Sprintf("*nrflo* — %s\n%s", label, details)
	}
	return fmt.Sprintf("*nrflo* — %s", label)
}

// eventLabel returns a human-readable summary line for the event type.
func eventLabel(eventType string, data map[string]interface{}) string {
	ticketID, _ := data["ticket_id"].(string)
	workflow, _ := data["workflow"].(string)
	agentType, _ := data["agent_type"].(string)
	scope := ticketID
	if scope == "" {
		scope = "project-scoped"
	}

	switch eventType {
	case "workflow.completed":
		return fmt.Sprintf("Workflow *%s* completed for %s", workflow, scope)
	case "workflow.failed":
		reason, _ := data["reason"].(string)
		if reason != "" {
			return fmt.Sprintf("Workflow *%s* failed for %s: %s", workflow, scope, reason)
		}
		return fmt.Sprintf("Workflow *%s* failed for %s", workflow, scope)
	case "agent.completed":
		return fmt.Sprintf("Agent *%s* failed in workflow *%s* (%s)", agentType, workflow, scope)
	case "agent.context_saving":
		return fmt.Sprintf("Agent *%s* saving context in workflow *%s* (%s)", agentType, workflow, scope)
	case "agent.stall_restart":
		return fmt.Sprintf("Agent *%s* stall-restarted in workflow *%s* (%s)", agentType, workflow, scope)
	default:
		return eventType
	}
}

// eventDetails extracts extra detail fields into a readable string.
func eventDetails(data map[string]interface{}) string {
	var parts []string
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
