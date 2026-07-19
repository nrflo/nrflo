package spawner

import (
	"encoding/json"
	"testing"
	"time"

	"be/internal/clock"
)

// TestDispatchTokenUsage_SplitsFreshVsCachedInput drives the real
// dispatchAppServerEvent -> dispatchTokenUsage path (not store.setUsage
// directly) with a cumulative codex tokenUsage event carrying
// cachedInputTokens, and asserts the resulting SessionCost only bills the
// fresh (non-cached) portion of inputTokens at the full input rate, with the
// cached portion billed at cache-read rate — inputTokens already includes
// cachedInputTokens, so double-billing would silently inflate cost.
func TestDispatchTokenUsage_SplitsFreshVsCachedInput(t *testing.T) {
	t.Parallel()
	pool := setupTestDB(t)
	insertCostTestSession(t, pool, "sess-codex-dispatch", "gpt-5.6-terra")

	clk := clock.NewTest(time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC))
	t.Cleanup(func() { globalCostStore.drop("sess-codex-dispatch") })
	RegisterSessionCost("sess-codex-dispatch", "gpt-5.6-terra", pool, clk, nil)

	sink := &testSink{}
	// total.inputTokens (120000) already includes cachedInputTokens (30000):
	// fresh input = 90000.
	params := json.RawMessage(`{"tokenUsage":{"total":{"inputTokens":120000,"cachedInputTokens":30000,"outputTokens":8000},"modelContextWindow":258400}}`)
	dispatchTokenUsage("sess-codex-dispatch", params, sink, 200000, nil)

	snap, ok := SessionCost("sess-codex-dispatch")
	if !ok {
		t.Fatal("SessionCost ok = false after dispatchTokenUsage")
	}
	if snap.InputTokens != 90_000 {
		t.Errorf("InputTokens = %d, want 90000 (120000 total - 30000 cached)", snap.InputTokens)
	}
	if snap.CacheReadTokens != 30_000 {
		t.Errorf("CacheReadTokens = %d, want 30000", snap.CacheReadTokens)
	}
	if snap.OutputTokens != 8_000 {
		t.Errorf("OutputTokens = %d, want 8000", snap.OutputTokens)
	}
	// gpt-5.6-terra: price_in=2.5, price_out=15, cache_read=0.25 per MTok.
	want := 90_000.0/1e6*2.5 + 8_000.0/1e6*15 + 30_000.0/1e6*0.25
	if diff := snap.CostUSD - want; diff < -0.0001 || diff > 0.0001 {
		t.Errorf("CostUSD = %v, want %v", snap.CostUSD, want)
	}

	// A second, larger cumulative event must overwrite (not add to) the
	// running counters — codex reports totals, not deltas.
	params2 := json.RawMessage(`{"tokenUsage":{"total":{"inputTokens":200000,"cachedInputTokens":50000,"outputTokens":15000},"modelContextWindow":258400}}`)
	dispatchTokenUsage("sess-codex-dispatch", params2, sink, 200000, nil)
	snap2, _ := SessionCost("sess-codex-dispatch")
	if snap2.InputTokens != 150_000 || snap2.CacheReadTokens != 50_000 || snap2.OutputTokens != 15_000 {
		t.Errorf("second dispatch snapshot = %+v, want in:150000 cacheRd:50000 out:15000 (overwritten, not summed)", snap2)
	}
}
