package spawner

import (
	"testing"
	"time"

	"be/internal/clock"
)

// TestCostStore_CostMathWithinFivePercent feeds fixture usage sequences
// through each provider's accumulation shape and asserts the resulting
// running cost matches a manual per-MTok computation within 5% (exact for
// this store, since there is no estimation involved — only real provider
// usage numbers flow through it).
func TestCostStore_CostMathWithinFivePercent(t *testing.T) {
	t.Parallel()
	pool := setupTestDB(t)

	cases := []struct {
		name        string
		sessionID   string
		modelID     string
		pricePerMT  [4]float64 // in, out, cacheWrite, cacheRead
		feed        func(store *costStore, sessionID string)
		wantIn      int
		wantOut     int
		wantCacheRd int
		wantCacheWr int
	}{
		{
			name:       "api mode: per-turn Usage deltas",
			sessionID:  "sess-fixture-api",
			modelID:    "sonnet-5",
			pricePerMT: [4]float64{3, 15, 3.75, 0.3},
			feed: func(store *costStore, sid string) {
				store.addUsage(sid, 12_000, 3_000, 500, 200)
				store.addUsage(sid, 8_000, 2_500, 300, 0)
				store.addUsage(sid, 15_000, 4_000, 0, 100)
			},
			wantIn: 35_000, wantOut: 9_500, wantCacheRd: 800, wantCacheWr: 300,
		},
		{
			name:       "claude CLI: assistant-only turn usage keys",
			sessionID:  "sess-fixture-cli",
			modelID:    "opus-4-8",
			pricePerMT: [4]float64{5, 25, 6.25, 0.5},
			feed: func(store *costStore, sid string) {
				// mirrors updateClaudeContext's input/cache_read/cache_creation/output feed
				store.addUsage(sid, 20_000, 5_000, 1_000, 500)
				store.addUsage(sid, 18_000, 4_500, 900, 0)
			},
			wantIn: 38_000, wantOut: 9_500, wantCacheRd: 1_900, wantCacheWr: 500,
		},
		{
			name:       "codex: cumulative totals split fresh/cached",
			sessionID:  "sess-fixture-codex",
			modelID:    "gpt-5.6-terra",
			pricePerMT: [4]float64{2.5, 15, 3.125, 0.25},
			feed: func(store *costStore, sid string) {
				store.setUsage(sid, 50_000, 10_000, 5_000, 0)
				store.setUsage(sid, 120_000, 22_000, 15_000, 0)
				store.setUsage(sid, 200_000, 40_000, 30_000, 0) // final cumulative wins
			},
			wantIn: 200_000, wantOut: 40_000, wantCacheRd: 30_000, wantCacheWr: 0,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			insertCostTestSession(t, pool, tc.sessionID, tc.modelID)
			clk := clock.NewTest(time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC))
			store := newCostStore(clk)
			store.register(tc.sessionID, tc.modelID, pool, clk, nil)

			tc.feed(store, tc.sessionID)

			snap, ok := store.snapshot(tc.sessionID)
			if !ok {
				t.Fatalf("%s: no snapshot", tc.name)
			}
			if !snap.PricingKnown {
				t.Fatalf("%s: PricingKnown = false, want true", tc.name)
			}
			if snap.InputTokens != tc.wantIn || snap.OutputTokens != tc.wantOut ||
				snap.CacheReadTokens != tc.wantCacheRd || snap.CacheWriteTokens != tc.wantCacheWr {
				t.Fatalf("%s: tokens = %+v, want in:%d out:%d cacheRd:%d cacheWr:%d",
					tc.name, snap, tc.wantIn, tc.wantOut, tc.wantCacheRd, tc.wantCacheWr)
			}

			wantCost := float64(tc.wantIn)/1e6*tc.pricePerMT[0] +
				float64(tc.wantOut)/1e6*tc.pricePerMT[1] +
				float64(tc.wantCacheWr)/1e6*tc.pricePerMT[2] +
				float64(tc.wantCacheRd)/1e6*tc.pricePerMT[3]

			tolerance := 0.05 * wantCost
			if tolerance == 0 {
				tolerance = 1e-9
			}
			diff := snap.CostUSD - wantCost
			if diff < 0 {
				diff = -diff
			}
			if diff > tolerance {
				t.Errorf("%s: CostUSD = %v, want within 5%% of manual computation %v (diff=%v)", tc.name, snap.CostUSD, wantCost, diff)
			}
		})
	}
}
