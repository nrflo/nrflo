package orchestrator

import (
	"context"
	"fmt"
	"time"

	"be/internal/db"
	"be/internal/logger"
	"be/internal/repo"
	"be/internal/spawner"
)

// TakeControl sends a take-control signal to the active spawner for a ticket-scoped workflow.
// Looks up the instance from the session's workflow_instance_id.
func (o *Orchestrator) TakeControl(projectID, ticketID, workflowName, sessionID string) (string, error) {
	database, err := db.Open(o.dataPath)
	if err != nil {
		return "", fmt.Errorf("failed to open database: %w", err)
	}
	defer database.Close()

	asRepo := repo.NewAgentSessionRepo(database, o.clock)
	session, err := asRepo.Get(sessionID)
	if err != nil {
		return "", fmt.Errorf("agent session not found: %w", err)
	}

	return o.takeControlByInstance(session.WorkflowInstanceID, workflowName, ticketID, sessionID)
}

// TakeControlProject sends a take-control signal for a project-scoped workflow.
func (o *Orchestrator) TakeControlProject(projectID, workflowName, sessionID, instanceID string) (string, error) {
	if instanceID == "" {
		return "", fmt.Errorf("instance_id is required for project-scoped workflow take-control")
	}
	return o.takeControlByInstance(instanceID, workflowName, projectID, sessionID)
}

func (o *Orchestrator) takeControlByInstance(wfiID, workflowName, target, sessionID string) (string, error) {
	logger.Info(context.Background(), "take-control requested", "session_id", sessionID, "workflow", workflowName)
	o.mu.Lock()
	rs, ok := o.runs[wfiID]
	o.mu.Unlock()
	if !ok {
		return "", fmt.Errorf("no running orchestration for workflow '%s' on %s", workflowName, target)
	}

	o.mu.Lock()
	sp := rs.spawners[sessionID]
	o.mu.Unlock()
	if sp == nil {
		return "", fmt.Errorf("no active spawner (agent may be between phases)")
	}

	sp.RequestTakeControl(sessionID)
	return sessionID, nil
}

// WaitTakeControlReady blocks until the spawner has finished the synchronous
// portion of a previously-requested take-control (kill + status flip to
// user_interactive, viewer-attach broadcast, or rejection), or until timeout.
// Returns true if the ready signal fired before timeout. Best-effort: returns
// false when the session/spawner can't be located.
func (o *Orchestrator) WaitTakeControlReady(sessionID string, timeout time.Duration) bool {
	o.mu.Lock()
	var sp *spawner.Spawner
	for _, rs := range o.runs {
		if rs == nil {
			continue
		}
		if found, ok := rs.spawners[sessionID]; ok {
			sp = found
			break
		}
	}
	o.mu.Unlock()
	if sp == nil {
		return false
	}
	return sp.WaitForTakeControlReady(sessionID, timeout)
}

// SignalSessionReady marks the matching running proc as TUI-ready, releasing
// its prompt-delivery wait. Best-effort: returns nil when session or run is
// not found. Idempotent on the spawner side.
func (o *Orchestrator) SignalSessionReady(sessionID string) error {
	o.mu.Lock()
	seen := make(map[*spawner.Spawner]struct{})
	for _, rs := range o.runs {
		if rs == nil {
			continue
		}
		for _, sp := range rs.spawners {
			if _, ok := seen[sp]; ok {
				continue
			}
			seen[sp] = struct{}{}
			sp.MarkSessionReady(sessionID)
		}
	}
	o.mu.Unlock()
	return nil
}
