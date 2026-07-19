package spawner

import (
	"testing"
	"time"

	"be/internal/clock"
	"be/internal/spawner/apirun"
)

// fakeCostEstimator records every EstCostSaved call and returns a fixed
// value, so a test can assert PlanGC's metrics line invoked it with the
// expected model/tokens.
type fakeCostEstimator struct {
	calls []costCall
	value float64
}

type costCall struct {
	model         string
	tokensEvicted int
}

func (f *fakeCostEstimator) EstCostSaved(model string, tokensEvicted int) float64 {
	f.calls = append(f.calls, costCall{model: model, tokensEvicted: tokensEvicted})
	return f.value
}

// newTestWatcher builds an apiContextWatcher against an isolated
// clock.NewTest-driven ledgerStore (never the process-global one), following
// newAPIContextWatcher for policy-knob resolution (nil pool -> hardcoded
// defaults) and then swapping in the isolated store.
func newTestWatcher(clk clock.Clock, sessionID string, budgetTokens int) *apiContextWatcher {
	w := newAPIContextWatcher(nil, clk, sessionID, "claude-x", budgetTokens)
	w.store = newLedgerStore(clk)
	w.cost = &fakeCostEstimator{}
	return w
}

// seedDialogEntries appends n dialog-kind ledger entries (always
// GC-eligible, no decay check) directly into the watcher's session ledger.
func seedDialogEntries(w *apiContextWatcher, n, tokensEach int) {
	l := w.store.get(w.sessionID)
	for i := 0; i < n; i++ {
		l.append(LedgerKindDialog, tokensEach, "", "", false)
	}
}

// TestAPIContextWatcher_IdleGap_FiresOnlyAfterCacheTTL verifies PlanGC
// declines while idle time since the last consult is under cache_ttl_sec,
// then fires exactly once the moment it reaches the threshold — and does not
// re-fire on an immediately following call (no idle gap has re-accumulated).
func TestAPIContextWatcher_IdleGap_FiresOnlyAfterCacheTTL(t *testing.T) {
	t0 := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)
	clk := clock.NewTest(t0)
	w := newTestWatcher(clk, "sess-idle", 0) // budget disabled: only idle-gap can trigger
	seedDialogEntries(w, 3, 50)

	state := apirun.WatcherState{MessageCount: 20}

	clk.Advance(100 * time.Second) // well under the 300s default cache_ttl_sec
	if _, ok := w.PlanGC(state); ok {
		t.Fatalf("PlanGC fired before cache_ttl_sec elapsed since the last consult")
	}

	clk.Advance(w.cacheTTL) // now >= cache_ttl_sec since the consult above
	plan, ok := w.PlanGC(state)
	if !ok {
		t.Fatalf("PlanGC did not fire once idle >= cache_ttl_sec")
	}
	if plan.TokensEvicted <= 0 {
		t.Errorf("plan.TokensEvicted = %d, want > 0", plan.TokensEvicted)
	}

	if _, ok := w.PlanGC(state); ok {
		t.Errorf("PlanGC fired again immediately after — idle must reset to the last consult, not the last fire")
	}
}

// TestAPIContextWatcher_Throttle_BlocksUntilMinInterval verifies an
// over-budget session is throttled to firing at most once per
// min_epoch_interval_calls consults, with no idle gap involved.
func TestAPIContextWatcher_Throttle_BlocksUntilMinInterval(t *testing.T) {
	t0 := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)
	clk := clock.NewTest(t0)
	w := newTestWatcher(clk, "sess-throttle", 10) // tiny budget: always over
	w.minInterval = 3
	seedDialogEntries(w, 5, 100)

	state := apirun.WatcherState{MessageCount: 20}

	for i := 1; i < w.minInterval; i++ {
		if _, ok := w.PlanGC(state); ok {
			t.Fatalf("PlanGC fired on call %d, want throttled until call %d", i, w.minInterval)
		}
	}
	plan, ok := w.PlanGC(state)
	if !ok {
		t.Fatalf("PlanGC did not fire on the %dth call (min_epoch_interval_calls reached)", w.minInterval)
	}
	if plan.TokensEvicted <= 0 {
		t.Errorf("plan.TokensEvicted = %d, want > 0", plan.TokensEvicted)
	}
}

