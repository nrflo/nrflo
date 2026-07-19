package spawner

import (
	"sync"

	"be/internal/clock"
	"be/internal/db"
	"be/internal/service"
)

// ContextCostEstimator estimates the dollar cost saved by evicting tokens
// from a session's live context.
type ContextCostEstimator interface {
	EstCostSaved(modelID string, tokensEvicted int) float64
}

// modelPricing is one model row's per-MTok USD pricing, resolved from the
// registry's nullable price_* columns.
type modelPricing struct {
	in, out, cacheWrite, cacheRead float64
}

// lookupModelPricing resolves modelID's per-MTok pricing from the models
// registry. modelID may carry a "cli:" prefix (processInfo.modelID's
// composite form); it is stripped before lookup. ok=false when pool is nil,
// modelID is empty, the row is unknown, or its price_in column is NULL.
func lookupModelPricing(pool *db.Pool, clk clock.Clock, modelID string) (modelPricing, bool) {
	if pool == nil || modelID == "" {
		return modelPricing{}, false
	}
	_, bare := parseModelID(modelID)
	row, err := service.NewModelService(pool, clk).Get(bare)
	if err != nil || row.PriceIn == nil {
		return modelPricing{}, false
	}
	pr := modelPricing{in: *row.PriceIn}
	if row.PriceOut != nil {
		pr.out = *row.PriceOut
	}
	if row.PriceCacheWrite != nil {
		pr.cacheWrite = *row.PriceCacheWrite
	}
	if row.PriceCacheRead != nil {
		pr.cacheRead = *row.PriceCacheRead
	}
	return pr, true
}

// pricingCostEstimator implements ContextCostEstimator against the models
// registry: pricing is resolved once per distinct modelID and cached for the
// estimator's lifetime (one per context watcher, i.e. one per session).
// Retained context is re-billed at the cache-read rate on every subsequent
// turn, so eviction is estimated to save cacheRead-rate dollars per token.
type cachedPricing struct {
	pricing modelPricing
	known   bool
}

type pricingCostEstimator struct {
	pool  *db.Pool
	clock clock.Clock

	mu    sync.Mutex
	cache map[string]cachedPricing
}

func newPricingCostEstimator(pool *db.Pool, clk clock.Clock) *pricingCostEstimator {
	return &pricingCostEstimator{pool: pool, clock: clk, cache: make(map[string]cachedPricing)}
}

func (e *pricingCostEstimator) EstCostSaved(modelID string, tokensEvicted int) float64 {
	if tokensEvicted <= 0 {
		return 0
	}
	e.mu.Lock()
	cached, ok := e.cache[modelID]
	if !ok {
		pr, known := lookupModelPricing(e.pool, e.clock, modelID)
		cached.pricing = pr
		cached.known = known
		e.cache[modelID] = cached
	}
	e.mu.Unlock()
	if !cached.known {
		return 0
	}
	return float64(tokensEvicted) / 1e6 * cached.pricing.cacheRead
}
