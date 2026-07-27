package orchestrator

import (
	"context"
	"fmt"

	"be/internal/db"
	"be/internal/logger"
	"be/internal/model"
	"be/internal/repo"
	"be/internal/spawner"
	"be/internal/ws"
)

// spawnerForSession resolves the spawner driving a session, via the session's
// own workflow instance. Returns nil when the session, its run, or its spawner
// is gone — every caller below treats that as a benign no-op. Going through
// resolveSessionSpawner is what lets run-less one-off children (the planner)
// receive heartbeats and terminal signals: keying only on runState.spawners
// silently drops every hook event for them, which reads as a permanent stall.
func (o *Orchestrator) spawnerForSession(sessionID string) (*spawner.Spawner, error) {
	database, err := db.Open(o.dataPath)
	if err != nil {
		return nil, fmt.Errorf("failed to open database: %w", err)
	}
	defer database.Close()

	session, err := repo.NewAgentSessionRepo(database, o.clock).Get(sessionID)
	if err != nil {
		return nil, nil // session may have already ended
	}
	return o.resolveSessionSpawner(session.WorkflowInstanceID, sessionID), nil
}

// BumpLastMessage resets stall-detection state for the matching running agent.
// Best-effort: returns nil when session or run not found.
func (o *Orchestrator) BumpLastMessage(projectID, ticketID, workflow, sessionID string) error {
	sp, err := o.spawnerForSession(sessionID)
	if err != nil || sp == nil {
		return err
	}
	sp.BumpLastMessage(sessionID)
	return nil
}

// SetLastMessage updates proc.lastMessage on the running agent so the
// "agent status" log line and any in-memory status surface show the most
// recent agent output. Best-effort: returns nil when session/run/spawner
// not found, or when content is empty.
func (o *Orchestrator) SetLastMessage(projectID, ticketID, workflow, sessionID, content string) error {
	if content == "" {
		return nil
	}
	sp, err := o.spawnerForSession(sessionID)
	if err != nil || sp == nil {
		return err
	}
	sp.SetLastMessage(sessionID, content)
	return nil
}

// TriggerIdleNudge asks the matching spawner to fire the idle nudge for
// sessionID immediately (in-band Notification-hook idle signal). Best-effort:
// returns nil when session, run, or spawner is not found — this is what
// structurally excludes console/observer sessions (no run/spawner) from the
// in-band nudge path. reason is "idle"|"permission" for the trace/log marker.
func (o *Orchestrator) TriggerIdleNudge(sessionID, reason string) error {
	sp, err := o.spawnerForSession(sessionID)
	if err != nil || sp == nil {
		return err
	}
	sp.TriggerIdleNudge(sessionID, reason)
	return nil
}

// RequestTerminalSignal kills the active agent for the given session so
// monitorAll exits and handleCompletion reads the DB result already written
// by the socket handler. Best-effort: returns nil when session or run not found.
func (o *Orchestrator) RequestTerminalSignal(projectID, ticketID, workflow, sessionID, result string) error {
	sp, err := o.spawnerForSession(sessionID)
	if err != nil || sp == nil {
		return err
	}
	sp.RequestTerminalSignal(sessionID, result)
	return nil
}

// CompleteInteractive signals that the interactive session has ended.
// It updates the agent session in DB and unblocks the spawner's wait.
func (o *Orchestrator) CompleteInteractive(sessionID string) error {
	logger.Info(context.Background(), "interactive session completing", "session_id", sessionID)

	// Update agent session in DB
	database, err := db.Open(o.dataPath)
	if err != nil {
		return fmt.Errorf("failed to open database: %w", err)
	}
	defer database.Close()

	asRepo := repo.NewAgentSessionRepo(database, o.clock)
	if err := asRepo.UpdateStatusToInteractiveCompleted(sessionID); err != nil {
		return fmt.Errorf("failed to update session: %w", err)
	}

	// Find the runState that has this session's spawner and call CompleteInteractive
	o.mu.Lock()
	seen := make(map[*spawner.Spawner]struct{})
	for _, rs := range o.runs {
		for _, sp := range rs.spawners {
			if _, ok := seen[sp]; ok {
				continue
			}
			seen[sp] = struct{}{}
			sp.CompleteInteractive(sessionID)
		}
	}
	o.mu.Unlock()

	return nil
}

// KillInteractive marks the interactive session as failed and unblocks the spawner wait.
func (o *Orchestrator) KillInteractive(sessionID string) error {
	logger.Info(context.Background(), "killing interactive session", "session_id", sessionID)

	database, err := db.Open(o.dataPath)
	if err != nil {
		return fmt.Errorf("failed to open database: %w", err)
	}
	defer database.Close()

	asRepo := repo.NewAgentSessionRepo(database, o.clock)
	session, err := asRepo.Get(sessionID)
	if err != nil {
		return fmt.Errorf("agent session not found: %s", sessionID)
	}
	if session.Status != model.AgentSessionUserInteractive {
		return fmt.Errorf("session %s is not user_interactive (status=%s)", sessionID, session.Status)
	}

	if err := asRepo.UpdateStatusToFailedWithReason(sessionID, "user_killed"); err != nil {
		return fmt.Errorf("failed to update session: %w", err)
	}

	if o.OnClosePtySession != nil {
		o.OnClosePtySession(sessionID)
	}

	pool := db.WrapAsPool(database)
	wfiRepo := repo.NewWorkflowInstanceRepo(pool, o.clock)
	workflowName := ""
	if wfi, err := wfiRepo.Get(session.WorkflowInstanceID); err == nil {
		workflowName = wfi.WorkflowID
	}

	o.mu.Lock()
	seen := make(map[*spawner.Spawner]struct{})
	for _, rs := range o.runs {
		for _, sp := range rs.spawners {
			if _, ok := seen[sp]; ok {
				continue
			}
			seen[sp] = struct{}{}
			sp.KillInteractive(sessionID)
		}
	}
	o.mu.Unlock()

	o.wsHub.Broadcast(ws.NewEvent(ws.EventAgentKilled, session.ProjectID, session.TicketID, workflowName, map[string]interface{}{
		"session_id": sessionID,
		"agent_type": session.AgentType,
		"model_id":   session.ModelID.String,
		"reason":     "user_killed",
	}))

	return nil
}
