package service

import (
	"testing"
	"time"
)

func rateLimitPct(f float64) *float64 { return &f }

// TestLatestExhaustedReset verifies window selection: only near-exhausted windows
// with a future reset count, and the latest such reset binds.
func TestLatestExhaustedReset(t *testing.T) {
	now := time.Date(2026, 6, 16, 0, 0, 0, 0, time.UTC)
	fiveReset := "2026-06-16T02:00:00Z"  // +2h
	sevenReset := "2026-06-19T00:00:00Z" // +3d

	cases := []struct {
		name     string
		fiveHour RateLimitWindow
		sevenDay RateLimitWindow
		want     string
	}{
		{"neither exhausted",
			RateLimitWindow{UsedPercentage: rateLimitPct(40), ResetsAt: fiveReset},
			RateLimitWindow{UsedPercentage: rateLimitPct(50), ResetsAt: sevenReset}, ""},
		{"only five exhausted",
			RateLimitWindow{UsedPercentage: rateLimitPct(96), ResetsAt: fiveReset},
			RateLimitWindow{UsedPercentage: rateLimitPct(50), ResetsAt: sevenReset}, fiveReset},
		{"both exhausted -> latest",
			RateLimitWindow{UsedPercentage: rateLimitPct(99), ResetsAt: fiveReset},
			RateLimitWindow{UsedPercentage: rateLimitPct(97), ResetsAt: sevenReset}, sevenReset},
		{"exact threshold counts",
			RateLimitWindow{UsedPercentage: rateLimitPct(95), ResetsAt: fiveReset},
			RateLimitWindow{}, fiveReset},
		{"reset in the past ignored",
			RateLimitWindow{UsedPercentage: rateLimitPct(99), ResetsAt: "2026-06-15T00:00:00Z"},
			RateLimitWindow{}, ""},
		{"nil pct ignored",
			RateLimitWindow{ResetsAt: fiveReset},
			RateLimitWindow{}, ""},
		{"unparseable reset ignored",
			RateLimitWindow{UsedPercentage: rateLimitPct(99), ResetsAt: "not-a-time"},
			RateLimitWindow{}, ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := latestExhaustedReset(now, tc.fiveHour, tc.sevenDay); got != tc.want {
				t.Errorf("latestExhaustedReset = %q, want %q", got, tc.want)
			}
		})
	}
}
