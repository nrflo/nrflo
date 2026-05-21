package orchestrator

import (
	"context"
	"fmt"

	"be/internal/db"
	"be/internal/logger"
	"be/internal/model"
	"be/internal/repo"
	"be/internal/service"
)

// FailWorkflow terminates a workflow instance with a custom reason.
//   - Terminal status (completed/failed/project_completed): returns an error.
//   - Running (in o.runs): sets failReason on the runState then cancels it.
//   - Waiting (not in o.runs): reconstructs a minimal RunRequest and calls markFailed directly
//     so the failure-finalize slot fires (custom reason != reasonCancelled).
func (o *Orchestrator) FailWorkflow(ctx context.Context, projectID, ticketID, workflowName, instanceID, reason string) error {
	if reason == "" {
		reason = "manual_fail"
	}

	database, err := db.Open(o.dataPath)
	if err != nil {
		return fmt.Errorf("failed to open database: %w", err)
	}
	defer database.Close()
	pool := db.WrapAsPool(database)

	wfiRepo := repo.NewWorkflowInstanceRepo(pool, o.clock)
	wi, err := wfiRepo.Get(instanceID)
	if err != nil {
		return fmt.Errorf("workflow instance not found: %w", err)
	}

	switch wi.Status {
	case model.WorkflowInstanceCompleted, model.WorkflowInstanceFailed, model.WorkflowInstanceProjectCompleted:
		return fmt.Errorf("workflow is already in terminal status (%s)", wi.Status)
	}

	o.mu.Lock()
	rs, running := o.runs[wi.ID]
	o.mu.Unlock()

	if running {
		// Active run: inject reason and let the cancellation path pick it up.
		o.mu.Lock()
		if rs := o.runs[wi.ID]; rs != nil {
			rs.failReason = reason
		}
		o.mu.Unlock()
		rs.cancel()
		logger.Info(ctx, "FailWorkflow: cancelled active run", "instance_id", instanceID, "reason", reason)
		return nil
	}

	// Waiting (or any other non-terminal, non-running): fail directly.
	if wi.Status != model.WorkflowInstanceWaiting {
		return fmt.Errorf("workflow is in unexpected status (%s)", wi.Status)
	}

	req, err := o.buildMinimalReq(pool, projectID, ticketID, workflowName, wi)
	if err != nil {
		return fmt.Errorf("failed to build run request: %w", err)
	}

	o.markFailed(instanceID, req, reason)
	logger.Info(ctx, "FailWorkflow: marked waiting instance failed", "instance_id", instanceID, "reason", reason)
	return nil
}

// buildMinimalReq constructs a RunRequest with just enough data for markFailed
// (finalize slots, scope type). Does not set up a worktree or pool.
func (o *Orchestrator) buildMinimalReq(pool *db.Pool, projectID, ticketID, workflowName string, wi *model.WorkflowInstance) (RunRequest, error) {
	wfRepo := repo.NewWorkflowRepo(pool, o.clock)
	dbWorkflow, err := wfRepo.Get(projectID, workflowName)
	if err != nil {
		return RunRequest{}, fmt.Errorf("workflow definition '%s' not found: %w", workflowName, err)
	}
	adRepo := repo.NewAgentDefinitionRepo(pool, o.clock)
	dbAgentDefs, err := adRepo.List(projectID, dbWorkflow.ID)
	if err != nil {
		return RunRequest{}, fmt.Errorf("failed to load agent definitions: %w", err)
	}

	svcWorkflows, _ := service.BuildSpawnerConfig([]*model.Workflow{dbWorkflow}, dbAgentDefs)
	svcWf := svcWorkflows[workflowName]

	return RunRequest{
		ProjectID:               projectID,
		TicketID:                ticketID,
		WorkflowName:            workflowName,
		ScopeType:               wi.ScopeType,
		CloseTicketOnComplete:   svcWf.CloseTicketOnComplete,
		FinalizeSuccessCommand:  svcWf.FinalizeSuccessCommand,
		FinalizeSuccessScriptID: svcWf.FinalizeSuccessScriptID,
		FinalizeFailureCommand:  svcWf.FinalizeFailureCommand,
		FinalizeFailureScriptID: svcWf.FinalizeFailureScriptID,
		PauseEventCommand:       svcWf.PauseEventCommand,
		PauseEventScriptID:      svcWf.PauseEventScriptID,
	}, nil
}
