package usagelimits

import (
	"regexp"
	"strconv"
	"strings"
)

var (
	reCursorRight = regexp.MustCompile(`\x1b\[\d+C`)
	reCSI         = regexp.MustCompile(`\x1b\[[0-9;]*[a-zA-Z]`)
	reDECPrivate  = regexp.MustCompile(`\x1b\[[?!>][0-9;]*[a-zA-Z]`)
	reOSCBel      = regexp.MustCompile("\x1b][^\x07\x1b]*\x07")
	reOSCST       = regexp.MustCompile("\x1b][^\x1b]*\x1b\\\\")
	reOtherESC    = regexp.MustCompile(`\x1b[^\[\]()]`)
	reMultiSpace  = regexp.MustCompile(` +`)
	reResetsGap   = regexp.MustCompile(`Rese\s+s\b`)

	// Claude /usage patterns
	reClaudeSession = regexp.MustCompile(`(?is)Current\s+session.*?(\d+(?:\.\d+)?)\s*%\s*used.*?Resets\s+(.+?)(?:Current|\n\s*\n|$)`)
	reClaudeWeekly  = regexp.MustCompile(`(?is)Current\s+week\s*\(all\s+models?\).*?(\d+(?:\.\d+)?)\s*%\s*used.*?Resets\s+(.+?)(?:Current|\n\s*\n|$)`)

	// Codex /status patterns (% left → convert to % used)
	reCodexSession = regexp.MustCompile(`(?is)5h\s+limit:.*?(\d+(?:\.\d+)?)\s*%\s*left.*?\(resets?\s+([^)]+)\)`)
	reCodexWeekly  = regexp.MustCompile(`(?is)weekly\s+limit:.*?(\d+(?:\.\d+)?)\s*%\s*left.*?\(resets?\s+([^)]+)\)`)
)

// stripANSI removes ANSI/VT escape sequences.
// Cursor-right (ESC[NC) is replaced with a space to preserve word separation.
func stripANSI(text string) string {
	text = reCursorRight.ReplaceAllString(text, " ")
	text = reCSI.ReplaceAllString(text, "")
	text = reDECPrivate.ReplaceAllString(text, "")
	text = reOSCBel.ReplaceAllString(text, "")
	text = reOSCST.ReplaceAllString(text, "")
	text = reOtherESC.ReplaceAllString(text, "")
	text = strings.ReplaceAll(text, "\x0f", "") // Shift-In
	text = strings.ReplaceAll(text, "\r", "")
	return text
}

// parseClaude extracts session and weekly metrics from Claude /usage output.
func parseClaude(raw string) (*UsageMetric, *UsageMetric) {
	text := stripANSI(raw)
	text = reResetsGap.ReplaceAllString(text, "Resets")

	var session, weekly *UsageMetric
	if m := reClaudeSession.FindStringSubmatch(text); m != nil {
		pct, _ := strconv.ParseFloat(m[1], 64)
		resets := strings.TrimSpace(reMultiSpace.ReplaceAllString(m[2], " "))
		session = &UsageMetric{UsedPct: pct, ResetsAt: resets}
	}
	if m := reClaudeWeekly.FindStringSubmatch(text); m != nil {
		pct, _ := strconv.ParseFloat(m[1], 64)
		resets := strings.TrimSpace(reMultiSpace.ReplaceAllString(m[2], " "))
		weekly = &UsageMetric{UsedPct: pct, ResetsAt: resets}
	}
	return session, weekly
}

// parseCodex extracts session and weekly metrics from Codex /status output.
// Codex renders each character on its own line; lines are joined before matching.
func parseCodex(raw string) (*UsageMetric, *UsageMetric) {
	text := stripANSI(raw)
	// Reconstruct: join all lines without separator to undo char-by-char rendering
	text = strings.Join(strings.Split(text, "\n"), "")
	text = reMultiSpace.ReplaceAllString(text, " ")
	text = reResetsGap.ReplaceAllString(text, "Resets")

	var session, weekly *UsageMetric
	if m := reCodexSession.FindStringSubmatch(text); m != nil {
		pct, _ := strconv.ParseFloat(m[1], 64)
		session = &UsageMetric{UsedPct: 100.0 - pct, ResetsAt: strings.TrimSpace(m[2])}
	}
	if m := reCodexWeekly.FindStringSubmatch(text); m != nil {
		pct, _ := strconv.ParseFloat(m[1], 64)
		weekly = &UsageMetric{UsedPct: 100.0 - pct, ResetsAt: strings.TrimSpace(m[2])}
	}
	return session, weekly
}
