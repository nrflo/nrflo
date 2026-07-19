package spawner

import (
	"testing"
	"time"

	"be/internal/clock"
	"be/internal/spawner/apirun/provider"
)

// TestCostOnlyObserver_OnUsage_FeedsSessionCost verifies the console api
// engine's costOnlyObserver.OnUsage feeds AddSessionCostUsage exactly as
// apiConsoleEngine.Start wires it (Config.Observer), and OnMessage stays a
// complete no-op (no ledger, no panic — a console chat has no context
// watcher-driven GC to reconcile against).
func TestCostOnlyObserver_OnUsage_FeedsSessionCost(t *testing.T) {
	t.Parallel()
	pool := setupTestDB(t)
	insertCostTestSession(t, pool, "sess-console-api-cost", "haiku-4-5")

	clk := clock.NewTest(time.Now())
	t.Cleanup(func() { globalCostStore.drop("sess-console-api-cost") })
	RegisterSessionCost("sess-console-api-cost", "haiku-4-5", pool, clk, nil)

	obs := costOnlyObserver{sessionID: "sess-console-api-cost"}
	obs.OnMessage("assistant", nil) // must not panic; must not create any state
	obs.OnUsage(provider.Usage{InputTokens: 4_000, OutputTokens: 1_000})

	snap, ok := SessionCost("sess-console-api-cost")
	if !ok {
		t.Fatal("SessionCost ok = false after costOnlyObserver.OnUsage")
	}
	if snap.InputTokens != 4_000 || snap.OutputTokens != 1_000 {
		t.Errorf("token snapshot = %+v, want in:4000 out:1000", snap)
	}
	// haiku-4-5: price_in=1, price_out=5 per MTok.
	want := 4_000.0/1e6*1 + 1_000.0/1e6*5
	if diff := snap.CostUSD - want; diff < -0.0001 || diff > 0.0001 {
		t.Errorf("CostUSD = %v, want %v", snap.CostUSD, want)
	}
}
