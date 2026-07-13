package orchestrator

import (
	"context"
	"fmt"

	"be/internal/db"
	"be/internal/model"
	"be/internal/repo"
	"be/internal/service"
	"be/internal/types"
	"be/internal/ws"
)

// StartDynamicWorkflow starts the bundled plan-driven `dynamic` workflow (see
// service.DynamicWorkflow) as a detached child sharing StartSubworkflow's
// guards via startChildRun. mode is "" or "approve" (default: the child
// suspends at waiting_approval for the caller to drive via RevisePlan/
// ApprovePlan) or "auto" (the child auto-approves and materializes its own
// draft without suspending — refused unless service.DynamicAutoEnabled is true
// for projectID). It implements apirun.SubworkflowRunner for the
// dynamic_workflow builtin.
func (o *Orchestrator) StartDynamicWorkflow(ctx context.Context, parentInstanceID, projectID, instructions, mode string) (string, error) {
	pool, err := db.NewPool(o.dataPath, db.DefaultPoolConfig())
	if err != nil {
		return "", fmt.Errorf("dynamic_workflow: open pool: %w", err)
	}
	defer pool.Close()

	planAuto := false
	switch mode {
	case "", "approve":
	case "auto":
		if !service.DynamicAutoEnabled(pool, projectID) {
			return "", fmt.Errorf("dynamic_workflow: mode=auto is disabled (dynamic_workflow_auto_enabled=false)")
		}
		planAuto = true
	default:
		return "", fmt.Errorf("dynamic_workflow: unknown mode %q (want \"approve\" or \"auto\")", mode)
	}

	return o.startChildRun(ctx, pool, parentInstanceID, projectID, service.DynamicWorkflow, instructions, planAuto)
}

// RevisePlan drives a child's plan lifecycle on behalf of the caller that
// started it (ownership enforced via assertChildOwnership, same as
// GetSubworkflow), appending a new plan revision exactly as
// POST .../plan/revise does. It implements apirun.SubworkflowRunner for the
// revise_plan builtin.
func (o *Orchestrator) RevisePlan(ctx context.Context, callerInstanceID, projectID, instanceID string, req types.PlanReviseRequest) (*model.PlanRevision, error) {
	pool, err := db.NewPool(o.dataPath, db.DefaultPoolConfig())
	if err != nil {
		return nil, fmt.Errorf("revise_plan: open pool: %w", err)
	}
	defer pool.Close()

	wi, err := o.assertChildOwnership(pool, callerInstanceID, projectID, instanceID)
	if err != nil {
		return nil, err
	}

	rev, err := service.NewPlanService(pool, o.clock, o).Revise(ctx, instanceID, req)
	if err != nil {
		return nil, err
	}

	eventType := ws.EventPlanRevised
	if rev.Revision == 1 {
		eventType = ws.EventPlanDrafted
	}
	o.wsHub.Broadcast(ws.NewEvent(eventType, wi.ProjectID, wi.TicketID, wi.WorkflowID, map[string]interface{}{
		"instance_id": instanceID,
		"revision":    rev.Revision,
		"author":      rev.Author,
	}))
	return rev, nil
}

// ApprovePlan approves a child's plan at the given revision (Approve already
// materializes) and, when the child was parked at the plan boundary, resumes
// its run — mirroring api/handlers_plan.go's handleApprovePlan. It implements
// apirun.SubworkflowRunner for the approve_plan builtin.
func (o *Orchestrator) ApprovePlan(ctx context.Context, callerInstanceID, projectID, instanceID string, revision int) (*model.PlanRevision, error) {
	pool, err := db.NewPool(o.dataPath, db.DefaultPoolConfig())
	if err != nil {
		return nil, fmt.Errorf("approve_plan: open pool: %w", err)
	}
	defer pool.Close()

	wi, err := o.assertChildOwnership(pool, callerInstanceID, projectID, instanceID)
	if err != nil {
		return nil, err
	}

	rev, err := service.NewPlanService(pool, o.clock, o).Approve(instanceID, revision)
	if err != nil {
		return nil, err
	}

	o.wsHub.Broadcast(ws.NewEvent(ws.EventPlanApproved, wi.ProjectID, wi.TicketID, wi.WorkflowID, map[string]interface{}{
		"instance_id": instanceID,
		"revision":    rev.Revision,
	}))
	o.wsHub.Broadcast(ws.NewEvent(ws.EventPlanMaterialized, wi.ProjectID, wi.TicketID, wi.WorkflowID, map[string]interface{}{
		"instance_id": instanceID,
	}))

	if refreshed, rerr := repo.NewWorkflowInstanceRepo(pool, o.clock).Get(instanceID); rerr == nil && model.IsPlanSuspended(refreshed.Status) {
		if err := o.ResumeAfterPlanApproval(ctx, instanceID); err != nil {
			return nil, fmt.Errorf("approve_plan: resume failed: %w", err)
		}
	}
	return rev, nil
}
