package usagelimits

import (
	"testing"
)

func TestParseClaude(t *testing.T) {
	tests := []struct {
		name          string
		input         string
		wantAvailable bool
		wantError     string
		wantSession   *UsageMetric
		wantWeekly    *UsageMetric
	}{
		{
			name: "full session and weekly",
			input: "Current session (5h limit)\n  45.2% used\n  Resets in 2h 15m\n\n" +
				"Current week (all models)\n  12.5% used\n  Resets Monday at 9am",
			wantAvailable: true,
			wantSession:   &UsageMetric{UsedPct: 45.2, ResetsAt: "in 2h 15m"},
			wantWeekly:    &UsageMetric{UsedPct: 12.5, ResetsAt: "Monday at 9am"},
		},
		{
			name:          "session only no weekly",
			input:         "Current session (5h limit)\n  100% used\n  Resets in 5h",
			wantAvailable: true,
			wantSession:   &UsageMetric{UsedPct: 100, ResetsAt: "in 5h"},
			wantWeekly:    nil,
		},
		{
			name:          "weekly only no session",
			input:         "Current week (all models)\n  0% used\n  Resets next Monday",
			wantAvailable: true,
			wantSession:   nil,
			wantWeekly:    &UsageMetric{UsedPct: 0, ResetsAt: "next Monday"},
		},
		{
			name: "Rese s cursor-movement artifact normalized",
			input: "Current session (5h limit)\n  50% used\n  Rese     s in 1h\n\n" +
				"Current week (all models)\n  0% used\n  Rese  s in 3 days",
			wantAvailable: true,
			wantSession:   &UsageMetric{UsedPct: 50, ResetsAt: "in 1h"},
			wantWeekly:    &UsageMetric{UsedPct: 0, ResetsAt: "in 3 days"},
		},
		{
			name:          "integer percentage",
			input:         "Current session (5h limit)\n  75% used\n  Resets tomorrow",
			wantAvailable: true,
			wantSession:   &UsageMetric{UsedPct: 75, ResetsAt: "tomorrow"},
		},
		{
			name: "case insensitive matching",
			input: "CURRENT SESSION (5h limit)\n  20% USED\n  RESETS in 3h\n\n" +
				"CURRENT WEEK (ALL MODELS)\n  10% USED\n  RESETS Friday",
			wantAvailable: true,
			wantSession:   &UsageMetric{UsedPct: 20, ResetsAt: "in 3h"},
			wantWeekly:    &UsageMetric{UsedPct: 10, ResetsAt: "Friday"},
		},
		{
			name:          "empty input returns error",
			input:         "",
			wantAvailable: true,
			wantError:     "failed to parse /usage output",
		},
		{
			name:          "garbage input returns error",
			input:         "no useful content here at all",
			wantAvailable: true,
			wantError:     "failed to parse /usage output",
		},
		{
			name:          "zero percent used",
			input:         "Current session (5h limit)\n  0% used\n  Resets in 5h",
			wantAvailable: true,
			wantSession:   &UsageMetric{UsedPct: 0, ResetsAt: "in 5h"},
		},
		{
			name: "whitespace in ResetsAt normalized",
			input: "Current session (5h limit)\n  30% used\n  Resets   in   2h\n",
			wantAvailable: true,
			wantSession:   &UsageMetric{UsedPct: 30, ResetsAt: "in 2h"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := parseClaude(tt.input)

			if got.Available != tt.wantAvailable {
				t.Errorf("Available = %v, want %v", got.Available, tt.wantAvailable)
			}
			if got.Error != tt.wantError {
				t.Errorf("Error = %q, want %q", got.Error, tt.wantError)
			}
			assertMetric(t, "Session", got.Session, tt.wantSession)
			assertMetric(t, "Weekly", got.Weekly, tt.wantWeekly)
		})
	}
}

