package cli

import (
	"encoding/json"
	"strings"
	"testing"
)

// unmarshalStatusline parses a statusline JSON payload for buildRateLimitsParams tests.
func unmarshalStatusline(t *testing.T, raw string) statusLinePayload {
	t.Helper()
	var p statusLinePayload
	if err := json.Unmarshal([]byte(raw), &p); err != nil {
		t.Fatalf("unmarshal payload: %v", err)
	}
	return p
}

// TestBuildRateLimitsParams_Absent: no rate_limits key → nothing to forward.
func TestBuildRateLimitsParams_Absent(t *testing.T) {
	p := unmarshalStatusline(t, `{"context_window":{"used_percentage":50},"model":{"display_name":"Sonnet"}}`)
	if got, ok := buildRateLimitsParams(p, "sess-1"); ok || got != nil {
		t.Errorf("absent rate_limits: want (nil,false), got (%+v,%v)", got, ok)
	}
}

// TestBuildRateLimitsParams_NoSession: present rate_limits but empty session → not forwarded.
func TestBuildRateLimitsParams_NoSession(t *testing.T) {
	p := unmarshalStatusline(t, `{"rate_limits":{"five_hour":{"used_percentage":42.5,"resets_at":"2026-06-16T05:00:00Z"}}}`)
	if got, ok := buildRateLimitsParams(p, ""); ok || got != nil {
		t.Errorf("empty session: want (nil,false), got (%+v,%v)", got, ok)
	}
}

// TestBuildRateLimitsParams_EmptyObject: rate_limits present but both windows absent → not forwarded.
func TestBuildRateLimitsParams_EmptyObject(t *testing.T) {
	p := unmarshalStatusline(t, `{"rate_limits":{}}`)
	if got, ok := buildRateLimitsParams(p, "sess-1"); ok || got != nil {
		t.Errorf("empty rate_limits object: want (nil,false), got (%+v,%v)", got, ok)
	}
}

// TestBuildRateLimitsParams_BothWindows: both windows carried through with pct + reset.
func TestBuildRateLimitsParams_BothWindows(t *testing.T) {
	p := unmarshalStatusline(t, `{"rate_limits":{
		"five_hour":{"used_percentage":35.0,"resets_at":"2026-06-16T05:00:00Z"},
		"seven_day":{"used_percentage":60.0,"resets_at":"2026-06-23T05:00:00Z"}}}`)
	got, ok := buildRateLimitsParams(p, "sess-1")
	if !ok || got == nil {
		t.Fatalf("both windows: want forwarded, got (%+v,%v)", got, ok)
	}
	if got.SessionID != "sess-1" {
		t.Errorf("session_id = %q, want sess-1", got.SessionID)
	}
	if got.FiveHour == nil || got.FiveHour.UsedPercentage == nil || *got.FiveHour.UsedPercentage != 35.0 {
		t.Errorf("five_hour pct not carried: %+v", got.FiveHour)
	}
	if got.FiveHour.ResetsAt != "2026-06-16T05:00:00Z" {
		t.Errorf("five_hour resets_at = %q", got.FiveHour.ResetsAt)
	}
	if got.SevenDay == nil || got.SevenDay.UsedPercentage == nil || *got.SevenDay.UsedPercentage != 60.0 {
		t.Errorf("seven_day pct not carried: %+v", got.SevenDay)
	}
}

// TestBuildRateLimitsParams_PartialNilPct: a window with null pct is still forwarded (pct nil).
func TestBuildRateLimitsParams_PartialNilPct(t *testing.T) {
	p := unmarshalStatusline(t, `{"rate_limits":{
		"five_hour":{"resets_at":"2026-06-16T05:00:00Z"},
		"seven_day":{"used_percentage":80.0,"resets_at":"2026-06-23T05:00:00Z"}}}`)
	got, ok := buildRateLimitsParams(p, "sess-1")
	if !ok || got == nil {
		t.Fatalf("want forwarded")
	}
	if got.FiveHour == nil || got.FiveHour.UsedPercentage != nil {
		t.Errorf("five_hour pct should be nil, got %+v", got.FiveHour)
	}
	if got.SevenDay == nil || got.SevenDay.UsedPercentage == nil || *got.SevenDay.UsedPercentage != 80.0 {
		t.Errorf("seven_day pct = %+v", got.SevenDay)
	}
}

// TestAgentStatusline_RateLimitsRenderUnaffected: rate_limits present must not break
// the rendered status line or the command (no server → dispatch short-circuits).
func TestAgentStatusline_RateLimitsRenderUnaffected(t *testing.T) {
	t.Setenv("NRF_SESSION_ID", "sess-render")
	const payload = `{"context_window":{"used_percentage":70},"model":{"display_name":"Opus"},"workspace":{"current_dir":"/tmp"},
		"rate_limits":{"five_hour":{"used_percentage":99.0,"resets_at":"2026-06-16T05:00:00Z"}}}`
	out, err := runStatusline(t, payload)
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	if !strings.Contains(out, "70%") {
		t.Errorf("output missing context pct: %q", out)
	}
}
