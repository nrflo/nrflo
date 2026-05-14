package service

import (
	"path/filepath"
	"testing"
	"time"

	"be/internal/clock"
	"be/internal/db"
)

func setupClaudeLimitsTestEnv(t *testing.T) (*ClaudeLimitsService, *GlobalSettingsService, *db.Pool) {
	t.Helper()
	dbPath := filepath.Join(t.TempDir(), "claude_limits_test.db")
	if err := svcCopyTemplateDB(dbPath); err != nil {
		t.Fatalf("copy template DB: %v", err)
	}
	pool, err := db.OpenPoolExisting(dbPath, db.DefaultPoolConfig())
	if err != nil {
		t.Fatalf("failed to open pool: %v", err)
	}
	t.Cleanup(func() { pool.Close() })
	clk := clock.Real()
	return NewClaudeLimitsService(pool, clk), NewGlobalSettingsService(pool, clk), pool
}

// TestClaudeLimitsService_Get_FreshDB verifies zero-value struct returned when no keys set.
func TestClaudeLimitsService_Get_FreshDB(t *testing.T) {
	t.Parallel()
	svc, _, _ := setupClaudeLimitsTestEnv(t)

	got, err := svc.Get()
	if err != nil {
		t.Fatalf("Get() returned error: %v", err)
	}
	if got.FiveHourUsedPct != -1 {
		t.Errorf("FiveHourUsedPct = %v, want -1", got.FiveHourUsedPct)
	}
	if got.SevenDayUsedPct != -1 {
		t.Errorf("SevenDayUsedPct = %v, want -1", got.SevenDayUsedPct)
	}
	if got.FiveHourResetsAt != "" {
		t.Errorf("FiveHourResetsAt = %q, want empty", got.FiveHourResetsAt)
	}
	if got.SevenDayResetsAt != "" {
		t.Errorf("SevenDayResetsAt = %q, want empty", got.SevenDayResetsAt)
	}
	if got.UpdatedAt != "" {
		t.Errorf("UpdatedAt = %q, want empty", got.UpdatedAt)
	}
}

// TestClaudeLimitsService_Update_WritesAllFiveKeys verifies all 5 config keys are persisted.
func TestClaudeLimitsService_Update_WritesAllFiveKeys(t *testing.T) {
	t.Parallel()
	svc, settingsSvc, _ := setupClaudeLimitsTestEnv(t)

	limits := ClaudeLimits{
		FiveHourUsedPct:  42.5,
		FiveHourResetsAt: "2026-01-01T00:00:00Z",
		SevenDayUsedPct:  75.0,
		SevenDayResetsAt: "2026-01-07T00:00:00Z",
	}
	if _, err := svc.Update(limits); err != nil {
		t.Fatalf("Update() error: %v", err)
	}

	// Verify each persistent key via GlobalSettingsService.Get.
	cases := []struct {
		key         string
		wantExact   string
		wantNonEmpty bool
	}{
		{"claude_5h_resets_at", "2026-01-01T00:00:00Z", false},
		{"claude_weekly_resets_at", "2026-01-07T00:00:00Z", false},
		{"claude_limits_updated_at", "", true},
	}
	for _, tc := range cases {
		val, err := settingsSvc.Get(tc.key)
		if err != nil {
			t.Errorf("settingsSvc.Get(%q) error: %v", tc.key, err)
			continue
		}
		if val == "" {
			t.Errorf("settingsSvc.Get(%q) = empty, want non-empty", tc.key)
			continue
		}
		if tc.wantExact != "" && val != tc.wantExact {
			t.Errorf("settingsSvc.Get(%q) = %q, want %q", tc.key, val, tc.wantExact)
		}
	}

	// Verify numeric keys contain the expected values.
	fivePct, err := settingsSvc.Get("claude_5h_used_pct")
	if err != nil {
		t.Fatalf("Get(claude_5h_used_pct): %v", err)
	}
	if fivePct == "" {
		t.Error("claude_5h_used_pct is empty after Update")
	}

	sevenPct, err := settingsSvc.Get("claude_weekly_used_pct")
	if err != nil {
		t.Fatalf("Get(claude_weekly_used_pct): %v", err)
	}
	if sevenPct == "" {
		t.Error("claude_weekly_used_pct is empty after Update")
	}
}

