package repo

import (
	"database/sql"
	"fmt"
	"strings"
	"time"

	"be/internal/db"
	"be/internal/model"
)

// ChainRepo handles chain execution CRUD operations
type ChainRepo struct {
	pool *db.Pool
}

// NewChainRepo creates a new chain repository
func NewChainRepo(pool *db.Pool) *ChainRepo {
	return &ChainRepo{pool: pool}
}

const chainCols = `id, project_id, name, status, workflow_name, epic_ticket_id, created_by, created_at, updated_at`

func scanChain(scanner interface{ Scan(...interface{}) error }) (*model.ChainExecution, error) {
	c := &model.ChainExecution{}
	var createdAt, updatedAt string
	var epicTicketID sql.NullString
	err := scanner.Scan(
		&c.ID, &c.ProjectID, &c.Name, &c.Status,
		&c.WorkflowName, &epicTicketID, &c.CreatedBy,
		&createdAt, &updatedAt,
	)
	if epicTicketID.Valid {
		c.EpicTicketID = epicTicketID.String
	}
	if err != nil {
		return nil, err
	}
	c.CreatedAt, _ = time.Parse(time.RFC3339, createdAt)
	c.UpdatedAt, _ = time.Parse(time.RFC3339, updatedAt)
	return c, nil
}

// Create creates a new chain execution
func (r *ChainRepo) Create(c *model.ChainExecution) error {
	now := time.Now().UTC().Format(time.RFC3339)
	c.CreatedAt, _ = time.Parse(time.RFC3339, now)
	c.UpdatedAt = c.CreatedAt

	var epicTicketID interface{}
	if c.EpicTicketID != "" {
		epicTicketID = c.EpicTicketID
	}
	_, err := r.pool.Exec(`
		INSERT INTO chain_executions (`+chainCols+`)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		c.ID, strings.ToLower(c.ProjectID), c.Name, c.Status,
		c.WorkflowName, epicTicketID, c.CreatedBy,
		now, now,
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
	var epicTicketID sql.NullString
	err := scanner.Scan(
		&c.ID, &c.ProjectID, &c.Name, &c.Status,
		&c.WorkflowName, &epicTicketID, &c.CreatedBy,
		&createdAt, &updatedAt,
		&c.TotalItems, &c.CompletedItems,
	)
	if epicTicketID.Valid {
		c.EpicTicketID = epicTicketID.String
	}
	if err != nil {
		return nil, err
	}
	c.CreatedAt, _ = time.Parse(time.RFC3339, createdAt)
	c.UpdatedAt, _ = time.Parse(time.RFC3339, updatedAt)
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
	now := time.Now().UTC().Format(time.RFC3339)
	result, err := r.pool.Exec(
		`UPDATE chain_executions SET status = ?, updated_at = ? WHERE id = ?`,
		status, now, id)
	if err != nil {
		return err
	}
	return checkChainAffected(result, id)
}

// UpdateName updates the chain name (pending only)
func (r *ChainRepo) UpdateName(id, name string) error {
	now := time.Now().UTC().Format(time.RFC3339)
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
