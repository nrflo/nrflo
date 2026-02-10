package repo

import (
	"database/sql"
	"fmt"
	"strings"
	"time"

	"nrworkflow/internal/db"
	"nrworkflow/internal/model"
)

// WorkflowInstanceRepo handles workflow instance CRUD operations using Pool
type WorkflowInstanceRepo struct {
	pool *db.Pool
}

// NewWorkflowInstanceRepo creates a new workflow instance repository
func NewWorkflowInstanceRepo(pool *db.Pool) *WorkflowInstanceRepo {
	return &WorkflowInstanceRepo{pool: pool}
}

const wfiCols = `id, project_id, ticket_id, workflow_id, status, category, current_phase,
	phase_order, phases, findings, retry_count, parent_session, created_at, updated_at`

func scanWFI(scanner interface{ Scan(...interface{}) error }) (*model.WorkflowInstance, error) {
	wi := &model.WorkflowInstance{}
	var createdAt, updatedAt string
	err := scanner.Scan(
		&wi.ID, &wi.ProjectID, &wi.TicketID, &wi.WorkflowID,
		&wi.Status, &wi.Category, &wi.CurrentPhase,
		&wi.PhaseOrder, &wi.Phases, &wi.Findings,
		&wi.RetryCount, &wi.ParentSession, &createdAt, &updatedAt,
	)
	if err != nil {
		return nil, err
	}
	wi.CreatedAt, _ = time.Parse(time.RFC3339, createdAt)
	wi.UpdatedAt, _ = time.Parse(time.RFC3339, updatedAt)
	return wi, nil
}

