package orchestrator

import (
	"fmt"
	"sort"
	"strings"

	"be/internal/spawner"
)

// callbackPlanStep is one unit of re-execution within a callback plan.
type callbackPlanStep struct {
	layer         int
	wholeLayer    bool
	agents        []string          // used when !wholeLayer
	layerInstr    string            // instruction for whole-layer steps
	perAgentInstr map[string]string // agent→instruction for !wholeLayer steps
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

// agentLayerOf returns the layer number for agentID and true, or 0+false if not found.
func agentLayerOf(agentID string, groups []layerGroup) (int, bool) {
	for _, g := range groups {
		for _, p := range g.phases {
			if p.Agent == agentID {
				return g.layer, true
			}
		}
	}
	return 0, false
}

// validateCallbackRequest returns an error if the callback request is invalid.
func validateCallbackRequest(req *spawner.CallbackError, originatorLayer int, groups []layerGroup) error {
	switch req.Mode {
	case "agent":
		tl, ok := agentLayerOf(req.TargetAgent, groups)
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
			l, ok := agentLayerOf(id, groups)
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

func decomposeLevelCallback(req *spawner.CallbackError, originatorLayer int, groups []layerGroup) decomposedRequest {
	var steps []callbackPlanStep
	var reset []string
	for _, g := range groups {
		if g.layer < req.Level || g.layer > originatorLayer {
			continue
		}
		steps = append(steps, callbackPlanStep{
			layer:      g.layer,
			wholeLayer: true,
			layerInstr: req.Instructions,
		})
		for _, p := range g.phases {
			reset = append(reset, p.Agent)
		}
	}
	sort.Slice(steps, func(i, j int) bool { return steps[i].layer < steps[j].layer })
	return decomposedRequest{
		steps:       steps,
		resetScope:  reset,
		resumeLayer: originatorLayer + 1,
		agentID:     req.AgentType,
	}
}

func decomposeAgentCallback(req *spawner.CallbackError, originatorLayer int, groups []layerGroup) decomposedRequest {
	targetLayer, _ := agentLayerOf(req.TargetAgent, groups)
	steps := []callbackPlanStep{{
		layer:         targetLayer,
		wholeLayer:    false,
		agents:        []string{req.TargetAgent},
		perAgentInstr: map[string]string{req.TargetAgent: req.Instructions},
	}}
	reset := []string{req.TargetAgent}
	for _, g := range groups {
		if g.layer <= targetLayer || g.layer > originatorLayer {
			continue
		}
		steps = append(steps, callbackPlanStep{layer: g.layer, wholeLayer: true})
		for _, p := range g.phases {
			reset = append(reset, p.Agent)
		}
	}
	sort.Slice(steps, func(i, j int) bool { return steps[i].layer < steps[j].layer })
	return decomposedRequest{
		steps:       steps,
		resetScope:  reset,
		resumeLayer: originatorLayer + 1,
		agentID:     req.AgentType,
	}
}

func decomposeChainCallback(req *spawner.CallbackError, groups []layerGroup) decomposedRequest {
	var steps []callbackPlanStep
	for i, id := range req.Chain {
		l, _ := agentLayerOf(id, groups)
		instr := ""
		if i == 0 {
			instr = req.Instructions // instructions only on first chain entry
		}
		steps = append(steps, callbackPlanStep{
			layer:         l,
			wholeLayer:    false,
			agents:        []string{id},
			perAgentInstr: map[string]string{id: instr},
		})
	}
	sort.Slice(steps, func(i, j int) bool { return steps[i].layer < steps[j].layer })
	lastLayer := 0
	if len(steps) > 0 {
		lastLayer = steps[len(steps)-1].layer
	}
	reset := make([]string, len(req.Chain))
	copy(reset, req.Chain)
	return decomposedRequest{
		steps:       steps,
		resetScope:  reset,
		resumeLayer: lastLayer + 1,
		agentID:     req.AgentType,
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
		agentSet   map[string]bool   // union of agents for !wholeLayer
		agentInstr map[string]string // first non-empty per-agent instruction
	}
	byLayer := make(map[int]*layerMerge)
	resetSet := make(map[string]bool)
	maxResume := 0

	for _, part := range parts {
		for _, s := range part.steps {
			m := byLayer[s.layer]
			if m == nil {
				m = &layerMerge{agentSet: make(map[string]bool), agentInstr: make(map[string]string)}
				byLayer[s.layer] = m
			}
			if s.wholeLayer {
				m.wholeLayer = true
				if s.layerInstr != "" {
					m.instrParts = append(m.instrParts, s.layerInstr)
				}
			} else if !m.wholeLayer {
				for _, a := range s.agents {
					m.agentSet[a] = true
					if _, exists := m.agentInstr[a]; !exists && s.perAgentInstr[a] != "" {
						m.agentInstr[a] = s.perAgentInstr[a]
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
			agents := make([]string, 0, len(m.agentSet))
			for a := range m.agentSet {
				agents = append(agents, a)
			}
			sort.Strings(agents)
			s.agents = agents
			s.perAgentInstr = make(map[string]string)
			for _, a := range agents {
				s.perAgentInstr[a] = m.agentInstr[a]
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
// Whole-layer steps contribute len(phases) from layerGroups; per-agent steps contribute len(step.agents).
func cumulativeAgentCount(plan callbackPlan, groups []layerGroup) int {
	total := 0
	for _, s := range plan.steps {
		if s.wholeLayer {
			if idx := layerIndexOf(s.layer, groups); idx >= 0 {
				total += len(groups[idx].phases)
			}
		} else {
			total += len(s.agents)
		}
	}
	return total
}
