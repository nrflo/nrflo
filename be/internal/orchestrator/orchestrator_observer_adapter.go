package orchestrator

import (
	"context"
	"encoding/json"

	"be/internal/model"
)

// StartWorkflow implements socket.WorkflowOrchestrator.StartWorkflow.
// Maps an observer-side trigger request to the standard RunRequest entrypoint.
func (o *Orchestrator) StartWorkflow(ctx context.Context, projectID, ticketID, workflowName, instructions, scopeType string) (string, error) {
	result, err := o.Start(ctx, RunRequest{
		ProjectID:    projectID,
		TicketID:     ticketID,
		WorkflowName: workflowName,
		Instructions: instructions,
		ScopeType:    scopeType,
	})
	if err != nil {
		return "", err
	}
	return result.InstanceID, nil
}

// StartConsoleWorkflow starts a workflow launched from the native console TUI,
// attributing the run's origin to the launching console session. A non-nil
// planManifest is seeded as the plan-driven run's revision 1 (author=caller)
// at the plan boundary instead of spawning the planner (nrworkflow-4d0243).
func (o *Orchestrator) StartConsoleWorkflow(ctx context.Context, projectID, ticketID, workflowName, instructions, scopeType, consoleSessionID string, planManifest json.RawMessage) (string, error) {
	result, err := o.Start(ctx, RunRequest{
		ProjectID:        projectID,
		TicketID:         ticketID,
		WorkflowName:     workflowName,
		Instructions:     instructions,
		ScopeType:        scopeType,
		Origin:           model.RunOriginConsole,
		OriginSessionID:  consoleSessionID,
		SeedPlanManifest: planManifest,
	})
	if err != nil {
		return "", err
	}
	return result.InstanceID, nil
}

// RetryFailed implements socket.WorkflowOrchestrator.RetryFailed.
func (o *Orchestrator) RetryFailed(ctx context.Context, projectID, ticketID, workflowName, sessionID string) error {
	return o.RetryFailedAgent(ctx, projectID, ticketID, workflowName, sessionID)
}

// RetryFailedProject implements socket.WorkflowOrchestrator.RetryFailedProject.
func (o *Orchestrator) RetryFailedProject(ctx context.Context, projectID, workflowName, sessionID, instanceID string) error {
	return o.RetryFailedProjectAgent(ctx, projectID, workflowName, sessionID, instanceID)
}
