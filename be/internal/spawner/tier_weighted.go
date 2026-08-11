package spawner

import (
	"context"
	"time"

	"be/internal/logger"
	"be/internal/repo"
	"be/internal/service"
)

// weightedRotationWindow is the sliding window over which landed spawns are
// counted for the weighted-rotation deficit pick.
const weightedRotationWindow = 24 * time.Hour

// applyWeightedRotation reorders chain per its configured entry weights: the
// most under-served weighted entry (landed agent_sessions counts vs weight
// share, keyed by canonical tier_models position) moves to the front; the
// rest stay ordinal as the fallback path. A chain without weights (the
// default), a credential-less weighted entry, or a count-query failure
// degrades to the unrotated chain — strict ordinal.
func (s *Spawner) applyWeightedRotation(ctx context.Context, chain []service.AgentChainEntry, projectID string) []service.AgentChainEntry {
	if len(chain) < 2 || !chainHasWeights(chain) {
		return chain
	}

	counts := map[int]int{}
	if pool := s.pool(); pool != nil {
		since := s.config.Clock.Now().Add(-weightedRotationWindow)
		c, err := repo.NewAgentSessionRepo(pool, s.config.Clock).CountTierResolutions(chain[0].Tier, since)
		if err != nil {
			logger.Warn(ctx, "weighted rotation: count tier resolutions failed", "tier", chain[0].Tier, "err", err)
		} else {
			counts = c
		}
	}

	rotated := service.WeightedChainOrder(chain, counts, func(e service.AgentChainEntry) bool {
		return !s.skipAPIEntryNoCredentials(ctx, e, projectID)
	})
	if rotated[0].Position != chain[0].Position {
		logger.Info(ctx, "weighted rotation: non-primary start", "tier", chain[0].Tier,
			"position", rotated[0].Position, "provider", rotated[0].Provider, "model", rotated[0].ModelID)
	}
	return rotated
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
