package spawner

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"be/internal/logger"
	"be/internal/model"
	"be/internal/repo"
	"be/internal/ws"
)

// registerAgentStart creates an agent_sessions row for a newly spawned agent
func (s *Spawner) registerAgentStart(projectID, ticketID, workflowName, wfiID, agentID, agentType string, pid int, sessionID, modelID, phase, spawnCommand, promptContext, ancestorSessionID string, restartCount, restartThreshold int) {
	pool := s.pool()
	if pool == nil {
		return
	}

	now := s.config.Clock.Now().UTC().Format(time.RFC3339Nano)
	sessionRepo := repo.NewAgentSessionRepo(pool, s.config.Clock)
	session := &model.AgentSession{
		ID:                 sessionID,
		ProjectID:          projectID,
		TicketID:           ticketID,
		WorkflowInstanceID: wfiID,
		Phase:              phase,
		AgentType:          agentType,
		ModelID:            sql.NullString{String: modelID, Valid: modelID != ""},
		Status:             model.AgentSessionRunning,
		PID:                sql.NullInt64{Int64: int64(pid), Valid: pid > 0},
		SpawnCommand:       sql.NullString{String: spawnCommand, Valid: spawnCommand != ""},
		PromptContext:      sql.NullString{String: promptContext, Valid: promptContext != ""},
		AncestorSessionID:  sql.NullString{String: ancestorSessionID, Valid: ancestorSessionID != ""},
		RestartCount:       restartCount,
		StartedAt:          sql.NullString{String: now, Valid: true},
	}
	sessionRepo.Create(session)

	s.broadcast(ws.EventAgentStarted, projectID, ticketID, workflowName, map[string]interface{}{
		"agent_id":          agentID,
		"agent_type":        agentType,
		"model_id":          modelID,
		"session_id":        sessionID,
		"phase":             phase,
		"restart_threshold": restartThreshold,
	})
}

// registerAgentStopWithReason updates the agent_sessions row when an agent stops
func (s *Spawner) registerAgentStopWithReason(projectID, ticketID, workflowName, sessionID, agentID, result, resultReason, modelID string) {
	pool := s.pool()
	if pool == nil {
		return
	}

	sessionRepo := repo.NewAgentSessionRepo(pool, s.config.Clock)

	// Update result and reason
	sessionRepo.UpdateResult(sessionID, result, resultReason)

	// Set ended_at timestamp
	sessionRepo.SetEndedAt(sessionID)

	// Update session status based on result
	status := model.AgentSessionCompleted
	switch result {
	case "fail", "timeout":
		status = model.AgentSessionFailed
	case "continue":
		status = model.AgentSessionContinued
	case "callback":
		status = model.AgentSessionCallback
	}
	sessionRepo.UpdateStatus(sessionID, status)

	s.broadcast(ws.EventAgentCompleted, projectID, ticketID, workflowName, map[string]interface{}{
		"agent_id":      agentID,
		"session_id":    sessionID,
		"result":        result,
		"result_reason": resultReason,
		"model_id":      modelID,
	})
}

// getWorkflowInstance retrieves the workflow instance for a ticket, returning an error if not initialized
func (s *Spawner) getWorkflowInstance(projectID, ticketID, workflowName string) (*model.WorkflowInstance, error) {
	pool := s.pool()
	if pool == nil {
		return nil, fmt.Errorf("failed to get database pool")
	}

	wfiRepo := repo.NewWorkflowInstanceRepo(pool, s.config.Clock)
	wi, err := wfiRepo.GetByTicketAndWorkflow(projectID, ticketID, workflowName)
	if err != nil {
		return nil, fmt.Errorf("workflow '%s' not initialized on ticket '%s'. Use the web UI or API to initialize it",
			workflowName, ticketID)
	}
	return wi, nil
}

