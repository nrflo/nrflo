package service

import (
	"path/filepath"
	"testing"
	"time"

	"be/internal/clock"
	"be/internal/db"
)

// newMonotonicTestEnv sets up a ClaudeLimitsService with a TestClock pinned to
// 2026-05-14T10:00:00Z for deterministic monotonic guard tests.
func newMonotonicTestEnv(t *testing.T) (*ClaudeLimitsService, *clock.TestClock) {
	t.Helper()
	dbPath := filepath.Join(t.TempDir(), "monotonic_test.db")
	if err := svcCopyTemplateDB(dbPath); err != nil {
		t.Fatalf("copy template DB: %v", err)
	}
	pool, err := db.OpenPoolExisting(dbPath, db.DefaultPoolConfig())
	if err != nil {
		t.Fatalf("failed to open pool: %v", err)
	}
	t.Cleanup(func() { pool.Close() })
	fixedTime := time.Date(2026, 5, 14, 10, 0, 0, 0, time.UTC)
	clk := clock.NewTest(fixedTime)
	return NewClaudeLimitsService(pool, clk), clk
}

// TestClaudeLimitsService_Monotonic_RejectsPctDecreaseWithinActiveWindow verifies that a
// pct decrease within an active reset window is rejected: Changed==false, stored value
// and updated_at are both unchanged.
//
// Note: SevenDayUsedPct is set to -1 (sentinel) to avoid unintentionally writing the 7d
// window (Go's zero value for float64 is 0.0 which is ≥ 0 and would be accepted).
func TestClaudeLimitsService_Monotonic_RejectsPctDecreaseWithinActiveWindow(t *testing.T) {
	t.Parallel()
	svc, _ := newMonotonicTestEnv(t)

	resetsAt := "2026-05-14T11:00:00Z" // 1h in the future

	first, err := svc.Update(ClaudeLimits{
		FiveHourUsedPct:  50,
		FiveHourResetsAt: resetsAt,
		SevenDayUsedPct:  -1,
	})
	if err != nil {
		t.Fatalf("first Update: %v", err)
	}
	if !first.Changed {
		t.Fatal("first Update: Changed == false, want true")
	}
	updatedAtAfterFirst := first.Stored.UpdatedAt

	// Second update: pct-only (sentinel for 7d, empty resetsAt) so the guard activates on the
	// stored resetsAt and the pairs list stays empty when the pct is rejected → Changed==false.
	second, err := svc.Update(ClaudeLimits{
		FiveHourUsedPct: 20,
		SevenDayUsedPct: -1,
	})
	if err != nil {
		t.Fatalf("second Update: %v", err)
	}
	if second.Changed {
		t.Error("second Update: Changed == true, want false (decrease rejected)")
	}

	got, err := svc.Get()
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got.FiveHourUsedPct != 50 {
		t.Errorf("FiveHourUsedPct = %v, want 50 (decrease rejected)", got.FiveHourUsedPct)
	}
	if got.UpdatedAt != updatedAtAfterFirst {
		t.Errorf("UpdatedAt changed on rejected update: got %q, want %q", got.UpdatedAt, updatedAtAfterFirst)
	}
}

// TestClaudeLimitsService_Monotonic_AcceptsPctIncreaseWithinActiveWindow verifies that a
// pct increase within an active reset window is accepted.
func TestClaudeLimitsService_Monotonic_AcceptsPctIncreaseWithinActiveWindow(t *testing.T) {
	t.Parallel()
	svc, _ := newMonotonicTestEnv(t)

	resetsAt := "2026-05-14T11:00:00Z"

	if _, err := svc.Update(ClaudeLimits{
		FiveHourUsedPct:  50,
		FiveHourResetsAt: resetsAt,
		SevenDayUsedPct:  -1,
	}); err != nil {
		t.Fatalf("first Update: %v", err)
	}

	result, err := svc.Update(ClaudeLimits{
		FiveHourUsedPct:  60,
		FiveHourResetsAt: resetsAt,
		SevenDayUsedPct:  -1,
	})
	if err != nil {
		t.Fatalf("second Update: %v", err)
	}
	if !result.Changed {
		t.Error("second Update: Changed == false, want true (increase accepted)")
	}

	got, err := svc.Get()
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got.FiveHourUsedPct != 60 {
		t.Errorf("FiveHourUsedPct = %v, want 60", got.FiveHourUsedPct)
	}
}

// TestClaudeLimitsService_Monotonic_AcceptsPctDecreaseAfterResetElapsed verifies that a
// pct decrease is accepted when the existing reset window has already elapsed.
func TestClaudeLimitsService_Monotonic_AcceptsPctDecreaseAfterResetElapsed(t *testing.T) {
	t.Parallel()
	svc, _ := newMonotonicTestEnv(t) // now = 2026-05-14T10:00:00Z

	pastResetsAt := "2026-05-14T09:59:00Z"   // 1m in the past → elapsed
	futureResetsAt := "2026-05-14T15:00:00Z" // new active window

	if _, err := svc.Update(ClaudeLimits{
		FiveHourUsedPct:  80,
		FiveHourResetsAt: pastResetsAt,
		SevenDayUsedPct:  -1,
	}); err != nil {
		t.Fatalf("first Update: %v", err)
	}

	result, err := svc.Update(ClaudeLimits{
		FiveHourUsedPct:  5,
		FiveHourResetsAt: futureResetsAt,
		SevenDayUsedPct:  -1,
	})
	if err != nil {
		t.Fatalf("second Update: %v", err)
	}
	if !result.Changed {
		t.Error("second Update: Changed == false, want true (elapsed window bypasses guard)")
	}

	got, err := svc.Get()
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got.FiveHourUsedPct != 5 {
		t.Errorf("FiveHourUsedPct = %v, want 5", got.FiveHourUsedPct)
	}
}

