package orchestrator

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"be/internal/logger"
	"be/internal/repo"
	"be/internal/spawner"
)

// resetCallbackSessions resets the plan's scope, logging (rather than dropping) a failure:
// a failed reset leaves stale sessions and findings that the callback replay would then read.
func resetCallbackSessions(ctx context.Context, asRepo *repo.AgentSessionRepo, wfiID string, scope []string) {
	if err := asRepo.ResetAgentSessionsInWorkflow(wfiID, scope); err != nil {
		logger.Error(ctx, "callback session reset failed", "instance_id", wfiID, "err", err)
	}
}

// callbackPlanStep is one unit of re-execution within a callback plan.
type callbackPlanStep struct {
	layer        int
	wholeLayer   bool
	nodes        []string          // node ids, used when !wholeLayer
	layerInstr   string            // instruction for whole-layer steps
	perNodeInstr map[string]string // node id→instruction for !wholeLayer steps
}

// callbackPlan is the pre-computed execution plan for a batch of callback requests.
type callbackPlan struct {
	steps       []callbackPlanStep
	resetScope  []string
	resumeLayer int
}

// decomposedRequest is the intermediate per-CallbackError plan before merging.
type decomposedRequest struct {
	steps       []callbackPlanStep
	resetScope  []string
	resumeLayer int
	agentID     string // contributing agent for deterministic merge ordering
}

// layerIndexOf returns the slice index of layer in groups, or -1 if not found.
func layerIndexOf(layer int, groups []layerGroup) int {
	for i, g := range groups {
		if g.layer == layer {
			return i
		}
	}
	return -1
}

// validateCallbackRequest returns an error if the callback request is invalid.
func validateCallbackRequest(req *spawner.CallbackError, originatorLayer int, groups []layerGroup) error {
	switch req.Mode {
	case "agent":
		tl, _, ok := agentLayerOf(req.TargetAgent, groups)
		if !ok {
			return fmt.Errorf("agent %q not found in workflow", req.TargetAgent)
		}
		if tl > originatorLayer {
			return fmt.Errorf("agent %q (layer %d) exceeds originator layer %d", req.TargetAgent, tl, originatorLayer)
		}
	case "chain":
		if len(req.Chain) == 0 {
			return fmt.Errorf("chain is empty")
		}
		prev := -1
		for _, id := range req.Chain {
			l, _, ok := agentLayerOf(id, groups)
			if !ok {
				return fmt.Errorf("chain agent %q not found in workflow", id)
			}
			if l > originatorLayer {
				return fmt.Errorf("chain agent %q (layer %d) exceeds originator layer %d", id, l, originatorLayer)
			}
			if l <= prev {
				return fmt.Errorf("chain layers must be strictly ascending: got layer %d after %d", l, prev)
			}
			prev = l
		}
	default: // level (or empty = level)
		if layerIndexOf(req.Level, groups) < 0 {
			return fmt.Errorf("level %d not found in workflow", req.Level)
		}
		if req.Level > originatorLayer {
			return fmt.Errorf("level %d exceeds originator layer %d", req.Level, originatorLayer)
		}
	}
	return nil
}

// decomposeCallback converts one CallbackError into a decomposedRequest using per-mode rules.
func decomposeCallback(req *spawner.CallbackError, originatorLayer int, groups []layerGroup) decomposedRequest {
	switch req.Mode {
	case "agent":
		return decomposeAgentCallback(req, originatorLayer, groups)
	case "chain":
		return decomposeChainCallback(req, groups)
	default:
		return decomposeLevelCallback(req, originatorLayer, groups)
	}
}

// mergeCallbackPlans merges multiple decomposedRequests into one callbackPlan.
// Whole-layer wins over per-agent for same layer; instructions joined sorted by contributor;
// per-agent first-non-empty sorted by contributor agent ID; resetScope = deduped union;
// resumeLayer = max.
func mergeCallbackPlans(parts []decomposedRequest) callbackPlan {
	// Sort by contributing agent ID for deterministic instruction ordering
	sort.Slice(parts, func(i, j int) bool { return parts[i].agentID < parts[j].agentID })

	type layerMerge struct {
		wholeLayer bool
		instrParts []string          // whole-layer instruction fragments (sorted by agentID)
		nodeSet    map[string]bool   // union of nodes for !wholeLayer
		nodeInstr  map[string]string // first non-empty per-node instruction
	}
	byLayer := make(map[int]*layerMerge)
	resetSet := make(map[string]bool)
	maxResume := 0

	for _, part := range parts {
		for _, s := range part.steps {
			m := byLayer[s.layer]
			if m == nil {
				m = &layerMerge{nodeSet: make(map[string]bool), nodeInstr: make(map[string]string)}
				byLayer[s.layer] = m
			}
			if s.wholeLayer {
				m.wholeLayer = true
				if s.layerInstr != "" {
					m.instrParts = append(m.instrParts, s.layerInstr)
				}
			} else if !m.wholeLayer {
				for _, n := range s.nodes {
					m.nodeSet[n] = true
					if _, exists := m.nodeInstr[n]; !exists && s.perNodeInstr[n] != "" {
						m.nodeInstr[n] = s.perNodeInstr[n]
					}
				}
			}
		}
		for _, r := range part.resetScope {
			resetSet[r] = true
		}
		if part.resumeLayer > maxResume {
			maxResume = part.resumeLayer
		}
	}

	var layers []int
	for l := range byLayer {
		layers = append(layers, l)
	}
	sort.Ints(layers)

	steps := make([]callbackPlanStep, 0, len(layers))
	for _, l := range layers {
		m := byLayer[l]
		s := callbackPlanStep{layer: l, wholeLayer: m.wholeLayer}
		if m.wholeLayer {
			s.layerInstr = strings.Join(m.instrParts, "\n---\n")
		} else {
			nodes := make([]string, 0, len(m.nodeSet))
			for n := range m.nodeSet {
				nodes = append(nodes, n)
			}
			sort.Strings(nodes)
			s.nodes = nodes
			s.perNodeInstr = make(map[string]string)
			for _, n := range nodes {
				s.perNodeInstr[n] = m.nodeInstr[n]
			}
		}
		steps = append(steps, s)
	}

	resetScope := make([]string, 0, len(resetSet))
	for r := range resetSet {
		resetScope = append(resetScope, r)
	}
	sort.Strings(resetScope)

	return callbackPlan{steps: steps, resetScope: resetScope, resumeLayer: maxResume}
}

// cumulativeAgentCount counts total agent spawns the plan would perform.
// Whole-layer steps contribute len(phases) from layerGroups; per-node steps contribute len(step.nodes).
func cumulativeAgentCount(plan callbackPlan, groups []layerGroup) int {
	total := 0
	for _, s := range plan.steps {
		if s.wholeLayer {
			if idx := layerIndexOf(s.layer, groups); idx >= 0 {
				total += len(groups[idx].phases)
			}
		} else {
			total += len(s.nodes)
		}
	}
	return total
}