// getProjectWorkflowInstance retrieves the most recent active project-scoped workflow instance.
func (s *Spawner) getProjectWorkflowInstance(projectID, workflowName string) (*model.WorkflowInstance, error) {
	pool := s.pool()
	if pool == nil {
		return nil, fmt.Errorf("failed to get database pool")
	}

	wfiRepo := repo.NewWorkflowInstanceRepo(pool, s.config.Clock)
	instances, err := wfiRepo.ListActiveByProjectAndWorkflow(projectID, workflowName)
	if err != nil || len(instances) == 0 {
		return nil, fmt.Errorf("project workflow '%s' not initialized. Use the web UI or API to initialize it",
			workflowName)
	}
	// Return the most recently created active instance
	return instances[len(instances)-1], nil
}

// getWorkflowInstanceByID retrieves a workflow instance by its ID.
func (s *Spawner) getWorkflowInstanceByID(instanceID string) (*model.WorkflowInstance, error) {
	pool := s.pool()
	if pool == nil {
		return nil, fmt.Errorf("failed to get database pool")
	}

	wfiRepo := repo.NewWorkflowInstanceRepo(pool, s.config.Clock)
	return wfiRepo.Get(instanceID)
}

// validateAndAdvancePhase validates phase order.
// Returns (phaseID, error). Uses workflow_instances table for state.
// With layer-based execution, validates that all agents in prior layers are completed.
func (s *Spawner) validateAndAdvancePhase(wi *model.WorkflowInstance, workflowName, requestedAgent string) (string, error) {
	workflow, ok := s.config.Workflows[workflowName]
	if !ok {
		return "", fmt.Errorf("unknown workflow: %s", workflowName)
	}

	// Find requested agent's phase
	var requestedPhase *PhaseDef
	for i := range workflow.Phases {
		if workflow.Phases[i].Agent == requestedAgent {
			requestedPhase = &workflow.Phases[i]
			break
		}
	}
	if requestedPhase == nil {
		return "", fmt.Errorf("agent '%s' not found in workflow '%s'", requestedAgent, workflowName)
	}

	phases := wi.GetPhases()

	// Validate that all agents in prior layers are completed
	for _, priorPhase := range workflow.Phases {
		if priorPhase.Layer >= requestedPhase.Layer {
			continue // same or later layer, skip validation
		}
		phaseStatus, exists := phases[priorPhase.ID]
		if exists && phaseStatus.Status == "completed" {
			continue
		}
		return "", fmt.Errorf("layer %d agent '%s' must complete before layer %d agent '%s'",
			priorPhase.Layer, priorPhase.ID, requestedPhase.Layer, requestedPhase.ID)
	}

	return requestedPhase.ID, nil
}

// startPhase marks a phase as in_progress using workflow_instances table
func (s *Spawner) startPhase(ctx context.Context, wfiID, projectID, ticketID, workflowName, phase string) {
	pool := s.pool()
	if pool == nil {
		return
	}

	wfiRepo := repo.NewWorkflowInstanceRepo(pool, s.config.Clock)
	if err := wfiRepo.StartPhase(wfiID, phase); err != nil {
		logger.Warn(ctx, "failed to start phase", "phase", phase, "err", err)
		return
	}

	s.broadcast(ws.EventPhaseStarted, projectID, ticketID, workflowName, map[string]interface{}{
		"phase": phase,
	})
}

// completePhase marks a phase as completed using workflow_instances table
func (s *Spawner) completePhase(ctx context.Context, wfiID, projectID, ticketID, workflowName, phase, result string) {
	pool := s.pool()
	if pool == nil {
		return
	}

	wfiRepo := repo.NewWorkflowInstanceRepo(pool, s.config.Clock)
	if err := wfiRepo.CompletePhase(wfiID, phase, result); err != nil {
		logger.Warn(ctx, "failed to complete phase", "phase", phase, "result", result, "err", err)
		return
	}

	s.broadcast(ws.EventPhaseCompleted, projectID, ticketID, workflowName, map[string]interface{}{
		"phase":  phase,
		"result": result,
	})
}
