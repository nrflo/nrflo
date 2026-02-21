package repo

import (
	"database/sql"
	"fmt"
	"strings"
	"time"

	"be/internal/clock"
	"be/internal/db"
	"be/internal/model"
)

// WorkflowInstanceRepo handles workflow instance CRUD operations using Pool
type WorkflowInstanceRepo struct {
	clock clock.Clock
	pool *db.Pool
}

// NewWorkflowInstanceRepo creates a new workflow instance repository
func NewWorkflowInstanceRepo(pool *db.Pool, clk clock.Clock) *WorkflowInstanceRepo {
	return &WorkflowInstanceRepo{pool: pool, clock: clk}
}

const wfiCols = `id, project_id, ticket_id, workflow_id, scope_type, status,
	findings, skip_tags, retry_count, parent_session, created_at, updated_at`

func scanWFI(scanner interface{ Scan(...interface{}) error }) (*model.WorkflowInstance, error) {
	wi := &model.WorkflowInstance{}
	var createdAt, updatedAt string
	err := scanner.Scan(
		&wi.ID, &wi.ProjectID, &wi.TicketID, &wi.WorkflowID, &wi.ScopeType,
		&wi.Status, &wi.Findings, &wi.SkipTags,
		&wi.RetryCount, &wi.ParentSession, &createdAt, &updatedAt,
	)
	if err != nil {
		return nil, err
	}
	if wi.ScopeType == "" {
		wi.ScopeType = "ticket"
	}
	wi.CreatedAt, _ = time.Parse(time.RFC3339Nano, createdAt)
	wi.UpdatedAt, _ = time.Parse(time.RFC3339Nano, updatedAt)
	return wi, nil
}

// Create creates a new workflow instance
func (r *WorkflowInstanceRepo) Create(wi *model.WorkflowInstance) error {
	now := r.clock.Now().UTC().Format(time.RFC3339Nano)
	wi.CreatedAt, _ = time.Parse(time.RFC3339Nano, now)
	wi.UpdatedAt = wi.CreatedAt
	if wi.ScopeType == "" {
		wi.ScopeType = "ticket"
	}

	_, err := r.pool.Exec(`
		INSERT INTO workflow_instances (`+wfiCols+`)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		wi.ID, strings.ToLower(wi.ProjectID), strings.ToLower(wi.TicketID),
		strings.ToLower(wi.WorkflowID), wi.ScopeType, wi.Status,
		wi.Findings, wi.SkipTags, wi.RetryCount, wi.ParentSession, now, now,
	)
	if err != nil {
		if wi.ScopeType == "ticket" && strings.Contains(err.Error(), "UNIQUE constraint") {
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

// UpdateStatus updates the workflow instance status
func (r *WorkflowInstanceRepo) UpdateStatus(id string, status model.WorkflowInstanceStatus) error {
	now := r.clock.Now().UTC().Format(time.RFC3339Nano)
	result, err := r.pool.Exec(
		`UPDATE workflow_instances SET status = ?, updated_at = ? WHERE id = ?`,
		status, now, id)
	if err != nil {
		return err
	}
	return checkAffected(result, id)
}

// UpdateRetryCount updates the retry_count field
func (r *WorkflowInstanceRepo) UpdateRetryCount(id string, retryCount int) error {
	now := r.clock.Now().UTC().Format(time.RFC3339Nano)
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
	now := r.clock.Now().UTC().Format(time.RFC3339Nano)
	result, err := r.pool.Exec(
		`UPDATE workflow_instances SET findings = ?, updated_at = ? WHERE id = ?`,
		findings, now, id)
	if err != nil {
		return err
	}
	return checkAffected(result, id)
}

// UpdateSkipTags updates the skip_tags JSON
func (r *WorkflowInstanceRepo) UpdateSkipTags(id string, skipTags string) error {
	now := r.clock.Now().UTC().Format(time.RFC3339Nano)
	result, err := r.pool.Exec(
		`UPDATE workflow_instances SET skip_tags = ?, updated_at = ? WHERE id = ?`,
		skipTags, now, id)
	if err != nil {
		return err
	}
	return checkAffected(result, id)
}

// CleanupKeepLatest deletes non-active workflow instances beyond the keep limit,
// ordered by updated_at DESC. Active instances are never deleted.
func (r *WorkflowInstanceRepo) CleanupKeepLatest(keep int) (int64, error) {
	result, err := r.pool.Exec(`
		DELETE FROM workflow_instances
		WHERE status != ?
		AND id NOT IN (
			SELECT id FROM workflow_instances
			WHERE status != ?
			ORDER BY updated_at DESC
			LIMIT ?
		)`, model.WorkflowInstanceActive, model.WorkflowInstanceActive, keep)
	if err != nil {
		return 0, err
	}
	return result.RowsAffected()
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
