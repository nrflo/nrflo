package spawner

import (
	"testing"
	"time"

	"be/internal/clock"
)

// TestCostStore_Interleaved_AddUsage_SetUsage_Monotonic is the flaw-1
// regression test: a refinery addUsage fold landing between two codex
// setUsage cumulative reports must survive the second setUsage call, and the
// snapshot must never decrease across the sequence.
func TestCostStore_Interleaved_AddUsage_SetUsage_Monotonic(t *testing.T) {
	t.Parallel()
	pool := setupTestDB(t)
	insertCostTestSession(t, pool, "sess-cost-interleave", "gpt-5.6-sol")

	clk := clock.NewTest(time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC))
	store := newCostStore(clk)
	store.register("sess-cost-interleave", "gpt-5.6-sol", pool, clk, nil)

	// codex cumulative report #1.
	store.setUsage("sess-cost-interleave", 100_000, 20_000, 0, 0)
	snap1, _ := store.snapshot("sess-cost-interleave")

	// refinery sidecar folds a per-turn delta onto the same session.
	store.addUsage("sess-cost-interleave", 5_000, 1_000, 0, 0)
	snapAfterFold, _ := store.snapshot("sess-cost-interleave")
	if snapAfterFold.InputTokens != snap1.InputTokens+5_000 || snapAfterFold.OutputTokens != snap1.OutputTokens+1_000 {
		t.Fatalf("snapshot after fold = %+v, want fold tokens added on top of %+v", snapAfterFold, snap1)
	}

	// codex cumulative report #2: its own high water only advances by the
	// codex-side increase (150000-100000=50000in, 25000-20000=5000out); the
	// fold's contribution must survive on top of it, and nothing may drop.
	store.setUsage("sess-cost-interleave", 150_000, 25_000, 0, 0)
	snap2, _ := store.snapshot("sess-cost-interleave")

	wantIn := snapAfterFold.InputTokens + 50_000
	wantOut := snapAfterFold.OutputTokens + 5_000
	if snap2.InputTokens != wantIn || snap2.OutputTokens != wantOut {
		t.Errorf("snapshot after 2nd setUsage = in:%d out:%d, want in:%d out:%d (fold survives, codex delta added on top)",
			snap2.InputTokens, snap2.OutputTokens, wantIn, wantOut)
	}
	if snap2.InputTokens < snapAfterFold.InputTokens || snap2.OutputTokens < snapAfterFold.OutputTokens {
		t.Errorf("snapshot decreased across interleaved feed: before=%+v after=%+v", snapAfterFold, snap2)
	}
}

// TestCostStore_ResetReported_RotationCarry is the flaw-2 regression test: an
// in-place console rotation resets the reported high water but must not touch
// the accumulated snapshot, so a fresh low cumulative report from the new
// thread adds on top of the carried total instead of producing a drop.
func TestCostStore_ResetReported_RotationCarry(t *testing.T) {
	t.Parallel()
	pool := setupTestDB(t)
	insertCostTestSession(t, pool, "sess-cost-rotate", "gpt-5.6-sol")

	clk := clock.NewTest(time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC))
	store := newCostStore(clk)
	store.register("sess-cost-rotate", "gpt-5.6-sol", pool, clk, nil)

	// Accumulate usage on the pre-rotation thread.
	store.setUsage("sess-cost-rotate", 300_000, 60_000, 0, 0)
	carried, _ := store.snapshot("sess-cost-rotate")
	if carried.InputTokens == 0 {
		t.Fatal("carried snapshot is zero, setup failed")
	}

	store.resetReported("sess-cost-rotate")

	// Fresh thread reports a low cumulative total (its own counters restart
	// at 0) — must add on top of the carried snapshot, never drop below it.
	store.setUsage("sess-cost-rotate", 10_000, 2_000, 0, 0)
	snap, _ := store.snapshot("sess-cost-rotate")

	wantIn := carried.InputTokens + 10_000
	wantOut := carried.OutputTokens + 2_000
	if snap.InputTokens != wantIn || snap.OutputTokens != wantOut {
		t.Errorf("snapshot after reset+fresh report = in:%d out:%d, want in:%d out:%d (carried + new, no re-zero)",
			snap.InputTokens, snap.OutputTokens, wantIn, wantOut)
	}
	if snap.InputTokens < carried.InputTokens || snap.OutputTokens < carried.OutputTokens {
		t.Errorf("snapshot dropped below pre-rotation carry: carried=%+v after=%+v", carried, snap)
	}
}

