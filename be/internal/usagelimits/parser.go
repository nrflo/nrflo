package usagelimits

import (
	"regexp"
	"strconv"
	"strings"
)

// Precompiled patterns for Claude /usage output parsing.
// Ported from scripts/usage-limits.sh lines 118-133.
var (
	resetArtifactRe = regexp.MustCompile(`Rese\s+s\b`)

	claudeSessionRe = regexp.MustCompile(
		`(?is)Current\s+session.*?(\d+(?:\.\d+)?)\s*%\s*used.*?Resets\s+(.+?)(?:Current|\n\s*\n|$)`)
	claudeWeeklyRe = regexp.MustCompile(
		`(?is)Current\s+week\s*\(all\s+models?\).*?(\d+(?:\.\d+)?)\s*%\s*used.*?Resets\s+(.+?)(?:Current|\n\s*\n|$)`)
)

// Precompiled patterns for Codex /status output parsing.
// Ported from scripts/usage-limits.sh lines 224-249.
var (
	codexSession5hRe = regexp.MustCompile(
		`(?is)5h\s+limit:.*?(\d+(?:\.\d+)?)\s*%\s*left.*?\(resets?\s+([^)]+)\)`)
	codexWeeklyLeftRe = regexp.MustCompile(
		`(?is)weekly\s+limit:.*?(\d+(?:\.\d+)?)\s*%\s*left.*?\(resets?\s+([^)]+)\)`)

	// Fallback: same as Claude patterns
	codexSessionFallbackRe = regexp.MustCompile(
		`(?is)Current\s+session.*?(\d+(?:\.\d+)?)\s*%\s*used.*?Resets\s+(.+?)(?:Current|\n\s*\n|$)`)
	codexWeeklyFallbackRe = regexp.MustCompile(
		`(?is)Current\s+week.*?(\d+(?:\.\d+)?)\s*%\s*used.*?Resets\s+(.+?)(?:Current|\n\s*\n|$)`)
)

// parseClaude extracts session and weekly usage from Claude /usage output.
func parseClaude(cleaned string) *ToolUsage {
	cleaned = resetArtifactRe.ReplaceAllString(cleaned, "Resets")

	usage := &ToolUsage{Available: true}
	found := false

	if m := claudeSessionRe.FindStringSubmatch(cleaned); m != nil {
		pct, _ := strconv.ParseFloat(m[1], 64)
		usage.Session = &UsageMetric{
			UsedPct:  pct,
			ResetsAt: normalizeWhitespace(m[2]),
		}
		found = true
	}

	if m := claudeWeeklyRe.FindStringSubmatch(cleaned); m != nil {
		pct, _ := strconv.ParseFloat(m[1], 64)
		usage.Weekly = &UsageMetric{
			UsedPct:  pct,
			ResetsAt: normalizeWhitespace(m[2]),
		}
		found = true
	}

	if !found {
		usage.Error = "failed to parse /usage output"
	}
	return usage
}

// parseCodex extracts session and weekly usage from Codex /status output.
func parseCodex(cleaned string) *ToolUsage {
	cleaned = resetArtifactRe.ReplaceAllString(cleaned, "Resets")

	usage := &ToolUsage{Available: true}
	found := false

	// Primary: Status tab with "% left" (invert to "% used")
	if m := codexSession5hRe.FindStringSubmatch(cleaned); m != nil {
		left, _ := strconv.ParseFloat(m[1], 64)
		usage.Session = &UsageMetric{
			UsedPct:  100.0 - left,
			ResetsAt: normalizeWhitespace(m[2]),
		}
		found = true
	}

	if m := codexWeeklyLeftRe.FindStringSubmatch(cleaned); m != nil {
		left, _ := strconv.ParseFloat(m[1], 64)
		usage.Weekly = &UsageMetric{
			UsedPct:  100.0 - left,
			ResetsAt: normalizeWhitespace(m[2]),
		}
		found = true
	}

	// Fallback: Usage tab format (same as Claude, "% used")
	if !found {
		if m := codexSessionFallbackRe.FindStringSubmatch(cleaned); m != nil {
			pct, _ := strconv.ParseFloat(m[1], 64)
			usage.Session = &UsageMetric{
				UsedPct:  pct,
				ResetsAt: normalizeWhitespace(m[2]),
			}
			found = true
		}

		if m := codexWeeklyFallbackRe.FindStringSubmatch(cleaned); m != nil {
			pct, _ := strconv.ParseFloat(m[1], 64)
			usage.Weekly = &UsageMetric{
				UsedPct:  pct,
				ResetsAt: normalizeWhitespace(m[2]),
			}
			found = true
		}
	}

	if !found {
		usage.Error = "failed to parse /status output"
	}
	return usage
}

var whitespaceRe = regexp.MustCompile(`\s+`)

func normalizeWhitespace(s string) string {
	return strings.TrimSpace(whitespaceRe.ReplaceAllString(s, " "))
}
