package notify

import (
	"regexp"
	"strings"
)

// telegramV2Special is the complete Telegram MarkdownV2 reserved character
// set. Backslash must escape itself first so a literal `\` in a value
// becomes `\\` rather than being re-processed as an escape marker.
const telegramV2Special = "\\_*[]()~`>#+-=|{}.!"

// escapeTelegramV2 escapes every MarkdownV2-reserved rune in s so dynamic
// (agent-authored) content can never open a formatting entity.
func escapeTelegramV2(s string) string {
	var b strings.Builder
	for _, c := range s {
		if strings.ContainsRune(telegramV2Special, c) {
			b.WriteRune('\\')
		}
		b.WriteRune(c)
	}
	return b.String()
}

var telegramLinkRe = regexp.MustCompile(`\[([^\]]*)\]\(([^)]*)\)`)

// stripMarkdownV2 undoes escapeTelegramV2-style escaping and unwraps inline
// links, producing a readable plaintext fallback for when a malformed
// template body can't be delivered with parse_mode=MarkdownV2.
func stripMarkdownV2(s string) string {
	runes := []rune(s)
	var b strings.Builder
	for i := 0; i < len(runes); i++ {
		c := runes[i]
		if c == '\\' && i+1 < len(runes) && strings.ContainsRune(telegramV2Special, runes[i+1]) {
			b.WriteRune(runes[i+1])
			i++
			continue
		}
		b.WriteRune(c)
	}
	return telegramLinkRe.ReplaceAllString(b.String(), "$1 ($2)")
}

// isEntityParseError reports whether desc is Telegram's "can't parse
// entities" description. Kept narrow so other 400s (chat not found, flood
// control, bot blocked) stay retryable instead of falling back to plaintext.
func isEntityParseError(desc string) bool {
	return strings.Contains(strings.ToLower(desc), "can't parse entities")
}