// Create creates a new workflow instance
func (r *WorkflowInstanceRepo) Create(wi *model.WorkflowInstance) error {
	now := time.Now().UTC().Format(time.RFC3339)
	wi.CreatedAt, _ = time.Parse(time.RFC3339, now)
	wi.UpdatedAt = wi.CreatedAt

	_, err := r.pool.Exec(`
		INSERT INTO workflow_instances (`+wfiCols+`)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		wi.ID, strings.ToLower(wi.ProjectID), strings.ToLower(wi.TicketID),
		strings.ToLower(wi.WorkflowID), wi.Status, wi.Category, wi.CurrentPhase,
		wi.PhaseOrder, wi.Phases, wi.Findings,
		wi.RetryCount, wi.ParentSession, now, now,
	)
	if err != nil {
		if strings.Contains(err.Error(), "UNIQUE constraint") {
			return fmt.Errorf("workflow '%s' already initialized on %s", wi.WorkflowID, wi.TicketID)
		}
		return err
	}
	return nil
}

// Get retrieves a workflow instance by ID
func (r *WorkflowInstanceRepo) Get(id string) (*model.WorkflowInstance, error) {
	row := r.pool.QueryRow(`SELECT `+wfiCols+` FROM workflow_instances WHERE id = ?`, id)
	wi, err := scanWFI(row)
	if err == sql.ErrNoRows {
		return nil, fmt.Errorf("workflow instance not found: %s", id)
	}
	return wi, err
}

// GetByTicketAndWorkflow retrieves a workflow instance by project, ticket, and workflow ID
func (r *WorkflowInstanceRepo) GetByTicketAndWorkflow(projectID, ticketID, workflowID string) (*model.WorkflowInstance, error) {
	row := r.pool.QueryRow(`
		SELECT `+wfiCols+` FROM workflow_instances
		WHERE LOWER(project_id) = LOWER(?) AND LOWER(ticket_id) = LOWER(?) AND LOWER(workflow_id) = LOWER(?)`,
		projectID, ticketID, workflowID)
	wi, err := scanWFI(row)
	if err == sql.ErrNoRows {
		return nil, fmt.Errorf("workflow '%s' not found on %s", workflowID, ticketID)
	}
	return wi, err
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

// UpdatePhases updates the phases JSON field
func (r *WorkflowInstanceRepo) UpdatePhases(id string, phases string) error {
	now := time.Now().UTC().Format(time.RFC3339)
	result, err := r.pool.Exec(
		`UPDATE workflow_instances SET phases = ?, updated_at = ? WHERE id = ?`,
		phases, now, id)
	if err != nil {
		return err
	}
	return checkAffected(result, id)
}

// UpdateCurrentPhase updates the current_phase field
func (r *WorkflowInstanceRepo) UpdateCurrentPhase(id, phase string) error {
	now := time.Now().UTC().Format(time.RFC3339)
	result, err := r.pool.Exec(
		`UPDATE workflow_instances SET current_phase = ?, updated_at = ? WHERE id = ?`,
		phase, now, id)
	if err != nil {
		return err
	}
	return checkAffected(result, id)
}

// UpdateStatus updates the workflow instance status
func (r *WorkflowInstanceRepo) UpdateStatus(id string, status model.WorkflowInstanceStatus) error {
	now := time.Now().UTC().Format(time.RFC3339)
	result, err := r.pool.Exec(
		`UPDATE workflow_instances SET status = ?, updated_at = ? WHERE id = ?`,
		status, now, id)
	if err != nil {
		return err
	}
	return checkAffected(result, id)
}

// UpdateCategory updates the category field
func (r *WorkflowInstanceRepo) UpdateCategory(id, category string) error {
	now := time.Now().UTC().Format(time.RFC3339)
	result, err := r.pool.Exec(
		`UPDATE workflow_instances SET category = ?, updated_at = ? WHERE id = ?`,
		category, now, id)
	if err != nil {
		return err
	}
	return checkAffected(result, id)
}

// UpdateRetryCount updates the retry_count field
func (r *WorkflowInstanceRepo) UpdateRetryCount(id string, retryCount int) error {
	now := time.Now().UTC().Format(time.RFC3339)
	result, err := r.pool.Exec(
		`UPDATE workflow_instances SET retry_count = ?, updated_at = ? WHERE id = ?`,
		retryCount, now, id)
	if err != nil {
		return err
	}
	return checkAffected(result, id)
}

// UpdateFindings updates the workflow-level findings JSON
func (r *WorkflowInstanceRepo) UpdateFindings(id string, findings string) error {
	now := time.Now().UTC().Format(time.RFC3339)
	result, err := r.pool.Exec(
		`UPDATE workflow_instances SET findings = ?, updated_at = ? WHERE id = ?`,
		findings, now, id)
	if err != nil {
		return err
	}
	return checkAffected(result, id)
}

// StartPhase sets a phase to in_progress and updates current_phase
func (r *WorkflowInstanceRepo) StartPhase(id, phase string) error {
	wi, err := r.Get(id)
	if err != nil {
		return err
	}

	phases := wi.GetPhases()
	phases[phase] = model.PhaseStatus{Status: "in_progress"}
	wi.SetPhases(phases)

	now := time.Now().UTC().Format(time.RFC3339)
	_, err = r.pool.Exec(
		`UPDATE workflow_instances SET phases = ?, current_phase = ?, updated_at = ? WHERE id = ?`,
		wi.Phases, phase, now, id)
	return err
}

// CompletePhase sets a phase to completed with a result
func (r *WorkflowInstanceRepo) CompletePhase(id, phase, result string) error {
	wi, err := r.Get(id)
	if err != nil {
		return err
	}

	phases := wi.GetPhases()
	phases[phase] = model.PhaseStatus{Status: "completed", Result: result}
	wi.SetPhases(phases)

	now := time.Now().UTC().Format(time.RFC3339)
	_, err = r.pool.Exec(
		`UPDATE workflow_instances SET phases = ?, updated_at = ? WHERE id = ?`,
		wi.Phases, now, id)
	return err
}

// Delete deletes a workflow instance
func (r *WorkflowInstanceRepo) Delete(id string) error {
	result, err := r.pool.Exec(`DELETE FROM workflow_instances WHERE id = ?`, id)
	if err != nil {
		return err
	}
	return checkAffected(result, id)
}

func checkAffected(result sql.Result, id string) error {
	n, _ := result.RowsAffected()
	if n == 0 {
		return fmt.Errorf("workflow instance not found: %s", id)
	}
	return nil
}
