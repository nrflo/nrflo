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

	// Codex /status patterns.
	// Tolerates:
	//   - "5h", "5-hour", "5 hour" for the session limit label
	//   - "% left", "% remaining" (inverted to used) OR "% used" (taken as-is)
	//   - optional "(resets …)" / "(reset in …)" capture — match succeeds without it
	reCodexSession = regexp.MustCompile(`(?is)5[-\s]?h(?:our)?\s+limit:.*?(\d+(?:\.\d+)?)\s*%\s*(left|remaining|used)(?:[^(]*\(rese[a-z\s]+([^)]*)\))?`)
	reCodexWeekly  = regexp.MustCompile(`(?is)weekly\s+limit:.*?(\d+(?:\.\d+)?)\s*%\s*(left|remaining|used)(?:[^(]*\(rese[a-z\s]+([^)]*)\))?`)
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
	// Reconstruct: join all lines without separator to undo char-by-char rendering.
	text = strings.Join(strings.Split(text, "\n"), "")
	text = reMultiSpace.ReplaceAllString(text, " ")
	text = reResetsGap.ReplaceAllString(text, "Resets")

	var session, weekly *UsageMetric
	if m := reCodexSession.FindStringSubmatch(text); m != nil {
		session = codexMetric(m)
	}
	if m := reCodexWeekly.FindStringSubmatch(text); m != nil {
		weekly = codexMetric(m)
	}
	return session, weekly
}

// codexMetric converts a reCodexSession/Weekly submatch into a UsageMetric.
// m[1]=pct, m[2]=direction (left|remaining|used), m[3]=resets string (may be empty).
func codexMetric(m []string) *UsageMetric {
	pct, _ := strconv.ParseFloat(m[1], 64)
	if strings.ToLower(m[2]) != "used" {
		pct = 100.0 - pct // "left"/"remaining" → invert to used
	}
	return &UsageMetric{UsedPct: pct, ResetsAt: strings.TrimSpace(m[3])}
}
