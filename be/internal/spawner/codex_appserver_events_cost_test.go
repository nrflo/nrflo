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

// TestDispatchTokenUsage_CacheWriteBilling drives dispatchTokenUsage with a
// 0.145-shaped payload carrying cacheWriteInputTokens and asserts the fresh
// input split subtracts BOTH cachedInputTokens and cacheWriteInputTokens from
// inputTokens, with CacheWriteTokens billed at its own per-MTok rate.
func TestDispatchTokenUsage_CacheWriteBilling(t *testing.T) {
	t.Parallel()
	pool := setupTestDB(t)
	insertCostTestSession(t, pool, "sess-codex-cachewrite", "gpt-5.6-terra")

	clk := clock.NewTest(time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC))
	t.Cleanup(func() { globalCostStore.drop("sess-codex-cachewrite") })
	RegisterSessionCost("sess-codex-cachewrite", "gpt-5.6-terra", pool, clk, nil)

	sink := &testSink{}
	params := json.RawMessage(`{"tokenUsage":{"total":{"inputTokens":120000,"cachedInputTokens":30000,"cacheWriteInputTokens":10000,"outputTokens":8000},"modelContextWindow":258400}}`)
	dispatchTokenUsage("sess-codex-cachewrite", params, sink, 200000, nil)

	snap, ok := SessionCost("sess-codex-cachewrite")
	if !ok {
		t.Fatal("SessionCost ok = false after dispatchTokenUsage")
	}
	if snap.InputTokens != 80_000 {
		t.Errorf("InputTokens = %d, want 80000 (120000 - 30000 cached - 10000 cache-write)", snap.InputTokens)
	}
	if snap.CacheReadTokens != 30_000 {
		t.Errorf("CacheReadTokens = %d, want 30000", snap.CacheReadTokens)
	}
	if snap.CacheWriteTokens != 10_000 {
		t.Errorf("CacheWriteTokens = %d, want 10000", snap.CacheWriteTokens)
	}
	if snap.OutputTokens != 8_000 {
		t.Errorf("OutputTokens = %d, want 8000", snap.OutputTokens)
	}
	// gpt-5.6-terra: price_in=2.5, price_out=15, cache_read=0.25, cache_write=3.125 per MTok.
	want := 80_000.0/1e6*2.5 + 8_000.0/1e6*15 + 30_000.0/1e6*0.25 + 10_000.0/1e6*3.125
	if diff := snap.CostUSD - want; diff < -0.0001 || diff > 0.0001 {
		t.Errorf("CostUSD = %v, want %v", snap.CostUSD, want)
	}
}

// TestDispatchTokenUsage_NoCacheWriteField_OlderPayloadCompat asserts a
// params blob with no cacheWriteInputTokens field (pre-0.145 codex, or the
// existing full_turn.jsonl fixture shape) parses with CacheWriteTokens 0 and
// the same fresh-input split as today.
func TestDispatchTokenUsage_NoCacheWriteField_OlderPayloadCompat(t *testing.T) {
	t.Parallel()
	pool := setupTestDB(t)
	insertCostTestSession(t, pool, "sess-codex-oldpayload", "gpt-5.6-terra")

	clk := clock.NewTest(time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC))
	t.Cleanup(func() { globalCostStore.drop("sess-codex-oldpayload") })
	RegisterSessionCost("sess-codex-oldpayload", "gpt-5.6-terra", pool, clk, nil)

	sink := &testSink{}
	params := json.RawMessage(`{"tokenUsage":{"total":{"inputTokens":9091,"cachedInputTokens":4480,"outputTokens":177},"modelContextWindow":258400}}`)
	dispatchTokenUsage("sess-codex-oldpayload", params, sink, 200000, nil)

	snap, ok := SessionCost("sess-codex-oldpayload")
	if !ok {
		t.Fatal("SessionCost ok = false after dispatchTokenUsage")
	}
	if snap.CacheWriteTokens != 0 {
		t.Errorf("CacheWriteTokens = %d, want 0 (field absent from payload)", snap.CacheWriteTokens)
	}
	if snap.InputTokens != 9091-4480 {
		t.Errorf("InputTokens = %d, want %d (9091 - 4480 cached, no cache-write to subtract)", snap.InputTokens, 9091-4480)
	}
}

// TestDispatchTokenUsage_FreshInputClampsAtZero guards against a provider
// quirk where inputTokens < cachedInputTokens+cacheWriteInputTokens: the
// fresh (full-rate) portion must floor at 0, never go negative — a negative
// value would silently reduce CostUSD via subtraction.
func TestDispatchTokenUsage_FreshInputClampsAtZero(t *testing.T) {
	t.Parallel()
	pool := setupTestDB(t)
	insertCostTestSession(t, pool, "sess-codex-clamp", "gpt-5.6-terra")

	clk := clock.NewTest(time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC))
	t.Cleanup(func() { globalCostStore.drop("sess-codex-clamp") })
	RegisterSessionCost("sess-codex-clamp", "gpt-5.6-terra", pool, clk, nil)

	sink := &testSink{}
	// cachedInputTokens+cacheWriteInputTokens (6000+5000=11000) exceeds inputTokens (10000).
	params := json.RawMessage(`{"tokenUsage":{"total":{"inputTokens":10000,"cachedInputTokens":6000,"cacheWriteInputTokens":5000,"outputTokens":100},"modelContextWindow":258400}}`)
	dispatchTokenUsage("sess-codex-clamp", params, sink, 200000, nil)

	snap, ok := SessionCost("sess-codex-clamp")
	if !ok {
		t.Fatal("SessionCost ok = false after dispatchTokenUsage")
	}
	if snap.InputTokens != 0 {
		t.Errorf("InputTokens = %d, want 0 (clamped, not negative)", snap.InputTokens)
	}
	if snap.CostUSD < 0 {
		t.Errorf("CostUSD = %v, want >= 0", snap.CostUSD)
	}
}