// TestClaudeLimitsService_Monotonic_AcceptsPctDecreaseWhenResetsAtChanged verifies that a
// pct decrease is accepted when the new resets_at differs from the stored one.
func TestClaudeLimitsService_Monotonic_AcceptsPctDecreaseWhenResetsAtChanged(t *testing.T) {
	t.Parallel()
	svc, _ := newMonotonicTestEnv(t)

	t1 := "2026-05-14T11:00:00Z" // future T1
	t2 := "2026-05-14T12:00:00Z" // future T2 (different → new window)

	if _, err := svc.Update(ClaudeLimits{
		FiveHourUsedPct:  80,
		FiveHourResetsAt: t1,
		SevenDayUsedPct:  -1,
	}); err != nil {
		t.Fatalf("first Update: %v", err)
	}

	result, err := svc.Update(ClaudeLimits{
		FiveHourUsedPct:  5,
		FiveHourResetsAt: t2,
		SevenDayUsedPct:  -1,
	})
	if err != nil {
		t.Fatalf("second Update: %v", err)
	}
	if !result.Changed {
		t.Error("second Update: Changed == false, want true (different resets_at bypasses guard)")
	}

	got, err := svc.Get()
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got.FiveHourUsedPct != 5 {
		t.Errorf("FiveHourUsedPct = %v, want 5", got.FiveHourUsedPct)
	}
}

// TestClaudeLimitsService_Monotonic_RejectsOneWindowAcceptsOther verifies that the
// per-window guards are independent: 5h decrease is rejected while 7d increase is accepted
// in the same Update call (Changed==true because at least 7d was written).
func TestClaudeLimitsService_Monotonic_RejectsOneWindowAcceptsOther(t *testing.T) {
	t.Parallel()
	svc, _ := newMonotonicTestEnv(t)

	resetsAt := "2026-05-14T11:00:00Z"

	if _, err := svc.Update(ClaudeLimits{
		FiveHourUsedPct:  50,
		FiveHourResetsAt: resetsAt,
		SevenDayUsedPct:  30,
		SevenDayResetsAt: resetsAt,
	}); err != nil {
		t.Fatalf("first Update: %v", err)
	}

	result, err := svc.Update(ClaudeLimits{
		FiveHourUsedPct:  10, // decrease → reject
		FiveHourResetsAt: resetsAt,
		SevenDayUsedPct:  40, // increase → accept
		SevenDayResetsAt: resetsAt,
	})
	if err != nil {
		t.Fatalf("second Update: %v", err)
	}
	if !result.Changed {
		t.Error("result.Changed == false, want true (7d increase accepted)")
	}

	got, err := svc.Get()
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got.FiveHourUsedPct != 50 {
		t.Errorf("FiveHourUsedPct = %v, want 50 (5h decrease rejected)", got.FiveHourUsedPct)
	}
	if got.SevenDayUsedPct != 40 {
		t.Errorf("SevenDayUsedPct = %v, want 40 (7d increase accepted)", got.SevenDayUsedPct)
	}
}

// TestClaudeLimitsService_Monotonic_EpsilonTolerance verifies the 0.5 epsilon boundary:
// 49.7 (within epsilon of 50) is accepted; 49.0 (outside epsilon) is rejected.
func TestClaudeLimitsService_Monotonic_EpsilonTolerance(t *testing.T) {
	t.Parallel()

	resetsAt := "2026-05-14T11:00:00Z"

	cases := []struct {
		name       string
		newPct     float64
		wantAccept bool
	}{
		{"within_epsilon_49.7", 49.7, true},   // 49.7 >= 50.0 - 0.5 = 49.5 → accept
		{"outside_epsilon_49.0", 49.0, false}, // 49.0 < 49.5 → reject
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			svc, _ := newMonotonicTestEnv(t)

			if _, err := svc.Update(ClaudeLimits{
				FiveHourUsedPct:  50.0,
				FiveHourResetsAt: resetsAt,
				SevenDayUsedPct:  -1,
			}); err != nil {
				t.Fatalf("seed Update: %v", err)
			}

			// Pct-only update with sentinel for 7d; no resetsAt so pairs stays empty when
			// pct is rejected → Changed==false.
			result, err := svc.Update(ClaudeLimits{
				FiveHourUsedPct: tc.newPct,
				SevenDayUsedPct: -1,
			})
			if err != nil {
				t.Fatalf("Update(%.1f): %v", tc.newPct, err)
			}
			if result.Changed != tc.wantAccept {
				t.Errorf("Changed = %v, want %v for pct 50.0 → %.1f", result.Changed, tc.wantAccept, tc.newPct)
			}

			got, err := svc.Get()
			if err != nil {
				t.Fatalf("Get: %v", err)
			}
			if tc.wantAccept && got.FiveHourUsedPct != tc.newPct {
				t.Errorf("FiveHourUsedPct = %v, want %v (accepted)", got.FiveHourUsedPct, tc.newPct)
			}
			if !tc.wantAccept && got.FiveHourUsedPct != 50.0 {
				t.Errorf("FiveHourUsedPct = %v, want 50.0 (rejected)", got.FiveHourUsedPct)
			}
		})
	}
}
