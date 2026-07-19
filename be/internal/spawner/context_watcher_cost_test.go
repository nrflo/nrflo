package spawner

import (
	"testing"
	"time"

	"be/internal/clock"
)

// TestLookupModelPricing_SeededModel_ReturnsPricing verifies a seeded model
// row resolves its four per-MTok prices exactly, and parseModelID's "cli:"
// composite form is stripped before lookup.
func TestLookupModelPricing_SeededModel_ReturnsPricing(t *testing.T) {
	t.Parallel()
	pool := setupTestDB(t)
	clk := clock.NewTest(time.Now())

	pr, ok := lookupModelPricing(pool, clk, "sonnet-5")
	if !ok {
		t.Fatal("lookupModelPricing ok = false for seeded sonnet-5")
	}
	if pr.in != 3 || pr.out != 15 || pr.cacheWrite != 3.75 || pr.cacheRead != 0.3 {
		t.Errorf("pricing = %+v, want in:3 out:15 cacheWrite:3.75 cacheRead:0.3", pr)
	}

	// "claude:sonnet-5" is the processInfo.modelID composite form; the cli
	// prefix must be stripped before the registry lookup.
	pr2, ok2 := lookupModelPricing(pool, clk, "claude:sonnet-5")
	if !ok2 {
		t.Fatal("lookupModelPricing ok = false for prefixed modelID")
	}
	if pr2 != pr {
		t.Errorf("prefixed lookup pricing = %+v, want identical to bare lookup %+v", pr2, pr)
	}
}

// TestLookupModelPricing_DegradesGracefully covers the ok=false paths: nil
// pool, empty modelID, unknown model id, and a model row with NULL price_in.
func TestLookupModelPricing_DegradesGracefully(t *testing.T) {
	t.Parallel()
	pool := setupTestDB(t)
	clk := clock.NewTest(time.Now())
	mustExec(t, pool, `UPDATE models SET price_in = NULL WHERE id = 'gpt-5.5'`)

	t.Run("nil pool", func(t *testing.T) {
		if _, ok := lookupModelPricing(nil, clk, "sonnet-5"); ok {
			t.Error("ok = true with a nil pool, want false")
		}
	})
	t.Run("empty modelID", func(t *testing.T) {
		if _, ok := lookupModelPricing(pool, clk, ""); ok {
			t.Error("ok = true with an empty modelID, want false")
		}
	})
	t.Run("unknown model id", func(t *testing.T) {
		if _, ok := lookupModelPricing(pool, clk, "not-a-real-model"); ok {
			t.Error("ok = true for an unknown model id, want false")
		}
	})
	t.Run("NULL price_in falls back", func(t *testing.T) {
		if _, ok := lookupModelPricing(pool, clk, "gpt-5.5"); ok {
			t.Error("ok = true for a model with NULL price_in, want false")
		}
	})
}

// TestPricingCostEstimator_KnownModel_ScalesByCacheReadRate verifies
// EstCostSaved prices evicted tokens at the model's cache-read rate (retained
// context is re-billed at cache-read on every subsequent turn).
func TestPricingCostEstimator_KnownModel_ScalesByCacheReadRate(t *testing.T) {
	t.Parallel()
	pool := setupTestDB(t)
	clk := clock.NewTest(time.Now())
	est := newPricingCostEstimator(pool, clk)

	// sonnet-5 cache_read = 0.3 per MTok.
	got := est.EstCostSaved("sonnet-5", 2_000_000)
	want := 2_000_000.0 / 1e6 * 0.3
	if got != want {
		t.Errorf("EstCostSaved = %v, want %v", got, want)
	}
}

// TestPricingCostEstimator_UnknownOrNullPricing_ReturnsZero verifies both an
// unknown model id and a NULL-pricing row yield 0, never a panic.
func TestPricingCostEstimator_UnknownOrNullPricing_ReturnsZero(t *testing.T) {
	t.Parallel()
	pool := setupTestDB(t)
	clk := clock.NewTest(time.Now())
	mustExec(t, pool, `UPDATE models SET price_in = NULL, price_cache_read = NULL WHERE id = 'gpt-5.5'`)
	est := newPricingCostEstimator(pool, clk)

	if got := est.EstCostSaved("not-a-real-model", 1_000_000); got != 0 {
		t.Errorf("EstCostSaved(unknown model) = %v, want 0", got)
	}
	if got := est.EstCostSaved("gpt-5.5", 1_000_000); got != 0 {
		t.Errorf("EstCostSaved(NULL pricing) = %v, want 0", got)
	}
}

// TestPricingCostEstimator_NonPositiveTokensEvicted_ReturnsZero verifies the
// tokensEvicted<=0 guard short-circuits before any pricing lookup.
func TestPricingCostEstimator_NonPositiveTokensEvicted_ReturnsZero(t *testing.T) {
	t.Parallel()
	est := newPricingCostEstimator(nil, clock.NewTest(time.Now()))
	if got := est.EstCostSaved("sonnet-5", 0); got != 0 {
		t.Errorf("EstCostSaved(0 tokens) = %v, want 0", got)
	}
	if got := est.EstCostSaved("sonnet-5", -500); got != 0 {
		t.Errorf("EstCostSaved(negative tokens) = %v, want 0", got)
	}
}

// TestPricingCostEstimator_NilPool_DegradesGracefully verifies a nil-pool
// estimator (constructed exactly as newAPIContextWatcher does when Config.Pool
// is unset, e.g. some existing context_watcher_gc_test.go constructions)
// returns 0 rather than panicking.
func TestPricingCostEstimator_NilPool_DegradesGracefully(t *testing.T) {
	t.Parallel()
	est := newPricingCostEstimator(nil, clock.NewTest(time.Now()))
	if got := est.EstCostSaved("sonnet-5", 1_000_000); got != 0 {
		t.Errorf("EstCostSaved with nil pool = %v, want 0", got)
	}
}

// TestPricingCostEstimator_CachesPerModel verifies repeated calls for the same
// modelID reuse the cached lookup: mutating the DB row after the first call
// must not change the second call's result.
func TestPricingCostEstimator_CachesPerModel(t *testing.T) {
	t.Parallel()
	pool := setupTestDB(t)
	clk := clock.NewTest(time.Now())
	est := newPricingCostEstimator(pool, clk)

	first := est.EstCostSaved("sonnet-5", 1_000_000)
	mustExec(t, pool, `UPDATE models SET price_cache_read = 999 WHERE id = 'sonnet-5'`)
	second := est.EstCostSaved("sonnet-5", 1_000_000)

	if first != second {
		t.Errorf("second call = %v, want %v (cached pricing, DB mutation after first call ignored)", second, first)
	}
}
