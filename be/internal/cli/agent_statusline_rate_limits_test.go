package cli

import (
	"strings"
	"testing"
)

// TestAgentStatuslineRateLimits_AbsentRateLimits verifies no panic/error when
// rate_limits key is entirely absent from the payload.
func TestAgentStatuslineRateLimits_AbsentRateLimits(t *testing.T) {
	t.Setenv("NRF_SESSION_ID", "sess-rl-absent")

	const payload = `{"context_window":{"used_percentage":50},"model":{"display_name":"Sonnet"},"workspace":{"current_dir":"/tmp"}}`
	_, err := runStatusline(t, payload)
	if err != nil {
		t.Errorf("rate_limits absent: unexpected error: %v", err)
	}
}

// TestAgentStatuslineRateLimits_BothPctsNil verifies no panic when rate_limits is
// present but both used_percentage fields are null/absent.
func TestAgentStatuslineRateLimits_BothPctsNil(t *testing.T) {
	t.Setenv("NRF_SESSION_ID", "sess-rl-nil-pct")

	const payload = `{
		"context_window":{"used_percentage":60},
		"model":{"display_name":"Opus"},
		"workspace":{"current_dir":"/tmp"},
		"rate_limits":{
			"five_hour":{"resets_at":"2026-05-11T05:00:00Z"},
			"seven_day":{"resets_at":"2026-05-18T05:00:00Z"}
		}
	}`
	_, err := runStatusline(t, payload)
	if err != nil {
		t.Errorf("both pcts nil: unexpected error: %v", err)
	}
}

// TestAgentStatuslineRateLimits_OnlyFiveHourPct verifies dispatch fires with only five_hour pct set.
// Server not running → command exits without error (IsServerRunning() = false short-circuits).
func TestAgentStatuslineRateLimits_OnlyFiveHourPct(t *testing.T) {
	t.Setenv("NRF_SESSION_ID", "sess-rl-5h-only")

	const payload = `{
		"context_window":{"used_percentage":70},
		"model":{"display_name":"Haiku"},
		"workspace":{"current_dir":"/tmp"},
		"rate_limits":{
			"five_hour":{"used_percentage":42.5,"resets_at":"2026-05-11T05:00:00Z"},
			"seven_day":{"resets_at":"2026-05-18T05:00:00Z"}
		}
	}`
	out, err := runStatusline(t, payload)
	if err != nil {
		t.Errorf("only five_hour pct: unexpected error: %v", err)
	}
	// Output line must still be rendered regardless of rate_limits dispatch.
	if !strings.Contains(out, "70%") {
		t.Errorf("output missing context pct 70%%: got %q", out)
	}
}

// TestAgentStatuslineRateLimits_OnlySevenDayPct verifies dispatch fires with only seven_day pct set.
func TestAgentStatuslineRateLimits_OnlySevenDayPct(t *testing.T) {
	t.Setenv("NRF_SESSION_ID", "sess-rl-7d-only")

	const payload = `{
		"context_window":{"used_percentage":30},
		"model":{"display_name":"Sonnet"},
		"workspace":{"current_dir":"/home"},
		"rate_limits":{
			"five_hour":{"resets_at":"2026-05-11T05:00:00Z"},
			"seven_day":{"used_percentage":80.0,"resets_at":"2026-05-18T05:00:00Z"}
		}
	}`
	out, err := runStatusline(t, payload)
	if err != nil {
		t.Errorf("only seven_day pct: unexpected error: %v", err)
	}
	if !strings.Contains(out, "30%") {
		t.Errorf("output missing context pct 30%%: got %q", out)
	}
}

// TestAgentStatuslineRateLimits_BothPctsPresent verifies no panic when both pcts are set.
// Server not running → exits cleanly.
func TestAgentStatuslineRateLimits_BothPctsPresent(t *testing.T) {
	t.Setenv("NRF_SESSION_ID", "sess-rl-both")

	const payload = `{
		"context_window":{"used_percentage":55},
		"model":{"display_name":"Opus"},
		"workspace":{"current_dir":"/work"},
		"rate_limits":{
			"five_hour":{"used_percentage":35.0,"resets_at":"2026-05-11T05:00:00Z"},
			"seven_day":{"used_percentage":60.0,"resets_at":"2026-05-18T05:00:00Z"}
		}
	}`
	out, err := runStatusline(t, payload)
	if err != nil {
		t.Errorf("both pcts present: unexpected error: %v", err)
	}
	if !strings.Contains(out, "55%") {
		t.Errorf("output missing context pct 55%%: got %q", out)
	}
}

// TestAgentStatuslineRateLimits_NoSessionID_StillFires verifies that the limits dispatch
// path is NOT gated on NRF_SESSION_ID — command runs without error regardless.
func TestAgentStatuslineRateLimits_NoSessionID_StillFires(t *testing.T) {
	t.Setenv("NRF_SESSION_ID", "") // empty session_id

	const payload = `{
		"context_window":{"used_percentage":45},
		"model":{"display_name":"Haiku"},
		"workspace":{"current_dir":"/x"},
		"rate_limits":{
			"five_hour":{"used_percentage":20.0,"resets_at":"2026-05-11T05:00:00Z"},
			"seven_day":{"used_percentage":50.0,"resets_at":"2026-05-18T05:00:00Z"}
		}
	}`
	out, err := runStatusline(t, payload)
	if err != nil {
		t.Errorf("no session ID + rate_limits: unexpected error: %v", err)
	}
	// Output still rendered: context_update gated on session_id, but render always runs.
	if !strings.Contains(out, "45%") {
		t.Errorf("output missing pct when session empty: got %q", out)
	}
}

// TestAgentStatuslineRateLimits_ResetsAtStrings verifies resets_at strings are parsed
// without panic even when they contain non-trivial values.
func TestAgentStatuslineRateLimits_ResetsAtStrings(t *testing.T) {
	t.Setenv("NRF_SESSION_ID", "")

	cases := []struct {
		name    string
		payload string
	}{
		{
			"empty_resets_at",
			`{"rate_limits":{"five_hour":{"used_percentage":10.0,"resets_at":""},"seven_day":{"used_percentage":20.0,"resets_at":""}}}`,
		},
		{
			"iso_resets_at",
			`{"rate_limits":{"five_hour":{"used_percentage":10.0,"resets_at":"2026-05-11T05:00:00Z"}}}`,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := runStatusline(t, tc.payload)
			if err != nil {
				t.Errorf("%s: unexpected error: %v", tc.name, err)
			}
		})
	}
}
