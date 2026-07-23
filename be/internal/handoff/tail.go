package handoff

import (
	"strings"

	"be/internal/foldfmt"
	"be/internal/repo"
)

var tailDropPrefixes = []string{"[Read] ", "[Glob] ", "[Grep] ", "[TodoWrite]"}

// renderTail renders the newest transcript rows verbatim (not summarized),
// dropping pure-exploration rows, and joins them through foldfmt.JoinTail so
// the tail-keep/oversize-message handling stays shared with the fold path.
func renderTail(msgs []repo.TailMessage) string {
	var lines []string
	for _, m := range msgs {
		if hasTailDropPrefix(m.Content) {
			continue
		}
		lines = append(lines, "["+m.Category+"] "+m.Content)
	}
	if len(lines) == 0 {
		return ""
	}
	joined := foldfmt.JoinTail(lines, maxTailBytes)
	if joined == "" {
		return ""
	}
	return "## Recent Uncompressed Context\n" + tailPreamble + "\n\n" + joined
}

func hasTailDropPrefix(content string) bool {
	for _, p := range tailDropPrefixes {
		if strings.HasPrefix(content, p) {
			return true
		}
	}
	return false
}
