package service

import (
	"fmt"
	"os/user"
	"strings"
	"time"
)

// AddDependency adds a dependency between tickets
func (s *TicketService) AddDependency(projectID, child, parent string) error {
	currentUser, err := user.Current()
	if err != nil {
		return fmt.Errorf("failed to get current user: %w", err)
	}

	now := s.clock.Now().UTC().Format(time.RFC3339Nano)

	_, err = s.pool.Exec(`
		INSERT INTO dependencies (project_id, issue_id, depends_on_id, type, created_at, created_by)
		VALUES (?, ?, ?, 'blocks', ?, ?)`,
		strings.ToLower(projectID),
		strings.ToLower(child),
		strings.ToLower(parent),
		now,
		currentUser.Username,
	)
	return err
}

// RemoveDependency removes a dependency between tickets
func (s *TicketService) RemoveDependency(projectID, child, parent string) error {
	result, err := s.pool.Exec(
		"DELETE FROM dependencies WHERE LOWER(project_id) = LOWER(?) AND LOWER(issue_id) = LOWER(?) AND LOWER(depends_on_id) = LOWER(?)",
		projectID, child, parent)
	if err != nil {
		return err
	}

	rowsAffected, _ := result.RowsAffected()
	if rowsAffected == 0 {
		return fmt.Errorf("dependency not found")
	}
	return nil
}
