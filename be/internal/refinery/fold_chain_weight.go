package refinery

import (
	"context"
	"time"

	"be/internal/logger"
	"be/internal/service"
)

// weightedRotationWindow mirrors the spawner's rotation window: the sliding
// window over which landed folds are counted for the deficit pick.
const weightedRotationWindow = 24 * time.Hour

// applyWeightedRotation reorders the fold chain per its entry weights,
// counting landed refinery_runs folds by canonical position (mirrors
// spawner.applyWeightedRotation over agent_sessions). Weightless chains,
// credential-less weighted entries, and count-query failures degrade to the
// unrotated chain.
func (m *Manager) applyWeightedRotation(ctx context.Context, chain []service.AgentChainEntry, projectID string) []service.AgentChainEntry {
	if len(chain) < 2 || !chainHasWeights(chain) {
		return chain
	}

	counts, err := m.runRepo.CountLandedByPosition(m.clock.Now().Add(-weightedRotationWindow))
	if err != nil {
		logger.Warn(ctx, "refinery: weighted rotation count failed", "error", err)
		counts = map[int]int{}
	}

	return service.WeightedChainOrder(chain, counts, func(e service.AgentChainEntry) bool {
		return e.ExecutionMode == "cli_interactive" || hasAPICreds(ctx, m.pool, m.clock, e.Provider, projectID)
	})
}

// chainHasWeights reports whether any entry opts into weighted rotation.
func chainHasWeights(chain []service.AgentChainEntry) bool {
	for _, e := range chain {
		if e.Weight > 0 {
			return true
		}
	}
	return false
}
