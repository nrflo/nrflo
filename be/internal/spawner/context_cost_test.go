package spawner

import (
	"testing"
	"time"

	"be/internal/clock"
)

// TestUpdateClaudeContext_FeedsSessionCost drives the real
// Spawner.updateClaudeContext (not the store directly) with a Claude
// assistant-event usage payload (message.usage — the exact shape output.go
// passes through), and asserts the session's running cost reflects that
// turn's delta at the seeded per-MTok pricing.
func TestUpdateClaudeContext_FeedsSessionCost(t *testing.T) {
	t.Parallel()
	pool := setupTestDB(t)
	insertCostTestSession(t, pool, "sess-claude-cost", "opus-4-8")

	clk := clock.NewTest(time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC))
	t.Cleanup(func() { globalCostStore.drop("sess-claude-cost") })
	RegisterSessionCost("sess-claude-cost", "opus-4-8", pool, clk, nil)

	s := New(Config{Pool: pool, Clock: clk})
	proc := &processInfo{sessionID: "sess-claude-cost", maxContext: 200000}

	data := map[string]interface{}{
		"message": map[string]interface{}{
			"usage": map[string]interface{}{
				"input_tokens":                10000.0,
				"cache_read_input_tokens":     2000.0,
				"cache_creation_input_tokens": 500.0,
				"output_tokens":               3000.0,
			},
		},
	}
	s.updateClaudeContext(proc, data)

	snap, ok := SessionCost("sess-claude-cost")
	if !ok {
		t.Fatal("SessionCost ok = false after updateClaudeContext")
	}
	if snap.InputTokens != 10_000 || snap.OutputTokens != 3_000 || snap.CacheReadTokens != 2_000 || snap.CacheWriteTokens != 500 {
		t.Errorf("token snapshot = %+v, want in:10000 out:3000 cacheRd:2000 cacheWr:500", snap)
	}
	// opus-4-8: price_in=5, price_out=25, cache_write=6.25, cache_read=0.5 per MTok.
	want := 10_000.0/1e6*5 + 3_000.0/1e6*25 + 500.0/1e6*6.25 + 2_000.0/1e6*0.5
	if diff := snap.CostUSD - want; diff < -0.0001 || diff > 0.0001 {
		t.Errorf("CostUSD = %v, want %v", snap.CostUSD, want)
	}

	// A second turn's usage must accumulate as a delta, not overwrite.
	data2 := map[string]interface{}{
		"message": map[string]interface{}{
			"usage": map[string]interface{}{
				"input_tokens":  5000.0,
				"output_tokens": 1000.0,
			},
		},
	}
	s.updateClaudeContext(proc, data2)
	snap2, _ := SessionCost("sess-claude-cost")
	if snap2.InputTokens != 15_000 || snap2.OutputTokens != 4_000 {
		t.Errorf("after second turn, tokens = %+v, want in:15000 out:4000 (accumulated delta)", snap2)
	}
}

// TestUpdateClaudeContext_NoUsage_DoesNotTouchSessionCost verifies an event
// with no usage payload (e.g. a non-assistant message) leaves the session's
// running cost untouched rather than zeroing or erroring.
func TestUpdateClaudeContext_NoUsage_DoesNotTouchSessionCost(t *testing.T) {
	t.Parallel()
	pool := setupTestDB(t)
	insertCostTestSession(t, pool, "sess-claude-no-usage", "sonnet-5")

	clk := clock.NewTest(time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC))
	t.Cleanup(func() { globalCostStore.drop("sess-claude-no-usage") })
	RegisterSessionCost("sess-claude-no-usage", "sonnet-5", pool, clk, nil)

	s := New(Config{Pool: pool, Clock: clk})
	proc := &processInfo{sessionID: "sess-claude-no-usage", maxContext: 200000}
	s.updateClaudeContext(proc, map[string]interface{}{"type": "system"})

	snap, ok := SessionCost("sess-claude-no-usage")
	if !ok {
		t.Fatal("SessionCost ok = false")
	}
	if snap.InputTokens != 0 || snap.OutputTokens != 0 || snap.CostUSD != 0 {
		t.Errorf("snapshot = %+v, want all zero (no usage payload)", snap)
	}
}
