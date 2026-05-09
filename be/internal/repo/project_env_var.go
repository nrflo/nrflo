package repo

import (
	"fmt"
	"strings"
	"time"

	"be/internal/clock"
	"be/internal/db"
	"be/internal/model"
)

// ProjectEnvVarRepo handles project_env_vars CRUD operations.
type ProjectEnvVarRepo struct {
	clock clock.Clock
	db    db.Querier
}

// NewProjectEnvVarRepo creates a new project env var repository.
func NewProjectEnvVarRepo(database db.Querier, clk clock.Clock) *ProjectEnvVarRepo {
	return &ProjectEnvVarRepo{db: database, clock: clk}
}

// List returns all env vars for a project ordered by name ASC.
func (r *ProjectEnvVarRepo) List(projectID string) ([]*model.ProjectEnvVar, error) {
	rows, err := r.db.Query(`
		SELECT project_id, name, value, created_at, updated_at
		FROM project_env_vars
		WHERE LOWER(project_id) = LOWER(?)
		ORDER BY name ASC`, projectID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var vars []*model.ProjectEnvVar
	for rows.Next() {
		v := &model.ProjectEnvVar{}
		var createdAt, updatedAt string
		if err := rows.Scan(&v.ProjectID, &v.Name, &v.Value, &createdAt, &updatedAt); err != nil {
			return nil, err
		}
		v.CreatedAt, _ = time.Parse(time.RFC3339Nano, createdAt)
		v.UpdatedAt, _ = time.Parse(time.RFC3339Nano, updatedAt)
		vars = append(vars, v)
	}
	return vars, nil
}

// Upsert inserts or updates an env var for a project.
func (r *ProjectEnvVarRepo) Upsert(projectID, name, value string) (*model.ProjectEnvVar, error) {
	now := r.clock.Now().UTC().Format(time.RFC3339Nano)
	pid := strings.ToLower(projectID)

	_, err := r.db.Exec(`
		INSERT INTO project_env_vars (project_id, name, value, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?)
		ON CONFLICT(project_id, name) DO UPDATE SET value = excluded.value, updated_at = excluded.updated_at`,
		pid, name, value, now, now,
	)
	if err != nil {
		return nil, err
	}

	v := &model.ProjectEnvVar{
		ProjectID: pid,
		Name:      name,
		Value:     value,
	}
	v.CreatedAt, _ = time.Parse(time.RFC3339Nano, now)
	v.UpdatedAt = v.CreatedAt
	return v, nil
}

// Delete removes an env var. Returns an error if the row did not exist.
func (r *ProjectEnvVarRepo) Delete(projectID, name string) error {
	result, err := r.db.Exec(
		"DELETE FROM project_env_vars WHERE LOWER(project_id) = LOWER(?) AND name = ?",
		projectID, name,
	)
	if err != nil {
		return err
	}
	rowsAffected, _ := result.RowsAffected()
	if rowsAffected == 0 {
		return fmt.Errorf("env var not found: %s", name)
	}
	return nil
}
