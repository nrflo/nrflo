package orchestrator

import (
	"context"
	"fmt"
	"strings"

	"be/internal/db"
	"be/internal/model"
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

// APIWorkflowControl exposes the unexported apiWorkflowControl adapter so the
// api package can fill a console ToolEnv's WorkflowControl without a third
// copy of the ContinueWorkflow/FailWorkflow instance-resolution logic (see
// cli/workflow_runner_adapter.go for the other existing copy).
func (o *Orchestrator) APIWorkflowControl(pool *db.Pool) apirun.WorkflowController {
	return apiWorkflowControl{o: o, pool: pool}
}

// guardedInstance resolves instanceID and rejects it when it belongs to another
// project. Callers supply instanceID freely (api-mode agents and console tools
// both take it as a tool argument), while ContinueWorkflow/FailWorkflow resolve
// the project root and workflow def from the *caller's* projectID — without this
// check a caller scoped to project A could fail project B's run, or resume B's
// instance inside A's repo.
func (c apiWorkflowControl) guardedInstance(projectID, instanceID string) (*model.WorkflowInstance, error) {
	wfi, err := repo.NewWorkflowInstanceRepo(c.pool, c.o.clock).Get(instanceID)
	if err != nil {
		return nil, err
	}
	if !strings.EqualFold(wfi.ProjectID, projectID) {
		return nil, fmt.Errorf("workflow instance %s does not belong to project %s", instanceID, projectID)
	}
	return wfi, nil
}

func (c apiWorkflowControl) ContinueWorkflow(ctx context.Context, projectID, instanceID, instructions string) error {
	wfi, err := c.guardedInstance(projectID, instanceID)
	if err != nil {
		return err
	}
	return c.o.ContinueWorkflow(ctx, projectID, wfi.TicketID, wfi.WorkflowID, instanceID, instructions)
}

func (c apiWorkflowControl) FailWorkflow(ctx context.Context, projectID, instanceID, reason string) error {
	wfi, err := c.guardedInstance(projectID, instanceID)
	if err != nil {
		return err
	}
	return c.o.FailWorkflow(ctx, projectID, wfi.TicketID, wfi.WorkflowID, instanceID, reason)
}
