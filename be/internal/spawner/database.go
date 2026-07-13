package spawner

import (
	"database/sql"
	"fmt"
	"time"

	"be/internal/model"
	"be/internal/repo"
	"be/internal/ws"
)

// createAgentSessionRow inserts the agent_sessions row for a newly spawned
// agent. It runs BEFORE backend.Start so the spawned child can never make its
// first socket call before the row exists — script agents call c.context() as
// their very first action, and writing the row only after Start let that lookup
// race the INSERT and fail with "agent session not found" under spawn
// contention. The runtime pid and final spawn_command are unknown until Start
// returns and are filled in by markAgentStarted.
//
// Returns true only when this call inserted the row, so the caller can roll it
// back on a failed Start. It returns false on a benign conflict — the observer
// path inserts its own row via ObserverService.Launch before this runs — or on
// any DB error; the insert is best-effort, matching the rest of the spawn hot
// path where sibling status writes are also fire-and-forget.
func (s *Spawner) createAgentSessionRow(projectID, ticketID, wfiID, agentType, nodeID, sessionID, modelID, phase, spawnCommand, prompt, systemPrompt, spawnToken, effectiveMode string, restartCount int) bool {
	pool := s.pool()
	if pool == nil {
		return false
	}

	now := s.config.Clock.Now().UTC().Format(time.RFC3339Nano)
	sessionRepo := repo.NewAgentSessionRepo(pool, s.config.Clock)
	session := &model.AgentSession{
		ID:                 sessionID,
		ProjectID:          projectID,
		TicketID:           ticketID,
		WorkflowInstanceID: wfiID,
		Phase:              phase,
		NodeID:             nodeID,
		AgentType:          agentType,
		ModelID:            sql.NullString{String: modelID, Valid: modelID != ""},
		Status:             model.AgentSessionRunning,
		SpawnCommand:       sql.NullString{String: spawnCommand, Valid: spawnCommand != ""},
		Prompt:             sql.NullString{String: prompt, Valid: prompt != ""},
		SystemPrompt:       sql.NullString{String: systemPrompt, Valid: systemPrompt != ""},
		SpawnToken:         sql.NullString{String: spawnToken, Valid: spawnToken != ""},
		EffectiveMode:      sql.NullString{String: effectiveMode, Valid: effectiveMode != ""},
		RestartCount:       restartCount,
		StartedAt:          sql.NullString{String: now, Valid: true},
		Config:             s.config.ClaudeSettingsJSON,
	}
	return sessionRepo.Create(session) == nil
}

// markAgentStarted records the runtime pid and final spawn_command on the
// session row (both known only after backend.Start) and broadcasts the
// agent-started events. Best-effort, mirroring the spawn hot path.
func (s *Spawner) markAgentStarted(projectID, ticketID, workflowName, agentID, agentType, modelID, sessionID, phase, spawnCommand string, pid, restartThreshold int) {
	if pool := s.pool(); pool != nil {
		_ = repo.NewAgentSessionRepo(pool, s.config.Clock).SetSpawnRuntime(sessionID, pid, spawnCommand)
	}

	s.broadcast(ws.EventAgentStarted, projectID, ticketID, workflowName, map[string]interface{}{
		"agent_id":          agentID,
		"agent_type":        agentType,
		"model_id":          modelID,
		"session_id":        sessionID,
		"phase":             phase,
		"restart_threshold": restartThreshold,
		"kind":              "workflow_agent",
	})
	s.broadcastGlobal()
}

// deleteAgentSessionRow removes a provisional agent_sessions row after a failed
// backend.Start, restoring the "no row for a spawn that never ran" invariant.
func (s *Spawner) deleteAgentSessionRow(sessionID string) {
	if pool := s.pool(); pool != nil {
		_ = repo.NewAgentSessionRepo(pool, s.config.Clock).Delete(sessionID)
	}
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

	kind := "workflow_agent"
	if row := pool.QueryRow("SELECT kind FROM agent_sessions WHERE id = ?", sessionID); row != nil {
		var k string
		if err := row.Scan(&k); err == nil && k != "" {
			kind = k
		}
	}

	s.broadcast(ws.EventAgentCompleted, projectID, ticketID, workflowName, map[string]interface{}{
		"agent_id":      agentID,
		"session_id":    sessionID,
		"result":        result,
		"result_reason": resultReason,
		"model_id":      modelID,
		"kind":          kind,
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
// Returns (nodeID, error). Queries agent_sessions for terminal status validation.
// With layer-based execution, validates that all nodes in prior layers are completed.
// The prior-layer gate keys on node_id, not agent_type, so fan-out siblings sharing
// a template cannot satisfy each other's gate.
func (s *Spawner) validateAndAdvancePhase(wi *model.WorkflowInstance, workflowName, requestedNode string) (string, error) {
	workflow, ok := s.config.Workflows[workflowName]
	if !ok {
		return "", fmt.Errorf("unknown workflow: %s", workflowName)
	}

	// Find requested node's phase
	var requestedPhase *PhaseDef
	for i := range workflow.Phases {
		if workflow.Phases[i].NodeID == requestedNode {
			requestedPhase = &workflow.Phases[i]
			break
		}
	}
	if requestedPhase == nil {
		return "", fmt.Errorf("node '%s' not found in workflow '%s'", requestedNode, workflowName)
	}

	// Collect prior-layer nodes that need to be completed
	var priorNodes []PhaseDef
	for _, p := range workflow.Phases {
		if p.Layer < requestedPhase.Layer {
			priorNodes = append(priorNodes, p)
		}
	}

	// No prior layers — no validation needed
	if len(priorNodes) == 0 {
		return requestedPhase.NodeID, nil
	}

	// Query terminal sessions for this workflow instance
	pool := s.pool()
	if pool == nil {
		return "", fmt.Errorf("failed to get database pool")
	}

	rows, err := pool.Query(`
		SELECT node_id, status FROM agent_sessions
		WHERE workflow_instance_id = ? AND status NOT IN ('running', 'continued', 'callback')
		ORDER BY created_at DESC`, wi.ID)
	if err != nil {
		return "", fmt.Errorf("failed to query agent sessions: %w", err)
	}
	defer rows.Close()

	// Track which node_ids have a terminal session
	terminalNodes := make(map[string]bool)
	for rows.Next() {
		var nodeID, status string
		rows.Scan(&nodeID, &status)
		if !terminalNodes[nodeID] {
			terminalNodes[nodeID] = true
		}
	}

	// Validate that all nodes in prior layers have a terminal session
	for _, prior := range priorNodes {
		if terminalNodes[prior.NodeID] {
			continue
		}
		return "", fmt.Errorf("layer %d agent '%s' must complete before layer %d agent '%s'",
			prior.Layer, prior.NodeID, requestedPhase.Layer, requestedPhase.NodeID)
	}

	return requestedPhase.NodeID, nil
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
