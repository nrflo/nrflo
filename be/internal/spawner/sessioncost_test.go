package spawner

import (
	"database/sql"
	"testing"
	"time"

	"be/internal/clock"
	"be/internal/db"
)

// insertCostTestSession inserts an agent_sessions row with the given model_id,
// reusing setupTestDB/mustExec's seeded proj/T-1/wfi-1 parents (context_test.go).
func insertCostTestSession(t *testing.T, pool *db.Pool, id, modelID string) {
	t.Helper()
	mustExec(t, pool, `INSERT INTO agent_sessions (id, project_id, ticket_id, workflow_instance_id, phase, agent_type, model_id, status, created_at, updated_at)
		VALUES (?, 'proj', 'T-1', 'wfi-1', 'phase1', 'analyzer', ?, 'running', datetime('now'), datetime('now'))`,
		id, modelID)
}

// TestCostStore_AddUsage_ComputesCostAndDebouncesFlush drives the api/claude-CLI
// delta-accumulation shape (AddSessionCostUsage) against a session registered
// with sonnet-5's seeded pricing (price_in=3, price_out=15, cache_write=3.75,
// cache_read=0.3 per MTok), and asserts: the first addUsage flushes+broadcasts
// immediately (zero-value lastFlush), a call inside the debounce window does
// neither, and one after the window elapses does both with the coalesced total.
func TestCostStore_AddUsage_ComputesCostAndDebouncesFlush(t *testing.T) {
	t.Parallel()
	pool := setupTestDB(t)
	insertCostTestSession(t, pool, "sess-cost-add", "sonnet-5")

	clk := clock.NewTest(time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC))
	store := newCostStore(clk)

	var broadcasts []CostSnapshot
	store.register("sess-cost-add", "sonnet-5", pool, clk, func(snap CostSnapshot) {
		broadcasts = append(broadcasts, snap)
	})

	// First call: lastFlush is the zero Time, so now.Sub(zero) always exceeds
	// the debounce window -> flush+broadcast fires immediately.
	store.addUsage("sess-cost-add", 1_000_000, 500_000, 0, 0)
	if len(broadcasts) != 1 {
		t.Fatalf("broadcasts after first addUsage = %d, want 1", len(broadcasts))
	}
	wantFirst := 1_000_000.0/1e6*3 + 500_000.0/1e6*15 // = 3 + 7.5 = 10.5
	if diff := broadcasts[0].CostUSD - wantFirst; diff < -0.0001 || diff > 0.0001 {
		t.Errorf("first broadcast CostUSD = %v, want %v", broadcasts[0].CostUSD, wantFirst)
	}
	if !broadcasts[0].PricingKnown {
		t.Error("first broadcast PricingKnown = false, want true (sonnet-5 has seeded pricing)")
	}

	// Second call within the same instant (debounce window not elapsed): must
	// not flush or broadcast again, even though the in-memory snapshot grows.
	store.addUsage("sess-cost-add", 1_000_000, 0, 0, 0)
	if len(broadcasts) != 1 {
		t.Fatalf("broadcasts after in-window addUsage = %d, want still 1 (debounced)", len(broadcasts))
	}
	var dbCost sql.NullFloat64
	if err := pool.QueryRow(`SELECT cost_estimate FROM agent_sessions WHERE id = ?`, "sess-cost-add").Scan(&dbCost); err != nil {
		t.Fatalf("query cost_estimate: %v", err)
	}
	if diff := dbCost.Float64 - wantFirst; diff < -0.0001 || diff > 0.0001 {
		t.Errorf("DB cost_estimate after in-window call = %v, want unchanged %v (no per-call write)", dbCost.Float64, wantFirst)
	}

	// Advance past the debounce window: the next call flushes+broadcasts the
	// coalesced total (2,000,000 in + 500,000 out; cache still 0).
	clk.Advance(costFlushDebounce)
	store.addUsage("sess-cost-add", 0, 0, 0, 0)
	if len(broadcasts) != 2 {
		t.Fatalf("broadcasts after debounce window elapsed = %d, want 2", len(broadcasts))
	}
	wantSecond := 2_000_000.0/1e6*3 + 500_000.0/1e6*15 // = 6 + 7.5 = 13.5
	if diff := broadcasts[1].CostUSD - wantSecond; diff < -0.0001 || diff > 0.0001 {
		t.Errorf("second broadcast CostUSD = %v, want %v", broadcasts[1].CostUSD, wantSecond)
	}
	if err := pool.QueryRow(`SELECT cost_estimate FROM agent_sessions WHERE id = ?`, "sess-cost-add").Scan(&dbCost); err != nil {
		t.Fatalf("query cost_estimate: %v", err)
	}
	if diff := dbCost.Float64 - wantSecond; diff < -0.0001 || diff > 0.0001 {
		t.Errorf("DB cost_estimate after debounced flush = %v, want %v", dbCost.Float64, wantSecond)
	}
}

