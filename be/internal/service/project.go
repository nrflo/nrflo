package service

import (
	"database/sql"
	"fmt"
	"strings"
	"time"

	"be/internal/db"
	"be/internal/model"
	"be/internal/types"
)

// ProjectService handles project business logic
type ProjectService struct {
	pool *db.Pool
}

// NewProjectService creates a new project service
func NewProjectService(pool *db.Pool) *ProjectService {
	return &ProjectService{pool: pool}
}

// Create creates a new project
func (s *ProjectService) Create(projectID string, req *types.ProjectCreateRequest) (*model.Project, error) {
	projectID = strings.ToLower(projectID)

	// Check if project already exists
	var exists bool
	err := s.pool.QueryRow("SELECT EXISTS(SELECT 1 FROM projects WHERE LOWER(id) = LOWER(?))", projectID).Scan(&exists)
	if err != nil {
		return nil, fmt.Errorf("failed to check project: %w", err)
	}
	if exists {
		return nil, fmt.Errorf("project already exists: %s", projectID)
	}

	now := time.Now().UTC().Format(time.RFC3339)

	name := req.Name
	if name == "" {
		name = projectID
	}

	project := &model.Project{
		ID:   projectID,
		Name: name,
	}

	if req.RootPath != "" {
		project.RootPath = sql.NullString{String: req.RootPath, Valid: true}
	}
	if req.DefaultWorkflow != "" {
		project.DefaultWorkflow = sql.NullString{String: req.DefaultWorkflow, Valid: true}
	}
	if req.DefaultBranch != "" {
		project.DefaultBranch = sql.NullString{String: req.DefaultBranch, Valid: true}
	}

	_, err = s.pool.Exec(`
		INSERT INTO projects (id, name, root_path, default_workflow, default_branch, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?)`,
		projectID,
		project.Name,
		project.RootPath,
		project.DefaultWorkflow,
		project.DefaultBranch,
		now,
		now,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create project: %w", err)
	}

	project.CreatedAt, _ = time.Parse(time.RFC3339, now)
	project.UpdatedAt = project.CreatedAt

	return project, nil
}

// Get retrieves a project by ID
func (s *ProjectService) Get(projectID string) (*model.Project, error) {
	project := &model.Project{}
	var createdAt, updatedAt string

	err := s.pool.QueryRow(`
		SELECT id, name, root_path, default_workflow, default_branch, created_at, updated_at
		FROM projects WHERE LOWER(id) = LOWER(?)`, projectID).Scan(
		&project.ID,
		&project.Name,
		&project.RootPath,
		&project.DefaultWorkflow,
		&project.DefaultBranch,
		&createdAt,
		&updatedAt,
	)
	if err == sql.ErrNoRows {
		return nil, fmt.Errorf("project not found: %s", projectID)
	}
	if err != nil {
		return nil, err
	}

	project.CreatedAt, _ = time.Parse(time.RFC3339, createdAt)
	project.UpdatedAt, _ = time.Parse(time.RFC3339, updatedAt)

	return project, nil
}

// List lists all projects
func (s *ProjectService) List() ([]*model.Project, error) {
	rows, err := s.pool.Query(`
		SELECT id, name, root_path, default_workflow, default_branch, created_at, updated_at
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
			&createdAt,
			&updatedAt,
		)
		if err != nil {
			return nil, err
		}

		project.CreatedAt, _ = time.Parse(time.RFC3339, createdAt)
		project.UpdatedAt, _ = time.Parse(time.RFC3339, updatedAt)

		projects = append(projects, project)
	}

	return projects, nil
}

// Delete deletes a project
func (s *ProjectService) Delete(projectID string) error {
	result, err := s.pool.Exec("DELETE FROM projects WHERE LOWER(id) = LOWER(?)", projectID)
	if err != nil {
		return err
	}

	rowsAffected, _ := result.RowsAffected()
	if rowsAffected == 0 {
		return fmt.Errorf("project not found: %s", projectID)
	}
	return nil
}
