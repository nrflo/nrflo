package repo

import (
	"database/sql"
	"fmt"
	"strings"

	"be/internal/model"
)

// GetByTicketAndWorkflow retrieves the most recent workflow instance by project, ticket, and workflow ID.
// After dropping the unique index, multiple instances may exist; returns the latest by created_at.
func (r *WorkflowInstanceRepo) GetByTicketAndWorkflow(projectID, ticketID, workflowID string) (*model.WorkflowInstance, error) {
	row := r.pool.QueryRow(`
		SELECT `+wfiCols+` FROM workflow_instances
		WHERE LOWER(project_id) = LOWER(?) AND LOWER(ticket_id) = LOWER(?) AND LOWER(workflow_id) = LOWER(?)
		ORDER BY created_at DESC LIMIT 1`,
		projectID, ticketID, workflowID)
	wi, err := scanWFI(row)
	if err == sql.ErrNoRows {
		return nil, fmt.Errorf("workflow '%s' not found on %s", workflowID, ticketID)
	}
	return wi, err
}

// ListActiveByProjectAndWorkflow returns all active project-scoped instances for a given workflow.
func (r *WorkflowInstanceRepo) ListActiveByProjectAndWorkflow(projectID, workflowID string) ([]*model.WorkflowInstance, error) {
	rows, err := r.pool.Query(`
		SELECT `+wfiCols+` FROM workflow_instances
		WHERE LOWER(project_id) = LOWER(?) AND LOWER(workflow_id) = LOWER(?) AND scope_type = 'project' AND status = ?
		ORDER BY created_at`, projectID, workflowID, model.WorkflowInstanceActive)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var instances []*model.WorkflowInstance
	for rows.Next() {
		wi, err := scanWFI(rows)
		if err != nil {
			return nil, err
		}
		instances = append(instances, wi)
	}
	return instances, nil
}

// ListByProjectScope lists all project-scoped workflow instances for a project
func (r *WorkflowInstanceRepo) ListByProjectScope(projectID string) ([]*model.WorkflowInstance, error) {
	rows, err := r.pool.Query(`
		SELECT `+wfiCols+` FROM workflow_instances
		WHERE LOWER(project_id) = LOWER(?) AND scope_type = 'project'
		ORDER BY created_at`, projectID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var instances []*model.WorkflowInstance
	for rows.Next() {
		wi, err := scanWFI(rows)
		if err != nil {
			return nil, err
		}
		instances = append(instances, wi)
	}
	return instances, nil
}

// ListActiveByProject returns active workflow instances grouped by ticket ID
func (r *WorkflowInstanceRepo) ListActiveByProject(projectID string) (map[string]*model.WorkflowInstance, error) {
	rows, err := r.pool.Query(`
		SELECT `+wfiCols+` FROM workflow_instances
		WHERE LOWER(project_id) = LOWER(?) AND status = ?
		ORDER BY updated_at DESC`, projectID, model.WorkflowInstanceActive)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	// Keep only the most recently updated instance per ticket
	result := make(map[string]*model.WorkflowInstance)
	for rows.Next() {
		wi, err := scanWFI(rows)
		if err != nil {
			return nil, err
		}
		ticketKey := strings.ToLower(wi.TicketID)
		if _, exists := result[ticketKey]; !exists {
			result[ticketKey] = wi
		}
	}
	return result, nil
}

// ListActive returns all active workflow instances across all projects, ordered by updated_at DESC.
func (r *WorkflowInstanceRepo) ListActive() ([]*model.WorkflowInstance, error) {
	rows, err := r.pool.Query(`
		SELECT `+wfiCols+` FROM workflow_instances
		WHERE status = ?
		ORDER BY updated_at DESC`, model.WorkflowInstanceActive)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var instances []*model.WorkflowInstance
	for rows.Next() {
		wi, err := scanWFI(rows)
		if err != nil {
			return nil, err
		}
		instances = append(instances, wi)
	}
	return instances, nil
}

// ListByParentInstance returns sub-workflow instances launched by the given parent instance.
func (r *WorkflowInstanceRepo) ListByParentInstance(parentInstanceID string) ([]*model.WorkflowInstance, error) {
	rows, err := r.pool.Query(`
		SELECT `+wfiCols+` FROM workflow_instances
		WHERE parent_instance_id = ?
		ORDER BY created_at`, parentInstanceID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var instances []*model.WorkflowInstance
	for rows.Next() {
		wi, err := scanWFI(rows)
		if err != nil {
			return nil, err
		}
		instances = append(instances, wi)
	}
	return instances, nil
}

// ListByTicket retrieves all workflow instances for a ticket
func (r *WorkflowInstanceRepo) ListByTicket(projectID, ticketID string) ([]*model.WorkflowInstance, error) {
	rows, err := r.pool.Query(`
		SELECT `+wfiCols+` FROM workflow_instances
		WHERE LOWER(project_id) = LOWER(?) AND LOWER(ticket_id) = LOWER(?)
		ORDER BY created_at`, projectID, ticketID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var instances []*model.WorkflowInstance
	for rows.Next() {
		wi, err := scanWFI(rows)
		if err != nil {
			return nil, err
		}
		instances = append(instances, wi)
	}
	return instances, nil
}
