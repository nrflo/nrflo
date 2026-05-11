package notify

import "be/internal/model"

// slackDefaultTemplate is the default Slack mrkdwn message template.
// Literal must match the SQL UPDATE in migration 000097.
const slackDefaultTemplate = "*nrflo* — ${event_type} ${link}\n${summary}\nagent: ${agent_type} | workflow: ${workflow} | reason: ${reason} | instance: ${instance_id}"

// telegramDefaultTemplate is the default Telegram MarkdownV2 message template.
// \| is a MarkdownV2-escaped pipe. Literal must match the SQL UPDATE in migration 000097.
const telegramDefaultTemplate = "*nrflo* — ${event_type} ${link}\n${summary}\nagent: ${agent_type} \\| workflow: ${workflow} \\| reason: ${reason} \\| instance: ${instance_id}"

// DefaultTemplate returns the default message template for a channel kind.
func DefaultTemplate(kind model.ChannelKind) string {
	if kind == model.ChannelKindTelegram {
		return telegramDefaultTemplate
	}
	return slackDefaultTemplate
}

// AvailableVariables returns supported ${var} placeholder names in display order.
func AvailableVariables() []string {
	return []string{
		"event_type",
		"link",
		"summary",
		"project_name",
		"project_id",
		"ticket_name",
		"ticket_id",
		"workflow",
		"instance_id",
		"agent_type",
		"reason",
	}
}
