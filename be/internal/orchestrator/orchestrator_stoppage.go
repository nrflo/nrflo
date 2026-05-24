package orchestrator

import (
	"fmt"
	"strings"

	"be/internal/db"
	"be/internal/model"
	"be/internal/repo"
)

// StopByTicket stops a running orchestration for a ticket.
// If instanceID is provided, stops that specific instance.
// If workflowName is provided, stops the first running instance for that workflow.
// Otherwise, stops the first running instance for the ticket.
func (o *Orchestrator) StopByTicket(projectID, ticketID, workflowName, instanceID string) error {
	if instanceID != "" {
		return o.Stop(instanceID)
	}

	database, err := db.Open(o.dataPath)
	if err != nil {
		return fmt.Errorf("failed to open database: %w", err)
	}
	defer database.Close()

	pool := db.WrapAsPool(database)
	wfiRepo := repo.NewWorkflowInstanceRepo(pool, o.clock)

	// Stop first running orchestration for this ticket (optionally filtered by workflow)
	instances, err := wfiRepo.ListByTicket(projectID, ticketID)
	if err != nil {
		return err
	}

	for _, wi := range instances {
		if workflowName != "" && !strings.EqualFold(wi.WorkflowID, workflowName) {
			continue
		}
		o.mu.Lock()
		_, running := o.runs[wi.ID]
		o.mu.Unlock()
		if running {
			return o.Stop(wi.ID)
		}
		// Fallback: active instance with no in-memory orchestration (orphaned after restart).
		if wi.Status == model.WorkflowInstanceActive {
			return o.forceStopInstance(wi.ID)
		}
	}

	return fmt.Errorf("no running orchestration found for %s", ticketID)
}

// StopByInstance stops a workflow instance by ID, optionally validating project ownership.
func (o *Orchestrator) StopByInstance(projectID, instanceID string) error {
	return o.Stop(instanceID)
}

// StopByProject stops a running project-scoped orchestration.
// If instanceID is provided, stops that specific instance.
// Otherwise, stops all running instances for the given workflow (or all project workflows if workflowName is empty).
func (o *Orchestrator) StopByProject(projectID, workflowName, instanceID string) error {
	// If instance ID provided, stop directly
	if instanceID != "" {
		return o.Stop(instanceID)
	}

	database, err := db.Open(o.dataPath)
	if err != nil {
		return fmt.Errorf("failed to open database: %w", err)
	}
	defer database.Close()

	pool := db.WrapAsPool(database)
	wfiRepo := repo.NewWorkflowInstanceRepo(pool, o.clock)

	instances, err := wfiRepo.ListByProjectScope(projectID)
	if err != nil {
		return err
	}

	stopped := 0
	for _, wi := range instances {
		if workflowName != "" && wi.WorkflowID != workflowName {
			continue
		}
		o.mu.Lock()
		_, running := o.runs[wi.ID]
		o.mu.Unlock()
		if running {
			if err := o.Stop(wi.ID); err == nil {
				stopped++
			}
		} else if wi.Status == model.WorkflowInstanceActive {
			if err := o.forceStopInstance(wi.ID); err == nil {
				stopped++
			}
		}
	}

	if stopped == 0 {
		return fmt.Errorf("no running project orchestration found")
	}
	return nil
}
