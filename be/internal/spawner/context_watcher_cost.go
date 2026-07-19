package spawner

// ContextCostEstimator estimates the dollar cost saved by evicting tokens
// from a session's live context. Real per-model pricing lands in
// nrworkflow-187d35; until then every watcher uses zeroCostEstimator so the
// metric is logged (always 0) without a premature pricing table.
type ContextCostEstimator interface {
	EstCostSaved(model string, tokensEvicted int) float64
}

// zeroCostEstimator is the default ContextCostEstimator: no pricing data, no
// estimate.
type zeroCostEstimator struct{}

func (zeroCostEstimator) EstCostSaved(model string, tokensEvicted int) float64 { return 0 }
