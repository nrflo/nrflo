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

// ChainRepo handles chain execution CRUD operations
type ChainRepo struct {
	clock clock.Clock
	pool *db.Pool
}

// NewChainRepo creates a new chain repository
func NewChainRepo(pool *db.Pool, clk clock.Clock) *ChainRepo {
	return &ChainRepo{pool: pool, clock: clk}
}

const chainCols = `id, project_id, name, status, workflow_name, epic_ticket_id, created_by, created_at, updated_at, started_at, completed_at`

func scanChain(scanner interface{ Scan(...interface{}) error }) (*model.ChainExecution, error) {
	c := &model.ChainExecution{}
	var createdAt, updatedAt string
	var epicTicketID, startedAtStr, completedAtStr sql.NullString
	err := scanner.Scan(
		&c.ID, &c.ProjectID, &c.Name, &c.Status,
		&c.WorkflowName, &epicTicketID, &c.CreatedBy,
		&createdAt, &updatedAt, &startedAtStr, &completedAtStr,
	)
	if err != nil {
		return nil, err
	}
	if epicTicketID.Valid {
		c.EpicTicketID = epicTicketID.String
	}
	c.CreatedAt, _ = time.Parse(time.RFC3339Nano, createdAt)
	c.UpdatedAt, _ = time.Parse(time.RFC3339Nano, updatedAt)
	if startedAtStr.Valid {
		t, _ := time.Parse(time.RFC3339Nano, startedAtStr.String)
		c.StartedAt = &t
	}
	if completedAtStr.Valid {
		t, _ := time.Parse(time.RFC3339Nano, completedAtStr.String)
		c.CompletedAt = &t
	}
	return c, nil
}

// Create creates a new chain execution
func (r *ChainRepo) Create(c *model.ChainExecution) error {
	now := r.clock.Now().UTC().Format(time.RFC3339Nano)
	c.CreatedAt, _ = time.Parse(time.RFC3339Nano, now)
	c.UpdatedAt = c.CreatedAt

	var epicTicketID interface{}
	if c.EpicTicketID != "" {
		epicTicketID = c.EpicTicketID
	}
	_, err := r.pool.Exec(`
		INSERT INTO chain_executions (`+chainCols+`)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		c.ID, strings.ToLower(c.ProjectID), c.Name, c.Status,
		c.WorkflowName, epicTicketID, c.CreatedBy,
		now, now, nil, nil,
	)
	return err
}

// Get retrieves a chain execution by ID
func (r *ChainRepo) Get(id string) (*model.ChainExecution, error) {
	row := r.pool.QueryRow(`SELECT `+chainCols+` FROM chain_executions WHERE id = ?`, id)
	c, err := scanChain(row)
	if err == sql.ErrNoRows {
		return nil, fmt.Errorf("chain not found: %s", id)
	}
	return c, err
}

const chainListCols = chainCols + `,
	COALESCE((SELECT COUNT(*) FROM chain_execution_items WHERE chain_id = chain_executions.id), 0),
	COALESCE((SELECT COUNT(*) FROM chain_execution_items WHERE chain_id = chain_executions.id AND status = 'completed'), 0)`

func scanChainWithCounts(scanner interface{ Scan(...interface{}) error }) (*model.ChainExecution, error) {
	c := &model.ChainExecution{}
	var createdAt, updatedAt string
	var epicTicketID, startedAtStr, completedAtStr sql.NullString
	err := scanner.Scan(
		&c.ID, &c.ProjectID, &c.Name, &c.Status,
		&c.WorkflowName, &epicTicketID, &c.CreatedBy,
		&createdAt, &updatedAt, &startedAtStr, &completedAtStr,
		&c.TotalItems, &c.CompletedItems,
	)
	if err != nil {
		return nil, err
	}
	if epicTicketID.Valid {
		c.EpicTicketID = epicTicketID.String
	}
	c.CreatedAt, _ = time.Parse(time.RFC3339Nano, createdAt)
	c.UpdatedAt, _ = time.Parse(time.RFC3339Nano, updatedAt)
	if startedAtStr.Valid {
		t, _ := time.Parse(time.RFC3339Nano, startedAtStr.String)
		c.StartedAt = &t
	}
	if completedAtStr.Valid {
		t, _ := time.Parse(time.RFC3339Nano, completedAtStr.String)
		c.CompletedAt = &t
	}
	return c, nil
}

// List lists chain executions for a project, optionally filtered by status and epic_ticket_id
func (r *ChainRepo) List(projectID, status, epicTicketID string) ([]*model.ChainExecution, error) {
	query := `SELECT ` + chainListCols + ` FROM chain_executions WHERE LOWER(project_id) = LOWER(?)`
	args := []interface{}{projectID}

	if status != "" {
		query += ` AND status = ?`
		args = append(args, status)
	}
	if epicTicketID != "" {
		query += ` AND epic_ticket_id = ?`
		args = append(args, epicTicketID)
	}

	query += ` ORDER BY created_at DESC`

	rows, err := r.pool.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var chains []*model.ChainExecution
	for rows.Next() {
		c, err := scanChainWithCounts(rows)
		if err != nil {
			return nil, err
		}
		chains = append(chains, c)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return chains, nil
}

// UpdateStatus updates the chain execution status
func (r *ChainRepo) UpdateStatus(id string, status model.ChainStatus) error {
	now := r.clock.Now().UTC().Format(time.RFC3339Nano)
	var result sql.Result
	var err error

	switch status {
	case model.ChainStatusRunning:
		result, err = r.pool.Exec(
			`UPDATE chain_executions SET status = ?, started_at = COALESCE(started_at, ?), updated_at = ? WHERE id = ?`,
			status, now, now, id)
	case model.ChainStatusCompleted, model.ChainStatusFailed, model.ChainStatusCanceled:
		result, err = r.pool.Exec(
			`UPDATE chain_executions SET status = ?, completed_at = ?, updated_at = ? WHERE id = ?`,
			status, now, now, id)
	default:
		result, err = r.pool.Exec(
			`UPDATE chain_executions SET status = ?, updated_at = ? WHERE id = ?`,
			status, now, id)
	}
	if err != nil {
		return err
	}
	return checkChainAffected(result, id)
}

// UpdateName updates the chain name (pending only)
func (r *ChainRepo) UpdateName(id, name string) error {
	now := r.clock.Now().UTC().Format(time.RFC3339Nano)
	result, err := r.pool.Exec(
		`UPDATE chain_executions SET name = ?, updated_at = ? WHERE id = ? AND status = 'pending'`,
		name, now, id)
	if err != nil {
		return err
	}
	return checkChainAffected(result, id)
}

// Delete deletes a chain execution
func (r *ChainRepo) Delete(id string) error {
	result, err := r.pool.Exec(`DELETE FROM chain_executions WHERE id = ?`, id)
	if err != nil {
		return err
	}
	return checkChainAffected(result, id)
}

func checkChainAffected(result sql.Result, id string) error {
	n, _ := result.RowsAffected()
	if n == 0 {
		return fmt.Errorf("chain not found: %s", id)
	}
	return nil
}
