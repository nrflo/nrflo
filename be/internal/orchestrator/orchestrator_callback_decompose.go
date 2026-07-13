package orchestrator

import (
	"sort"

	"be/internal/spawner"
)

// agentLayerOf resolves a *template* agent id to the node(s) whose Agent
// matches, returning the layer and node id of the (currently single, for
// static workflows) match. Fan-out would return multiple nodes sharing a
// layer; callers that need only the layer/node keep taking the first match.
func agentLayerOf(agentID string, groups []layerGroup) (layer int, nodeID string, ok bool) {
	for _, g := range groups {
		for _, p := range g.phases {
			if p.Agent == agentID {
				return g.layer, p.NodeID, true
			}
		}
	}
	return 0, "", false
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
			reset = append(reset, p.NodeID)
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
	targetLayer, targetNode, _ := agentLayerOf(req.TargetAgent, groups)
	steps := []callbackPlanStep{{
		layer:        targetLayer,
		wholeLayer:   false,
		nodes:        []string{targetNode},
		perNodeInstr: map[string]string{targetNode: req.Instructions},
	}}
	reset := []string{targetNode}
	for _, g := range groups {
		if g.layer <= targetLayer || g.layer > originatorLayer {
			continue
		}
		steps = append(steps, callbackPlanStep{layer: g.layer, wholeLayer: true})
		for _, p := range g.phases {
			reset = append(reset, p.NodeID)
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
	nodes := make([]string, 0, len(req.Chain))
	for i, id := range req.Chain {
		l, nodeID, _ := agentLayerOf(id, groups)
		instr := ""
		if i == 0 {
			instr = req.Instructions // instructions only on first chain entry
		}
		steps = append(steps, callbackPlanStep{
			layer:        l,
			wholeLayer:   false,
			nodes:        []string{nodeID},
			perNodeInstr: map[string]string{nodeID: instr},
		})
		nodes = append(nodes, nodeID)
	}
	sort.Slice(steps, func(i, j int) bool { return steps[i].layer < steps[j].layer })
	lastLayer := 0
	if len(steps) > 0 {
		lastLayer = steps[len(steps)-1].layer
	}
	return decomposedRequest{
		steps:       steps,
		resetScope:  nodes,
		resumeLayer: lastLayer + 1,
		agentID:     req.AgentType,
	}
}