// TestCostStore_SeedReported_TwoHopResume_NoDoubleBilling verifies a resumed
// thread's baseline (seeded via reportedSnapshot at hand-off, exactly as
// backend_resume.go's transferResume does) attributes only the post-resume
// delta on the first hop, and a second consecutive resume of the same thread
// does not re-bill the first segment.
func TestCostStore_SeedReported_TwoHopResume_NoDoubleBilling(t *testing.T) {
	t.Parallel()
	pool := setupTestDB(t)
	insertCostTestSession(t, pool, "sess-cost-resume1", "gpt-5.6-sol")
	insertCostTestSession(t, pool, "sess-cost-resume2", "gpt-5.6-sol")
	insertCostTestSession(t, pool, "sess-cost-resume3", "gpt-5.6-sol")

	clk := clock.NewTest(time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC))
	store := newCostStore(clk)

	// Segment 1: original session accumulates usage pre-crash.
	store.register("sess-cost-resume1", "gpt-5.6-sol", pool, clk, nil)
	store.setUsage("sess-cost-resume1", 100_000, 20_000, 0, 0)
	seg1, _ := store.snapshot("sess-cost-resume1")
	reported1, ok := store.reportedSnapshot("sess-cost-resume1")
	if !ok {
		t.Fatal("reportedSnapshot ok = false for sess-cost-resume1")
	}

	// Hop 1: seed the resumed session's reported high water from the dying
	// session's raw reported cumulative (transferResume's captured baseline).
	store.register("sess-cost-resume2", "gpt-5.6-sol", pool, clk, nil)
	store.seedReported("sess-cost-resume2", reported1.InputTokens, reported1.OutputTokens, reported1.CacheReadTokens, reported1.CacheWriteTokens)
	store.setUsage("sess-cost-resume2", 130_000, 28_000, 0, 0)
	seg2, _ := store.snapshot("sess-cost-resume2")
	if seg2.InputTokens != 30_000 || seg2.OutputTokens != 8_000 {
		t.Errorf("hop-1 snapshot = in:%d out:%d, want in:30000 out:8000 (only the post-resume delta)",
			seg2.InputTokens, seg2.OutputTokens)
	}

	// Hop 2: a second consecutive resume. Its baseline is captured from
	// sess-cost-resume2's reportedSnapshot, not its attributed snap — proving
	// the fix, a stale baseline built from `snap` here would re-bill segment 1.
	reported2, ok := store.reportedSnapshot("sess-cost-resume2")
	if !ok {
		t.Fatal("reportedSnapshot ok = false for sess-cost-resume2")
	}
	store.register("sess-cost-resume3", "gpt-5.6-sol", pool, clk, nil)
	store.seedReported("sess-cost-resume3", reported2.InputTokens, reported2.OutputTokens, reported2.CacheReadTokens, reported2.CacheWriteTokens)
	store.setUsage("sess-cost-resume3", 150_000, 33_000, 0, 0)
	seg3, _ := store.snapshot("sess-cost-resume3")
	if seg3.InputTokens != 20_000 || seg3.OutputTokens != 5_000 {
		t.Errorf("hop-2 snapshot = in:%d out:%d, want in:20000 out:5000 (only the 2nd post-resume delta, segment 1 not re-billed)",
			seg3.InputTokens, seg3.OutputTokens)
	}
	_ = seg1
}

// TestCostStore_MaybeFlushAndBroadcast_HighWaterGuard drives two debounce
// windows where the second in-memory snapshot is lower than the first
// broadcast, and asserts the guard skips both the DB flush and the broadcast
// callback rather than pushing a visible regression.
func TestCostStore_MaybeFlushAndBroadcast_HighWaterGuard(t *testing.T) {
	t.Parallel()
	pool := setupTestDB(t)
	insertCostTestSession(t, pool, "sess-cost-guard", "sonnet-5")

	clk := clock.NewTest(time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC))
	store := newCostStore(clk)

	var broadcasts []CostSnapshot
	store.register("sess-cost-guard", "sonnet-5", pool, clk, func(snap CostSnapshot) {
		broadcasts = append(broadcasts, snap)
	})

	// First window: flushes immediately (zero-value lastFlush).
	store.addUsage("sess-cost-guard", 1_000_000, 200_000, 0, 0)
	if len(broadcasts) != 1 {
		t.Fatalf("broadcasts after first window = %d, want 1", len(broadcasts))
	}
	highCost := broadcasts[0].CostUSD

	// Simulate a reordered debounce goroutine: directly craft a lower
	// in-memory snapshot on the entry (as two racing goroutines releasing the
	// mutex out of order could), then advance past the debounce window and
	// trigger another flush attempt.
	e := store.get("sess-cost-guard")
	e.mu.Lock()
	e.snap.CostUSD = highCost - 1.0
	e.mu.Unlock()

	clk.Advance(costFlushDebounce)
	e.maybeFlushAndBroadcast("sess-cost-guard")

	if len(broadcasts) != 1 {
		t.Errorf("broadcasts after stale-lower snapshot = %d, want still 1 (guard must skip the broadcast)", len(broadcasts))
	}

	var dbCost float64
	if err := pool.QueryRow(`SELECT cost_estimate FROM agent_sessions WHERE id = ?`, "sess-cost-guard").Scan(&dbCost); err != nil {
		t.Fatalf("query cost_estimate: %v", err)
	}
	if diff := dbCost - highCost; diff < -0.0001 || diff > 0.0001 {
		t.Errorf("DB cost_estimate after stale-lower snapshot = %v, want unchanged %v (guard must skip the flush)", dbCost, highCost)
	}
}
