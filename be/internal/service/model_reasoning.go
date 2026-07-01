package service

import "strings"

// supportsXHighEffort reports whether mappedModel supports the "xhigh"
// reasoning effort (Opus 4.7, Opus 4.8, and Sonnet 5). Shared by the CLI and
// API model reasoning-effort validators.
func supportsXHighEffort(mappedModel string) bool {
	return strings.HasPrefix(mappedModel, "claude-opus-4-7") ||
		strings.HasPrefix(mappedModel, "claude-opus-4-8") ||
		strings.HasPrefix(mappedModel, "claude-sonnet-5")
}