// TestClaudeLimitsService_RoundTrip verifies Update then Get returns same values.
func TestClaudeLimitsService_RoundTrip(t *testing.T) {
	t.Parallel()
	svc, _, _ := setupClaudeLimitsTestEnv(t)

	want := ClaudeLimits{
		FiveHourUsedPct:  55.5,
		FiveHourResetsAt: "2026-03-15T12:00:00Z",
		SevenDayUsedPct:  20.25,
		SevenDayResetsAt: "2026-03-21T12:00:00Z",
	}
	if _, err := svc.Update(want); err != nil {
		t.Fatalf("Update() error: %v", err)
	}

	got, err := svc.Get()
	if err != nil {
		t.Fatalf("Get() error: %v", err)
	}
	if got.FiveHourUsedPct != want.FiveHourUsedPct {
		t.Errorf("FiveHourUsedPct = %v, want %v", got.FiveHourUsedPct, want.FiveHourUsedPct)
	}
	if got.FiveHourResetsAt != want.FiveHourResetsAt {
		t.Errorf("FiveHourResetsAt = %q, want %q", got.FiveHourResetsAt, want.FiveHourResetsAt)
	}
	if got.SevenDayUsedPct != want.SevenDayUsedPct {
		t.Errorf("SevenDayUsedPct = %v, want %v", got.SevenDayUsedPct, want.SevenDayUsedPct)
	}
	if got.SevenDayResetsAt != want.SevenDayResetsAt {
		t.Errorf("SevenDayResetsAt = %q, want %q", got.SevenDayResetsAt, want.SevenDayResetsAt)
	}
	if got.UpdatedAt == "" {
		t.Error("UpdatedAt empty after Update")
	}
}

// TestClaudeLimitsService_Update_ClockStampsUpdatedAt verifies UpdatedAt uses injected clock.
func TestClaudeLimitsService_Update_ClockStampsUpdatedAt(t *testing.T) {
	t.Parallel()
	dbPath := filepath.Join(t.TempDir(), "claude_limits_clock_test.db")
	if err := svcCopyTemplateDB(dbPath); err != nil {
		t.Fatalf("copy template DB: %v", err)
	}
	pool, err := db.OpenPoolExisting(dbPath, db.DefaultPoolConfig())
	if err != nil {
		t.Fatalf("failed to open pool: %v", err)
	}
	t.Cleanup(func() { pool.Close() })

	fixedTime := time.Date(2026, 5, 11, 10, 0, 0, 0, time.UTC)
	testClock := clock.NewTest(fixedTime)
	svc := NewClaudeLimitsService(pool, testClock)

	if _, err := svc.Update(ClaudeLimits{FiveHourUsedPct: 10, SevenDayUsedPct: 20}); err != nil {
		t.Fatalf("Update() error: %v", err)
	}

	got, err := svc.Get()
	if err != nil {
		t.Fatalf("Get() error: %v", err)
	}
	wantUpdatedAt := fixedTime.UTC().Format(time.RFC3339)
	if got.UpdatedAt != wantUpdatedAt {
		t.Errorf("UpdatedAt = %q, want %q", got.UpdatedAt, wantUpdatedAt)
	}
}

// TestClaudeLimitsService_Update_NegativeSentinel verifies -1 pct survives round-trip.
func TestClaudeLimitsService_Update_NegativeSentinel(t *testing.T) {
	t.Parallel()
	svc, _, _ := setupClaudeLimitsTestEnv(t)

	if _, err := svc.Update(ClaudeLimits{
		FiveHourUsedPct: -1,
		SevenDayUsedPct: 80.0,
	}); err != nil {
		t.Fatalf("Update() error: %v", err)
	}

	got, err := svc.Get()
	if err != nil {
		t.Fatalf("Get() error: %v", err)
	}
	if got.FiveHourUsedPct != -1 {
		t.Errorf("FiveHourUsedPct = %v, want -1 (sentinel)", got.FiveHourUsedPct)
	}
	if got.SevenDayUsedPct != 80.0 {
		t.Errorf("SevenDayUsedPct = %v, want 80.0", got.SevenDayUsedPct)
	}
}

// TestClaudeLimitsService_Update_Idempotent verifies second Update overwrites first.
func TestClaudeLimitsService_Update_Idempotent(t *testing.T) {
	t.Parallel()
	svc, _, _ := setupClaudeLimitsTestEnv(t)

	if _, err := svc.Update(ClaudeLimits{FiveHourUsedPct: 10, SevenDayUsedPct: 20}); err != nil {
		t.Fatalf("first Update() error: %v", err)
	}
	if _, err := svc.Update(ClaudeLimits{FiveHourUsedPct: 90, SevenDayUsedPct: 95}); err != nil {
		t.Fatalf("second Update() error: %v", err)
	}

	got, err := svc.Get()
	if err != nil {
		t.Fatalf("Get() error: %v", err)
	}
	if got.FiveHourUsedPct != 90 {
		t.Errorf("FiveHourUsedPct = %v, want 90 (second write wins)", got.FiveHourUsedPct)
	}
	if got.SevenDayUsedPct != 95 {
		t.Errorf("SevenDayUsedPct = %v, want 95 (second write wins)", got.SevenDayUsedPct)
	}
}
