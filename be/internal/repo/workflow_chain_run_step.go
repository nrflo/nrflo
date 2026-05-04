package repo

import (
	"database/sql"
	"fmt"
	"time"

	"be/internal/model"
)

// GetRunStepByInstanceID returns the run step whose workflow_instance_id matches the given value.
func (r *WorkflowChainRunRepo) GetRunStepByInstanceID(instanceID string) (*model.WorkflowChainRunStep, error) {
	row := r.db.QueryRow(
		`SELECT id, chain_run_id, position, workflow_name, scope_type, require_ticket_handoff, workflow_instance_id, ticket_id, instructions_used, status, started_at, ended_at, created_at, updated_at
         FROM workflow_chain_run_steps
         WHERE workflow_instance_id = ? LIMIT 1`, instanceID)
	rs, err := scanWorkflowChainRunStep(row)
	if err == sql.ErrNoRows {
		return nil, fmt.Errorf("chain run step not found for instance: %s", instanceID)
	}
	return rs, err
}

// SetNextPendingStepInstructions finds the next pending step in the run containing the given
// workflow_instance_id and sets its instructions_used.
func (r *WorkflowChainRunRepo) SetNextPendingStepInstructions(instanceID, instructions string) error {
	current, err := r.GetRunStepByInstanceID(instanceID)
	if err != nil {
		return err
	}
	now := r.clock.Now().UTC().Format(time.RFC3339Nano)
	result, err := r.db.Exec(
		`UPDATE workflow_chain_run_steps SET instructions_used=?, updated_at=?
         WHERE chain_run_id=? AND position=? AND status='pending'`,
		instructions, now, current.ChainRunID, current.Position+1)
	if err != nil {
		return err
	}
	n, _ := result.RowsAffected()
	if n == 0 {
		return fmt.Errorf("no pending next step at position %d in run %s", current.Position+1, current.ChainRunID)
	}
	return nil
}

// SetNextPendingStepTicket finds the next pending step in the run containing the given
// workflow_instance_id and sets its ticket_id.
func (r *WorkflowChainRunRepo) SetNextPendingStepTicket(instanceID, ticketID string) error {
	current, err := r.GetRunStepByInstanceID(instanceID)
	if err != nil {
		return err
	}
	now := r.clock.Now().UTC().Format(time.RFC3339Nano)
	result, err := r.db.Exec(
		`UPDATE workflow_chain_run_steps SET ticket_id=?, updated_at=?
         WHERE chain_run_id=? AND position=? AND status='pending'`,
		ticketID, now, current.ChainRunID, current.Position+1)
	if err != nil {
		return err
	}
	n, _ := result.RowsAffected()
	if n == 0 {
		return fmt.Errorf("no pending next step at position %d in run %s", current.Position+1, current.ChainRunID)
	}
	return nil
}

// ListRunSteps returns all run steps for a chain run, ordered by position.
func (r *WorkflowChainRunRepo) ListRunSteps(runID string) ([]*model.WorkflowChainRunStep, error) {
	rows, err := r.db.Query(
		`SELECT id, chain_run_id, position, workflow_name, scope_type, require_ticket_handoff, workflow_instance_id, ticket_id, instructions_used, status, started_at, ended_at, created_at, updated_at
         FROM workflow_chain_run_steps
         WHERE chain_run_id = ?
         ORDER BY position ASC`, runID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var steps []*model.WorkflowChainRunStep
	for rows.Next() {
		rs, err := scanWorkflowChainRunStep(rows)
		if err != nil {
			return nil, err
		}
		steps = append(steps, rs)
	}
	return steps, rows.Err()
}
