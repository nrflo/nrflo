package repo

import (
	"database/sql"
	"fmt"
	"strings"
	"time"

	"be/internal/clock"
	"be/internal/db"
	"be/internal/model"
	"be/internal/types"
)

// PythonScriptRepo handles python_scripts CRUD operations
type PythonScriptRepo struct {
	clock clock.Clock
	db    db.Querier
}

// NewPythonScriptRepo creates a new python script repository
func NewPythonScriptRepo(database db.Querier, clk clock.Clock) *PythonScriptRepo {
	return &PythonScriptRepo{db: database, clock: clk}
}

// Create creates a new python script
func (r *PythonScriptRepo) Create(script *model.PythonScript) error {
	now := r.clock.Now().UTC().Format(time.RFC3339Nano)
	script.CreatedAt, _ = time.Parse(time.RFC3339Nano, now)
	script.UpdatedAt = script.CreatedAt

	_, err := r.db.Exec(`
		INSERT INTO python_scripts
		    (id, project_id, name, description, code, file_path, kind, tool_description, input_schema, timeout_sec, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		strings.ToLower(script.ID),
		strings.ToLower(script.ProjectID),
		script.Name,
		script.Description,
		script.Code,
		script.FilePath,
		script.Kind,
		script.ToolDescription,
		script.InputSchema,
		script.TimeoutSec,
		now,
		now,
	)
	return err
}

// Get retrieves a python script by project ID and script ID
func (r *PythonScriptRepo) Get(projectID, id string) (*model.PythonScript, error) {
	script := &model.PythonScript{}
	var createdAt, updatedAt string

	err := r.db.QueryRow(`
		SELECT id, project_id, name, description, code, file_path, kind, tool_description, input_schema, timeout_sec, created_at, updated_at
		FROM python_scripts
		WHERE LOWER(project_id) = LOWER(?) AND LOWER(id) = LOWER(?)`,
		projectID, id).Scan(
		&script.ID,
		&script.ProjectID,
		&script.Name,
		&script.Description,
		&script.Code,
		&script.FilePath,
		&script.Kind,
		&script.ToolDescription,
		&script.InputSchema,
		&script.TimeoutSec,
		&createdAt,
		&updatedAt,
	)
	if err == sql.ErrNoRows {
		return nil, fmt.Errorf("python script not found: %s", id)
	}
	if err != nil {
		return nil, err
	}

	script.CreatedAt, _ = time.Parse(time.RFC3339Nano, createdAt)
	script.UpdatedAt, _ = time.Parse(time.RFC3339Nano, updatedAt)
	return script, nil
}

// List retrieves all python scripts for a project, ordered by name
func (r *PythonScriptRepo) List(projectID string) ([]*model.PythonScript, error) {
	return r.listByFilter(projectID, "")
}

// ListByKind retrieves all python scripts for a project filtered by kind, ordered by name
func (r *PythonScriptRepo) ListByKind(projectID, kind string) ([]*model.PythonScript, error) {
	return r.listByFilter(projectID, kind)
}

func (r *PythonScriptRepo) listByFilter(projectID, kind string) ([]*model.PythonScript, error) {
	query := `
		SELECT id, project_id, name, description, code, file_path, kind, tool_description, input_schema, timeout_sec, created_at, updated_at
		FROM python_scripts
		WHERE LOWER(project_id) = LOWER(?)`
	args := []interface{}{projectID}
	if kind != "" {
		query += " AND kind = ?"
		args = append(args, kind)
	}
	query += " ORDER BY name ASC"

	rows, err := r.db.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var scripts []*model.PythonScript
	for rows.Next() {
		script := &model.PythonScript{}
		var createdAt, updatedAt string

		err := rows.Scan(
			&script.ID,
			&script.ProjectID,
			&script.Name,
			&script.Description,
			&script.Code,
			&script.FilePath,
			&script.Kind,
			&script.ToolDescription,
			&script.InputSchema,
			&script.TimeoutSec,
			&createdAt,
			&updatedAt,
		)
		if err != nil {
			return nil, err
		}

		script.CreatedAt, _ = time.Parse(time.RFC3339Nano, createdAt)
		script.UpdatedAt, _ = time.Parse(time.RFC3339Nano, updatedAt)
		scripts = append(scripts, script)
	}

	return scripts, nil
}

// Update updates a python script
func (r *PythonScriptRepo) Update(projectID, id string, req *types.PythonScriptUpdateRequest) error {
	updates := []string{}
	args := []interface{}{}

	if req.Name != nil {
		updates = append(updates, "name = ?")
		args = append(args, *req.Name)
	}
	if req.Description != nil {
		updates = append(updates, "description = ?")
		args = append(args, *req.Description)
	}
	if req.Code != nil {
		updates = append(updates, "code = ?")
		args = append(args, *req.Code)
	}
	if req.FilePath != nil {
		updates = append(updates, "file_path = ?")
		args = append(args, *req.FilePath)
	}
	if req.ToolDescription != nil {
		updates = append(updates, "tool_description = ?")
		args = append(args, *req.ToolDescription)
	}
	if req.InputSchema != nil {
		updates = append(updates, "input_schema = ?")
		args = append(args, *req.InputSchema)
	}
	if req.TimeoutSec != nil {
		updates = append(updates, "timeout_sec = ?")
		args = append(args, *req.TimeoutSec)
	}

	if len(updates) == 0 {
		return nil
	}

	now := r.clock.Now().UTC().Format(time.RFC3339Nano)
	updates = append(updates, "updated_at = ?")
	args = append(args, now)
	args = append(args, projectID, id)

	query := "UPDATE python_scripts SET " + strings.Join(updates, ", ") +
		" WHERE LOWER(project_id) = LOWER(?) AND LOWER(id) = LOWER(?)"

	result, err := r.db.Exec(query, args...)
	if err != nil {
		return err
	}

	rowsAffected, _ := result.RowsAffected()
	if rowsAffected == 0 {
		return fmt.Errorf("python script not found: %s", id)
	}
	return nil
}

// Delete deletes a python script
func (r *PythonScriptRepo) Delete(projectID, id string) error {
	result, err := r.db.Exec(
		"DELETE FROM python_scripts WHERE LOWER(project_id) = LOWER(?) AND LOWER(id) = LOWER(?)",
		projectID, id)
	if err != nil {
		return err
	}

	rowsAffected, _ := result.RowsAffected()
	if rowsAffected == 0 {
		return fmt.Errorf("python script not found: %s", id)
	}
	return nil
}
