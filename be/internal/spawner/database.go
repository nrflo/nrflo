package spawner

import (
	"database/sql"
	"fmt"
	"os"
	"time"

	"be/internal/db"
	"be/internal/model"
	"be/internal/repo"
	"be/internal/ws"
)

// registerAgentStart creates an agent_sessions row for a newly spawned agent
func (s *Spawner) registerAgentStart(projectID, ticketID, workflowName, wfiID, agentID, agentType string, pid int, sessionID, modelID, phase, spawnCommand, promptContext, ancestorSessionID string, restartCount, restartThreshold int) {
	database, err := db.Open(s.config.DataPath)
	if err != nil {
		return
	}
	defer database.Close()

	now := time.Now().UTC().Format(time.RFC3339)
	sessionRepo := repo.NewAgentSessionRepo(database)
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
	database, err := db.Open(s.config.DataPath)
	if err != nil {
		return
	}
	defer database.Close()

	sessionRepo := repo.NewAgentSessionRepo(database)

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
	}
	sessionRepo.UpdateStatus(sessionID, status)

	s.broadcast(ws.EventAgentCompleted, projectID, ticketID, workflowName, map[string]interface{}{
		"agent_id":      agentID,
		"result":        result,
		"result_reason": resultReason,
		"model_id":      modelID,
	})
}

// getWorkflowInstance retrieves the workflow instance for a ticket, returning an error if not initialized
func (s *Spawner) getWorkflowInstance(projectID, ticketID, workflowName string) (*model.WorkflowInstance, error) {
	database, err := db.Open(s.config.DataPath)
	if err != nil {
		return nil, fmt.Errorf("failed to open database: %w", err)
	}
	defer database.Close()

	pool := db.WrapAsPool(database)
	wfiRepo := repo.NewWorkflowInstanceRepo(pool)
	wi, err := wfiRepo.GetByTicketAndWorkflow(projectID, ticketID, workflowName)
	if err != nil {
		return nil, fmt.Errorf("workflow '%s' not initialized on ticket '%s'. Use the web UI or API to initialize it",
			workflowName, ticketID)
	}
	return wi, nil
}

// validateAndAdvancePhase validates phase order and auto-skips phases with matching skip_for rules.
// Returns (phaseID, shouldSkip, error). Uses workflow_instances table for state.
func (s *Spawner) validateAndAdvancePhase(wi *model.WorkflowInstance, workflowName, requestedAgent string) (string, bool, error) {
	workflow, ok := s.config.Workflows[workflowName]
	if !ok {
		return "", false, fmt.Errorf("unknown workflow: %s", workflowName)
	}

	// Find requested agent's phase
	var requestedPhase *PhaseDef
	var requestedIndex int = -1
	for i := range workflow.Phases {
		if workflow.Phases[i].Agent == requestedAgent {
			requestedPhase = &workflow.Phases[i]
			requestedIndex = i
			break
		}
	}
	if requestedPhase == nil {
		return "", false, fmt.Errorf("agent '%s' not found in workflow '%s'", requestedAgent, workflowName)
	}

	phases := wi.GetPhases()
	category := ""
	if wi.Category.Valid {
		category = wi.Category.String
	}

	// Check if requested phase should be skipped
	if s.categoryMatchesSkipFor(category, requestedPhase.SkipFor) {
		s.completePhase(wi.ID, wi.ProjectID, wi.TicketID, workflowName, requestedPhase.ID, "skipped")
		return requestedPhase.ID, true, nil
	}

	// Validate that prior phases are completed or skipped
	for i := 0; i < requestedIndex; i++ {
		priorPhase := workflow.Phases[i]
		phaseStatus, exists := phases[priorPhase.ID]

		if exists && phaseStatus.Status == "completed" {
			continue
		}

		// Check if phase can be auto-skipped due to category
		if s.categoryMatchesSkipFor(category, priorPhase.SkipFor) {
			s.completePhase(wi.ID, wi.ProjectID, wi.TicketID, workflowName, priorPhase.ID, "skipped")
			continue
		}

		return "", false, fmt.Errorf("phase '%s' must complete before '%s'", priorPhase.ID, requestedPhase.ID)
	}

	return requestedPhase.ID, false, nil
}

// categoryMatchesSkipFor checks if the category matches any of the skip_for rules
func (s *Spawner) categoryMatchesSkipFor(category string, skipFor []string) bool {
	if category == "" || len(skipFor) == 0 {
		return false
	}
	for _, skip := range skipFor {
		if skip == category {
			return true
		}
	}
	return false
}

// startPhase marks a phase as in_progress using workflow_instances table
func (s *Spawner) startPhase(wfiID, projectID, ticketID, workflowName, phase string) {
	database, err := db.Open(s.config.DataPath)
	if err != nil {
		return
	}
	defer database.Close()

	pool := db.WrapAsPool(database)
	wfiRepo := repo.NewWorkflowInstanceRepo(pool)
	if err := wfiRepo.StartPhase(wfiID, phase); err != nil {
		fmt.Fprintf(os.Stderr, "Warning: failed to start phase %s: %v\n", phase, err)
		return
	}

	s.broadcast(ws.EventPhaseStarted, projectID, ticketID, workflowName, map[string]interface{}{
		"phase": phase,
	})
}

// completePhase marks a phase as completed using workflow_instances table
func (s *Spawner) completePhase(wfiID, projectID, ticketID, workflowName, phase, result string) {
	database, err := db.Open(s.config.DataPath)
	if err != nil {
		return
	}
	defer database.Close()

	pool := db.WrapAsPool(database)
	wfiRepo := repo.NewWorkflowInstanceRepo(pool)
	if err := wfiRepo.CompletePhase(wfiID, phase, result); err != nil {
		fmt.Fprintf(os.Stderr, "Warning: failed to complete phase %s: %v\n", phase, err)
		return
	}

	s.broadcast(ws.EventPhaseCompleted, projectID, ticketID, workflowName, map[string]interface{}{
		"phase":  phase,
		"result": result,
	})
}
