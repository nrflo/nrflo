package service

// WeightedChainOrder reorders chain for a weighted-rotation spawn: the entry
// with the largest usage deficit (its weight share minus its share of counts,
// keyed by canonical Position) moves to the front; the rest keep ordinal
// order and remain the fallback path. Entries with Weight <= 0 or rejected by
// eligible (nil = all eligible) never start. A chain with no eligible
// weighted entry is returned unchanged — strict ordinal, today's behavior.
// Deterministic: ties resolve to the lowest position, no randomness.
func WeightedChainOrder(chain []AgentChainEntry, counts map[int]int, eligible func(AgentChainEntry) bool) []AgentChainEntry {
	start := weightedStartIndex(chain, counts, eligible)
	if start <= 0 {
		return chain
	}
	out := make([]AgentChainEntry, 0, len(chain))
	out = append(out, chain[start])
	for i, e := range chain {
		if i != start {
			out = append(out, e)
		}
	}
	return out
}

// weightedStartIndex picks the chain index with the largest deficit between
// its configured weight share and its observed count share among eligible
// weighted entries. Zero observed counts degrade to a pure weight-share pick.
func weightedStartIndex(chain []AgentChainEntry, counts map[int]int, eligible func(AgentChainEntry) bool) int {
	candidates := []int{}
	totalWeight, totalCount := 0, 0
	for i, e := range chain {
		if e.Weight <= 0 {
			continue
		}
		if eligible != nil && !eligible(e) {
			continue
		}
		candidates = append(candidates, i)
		totalWeight += e.Weight
		totalCount += counts[e.Position]
	}
	if len(candidates) == 0 {
		return 0
	}

	best := -1
	bestDeficit := 0.0
	for _, i := range candidates {
		share := float64(chain[i].Weight) / float64(totalWeight)
		actual := 0.0
		if totalCount > 0 {
			actual = float64(counts[chain[i].Position]) / float64(totalCount)
		}
		if deficit := share - actual; best == -1 || deficit > bestDeficit {
			best, bestDeficit = i, deficit
		}
	}
	return best
}
