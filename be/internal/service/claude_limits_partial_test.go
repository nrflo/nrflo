package service

import (
	"testing"
)

// TestClaudeLimitsService_PartialUpdate_PreservesUnsetFields verifies that a second
// Update with sentinel values for weekly fields does not overwrite the first write.
func TestClaudeLimitsService_PartialUpdate_PreservesUnsetFields(t *testing.T) {
	t.Parallel()
	svc, settingsSvc, _ := setupClaudeLimitsTestEnv(t)

	first := ClaudeLimits{
		FiveHourUsedPct:  40.0,
		FiveHourResetsAt: "2026-01-01T00:00:00Z",
		SevenDayUsedPct:  60.0,
		SevenDayResetsAt: "2026-01-07T00:00:00Z",
	}
	if _, err := svc.Update(first); err != nil {
		t.Fatalf("first Update: %v", err)
	}

	// Second update: only 5h fields; weekly fields are sentinels.
	second := ClaudeLimits{
		FiveHourUsedPct:  99.9,
		FiveHourResetsAt: "2026-02-01T00:00:00Z",
		SevenDayUsedPct:  -1,
		SevenDayResetsAt: "",
	}
	if _, err := svc.Update(second); err != nil {
		t.Fatalf("second Update: %v", err)
	}

	weeklyReset, err := settingsSvc.Get("claude_weekly_resets_at")
	if err != nil {
		t.Fatalf("Get claude_weekly_resets_at: %v", err)
	}
	if weeklyReset != "2026-01-07T00:00:00Z" {
		t.Errorf("claude_weekly_resets_at = %q, want %q (must be preserved)", weeklyReset, "2026-01-07T00:00:00Z")
	}

	weeklyPct, err := settingsSvc.Get("claude_weekly_used_pct")
	if err != nil {
		t.Fatalf("Get claude_weekly_used_pct: %v", err)
	}
	if weeklyPct == "" {
		t.Error("claude_weekly_used_pct was cleared by partial update, want preserved")
	}

	got, err := svc.Get()
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got.FiveHourUsedPct != 99.9 {
		t.Errorf("FiveHourUsedPct = %v, want 99.9 (second update)", got.FiveHourUsedPct)
	}
	if got.FiveHourResetsAt != "2026-02-01T00:00:00Z" {
		t.Errorf("FiveHourResetsAt = %q, want %q (second update)", got.FiveHourResetsAt, "2026-02-01T00:00:00Z")
	}
}

// TestClaudeLimitsService_AllSentinel_NoOp verifies that an Update with all sentinel
// values writes nothing, leaving claude_limits_updated_at unset.
func TestClaudeLimitsService_AllSentinel_NoOp(t *testing.T) {
	t.Parallel()
	svc, settingsSvc, _ := setupClaudeLimitsTestEnv(t)

	if _, err := svc.Update(ClaudeLimits{
		FiveHourUsedPct:  -1,
		SevenDayUsedPct:  -1,
		FiveHourResetsAt: "",
		SevenDayResetsAt: "",
	}); err != nil {
		t.Fatalf("Update(all-sentinel): %v", err)
	}

	updatedAt, err := settingsSvc.Get("claude_limits_updated_at")
	if err != nil {
		t.Fatalf("Get claude_limits_updated_at: %v", err)
	}
	if updatedAt != "" {
		t.Errorf("claude_limits_updated_at = %q after all-sentinel Update, want empty", updatedAt)
	}
}

// TestClaudeLimitsService_ResetOnly_WritesResetsAtAndUpdatedAt verifies that an Update
// with only resets_at fields (pcts sentinel) writes the resets and bumps updated_at,
// but does not create pct keys.
func TestClaudeLimitsService_ResetOnly_WritesResetsAtAndUpdatedAt(t *testing.T) {
	t.Parallel()
	svc, settingsSvc, _ := setupClaudeLimitsTestEnv(t)

	if _, err := svc.Update(ClaudeLimits{
		FiveHourUsedPct:  -1,
		SevenDayUsedPct:  -1,
		FiveHourResetsAt: "2026-06-01T00:00:00Z",
		SevenDayResetsAt: "2026-06-07T00:00:00Z",
	}); err != nil {
		t.Fatalf("Update(reset-only): %v", err)
	}

	// Pct keys must not be written.
	fivePct, err := settingsSvc.Get("claude_5h_used_pct")
	if err != nil {
		t.Fatalf("Get claude_5h_used_pct: %v", err)
	}
	if fivePct != "" {
		t.Errorf("claude_5h_used_pct = %q after reset-only update, want empty", fivePct)
	}

	sevenPct, err := settingsSvc.Get("claude_weekly_used_pct")
	if err != nil {
		t.Fatalf("Get claude_weekly_used_pct: %v", err)
	}
	if sevenPct != "" {
		t.Errorf("claude_weekly_used_pct = %q after reset-only update, want empty", sevenPct)
	}

	// Reset keys must be written.
	fiveReset, err := settingsSvc.Get("claude_5h_resets_at")
	if err != nil {
		t.Fatalf("Get claude_5h_resets_at: %v", err)
	}
	if fiveReset != "2026-06-01T00:00:00Z" {
		t.Errorf("claude_5h_resets_at = %q, want %q", fiveReset, "2026-06-01T00:00:00Z")
	}

	sevenReset, err := settingsSvc.Get("claude_weekly_resets_at")
	if err != nil {
		t.Fatalf("Get claude_weekly_resets_at: %v", err)
	}
	if sevenReset != "2026-06-07T00:00:00Z" {
		t.Errorf("claude_weekly_resets_at = %q, want %q", sevenReset, "2026-06-07T00:00:00Z")
	}

	// updated_at must be set (at least one real field was written).
	updatedAt, err := settingsSvc.Get("claude_limits_updated_at")
	if err != nil {
		t.Fatalf("Get claude_limits_updated_at: %v", err)
	}
	if updatedAt == "" {
		t.Error("claude_limits_updated_at empty after reset-only update, want non-empty")
	}
}
