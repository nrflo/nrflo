package orchestrator

import (
	"context"
	"database/sql"
	"fmt"

	"be/internal/db"
	"be/internal/logger"
	"be/internal/model"
	"be/internal/repo"
	"be/internal/service"
	"be/internal/spawner"
	"be/internal/ws"
)

// reloadPlanLayers is called from runLoop when the forward loop would exit
// (layerIdx >= len(layerGroups)) and the plan has not yet been spliced into
// this run. It is a no-op (extended=false, terminal=false — the caller
// proceeds to normal completion) for workflows that carry no fanout_template
// agent definitions at all.
//
// For a plan-driven workflow: an approved plan is materialized (idempotent)
// and spliced into layerGroups/layerPolicies/workflows, and extended=true is
// returned so runLoop continues at the same layerIdx. A draft (or missing)
// plan suspends the run in its derived status; a cancelled plan fails the
// run. Both set terminal=true — the caller must return immediately.
//
// This is deliberately NOT the pause_after machinery: no _pause finding, no
// pause hook, and the four plan statuses (not `waiting`) keep the run
// invisible to the callable-as-subworkflow no-pause guard.
func (o *Orchestrator) reloadPlanLayers(
	ctx context.Context,
	wfiID string,
	req RunRequest,
	pool *db.Pool,
	svcWf service.SpawnerWorkflowDef,
	layerGroups []layerGroup,
	layerPolicies map[int]string,
	workflows map[string]spawner.WorkflowDef,
	agents map[string]spawner.AgentConfig,
) (newLayerGroups []layerGroup, extended bool, terminal bool, worktreeHandled bool) {
	dbWorkflow, _, defProjectID, err := o.resolveWorkflowDef(pool, req.ProjectID, req.WorkflowName)
	if err != nil {
		o.markFailed(wfiID, req, fmt.Sprintf("plan boundary: resolve workflow def: %v", err))
		return layerGroups, false, true, false
	}

	driven, err := service.IsPlanDriven(pool, defProjectID, dbWorkflow.ID)
	if err != nil {
		o.markFailed(wfiID, req, fmt.Sprintf("plan boundary: check plan-driven: %v", err))
		return layerGroups, false, true, false
	}
	if !driven {
		return layerGroups, false, false, false
	}

	planRepo := repo.NewPlanRepo(pool, o.clock)
	head, err := planRepo.GetHead(wfiID)
	switch {
	case err == sql.ErrNoRows:
		o.suspendForPlan(ctx, wfiID, req, pool, model.WorkflowInstancePlanning)
		return layerGroups, false, true, true
	case err != nil:
		o.markFailed(wfiID, req, fmt.Sprintf("plan boundary: load plan head: %v", err))
		return layerGroups, false, true, false
	case head.Status == model.PlanStatusCancelled:
		o.markFailed(wfiID, req, "plan cancelled")
		return layerGroups, false, true, false
	case head.Status == model.PlanStatusApproved:
		return o.materializeAndSplice(ctx, wfiID, req, pool, svcWf, layerGroups, layerPolicies, workflows, agents, defProjectID)
	default: // draft
		status, derr := service.DerivePlanInstanceStatus(pool, o.clock, wfiID)
		if derr != nil {
			o.markFailed(wfiID, req, fmt.Sprintf("plan boundary: derive plan status: %v", derr))
			return layerGroups, false, true, false
		}
		o.suspendForPlan(ctx, wfiID, req, pool, status)
		return layerGroups, false, true, true
	}
}

// materializeAndSplice materializes an approved plan (idempotent) and splices
// the resulting nodes into layerGroups/layerPolicies/workflows/agents.
func (o *Orchestrator) materializeAndSplice(
	ctx context.Context,
	wfiID string,
	req RunRequest,
	pool *db.Pool,
	svcWf service.SpawnerWorkflowDef,
	layerGroups []layerGroup,
	layerPolicies map[int]string,
	workflows map[string]spawner.WorkflowDef,
	agents map[string]spawner.AgentConfig,
	defProjectID string,
) (newLayerGroups []layerGroup, extended bool, terminal bool, worktreeHandled bool) {
	planSvc := service.NewPlanService(pool, o.clock, o)
	if _, err := planSvc.Materialize(wfiID); err != nil {
		o.markFailed(wfiID, req, fmt.Sprintf("plan boundary: materialize: %v", err))
		return layerGroups, false, true, false
	}

	materializedPhases, materializedPolicies, err := service.LoadInstanceNodePhases(pool, o.clock, wfiID)
	if err != nil {
		o.markFailed(wfiID, req, fmt.Sprintf("plan boundary: load materialized nodes: %v", err))
		return layerGroups, false, true, false
	}
	for layer, policy := range materializedPolicies {
		layerPolicies[layer] = policy
	}
	mergeMaterializedIntoSpawnerWorkflow(workflows, req.WorkflowName, materializedPhases)
	for id, cfg := range service.LoadMaterializedAgentConfigs(pool, o.clock, defProjectID, req.WorkflowName, materializedPhases) {
		agents[id] = spawner.AgentConfig{Model: cfg.Model, Timeout: cfg.Timeout}
	}

	logger.Info(ctx, "plan materialized and spliced", "instance_id", wfiID, "nodes", len(materializedPhases))
	o.wsHub.Broadcast(ws.NewEvent(ws.EventPlanMaterialized, req.ProjectID, req.TicketID, req.WorkflowName, map[string]interface{}{
		"instance_id": wfiID,
	}))

	return groupPhasesByLayer(service.EffectivePhases(svcWf.Phases, materializedPhases)), true, false, false
}

// suspendForPlan persists the derived plan status and broadcasts
// EventPlanWaiting. The worktree must survive for the eventual resumed run
// (mirrors pause.go's worktreeHandled contract) — the caller sets
// worktreeHandled=true and returns immediately.
func (o *Orchestrator) suspendForPlan(ctx context.Context, wfiID string, req RunRequest, pool *db.Pool, status model.WorkflowInstanceStatus) {
	logger.Info(ctx, "suspending workflow at plan boundary", "instance_id", wfiID, "status", status)
	_ = repo.NewWorkflowInstanceRepo(pool, o.clock).UpdateStatus(wfiID, status)
	o.wsHub.Broadcast(ws.NewEvent(ws.EventPlanWaiting, req.ProjectID, req.TicketID, req.WorkflowName, map[string]interface{}{
		"instance_id": wfiID,
		"status":      string(status),
	}))
}

// mergeMaterializedIntoSpawnerWorkflow appends materialized plan phases into
// the spawner-facing workflow def so spawner.Spawn's phase lookup (keyed on
// NodeID) succeeds for dynamic nodes exactly as it does for static ones.
func mergeMaterializedIntoSpawnerWorkflow(workflows map[string]spawner.WorkflowDef, workflowName string, materialized []service.SpawnerPhaseDef) {
	if len(materialized) == 0 {
		return
	}
	wf, ok := workflows[workflowName]
	if !ok {
		return
	}
	for _, p := range materialized {
		wf.Phases = append(wf.Phases, spawner.PhaseDef{NodeID: p.NodeID, Agent: p.Agent, Layer: p.Layer, Instructions: p.Instructions})
	}
	workflows[workflowName] = wf
}
