package orchestrator

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"

	"be/internal/db"
	"be/internal/logger"
	"be/internal/repo"
	"be/internal/service"
	"be/internal/spawner"
	"be/internal/ws"
)

// handleCallback builds a callback plan from reqs, enforces the agent-spawn cap,
// resets sessions, persists findings, broadcasts, and stores the plan on runState.
// Returns false if the workflow was marked failed.
func (o *Orchestrator) handleCallback(
	ctx context.Context,
	wfiID string,
	req RunRequest,
	layerGroups []layerGroup,
	originatorLayerIdx int,
	reqs []*spawner.CallbackError,
	callbackCount *int,
) bool {
	originatorLayer := layerGroups[originatorLayerIdx].layer

	// Validate all requests before touching any state
	for _, r := range reqs {
		if err := validateCallbackRequest(r, originatorLayer, layerGroups); err != nil {
			o.markFailed(wfiID, req, fmt.Sprintf("invalid callback from %s: %v", r.AgentType, err))
			return false
		}
	}

	// Decompose and merge into a single plan
	parts := make([]decomposedRequest, len(reqs))
	for i, r := range reqs {
		parts[i] = decomposeCallback(r, originatorLayer, layerGroups)
	}
	plan := mergeCallbackPlans(parts)

	// Enforce cumulative agent-spawn cap
	agentCount := cumulativeAgentCount(plan, layerGroups)
	if *callbackCount+agentCount > maxCallbacks {
		o.markFailed(wfiID, req, fmt.Sprintf(
			"max callbacks (%d) exceeded: cumulative=%d, limit=%d",
			maxCallbacks, *callbackCount+agentCount, maxCallbacks))
		return false
	}
	*callbackCount += agentCount

	logger.Info(ctx, "callback detected",
		"from_layer", originatorLayer,
		"resume_layer", plan.resumeLayer,
		"plan_size", len(plan.steps),
		"reset_scope_size", len(plan.resetScope))

	database, err := db.Open(o.dataPath)
	if err != nil {
		logger.Error(ctx, "failed to open DB for callback", "err", err)
		o.markFailed(wfiID, req, "db_open_failed")
		return false
	}
	defer database.Close()

	pool := db.WrapAsPool(database)
	asRepo := repo.NewAgentSessionRepo(database, o.clock)

	// Reset all sessions in the plan's scope (single call)
	resetCallbackSessions(ctx, asRepo, wfiID, plan.resetScope)

	// Build serialisable summaries for findings and broadcast
	planData := make([]map[string]interface{}, len(plan.steps))
	for i, s := range plan.steps {
		sd := map[string]interface{}{"layer": s.layer, "whole_layer": s.wholeLayer}
		if !s.wholeLayer {
			// FE contract (callbackEdges.ts) reads "agents"; node_id == agent_type
			// for every static workflow today, so the payload is unchanged.
			sd["agents"] = s.nodes
		}
		planData[i] = sd
	}
	reqData := make([]map[string]interface{}, len(reqs))
	for i, r := range reqs {
		reqData[i] = map[string]interface{}{"agent": r.AgentType, "mode": r.Mode}
	}

	// Legacy fields for backward-compat (first plan step)
	legacyToLayer := originatorLayer
	legacyInstr := ""
	if len(plan.steps) > 0 {
		legacyToLayer = plan.steps[0].layer
		legacyInstr = plan.steps[0].layerInstr
	}

	// Persist _callback finding
	cbFindingRepo := repo.NewFindingRepo(pool, o.clock)
	cbData := map[string]interface{}{
		"plan":         planData,
		"resume_layer": plan.resumeLayer,
		"requests":     reqData,
		"from_layer":   originatorLayer,
		"instructions": legacyInstr, // template variable ${CALLBACK_INSTRUCTIONS}
	}
	cbVal, _ := json.Marshal(cbData)
	cbFindingRepo.Upsert("workflow_instance", wfiID, "_callback", cbVal, //nolint:errcheck
		repo.Denorm{ProjectID: req.ProjectID, WorkflowInstanceID: wfiID},
		repo.Actor{Source: "orchestrator"})

	// Broadcast single enriched event
	o.wsHub.Broadcast(ws.NewEvent(ws.EventOrchestrationCallback, req.ProjectID, req.TicketID, req.WorkflowName, map[string]interface{}{
		"instance_id":  wfiID,
		"from_layer":   originatorLayer,
		"to_layer":     legacyToLayer,
		"instructions": legacyInstr,
		"plan":         planData,
		"resume_layer": plan.resumeLayer,
		"requests":     reqData,
	}))

	// Store plan on runState for the execution loop
	o.mu.Lock()
	if rs, ok := o.runs[wfiID]; ok {
		rs.callbackPlan = plan
		rs.callbackPlanIdx = 0
	}
	o.mu.Unlock()

	return true
}

// clearCallbackMetadata removes the _callback key from workflow instance findings
// after the callback target layer completes successfully.
func (o *Orchestrator) clearCallbackMetadata(ctx context.Context, wfiID string) {
	database, err := db.Open(o.dataPath)
	if err != nil {
		logger.Error(ctx, "failed to open DB to clear callback metadata", "err", err)
		return
	}
	defer database.Close()

	pool := db.WrapAsPool(database)
	findingRepo := repo.NewFindingRepo(pool, o.clock)
	findingRepo.DeleteKeys("workflow_instance", wfiID, []string{"_callback"}, //nolint:errcheck
		repo.Actor{Source: "orchestrator"})
}

// layerGroup holds phases that share the same layer number.
type layerGroup struct {
	layer  int
	phases []service.SpawnerPhaseDef
}

// groupPhasesByLayer groups phases by layer number, sorted ascending.
func groupPhasesByLayer(phases []service.SpawnerPhaseDef) []layerGroup {
	groups := make(map[int][]service.SpawnerPhaseDef)
	for _, p := range phases {
		groups[p.Layer] = append(groups[p.Layer], p)
	}

	var layers []int
	for l := range groups {
		layers = append(layers, l)
	}
	sort.Ints(layers)

	result := make([]layerGroup, len(layers))
	for i, l := range layers {
		result[i] = layerGroup{layer: l, phases: groups[l]}
	}
	return result
}
