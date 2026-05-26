package spawner

import (
	"fmt"
	"strings"
)

// formatMessagesForSave joins messages with newlines. If total length exceeds
// maxChars, keeps the LAST N messages (most recent work is most relevant) and
// prepends a truncation header.
func formatMessagesForSave(messages []string, maxChars int) string {
	joined := strings.Join(messages, "\n")
	if len(joined) <= maxChars {
		return joined
	}

	// Keep tail messages that fit within maxChars
	var kept []string
	total := 0
	for i := len(messages) - 1; i >= 0; i-- {
		msgLen := len(messages[i])
		if total > 0 {
			msgLen++ // account for newline separator
		}
		if total+msgLen > maxChars {
			break
		}
		total += msgLen
		kept = append(kept, messages[i])
	}

	// Reverse to restore original order
	for i, j := 0, len(kept)-1; i < j; i, j = i+1, j-1 {
		kept[i], kept[j] = kept[j], kept[i]
	}

	header := fmt.Sprintf("[truncated: showing last %d of %d messages]", len(kept), len(messages))
	return header + "\n" + strings.Join(kept, "\n")
}
