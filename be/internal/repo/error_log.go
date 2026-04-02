package repo

import (
	"fmt"
	"time"

	"be/internal/clock"
	"be/internal/db"
	"be/internal/model"
)

// ErrorLogRepo handles error log CRUD operations
type ErrorLogRepo struct {
	clock clock.Clock
	db    db.Querier
}

// NewErrorLogRepo creates a new error log repository
func NewErrorLogRepo(database db.Querier, clk clock.Clock) *ErrorLogRepo {
	return &ErrorLogRepo{db: database, clock: clk}
}

const errorLogCols = `id, project_id, error_type, instance_id, message, created_at`

func scanErrorLog(scanner interface{ Scan(...interface{}) error }) (*model.ErrorLog, error) {
	e := &model.ErrorLog{}
	err := scanner.Scan(&e.ID, &e.ProjectID, &e.ErrorType, &e.InstanceID, &e.Message, &e.CreatedAt)
	if err != nil {
		return nil, err
	}
	return e, nil
}

// Insert inserts a new error log record.
func (r *ErrorLogRepo) Insert(e *model.ErrorLog) error {
	if e.CreatedAt == "" {
		e.CreatedAt = r.clock.Now().UTC().Format(time.RFC3339Nano)
	}
	_, err := r.db.Exec(
		`INSERT INTO errors (`+errorLogCols+`) VALUES (?, ?, ?, ?, ?, ?)`,
		e.ID, e.ProjectID, e.ErrorType, e.InstanceID, e.Message, e.CreatedAt,
	)
	return err
}

// List returns paginated error logs for a project, optionally filtered by type.
// Results are ordered by created_at DESC.
func (r *ErrorLogRepo) List(projectID string, errorType string, limit, offset int) ([]*model.ErrorLog, error) {
	query := `SELECT ` + errorLogCols + ` FROM errors WHERE project_id = ?`
	args := []interface{}{projectID}

	if errorType != "" {
		query += ` AND error_type = ?`
		args = append(args, errorType)
	}

	query += ` ORDER BY created_at DESC LIMIT ? OFFSET ?`
	args = append(args, limit, offset)

	rows, err := r.db.Query(query, args...)
	if err != nil {
		return nil, fmt.Errorf("list errors: %w", err)
	}
	defer rows.Close()

	var result []*model.ErrorLog
	for rows.Next() {
		e, err := scanErrorLog(rows)
		if err != nil {
			return nil, fmt.Errorf("scan error log: %w", err)
		}
		result = append(result, e)
	}
	return result, rows.Err()
}

// Count returns the total number of errors for a project, optionally filtered by type.
func (r *ErrorLogRepo) Count(projectID string, errorType string) (int, error) {
	query := `SELECT COUNT(*) FROM errors WHERE project_id = ?`
	args := []interface{}{projectID}

	if errorType != "" {
		query += ` AND error_type = ?`
		args = append(args, errorType)
	}

	var count int
	err := r.db.QueryRow(query, args...).Scan(&count)
	return count, err
}
