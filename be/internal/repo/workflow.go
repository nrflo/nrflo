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

// WorkflowRepo handles workflow definition CRUD operations
type WorkflowRepo struct {
	clock clock.Clock
	db *db.DB
}

// NewWorkflowRepo creates a new workflow repository
func NewWorkflowRepo(database *db.DB, clk clock.Clock) *WorkflowRepo {
	return &WorkflowRepo{db: database, clock: clk}
}

// Create creates a new workflow definition
func (r *WorkflowRepo) Create(wf *model.Workflow) error {
	now := r.clock.Now().UTC().Format(time.RFC3339Nano)
	wf.CreatedAt, _ = time.Parse(time.RFC3339Nano, now)
	wf.UpdatedAt = wf.CreatedAt
	if wf.ScopeType == "" {
		wf.ScopeType = "ticket"
	}

	_, err := r.db.Exec(`
		INSERT INTO workflows (id, project_id, description, phases, scope_type, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?)`,
		strings.ToLower(wf.ID),
		strings.ToLower(wf.ProjectID),
		wf.Description,
		wf.Phases,
		wf.ScopeType,
		now,
		now,
	)
	return err
}

// Get retrieves a workflow definition by project and ID
func (r *WorkflowRepo) Get(projectID, id string) (*model.Workflow, error) {
	wf := &model.Workflow{}
	var createdAt, updatedAt string

	err := r.db.QueryRow(`
		SELECT id, project_id, description, phases, scope_type, created_at, updated_at
		FROM workflows WHERE LOWER(project_id) = LOWER(?) AND LOWER(id) = LOWER(?)`,
		projectID, id).Scan(
		&wf.ID,
		&wf.ProjectID,
		&wf.Description,
		&wf.Phases,
		&wf.ScopeType,
		&createdAt,
		&updatedAt,
	)
	if err == sql.ErrNoRows {
		return nil, fmt.Errorf("workflow not found: %s", id)
	}
	if err != nil {
		return nil, err
	}

	wf.CreatedAt, _ = time.Parse(time.RFC3339Nano, createdAt)
	wf.UpdatedAt, _ = time.Parse(time.RFC3339Nano, updatedAt)

	return wf, nil
}

// List retrieves all workflow definitions for a project
func (r *WorkflowRepo) List(projectID string) ([]*model.Workflow, error) {
	rows, err := r.db.Query(`
		SELECT id, project_id, description, phases, scope_type, created_at, updated_at
		FROM workflows WHERE LOWER(project_id) = LOWER(?)
		ORDER BY id`, projectID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var workflows []*model.Workflow
	for rows.Next() {
		wf := &model.Workflow{}
		var createdAt, updatedAt string

		err := rows.Scan(
			&wf.ID,
			&wf.ProjectID,
			&wf.Description,
			&wf.Phases,
			&wf.ScopeType,
			&createdAt,
			&updatedAt,
		)
		if err != nil {
			return nil, err
		}

		wf.CreatedAt, _ = time.Parse(time.RFC3339Nano, createdAt)
		wf.UpdatedAt, _ = time.Parse(time.RFC3339Nano, updatedAt)

		workflows = append(workflows, wf)
	}

	return workflows, nil
}

// WorkflowUpdateFields contains fields that can be updated
type WorkflowUpdateFields struct {
	Description *string
	Phases      *string
}

// Update updates a workflow definition
func (r *WorkflowRepo) Update(projectID, id string, fields *WorkflowUpdateFields) error {
	updates := []string{}
	args := []interface{}{}

	if fields.Description != nil {
		updates = append(updates, "description = ?")
		args = append(args, *fields.Description)
	}
	if fields.Phases != nil {
		updates = append(updates, "phases = ?")
		args = append(args, *fields.Phases)
	}

	if len(updates) == 0 {
		return nil
	}

	now := r.clock.Now().UTC().Format(time.RFC3339Nano)
	updates = append(updates, "updated_at = ?")
	args = append(args, now)
	args = append(args, projectID, id)

	query := "UPDATE workflows SET " + strings.Join(updates, ", ") +
		" WHERE LOWER(project_id) = LOWER(?) AND LOWER(id) = LOWER(?)"

	result, err := r.db.Exec(query, args...)
	if err != nil {
		return err
	}

	rowsAffected, _ := result.RowsAffected()
	if rowsAffected == 0 {
		return fmt.Errorf("workflow not found: %s", id)
	}
	return nil
}

// Delete deletes a workflow definition
func (r *WorkflowRepo) Delete(projectID, id string) error {
	result, err := r.db.Exec(
		"DELETE FROM workflows WHERE LOWER(project_id) = LOWER(?) AND LOWER(id) = LOWER(?)",
		projectID, id)
	if err != nil {
		return err
	}

	rowsAffected, _ := result.RowsAffected()
	if rowsAffected == 0 {
		return fmt.Errorf("workflow not found: %s", id)
	}
	return nil
}

// Exists checks if a workflow definition exists
func (r *WorkflowRepo) Exists(projectID, id string) (bool, error) {
	var count int
	err := r.db.QueryRow(
		"SELECT COUNT(*) FROM workflows WHERE LOWER(project_id) = LOWER(?) AND LOWER(id) = LOWER(?)",
		projectID, id).Scan(&count)
	if err != nil {
		return false, err
	}
	return count > 0, nil
}