// TestCostStore_SetUsage_OverwritesCumulativeCounters drives the codex
// app-server shape (SetSessionCostUsage): each call reports cumulative totals,
// not a delta, so the running snapshot must reflect only the LAST call, never
// the sum across calls.
func TestCostStore_SetUsage_OverwritesCumulativeCounters(t *testing.T) {
	t.Parallel()
	pool := setupTestDB(t)
	insertCostTestSession(t, pool, "sess-cost-set", "gpt-5.6-sol")

	clk := clock.NewTest(time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC))
	store := newCostStore(clk)
	store.register("sess-cost-set", "gpt-5.6-sol", pool, clk, nil)

	// codex reports fresh-vs-cached input already split by the caller
	// (dispatchTokenUsage); simulate three growing cumulative events.
	store.setUsage("sess-cost-set", 100_000, 20_000, 0, 0)
	store.setUsage("sess-cost-set", 250_000, 50_000, 0, 0)
	store.setUsage("sess-cost-set", 400_000, 90_000, 0, 0)

	snap, ok := store.snapshot("sess-cost-set")
	if !ok {
		t.Fatal("snapshot ok = false after setUsage")
	}
	if snap.InputTokens != 400_000 || snap.OutputTokens != 90_000 {
		t.Errorf("snapshot tokens = in:%d out:%d, want in:400000 out:90000 (last cumulative call, not summed)",
			snap.InputTokens, snap.OutputTokens)
	}
	// gpt-5.6-sol: price_in=5, price_out=30 per MTok.
	want := 400_000.0/1e6*5 + 90_000.0/1e6*30
	if diff := snap.CostUSD - want; diff < -0.0001 || diff > 0.0001 {
		t.Errorf("CostUSD = %v, want %v", snap.CostUSD, want)
	}
}

// TestCostStore_UnknownModel_PricingUnknownCostStaysZero verifies a session
// registered against a model with no seeded pricing (or an unknown id) reports
// PricingKnown=false and CostUSD=0 regardless of usage volume.
func TestCostStore_UnknownModel_PricingUnknownCostStaysZero(t *testing.T) {
	t.Parallel()
	pool := setupTestDB(t)
	insertCostTestSession(t, pool, "sess-cost-unknown", "not-a-real-model")

	clk := clock.NewTest(time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC))
	store := newCostStore(clk)
	store.register("sess-cost-unknown", "not-a-real-model", pool, clk, nil)

	store.addUsage("sess-cost-unknown", 5_000_000, 1_000_000, 0, 0)

	snap, ok := store.snapshot("sess-cost-unknown")
	if !ok {
		t.Fatal("snapshot ok = false")
	}
	if snap.PricingKnown {
		t.Error("PricingKnown = true for an unregistered model id, want false")
	}
	if snap.CostUSD != 0 {
		t.Errorf("CostUSD = %v, want 0 when pricing is unknown", snap.CostUSD)
	}
	// Token counters must still accumulate even when pricing is unknown.
	if snap.InputTokens != 5_000_000 || snap.OutputTokens != 1_000_000 {
		t.Errorf("tokens = in:%d out:%d, want in:5000000 out:1000000 (counters track regardless of pricing)",
			snap.InputTokens, snap.OutputTokens)
	}
}

// TestCostStore_NoRegisteredSession_IsNoOp verifies addUsage/setUsage/snapshot
// on a session that was never registered (or already dropped) never panics and
// reports ok=false.
func TestCostStore_NoRegisteredSession_IsNoOp(t *testing.T) {
	t.Parallel()
	clk := clock.NewTest(time.Now())
	store := newCostStore(clk)

	store.addUsage("sess-never-registered", 100, 50, 0, 0)
	store.setUsage("sess-never-registered", 100, 50, 0, 0)

	if _, ok := store.snapshot("sess-never-registered"); ok {
		t.Error("snapshot ok = true for a never-registered session, want false")
	}
}

// TestCostStore_FinalizeSessionCost_FlushesImmediatelyAndDrops verifies
// FinalizeSessionCost bypasses the debounce window (so a session's last delta
// is never lost at session end) and removes the entry from the store.
func TestCostStore_FinalizeSessionCost_FlushesImmediatelyAndDrops(t *testing.T) {
	t.Parallel()
	pool := setupTestDB(t)
	insertCostTestSession(t, pool, "sess-cost-finalize", "haiku-4-5")

	clk := clock.NewTest(time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC))
	t.Cleanup(func() { globalCostStore.drop("sess-cost-finalize") })

	RegisterSessionCost("sess-cost-finalize", "haiku-4-5", pool, clk, nil)
	AddSessionCostUsage("sess-cost-finalize", 1_000_000, 200_000, 0, 0)

	// A second call inside the same instant would normally be debounced —
	// prove the pending delta is still captured by the forced final flush.
	AddSessionCostUsage("sess-cost-finalize", 0, 100_000, 0, 0)

	FinalizeSessionCost("sess-cost-finalize")

	// haiku-4-5: price_in=1, price_out=5 per MTok.
	want := 1_000_000.0/1e6*1 + 300_000.0/1e6*5
	var dbCost sql.NullFloat64
	if err := pool.QueryRow(`SELECT cost_estimate FROM agent_sessions WHERE id = ?`, "sess-cost-finalize").Scan(&dbCost); err != nil {
		t.Fatalf("query cost_estimate: %v", err)
	}
	if !dbCost.Valid {
		t.Fatal("cost_estimate is NULL after FinalizeSessionCost, want a flushed value")
	}
	if diff := dbCost.Float64 - want; diff < -0.0001 || diff > 0.0001 {
		t.Errorf("DB cost_estimate after finalize = %v, want %v", dbCost.Float64, want)
	}

	if _, ok := SessionCost("sess-cost-finalize"); ok {
		t.Error("SessionCost ok = true after FinalizeSessionCost, want false (entry dropped)")
	}
}
