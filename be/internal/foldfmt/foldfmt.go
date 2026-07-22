// Package foldfmt holds the tail-keep message join and rune-safe byte cap
// shared by refinery (fold.go, session_sidecar.go) and spawner
// (context_save.go, consult.go) — a stdlib-only leaf package so neither
// imports the other.
package foldfmt

import (
	"fmt"
	"strings"
)

// JoinTail joins messages with newlines, keeping only the tail that fits
// within maxChars (most recent content is most relevant). When even the
// single newest message alone exceeds maxChars, hard-truncates it to the
// byte budget rather than returning a header-only, content-free string.
func JoinTail(messages []string, maxChars int) string {
	joined := strings.Join(messages, "\n")
	if len(joined) <= maxChars {
		return joined
	}

	var kept []string
	total := 0
	for i := len(messages) - 1; i >= 0; i-- {
		msgLen := len(messages[i])
		if total > 0 {
			msgLen++
		}
		if total+msgLen > maxChars {
			break
		}
		total += msgLen
		kept = append(kept, messages[i])
	}

	if len(kept) == 0 {
		last := messages[len(messages)-1]
		truncated := CapBytes(last, maxChars)
		marker := fmt.Sprintf("[message truncated to %d bytes]", len(truncated))
		return marker + "\n" + truncated
	}

	for i, j := 0, len(kept)-1; i < j; i, j = i+1, j-1 {
		kept[i], kept[j] = kept[j], kept[i]
	}

	header := fmt.Sprintf("[truncated: showing last %d of %d messages]", len(kept), len(messages))
	return header + "\n" + strings.Join(kept, "\n")
}

// CapBytes truncates s to at most n bytes, backing off to a UTF-8 rune
// boundary so a multi-byte character is never split.
func CapBytes(s string, n int) string {
	if len(s) <= n {
		return s
	}
	for n > 0 && s[n]&0xC0 == 0x80 {
		n--
	}
	return s[:n]
}
