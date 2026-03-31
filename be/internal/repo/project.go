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

// ProjectRepo handles project CRUD operations
type ProjectRepo struct {
	clock clock.Clock
	db db.Querier
}

// NewProjectRepo creates a new project repository
func NewProjectRepo(database db.Querier, clk clock.Clock) *ProjectRepo {
	return &ProjectRepo{db: database, clock: clk}
}

// Create creates a new project
func (r *ProjectRepo) Create(project *model.Project) error {
	now := r.clock.Now().UTC().Format(time.RFC3339Nano)
	project.CreatedAt, _ = time.Parse(time.RFC3339Nano, now)
	project.UpdatedAt = project.CreatedAt

	_, err := r.db.Exec(`
		INSERT INTO projects (id, name, root_path, default_workflow, default_branch, use_git_worktrees, use_docker_isolation, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		strings.ToLower(project.ID),
		project.Name,
		project.RootPath,
		project.DefaultWorkflow,
		project.DefaultBranch,
		project.UseGitWorktrees,
		project.UseDockerIsolation,
		now,
		now,
	)
	return err
}

// Get retrieves a project by ID
func (r *ProjectRepo) Get(id string) (*model.Project, error) {
	project := &model.Project{}
	var createdAt, updatedAt string

	err := r.db.QueryRow(`
		SELECT id, name, root_path, default_workflow, default_branch, use_git_worktrees, use_docker_isolation, created_at, updated_at
		FROM projects WHERE LOWER(id) = LOWER(?)`, id).Scan(
		&project.ID,
		&project.Name,
		&project.RootPath,
		&project.DefaultWorkflow,
		&project.DefaultBranch,
		&project.UseGitWorktrees,
		&project.UseDockerIsolation,
		&createdAt,
		&updatedAt,
	)
	if err == sql.ErrNoRows {
		return nil, fmt.Errorf("project not found: %s", id)
	}
	if err != nil {
		return nil, err
	}

	project.CreatedAt, _ = time.Parse(time.RFC3339Nano, createdAt)
	project.UpdatedAt, _ = time.Parse(time.RFC3339Nano, updatedAt)

	return project, nil
}

// Exists checks if a project exists
func (r *ProjectRepo) Exists(id string) (bool, error) {
	var count int
	err := r.db.QueryRow(`SELECT COUNT(*) FROM projects WHERE LOWER(id) = LOWER(?)`, id).Scan(&count)
	if err != nil {
		return false, err
	}
	return count > 0, nil
}

// List retrieves all projects
func (r *ProjectRepo) List() ([]*model.Project, error) {
	rows, err := r.db.Query(`
		SELECT id, name, root_path, default_workflow, default_branch, use_git_worktrees, use_docker_isolation, created_at, updated_at
		FROM projects
		ORDER BY created_at DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var projects []*model.Project
	for rows.Next() {
		project := &model.Project{}
		var createdAt, updatedAt string

		err := rows.Scan(
			&project.ID,
			&project.Name,
			&project.RootPath,
			&project.DefaultWorkflow,
			&project.DefaultBranch,
			&project.UseGitWorktrees,
			&project.UseDockerIsolation,
			&createdAt,
			&updatedAt,
		)
		if err != nil {
			return nil, err
		}

		project.CreatedAt, _ = time.Parse(time.RFC3339Nano, createdAt)
		project.UpdatedAt, _ = time.Parse(time.RFC3339Nano, updatedAt)

		projects = append(projects, project)
	}

	return projects, nil
}

// ProjectUpdateFields contains fields that can be updated
type ProjectUpdateFields struct {
	Name            *string
	RootPath        *string
	DefaultBranch   *string
	UseGitWorktrees    *bool
	UseDockerIsolation *bool
}

// Update updates a project
func (r *ProjectRepo) Update(id string, fields *ProjectUpdateFields) error {
	// First check if project exists
	_, err := r.Get(id)
	if err != nil {
		return err
	}

	updates := []string{}
	args := []interface{}{}

	if fields.Name != nil {
		updates = append(updates, "name = ?")
		args = append(args, *fields.Name)
	}
	if fields.RootPath != nil {
		updates = append(updates, "root_path = ?")
		args = append(args, *fields.RootPath)
	}
	if fields.DefaultBranch != nil {
		updates = append(updates, "default_branch = ?")
		args = append(args, *fields.DefaultBranch)
	}
	if fields.UseGitWorktrees != nil {
		updates = append(updates, "use_git_worktrees = ?")
		args = append(args, *fields.UseGitWorktrees)
	}
	if fields.UseDockerIsolation != nil {
		updates = append(updates, "use_docker_isolation = ?")
		args = append(args, *fields.UseDockerIsolation)
	}

	if len(updates) == 0 {
		return nil
	}

	now := r.clock.Now().UTC().Format(time.RFC3339Nano)
	updates = append(updates, "updated_at = ?")
	args = append(args, now)
	args = append(args, id)

	query := "UPDATE projects SET "
	for i, u := range updates {
		if i > 0 {
			query += ", "
		}
		query += u
	}
	query += " WHERE LOWER(id) = LOWER(?)"

	_, err = r.db.Exec(query, args...)
	return err
}

// Delete deletes a project
func (r *ProjectRepo) Delete(id string) error {
	result, err := r.db.Exec("DELETE FROM projects WHERE LOWER(id) = LOWER(?)", id)
	if err != nil {
		return err
	}

	rowsAffected, _ := result.RowsAffected()
	if rowsAffected == 0 {
		return fmt.Errorf("project not found: %s", id)
	}
	return nil
}
