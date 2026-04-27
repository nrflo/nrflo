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

// ToolDefinitionRepo handles tool definition CRUD operations.
type ToolDefinitionRepo struct {
	clock clock.Clock
	db    db.Querier
}

// NewToolDefinitionRepo creates a new tool definition repository.
func NewToolDefinitionRepo(database db.Querier, clk clock.Clock) *ToolDefinitionRepo {
	return &ToolDefinitionRepo{db: database, clock: clk}
}

// Create inserts a new tool definition.
func (r *ToolDefinitionRepo) Create(def *model.ToolDefinition) error {
	now := r.clock.Now().UTC().Format(time.RFC3339Nano)
	def.CreatedAt, _ = time.Parse(time.RFC3339Nano, now)
	def.UpdatedAt = def.CreatedAt

	if def.AuthMethod == "" {
		def.AuthMethod = "none"
	}
	if def.TimeoutSec == 0 {
		def.TimeoutSec = 30
	}

	_, err := r.db.Exec(`
		INSERT INTO tool_definitions (id, name, description, input_schema, endpoint, auth_method, auth_ref, timeout_sec, project_id, workflow_id, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		strings.ToLower(def.ID),
		def.Name,
		def.Description,
		string(def.InputSchema),
		def.Endpoint,
		def.AuthMethod,
		def.AuthRef,
		def.TimeoutSec,
		def.ProjectID,
		def.WorkflowID,
		now,
		now,
	)
	return err
}

func (r *ToolDefinitionRepo) scan(row interface {
	Scan(...interface{}) error
}) (*model.ToolDefinition, error) {
	def := &model.ToolDefinition{}
	var createdAt, updatedAt, inputSchema string
	var authRef, projectID, workflowID sql.NullString
	if err := row.Scan(
		&def.ID,
		&def.Name,
		&def.Description,
		&inputSchema,
		&def.Endpoint,
		&def.AuthMethod,
		&authRef,
		&def.TimeoutSec,
		&projectID,
		&workflowID,
		&createdAt,
		&updatedAt,
	); err != nil {
		return nil, err
	}
	def.InputSchema = []byte(inputSchema)
	if authRef.Valid {
		v := authRef.String
		def.AuthRef = &v
	}
	if projectID.Valid {
		v := projectID.String
		def.ProjectID = &v
	}
	if workflowID.Valid {
		v := workflowID.String
		def.WorkflowID = &v
	}
	def.CreatedAt, _ = time.Parse(time.RFC3339Nano, createdAt)
	def.UpdatedAt, _ = time.Parse(time.RFC3339Nano, updatedAt)
	return def, nil
}

// Get returns a tool definition by id.
func (r *ToolDefinitionRepo) Get(id string) (*model.ToolDefinition, error) {
	row := r.db.QueryRow(`
		SELECT id, name, description, input_schema, endpoint, auth_method, auth_ref, timeout_sec, project_id, workflow_id, created_at, updated_at
		FROM tool_definitions WHERE LOWER(id) = LOWER(?)`, id)
	def, err := r.scan(row)
	if err == sql.ErrNoRows {
		return nil, fmt.Errorf("tool definition not found: %s", id)
	}
	return def, err
}

func (r *ToolDefinitionRepo) listRows(query string, args ...interface{}) ([]*model.ToolDefinition, error) {
	rows, err := r.db.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []*model.ToolDefinition{}
	for rows.Next() {
		def, err := r.scan(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, def)
	}
	return out, nil
}

// List returns all tool definitions ordered by name.
func (r *ToolDefinitionRepo) List() ([]*model.ToolDefinition, error) {
	return r.listRows(`
		SELECT id, name, description, input_schema, endpoint, auth_method, auth_ref, timeout_sec, project_id, workflow_id, created_at, updated_at
		FROM tool_definitions ORDER BY name ASC`)
}

// ListByProject returns tool definitions scoped to the given project (or global, NULL).
func (r *ToolDefinitionRepo) ListByProject(projectID string) ([]*model.ToolDefinition, error) {
	return r.listRows(`
		SELECT id, name, description, input_schema, endpoint, auth_method, auth_ref, timeout_sec, project_id, workflow_id, created_at, updated_at
		FROM tool_definitions WHERE project_id IS NULL OR LOWER(project_id) = LOWER(?)
		ORDER BY name ASC`, projectID)
}

// ListByWorkflow returns tool definitions scoped to the given workflow id.
func (r *ToolDefinitionRepo) ListByWorkflow(workflowID string) ([]*model.ToolDefinition, error) {
	return r.listRows(`
		SELECT id, name, description, input_schema, endpoint, auth_method, auth_ref, timeout_sec, project_id, workflow_id, created_at, updated_at
		FROM tool_definitions WHERE LOWER(workflow_id) = LOWER(?) ORDER BY name ASC`, workflowID)
}

// ToolDefUpdateFields lists updatable fields. Nil pointers are skipped.
type ToolDefUpdateFields struct {
	Name        *string
	Description *string
	InputSchema *string
	Endpoint    *string
	AuthMethod  *string
	AuthRef     *string
	TimeoutSec  *int
	ProjectID   *string
	WorkflowID  *string
}

// Update applies the provided fields to an existing tool definition.
func (r *ToolDefinitionRepo) Update(id string, fields *ToolDefUpdateFields) error {
	updates := []string{}
	args := []interface{}{}

	if fields.Name != nil {
		updates = append(updates, "name = ?")
		args = append(args, *fields.Name)
	}
	if fields.Description != nil {
		updates = append(updates, "description = ?")
		args = append(args, *fields.Description)
	}
	if fields.InputSchema != nil {
		updates = append(updates, "input_schema = ?")
		args = append(args, *fields.InputSchema)
	}
	if fields.Endpoint != nil {
		updates = append(updates, "endpoint = ?")
		args = append(args, *fields.Endpoint)
	}
	if fields.AuthMethod != nil {
		updates = append(updates, "auth_method = ?")
		args = append(args, *fields.AuthMethod)
	}
	if fields.AuthRef != nil {
		updates = append(updates, "auth_ref = ?")
		args = append(args, *fields.AuthRef)
	}
	if fields.TimeoutSec != nil {
		updates = append(updates, "timeout_sec = ?")
		args = append(args, *fields.TimeoutSec)
	}
	if fields.ProjectID != nil {
		updates = append(updates, "project_id = ?")
		args = append(args, *fields.ProjectID)
	}
	if fields.WorkflowID != nil {
		updates = append(updates, "workflow_id = ?")
		args = append(args, *fields.WorkflowID)
	}

	if len(updates) == 0 {
		return nil
	}

	now := r.clock.Now().UTC().Format(time.RFC3339Nano)
	updates = append(updates, "updated_at = ?")
	args = append(args, now, id)

	query := "UPDATE tool_definitions SET " + strings.Join(updates, ", ") + " WHERE LOWER(id) = LOWER(?)"
	result, err := r.db.Exec(query, args...)
	if err != nil {
		return err
	}
	if rows, _ := result.RowsAffected(); rows == 0 {
		return fmt.Errorf("tool definition not found: %s", id)
	}
	return nil
}

// Delete removes a tool definition.
func (r *ToolDefinitionRepo) Delete(id string) error {
	result, err := r.db.Exec("DELETE FROM tool_definitions WHERE LOWER(id) = LOWER(?)", id)
	if err != nil {
		return err
	}
	if rows, _ := result.RowsAffected(); rows == 0 {
		return fmt.Errorf("tool definition not found: %s", id)
	}
	return nil
}
