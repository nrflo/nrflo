package repo

import (
	"database/sql"
	"fmt"
	"time"

	"be/internal/db"
	"be/internal/model"
)

// ChainItemRepo handles chain execution item operations
type ChainItemRepo struct {
	pool *db.Pool
}

// NewChainItemRepo creates a new chain item repository
func NewChainItemRepo(pool *db.Pool) *ChainItemRepo {
	return &ChainItemRepo{pool: pool}
}

const chainItemCols = `id, chain_id, ticket_id, position, status, workflow_instance_id, started_at, ended_at`

func scanChainItem(scanner interface{ Scan(...interface{}) error }) (*model.ChainExecutionItem, error) {
	item := &model.ChainExecutionItem{}
	err := scanner.Scan(
		&item.ID, &item.ChainID, &item.TicketID, &item.Position,
		&item.Status, &item.WorkflowInstanceID,
		&item.StartedAt, &item.EndedAt,
	)
	if err != nil {
		return nil, err
	}
	return item, nil
}

// BatchInsert inserts multiple chain items
func (r *ChainItemRepo) BatchInsert(items []*model.ChainExecutionItem) error {
	for _, item := range items {
		_, err := r.pool.Exec(`
			INSERT INTO chain_execution_items (`+chainItemCols+`)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
			item.ID, item.ChainID, item.TicketID, item.Position,
			item.Status, item.WorkflowInstanceID,
			item.StartedAt, item.EndedAt,
		)
		if err != nil {
			return fmt.Errorf("failed to insert item %s: %w", item.TicketID, err)
		}
	}
	return nil
}

// ListByChain returns all items for a chain, ordered by position
func (r *ChainItemRepo) ListByChain(chainID string) ([]*model.ChainExecutionItem, error) {
	rows, err := r.pool.Query(`
		SELECT `+chainItemCols+` FROM chain_execution_items
		WHERE chain_id = ?
		ORDER BY position ASC`, chainID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var items []*model.ChainExecutionItem
	for rows.Next() {
		item, err := scanChainItem(rows)
		if err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return items, nil
}

// UpdateItemStatus updates the status of a chain item
func (r *ChainItemRepo) UpdateItemStatus(id string, status model.ChainItemStatus) error {
	now := time.Now().UTC().Format(time.RFC3339)
	var result sql.Result
	var err error

	switch status {
	case model.ChainItemRunning:
		result, err = r.pool.Exec(
			`UPDATE chain_execution_items SET status = ?, started_at = ? WHERE id = ?`,
			status, now, id)
	case model.ChainItemCompleted, model.ChainItemFailed, model.ChainItemSkipped, model.ChainItemCanceled:
		result, err = r.pool.Exec(
			`UPDATE chain_execution_items SET status = ?, ended_at = ? WHERE id = ?`,
			status, now, id)
	default:
		result, err = r.pool.Exec(
			`UPDATE chain_execution_items SET status = ? WHERE id = ?`,
			status, id)
	}
	if err != nil {
		return err
	}
	n, _ := result.RowsAffected()
	if n == 0 {
		return fmt.Errorf("chain item not found: %s", id)
	}
	return nil
}

// SetWorkflowInstanceID sets the workflow instance ID for a chain item
func (r *ChainItemRepo) SetWorkflowInstanceID(itemID, wfiID string) error {
	result, err := r.pool.Exec(
		`UPDATE chain_execution_items SET workflow_instance_id = ? WHERE id = ?`,
		wfiID, itemID)
	if err != nil {
		return err
	}
	n, _ := result.RowsAffected()
	if n == 0 {
		return fmt.Errorf("chain item not found: %s", itemID)
	}
	return nil
}

// GetNextPending returns the next pending item in a chain
func (r *ChainItemRepo) GetNextPending(chainID string) (*model.ChainExecutionItem, error) {
	row := r.pool.QueryRow(`
		SELECT `+chainItemCols+` FROM chain_execution_items
		WHERE chain_id = ? AND status = 'pending'
		ORDER BY position ASC LIMIT 1`, chainID)
	item, err := scanChainItem(row)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	return item, err
}

// DeleteByChain deletes all items for a chain
func (r *ChainItemRepo) DeleteByChain(chainID string) error {
	_, err := r.pool.Exec(`DELETE FROM chain_execution_items WHERE chain_id = ?`, chainID)
	return err
}
