package spawner

import (
	"database/sql"
	"fmt"
	"time"

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
	s.broadcastGlobal()
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
	case "user_interactive":
		status = model.AgentSessionUserInteractive
	}
	sessionRepo.UpdateStatus(sessionID, status)

	s.broadcast(ws.EventAgentCompleted, projectID, ticketID, workflowName, map[string]interface{}{
		"agent_id":      agentID,
		"session_id":    sessionID,
		"result":        result,
		"result_reason": resultReason,
		"model_id":      modelID,
	})
	s.broadcastGlobal()
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
// Returns (phaseID, error). Queries agent_sessions for terminal status validation.
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

	// Collect prior-layer agent types that need to be completed
	var priorAgents []PhaseDef
	for _, p := range workflow.Phases {
		if p.Layer < requestedPhase.Layer {
			priorAgents = append(priorAgents, p)
		}
	}

	// No prior layers — no validation needed
	if len(priorAgents) == 0 {
		return requestedPhase.ID, nil
	}

	// Query terminal sessions for this workflow instance
	pool := s.pool()
	if pool == nil {
		return "", fmt.Errorf("failed to get database pool")
	}

	rows, err := pool.Query(`
		SELECT agent_type, status FROM agent_sessions
		WHERE workflow_instance_id = ? AND status NOT IN ('running', 'continued', 'callback')
		ORDER BY created_at DESC`, wi.ID)
	if err != nil {
		return "", fmt.Errorf("failed to query agent sessions: %w", err)
	}
	defer rows.Close()

	// Track which agent_types have a terminal session
	terminalAgents := make(map[string]bool)
	for rows.Next() {
		var agentType, status string
		rows.Scan(&agentType, &status)
		if !terminalAgents[agentType] {
			terminalAgents[agentType] = true
		}
	}

	// Validate that all agents in prior layers have a terminal session
	for _, prior := range priorAgents {
		if terminalAgents[prior.Agent] {
			continue
		}
		return "", fmt.Errorf("layer %d agent '%s' must complete before layer %d agent '%s'",
			prior.Layer, prior.ID, requestedPhase.Layer, requestedPhase.ID)
	}

	return requestedPhase.ID, nil
}

// broadcastGlobal sends a signal-only global.running_agents event to all WS clients.
// The frontend refetches via REST on receipt — no data payload needed.
func (s *Spawner) broadcastGlobal() {
	if s.config.WSHub == nil {
		return
	}
	event := ws.NewEvent(ws.EventGlobalRunningAgents, "", "", "", nil)
	s.config.WSHub.BroadcastGlobal(event)
}

