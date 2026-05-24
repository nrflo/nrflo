package orchestrator

import (
	"context"
	"fmt"

	"be/internal/db"
	"be/internal/logger"
	"be/internal/repo"
)

// RetryFailedAgent resets a failed workflow instance and re-runs from the failed layer.
func (o *Orchestrator) RetryFailedAgent(ctx context.Context, projectID, ticketID, workflowName, sessionID string) error {
	return o.retryFailed(ctx, projectID, ticketID, workflowName, sessionID, "ticket", "")
}

// RetryFailedProjectAgent resets a failed project-scoped workflow and re-runs from the failed layer.
func (o *Orchestrator) RetryFailedProjectAgent(ctx context.Context, projectID, workflowName, sessionID, instanceID string) error {
	return o.retryFailed(ctx, projectID, "", workflowName, sessionID, "project", instanceID)
}

// RestartAgent sends a manual restart signal to the active spawner for a workflow.
// Looks up the instance from the session's workflow_instance_id.
func (o *Orchestrator) RestartAgent(projectID, ticketID, workflowName, sessionID string) error {
	database, err := db.Open(o.dataPath)
	if err != nil {
		return fmt.Errorf("failed to open database: %w", err)
	}
	defer database.Close()

	asRepo := repo.NewAgentSessionRepo(database, o.clock)
	session, err := asRepo.Get(sessionID)
	if err != nil {
		return fmt.Errorf("agent session not found: %w", err)
	}

	return o.restartAgentByInstance(session.WorkflowInstanceID, workflowName, ticketID, sessionID)
}

// RestartProjectAgent sends a restart signal for a project-scoped workflow agent.
// instanceID is required to identify which instance to restart.
func (o *Orchestrator) RestartProjectAgent(projectID, workflowName, sessionID, instanceID string) error {
	if instanceID == "" {
		return fmt.Errorf("instance_id is required for project-scoped workflow restart")
	}
	return o.restartAgentByInstance(instanceID, workflowName, projectID, sessionID)
}

func (o *Orchestrator) restartAgentByInstance(wfiID, workflowName, target, sessionID string) error {
	logger.Info(context.Background(), "agent restart requested", "session_id", sessionID, "workflow", workflowName)
	o.mu.Lock()
	rs, ok := o.runs[wfiID]
	o.mu.Unlock()
	if !ok {
		return fmt.Errorf("no running orchestration for workflow '%s' on %s", workflowName, target)
	}

	o.mu.Lock()
	sp := rs.spawners[sessionID]
	o.mu.Unlock()
	if sp == nil {
		return fmt.Errorf("no active spawner (agent may be between phases)")
	}

	sp.RequestRestart(sessionID)
	return nil
}
