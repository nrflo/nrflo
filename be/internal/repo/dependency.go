package repo

import (
	"fmt"
	"strings"
	"time"

	"be/internal/clock"
	"be/internal/db"
	"be/internal/model"
)

// DependencyRepo handles dependency CRUD operations
type DependencyRepo struct {
	clock clock.Clock
	db db.Querier
}

// NewDependencyRepo creates a new dependency repository
func NewDependencyRepo(database db.Querier, clk clock.Clock) *DependencyRepo {
	return &DependencyRepo{db: database, clock: clk}
}

// Create adds a dependency (child depends on parent)
func (r *DependencyRepo) Create(dep *model.Dependency) error {
	// Check if both tickets exist in the project
	var exists int
	err := r.db.QueryRow("SELECT COUNT(*) FROM tickets WHERE LOWER(project_id) = LOWER(?) AND LOWER(id) = LOWER(?)",
		dep.ProjectID, dep.IssueID).Scan(&exists)
	if err != nil {
		return err
	}
	if exists == 0 {
		return fmt.Errorf("ticket not found: %s", dep.IssueID)
	}

	err = r.db.QueryRow("SELECT COUNT(*) FROM tickets WHERE LOWER(project_id) = LOWER(?) AND LOWER(id) = LOWER(?)",
		dep.ProjectID, dep.DependsOnID).Scan(&exists)
	if err != nil {
		return err
	}
	if exists == 0 {
		return fmt.Errorf("ticket not found: %s", dep.DependsOnID)
	}

	now := r.clock.Now().UTC().Format(time.RFC3339Nano)
	dep.CreatedAt, _ = time.Parse(time.RFC3339Nano, now)

	_, err = r.db.Exec(`
		INSERT INTO dependencies (project_id, issue_id, depends_on_id, type, created_at, created_by)
		VALUES (?, ?, ?, ?, ?, ?)`,
		strings.ToLower(dep.ProjectID),
		strings.ToLower(dep.IssueID),
		strings.ToLower(dep.DependsOnID),
		dep.Type,
		now,
		dep.CreatedBy)
	return err
}

// Delete removes a dependency
func (r *DependencyRepo) Delete(projectID, childID, parentID string) error {
	result, err := r.db.Exec(`
		DELETE FROM dependencies
		WHERE LOWER(project_id) = LOWER(?) AND LOWER(issue_id) = LOWER(?) AND LOWER(depends_on_id) = LOWER(?)`,
		projectID, childID, parentID)
	if err != nil {
		return err
	}

	rowsAffected, _ := result.RowsAffected()
	if rowsAffected == 0 {
		return fmt.Errorf("dependency not found")
	}
	return nil
}

// GetBlockers returns all tickets that block the given ticket
func (r *DependencyRepo) GetBlockers(projectID, ticketID string) ([]*model.Dependency, error) {
	rows, err := r.db.Query(`
		SELECT d.project_id, d.issue_id, d.depends_on_id, d.type, d.created_at, d.created_by,
		       COALESCE(t.title, '') AS depends_on_title
		FROM dependencies d
		LEFT JOIN tickets t ON LOWER(d.depends_on_id) = LOWER(t.id) AND LOWER(d.project_id) = LOWER(t.project_id)
		WHERE LOWER(d.project_id) = LOWER(?) AND LOWER(d.issue_id) = LOWER(?)`, projectID, ticketID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var deps []*model.Dependency
	for rows.Next() {
		dep := &model.Dependency{}
		var createdAt string
		err := rows.Scan(&dep.ProjectID, &dep.IssueID, &dep.DependsOnID, &dep.Type, &createdAt, &dep.CreatedBy, &dep.DependsOnTitle)
		if err != nil {
			return nil, err
		}
		dep.CreatedAt, _ = time.Parse(time.RFC3339Nano, createdAt)
		deps = append(deps, dep)
	}
	return deps, nil
}

// GetBlocked returns all tickets blocked by the given ticket
func (r *DependencyRepo) GetBlocked(projectID, ticketID string) ([]*model.Dependency, error) {
	rows, err := r.db.Query(`
		SELECT d.project_id, d.issue_id, d.depends_on_id, d.type, d.created_at, d.created_by,
		       COALESCE(t.title, '') AS issue_title
		FROM dependencies d
		LEFT JOIN tickets t ON LOWER(d.issue_id) = LOWER(t.id) AND LOWER(d.project_id) = LOWER(t.project_id)
		WHERE LOWER(d.project_id) = LOWER(?) AND LOWER(d.depends_on_id) = LOWER(?)`, projectID, ticketID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var deps []*model.Dependency
	for rows.Next() {
		dep := &model.Dependency{}
		var createdAt string
		err := rows.Scan(&dep.ProjectID, &dep.IssueID, &dep.DependsOnID, &dep.Type, &createdAt, &dep.CreatedBy, &dep.IssueTitle)
		if err != nil {
			return nil, err
		}
		dep.CreatedAt, _ = time.Parse(time.RFC3339Nano, createdAt)
		deps = append(deps, dep)
	}
	return deps, nil
}