func TestParseCodex(t *testing.T) {
	tests := []struct {
		name          string
		input         string
		wantAvailable bool
		wantError     string
		wantSession   *UsageMetric
		wantWeekly    *UsageMetric
	}{
		{
			name: "primary format both session and weekly",
			input: "5h limit: 30% left (resets in 2h)\n" +
				"weekly limit: 40% left (resets Sunday)\n",
			wantAvailable: true,
			wantSession:   &UsageMetric{UsedPct: 70, ResetsAt: "in 2h"},
			wantWeekly:    &UsageMetric{UsedPct: 60, ResetsAt: "Sunday"},
		},
		{
			name:          "primary format inverts percent left to used",
			input:         "5h limit: 0% left (resets in 5h)\nweekly limit: 100% left (resets next week)",
			wantAvailable: true,
			wantSession:   &UsageMetric{UsedPct: 100, ResetsAt: "in 5h"},
			wantWeekly:    &UsageMetric{UsedPct: 0, ResetsAt: "next week"},
		},
		{
			name:          "primary format session only",
			input:         "5h limit: 20% left (resets tomorrow)",
			wantAvailable: true,
			wantSession:   &UsageMetric{UsedPct: 80, ResetsAt: "tomorrow"},
			wantWeekly:    nil,
		},
		{
			name:          "primary format weekly only",
			input:         "weekly limit: 50% left (resets Friday)",
			wantAvailable: true,
			wantSession:   nil,
			wantWeekly:    &UsageMetric{UsedPct: 50, ResetsAt: "Friday"},
		},
		{
			name: "primary format with resets variant (no trailing s)",
			input: "5h limit: 10% left (reset in 4h)\n" +
				"weekly limit: 5% left (reset Monday)",
			wantAvailable: true,
			wantSession:   &UsageMetric{UsedPct: 90, ResetsAt: "in 4h"},
			wantWeekly:    &UsageMetric{UsedPct: 95, ResetsAt: "Monday"},
		},
		{
			name: "fallback to claude-style percent used format",
			input: "Current session (5h limit)\n  60% used\n  Resets in 4h\n\n" +
				"Current week (all models)\n  30% used\n  Resets Wednesday",
			wantAvailable: true,
			wantSession:   &UsageMetric{UsedPct: 60, ResetsAt: "in 4h"},
			wantWeekly:    &UsageMetric{UsedPct: 30, ResetsAt: "Wednesday"},
		},
		{
			name:          "fallback session only",
			input:         "Current session (5h limit)\n  80% used\n  Resets in 1h",
			wantAvailable: true,
			wantSession:   &UsageMetric{UsedPct: 80, ResetsAt: "in 1h"},
			wantWeekly:    nil,
		},
		{
			name: "Rese s artifact in codex output",
			input: "5h limit: 25% left (rese  ts in 3h)\n" +
				"Current session (5h limit)\n  25% used\n  Rese  s in 3h",
			// Primary matches the "5h limit" pattern, artifact normalization applies to whole string
			wantAvailable: true,
		},
		{
			name:          "empty input returns error",
			input:         "",
			wantAvailable: true,
			wantError:     "failed to parse /status output",
		},
		{
			name:          "garbage input returns error",
			input:         "some random unrelated output",
			wantAvailable: true,
			wantError:     "failed to parse /status output",
		},
		{
			name:          "primary format does not trigger fallback",
			input:         "5h limit: 15% left (resets in 2h)\nsome garbage that won't match weekly",
			wantAvailable: true,
			wantSession:   &UsageMetric{UsedPct: 85, ResetsAt: "in 2h"},
			// No weekly match, but found=true from session, so no error
			wantWeekly: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := parseCodex(tt.input)

			if got.Available != tt.wantAvailable {
				t.Errorf("Available = %v, want %v", got.Available, tt.wantAvailable)
			}
			if got.Error != tt.wantError {
				t.Errorf("Error = %q, want %q", got.Error, tt.wantError)
			}
			if tt.wantSession != nil {
				assertMetric(t, "Session", got.Session, tt.wantSession)
			}
			if tt.wantWeekly != nil {
				assertMetric(t, "Weekly", got.Weekly, tt.wantWeekly)
			}
		})
	}
}

func TestParseCodex_PercentInversion(t *testing.T) {
	// Explicitly verify that "% left" is inverted to "% used" (100 - left)
	tests := []struct {
		leftPct float64
		wantPct float64
	}{
		{0, 100},
		{100, 0},
		{50, 50},
		{30, 70},
		{75, 25},
	}
	for _, tt := range tests {
		input := "5h limit: N% left (resets tomorrow)"
		// Build input with the specific percentage
		var pctStr string
		if tt.leftPct == 0 {
			pctStr = "0"
		} else if tt.leftPct == 100 {
			pctStr = "100"
		} else if tt.leftPct == 50 {
			pctStr = "50"
		} else if tt.leftPct == 30 {
			pctStr = "30"
		} else {
			pctStr = "75"
		}
		input = "5h limit: " + pctStr + "% left (resets tomorrow)"
		got := parseCodex(input)
		if got.Session == nil {
			t.Errorf("leftPct=%v: Session is nil", tt.leftPct)
			continue
		}
		if got.Session.UsedPct != tt.wantPct {
			t.Errorf("leftPct=%v: UsedPct = %v, want %v", tt.leftPct, got.Session.UsedPct, tt.wantPct)
		}
	}
}

func TestNormalizeWhitespace(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"in 2h 15m", "in 2h 15m"},
		{"  trimmed  ", "trimmed"},
		{"multiple   spaces", "multiple spaces"},
		{"tab\there", "tab here"},
		{"newline\nhere", "newline here"},
		{"", ""},
	}
	for _, tt := range tests {
		got := normalizeWhitespace(tt.input)
		if got != tt.want {
			t.Errorf("normalizeWhitespace(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}

// assertMetric is a helper to compare UsageMetric pointers.
func assertMetric(t *testing.T, name string, got, want *UsageMetric) {
	t.Helper()
	if want == nil {
		if got != nil {
			t.Errorf("%s = %+v, want nil", name, got)
		}
		return
	}
	if got == nil {
		t.Fatalf("%s is nil, want %+v", name, want)
	}
	if got.UsedPct != want.UsedPct {
		t.Errorf("%s.UsedPct = %v, want %v", name, got.UsedPct, want.UsedPct)
	}
	if got.ResetsAt != want.ResetsAt {
		t.Errorf("%s.ResetsAt = %q, want %q", name, got.ResetsAt, want.ResetsAt)
	}
}