// TestAPIContextWatcher_NotOverBudgetNotIdle_NeverFires verifies a session
// within budget and with no idle gap never triggers a GC, regardless of how
// many times PlanGC is consulted.
func TestAPIContextWatcher_NotOverBudgetNotIdle_NeverFires(t *testing.T) {
	clk := clock.NewTest(time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC))
	w := newTestWatcher(clk, "sess-under-budget", 1_000_000)
	seedDialogEntries(w, 3, 50)

	state := apirun.WatcherState{MessageCount: 20}
	for i := 0; i < 5; i++ {
		if _, ok := w.PlanGC(state); ok {
			t.Fatalf("PlanGC fired on call %d while under budget and not idle", i)
		}
	}
}

// TestAPIContextWatcher_Metrics_CostEstimatorInvoked verifies a firing GC
// calls the injected ContextCostEstimator with the model and the exact
// evicted-token count from the eviction selection.
func TestAPIContextWatcher_Metrics_CostEstimatorInvoked(t *testing.T) {
	clk := clock.NewTest(time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC))
	w := newTestWatcher(clk, "sess-metrics", 10)
	w.minInterval = 1 // isolate the metrics assert from throttle behavior
	cost := &fakeCostEstimator{value: 0.0042}
	w.cost = cost
	seedDialogEntries(w, 2, 75) // 150 tokens total, well over the 10-token budget

	plan, ok := w.PlanGC(apirun.WatcherState{MessageCount: 20})
	if !ok {
		t.Fatalf("PlanGC did not fire for an over-budget session")
	}
	if len(cost.calls) != 1 {
		t.Fatalf("EstCostSaved called %d times, want 1", len(cost.calls))
	}
	if cost.calls[0].model != "claude-x" {
		t.Errorf("EstCostSaved model = %q, want claude-x", cost.calls[0].model)
	}
	if cost.calls[0].tokensEvicted != plan.TokensEvicted {
		t.Errorf("EstCostSaved tokensEvicted = %d, want %d (plan.TokensEvicted)", cost.calls[0].tokensEvicted, plan.TokensEvicted)
	}
}

// TestAPIContextWatcher_PlanGC_PinnedPrefixNeverEvicted verifies the
// returned plan's KeepPrefixMsgs always equals pinnedPrefixMessages — the
// cache-stability invariant applyCompactionPlan relies on to keep
// msgs[:KeepPrefixMsgs] byte-identical across a GC.
func TestAPIContextWatcher_PlanGC_PinnedPrefixNeverEvicted(t *testing.T) {
	clk := clock.NewTest(time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC))
	w := newTestWatcher(clk, "sess-prefix", 10)
	w.minInterval = 1 // isolate the assert from throttle behavior
	seedDialogEntries(w, 4, 100)

	plan, ok := w.PlanGC(apirun.WatcherState{MessageCount: 30})
	if !ok {
		t.Fatalf("PlanGC did not fire for an over-budget session")
	}
	if plan.KeepPrefixMsgs != pinnedPrefixMessages {
		t.Errorf("plan.KeepPrefixMsgs = %d, want %d (pinnedPrefixMessages)", plan.KeepPrefixMsgs, pinnedPrefixMessages)
	}
}

// TestAPIContextWatcher_NoLedger_NeverFires verifies a session with no
// tracked ledger (never written to, or already dropped) declines cleanly
// instead of panicking on a missing snapshot/epochSummary.
func TestAPIContextWatcher_NoLedger_NeverFires(t *testing.T) {
	clk := clock.NewTest(time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC))
	w := newTestWatcher(clk, "sess-no-ledger", 10)
	// No seedDialogEntries call: the store has never seen this session.

	if _, ok := w.PlanGC(apirun.WatcherState{MessageCount: 20}); ok {
		t.Errorf("PlanGC fired for a session with no tracked ledger")
	}
}

// TestAPIContextWatcher_TinyMessageCount_DoesNotFire verifies PlanGC bails
// when the resolved keep counts would consume the entire (tiny) message
// history — never returning a plan that would evict the pinned prefix or
// recent window.
func TestAPIContextWatcher_TinyMessageCount_DoesNotFire(t *testing.T) {
	clk := clock.NewTest(time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC))
	w := newTestWatcher(clk, "sess-tiny", 10)
	seedDialogEntries(w, 2, 100)

	if _, ok := w.PlanGC(apirun.WatcherState{MessageCount: 3}); ok {
		t.Errorf("PlanGC fired for a message count too small to leave a plan below MessageCount")
	}
}
