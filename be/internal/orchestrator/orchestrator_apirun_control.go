package orchestrator

import (
	"context"

	"be/internal/db"
	"be/internal/repo"
	"be/internal/spawner/apirun"
)

// apiWorkflowControl implements apirun.WorkflowController for use by API-mode agents.
// It resolves ticket/workflow from the instance and delegates to the orchestrator's
// 6-arg ContinueWorkflow/FailWorkflow methods.
type apiWorkflowControl struct {
	o    *Orchestrator
	pool *db.Pool
}

var _ apirun.WorkflowController = apiWorkflowControl{}

func (c apiWorkflowControl) ContinueWorkflow(ctx context.Context, projectID, instanceID, instructions string) error {
	wfi, err := repo.NewWorkflowInstanceRepo(c.pool, c.o.clock).Get(instanceID)
	if err != nil {
		return err
	}
	return c.o.ContinueWorkflow(ctx, projectID, wfi.TicketID, wfi.WorkflowID, instanceID, instructions)
}

func (c apiWorkflowControl) FailWorkflow(ctx context.Context, projectID, instanceID, reason string) error {
	wfi, err := repo.NewWorkflowInstanceRepo(c.pool, c.o.clock).Get(instanceID)
	if err != nil {
		return err
	}
	return c.o.FailWorkflow(ctx, projectID, wfi.TicketID, wfi.WorkflowID, instanceID, reason)
}
