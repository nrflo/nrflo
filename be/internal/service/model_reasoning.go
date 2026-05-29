package service

import "strings"

// isXHighOpus reports whether mappedModel is an Opus version that supports the
// "xhigh" reasoning effort (Opus 4.7 and 4.8). Shared by the CLI and API model
// reasoning-effort validators.
func isXHighOpus(mappedModel string) bool {
	return strings.HasPrefix(mappedModel, "claude-opus-4-7") ||
		strings.HasPrefix(mappedModel, "claude-opus-4-8")
}
