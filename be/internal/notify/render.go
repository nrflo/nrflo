package notify

import (
	"fmt"
	"regexp"
	"strings"

	"be/internal/model"
)

// NotificationBaseURL is the URL prefix for ticket / project-workflow links.
// Overridable in tests or via wiring.
var NotificationBaseURL = "http://localhost:6587"

var placeholderRe = regexp.MustCompile(`\$\{[^}]+\}`)

// Render substitutes ${var} placeholders in the template using event data.
// Values destined for Telegram are escaped per MarkdownV2 rules; template
// text itself is left unchanged. Unknown ${...} placeholders are stripped.
func Render(kind model.ChannelKind, template string, data map[string]interface{}) string {
	isTelegram := kind == model.ChannelKindTelegram
	escape := func(s string) string {
		if isTelegram {
			return escapeTelegramV2(s)
		}
		return s
	}
	linkFn := slackLink
	if isTelegram {
		linkFn = telegramLink
	}

	vars := buildVars(data, escape, linkFn)
	pairs := make([]string, 0, len(vars)*2)
	for k, v := range vars {
		pairs = append(pairs, "${"+k+"}", v)
	}
	result := strings.NewReplacer(pairs...).Replace(template)
	return placeholderRe.ReplaceAllString(result, "")
}

func buildVars(data map[string]interface{}, escape func(string) string, linkFn func(text, url string) string) map[string]string {
	vars := map[string]string{
		"event_type":   escape(strVal(data, "event_type")),
		"ticket_id":    escape(strVal(data, "ticket_id")),
		"project_id":   escape(strVal(data, "project_id")),
		"project_name": escape(strVal(data, "project_name")),
		"ticket_name":  escape(strVal(data, "ticket_name")),
		"workflow":     escape(strVal(data, "workflow")),
		"instance_id":  escape(strVal(data, "instance_id")),
		"agent_type":   escape(strVal(data, "agent_type")),
		"reason":       escape(strVal(data, "reason")),
		"link":         buildLink(data, linkFn, escape),
	}
	if s := strVal(data, "workflow_final_result"); s != "" {
		vars["summary"] = renderSummaryBlock(s, escape)
	} else {
		vars["summary"] = ""
	}
	return vars
}

func buildLink(data map[string]interface{}, linkFn func(text, url string) string, escape func(string) string) string {
	if id := strVal(data, "ticket_id"); id != "" {
		url := fmt.Sprintf("%s/tickets/%s", NotificationBaseURL, id)
		return linkFn(escape(id), url)
	}
	if id := strVal(data, "instance_id"); id != "" {
		url := fmt.Sprintf("%s/project-workflows?instance_id=%s", NotificationBaseURL, id)
		return linkFn(escape("project"), url)
	}
	return ""
}

func strVal(data map[string]interface{}, key string) string {
	s, _ := data[key].(string)
	return s
}

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

func truncateRunes(s string, max int) string {
	runes := []rune(s)
	if len(runes) <= max {
		return s
	}
	return string(runes[:max]) + "…"
}

func slackLink(text, url string) string {
	return fmt.Sprintf("<%s|%s>", url, text)
}

func telegramLink(text, url string) string {
	r := strings.NewReplacer(`\`, `\\`, `)`, `\)`)
	return fmt.Sprintf("[%s](%s)", text, r.Replace(url))
}

func escapeTelegramV2(s string) string {
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
