package cli

import (
	"context"

	"be/internal/clock"
	"be/internal/db"
	"be/internal/orchestrator"
	"be/internal/repo"
)

// workflowRunnerAdapter wraps *orchestrator.Orchestrator to satisfy
// socket.WorkflowOrchestrator. It embeds the orchestrator (promoting
// StartWorkflow/RetryFailed/RetryFailedProject) and adds 4-arg
// ContinueWorkflow/FailWorkflow that resolve ticket/workflow from the
// instance before delegating to the 6-arg orchestrator methods.
type workflowRunnerAdapter struct {
	*orchestrator.Orchestrator
	pool *db.Pool
	clk  clock.Clock
}

func newWorkflowRunnerAdapter(o *orchestrator.Orchestrator, pool *db.Pool, clk clock.Clock) *workflowRunnerAdapter {
	return &workflowRunnerAdapter{Orchestrator: o, pool: pool, clk: clk}
}

func (a *workflowRunnerAdapter) ContinueWorkflow(ctx context.Context, projectID, instanceID, instructions string) error {
	wfi, err := repo.NewWorkflowInstanceRepo(a.pool, a.clk).Get(instanceID)
	if err != nil {
		return err
	}
	return a.Orchestrator.ContinueWorkflow(ctx, projectID, wfi.TicketID, wfi.WorkflowID, instanceID, instructions)
}

func (a *workflowRunnerAdapter) FailWorkflow(ctx context.Context, projectID, instanceID, reason string) error {
	wfi, err := repo.NewWorkflowInstanceRepo(a.pool, a.clk).Get(instanceID)
	if err != nil {
		return err
	}
	return a.Orchestrator.FailWorkflow(ctx, projectID, wfi.TicketID, wfi.WorkflowID, instanceID, reason)
}
