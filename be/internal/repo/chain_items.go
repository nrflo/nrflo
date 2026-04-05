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

// ChainItemRepo handles chain execution item operations
type ChainItemRepo struct {
	clock clock.Clock
	pool *db.Pool
}

// NewChainItemRepo creates a new chain item repository
func NewChainItemRepo(pool *db.Pool, clk clock.Clock) *ChainItemRepo {
	return &ChainItemRepo{pool: pool, clock: clk}
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

// ListByChain returns all items for a chain, ordered by position.
// Joins tickets table to include ticket title.
func (r *ChainItemRepo) ListByChain(chainID string) ([]*model.ChainExecutionItem, error) {
	rows, err := r.pool.Query(`
		SELECT ci.id, ci.chain_id, ci.ticket_id, ci.position, ci.status,
			ci.workflow_instance_id, ci.started_at, ci.ended_at,
			COALESCE(t.title, '') AS ticket_title,
			COALESCE(tok.total_tokens, 0) AS total_tokens_used
		FROM chain_execution_items ci
		LEFT JOIN chain_executions ce ON ce.id = ci.chain_id
		LEFT JOIN tickets t ON LOWER(t.id) = LOWER(ci.ticket_id) AND LOWER(t.project_id) = LOWER(ce.project_id)
		LEFT JOIN (
			SELECT workflow_instance_id, SUM(200000 * (100 - context_left) / 100) AS total_tokens
			FROM agent_sessions
			WHERE status NOT IN ('running', 'continued') AND context_left IS NOT NULL
			GROUP BY workflow_instance_id
		) tok ON tok.workflow_instance_id = ci.workflow_instance_id
		WHERE ci.chain_id = ?
		ORDER BY ci.position ASC`, chainID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var items []*model.ChainExecutionItem
	for rows.Next() {
		item := &model.ChainExecutionItem{}
		err := rows.Scan(
			&item.ID, &item.ChainID, &item.TicketID, &item.Position,
			&item.Status, &item.WorkflowInstanceID,
			&item.StartedAt, &item.EndedAt,
			&item.TicketTitle,
			&item.TotalTokensUsed,
		)
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
	now := r.clock.Now().UTC().Format(time.RFC3339Nano)
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

// GetMaxPosition returns the maximum position in a chain, or -1 if no items exist.
func (r *ChainItemRepo) GetMaxPosition(chainID string) (int, error) {
	var maxPos int
	err := r.pool.QueryRow(
		`SELECT COALESCE(MAX(position), -1) FROM chain_execution_items WHERE chain_id = ?`,
		chainID).Scan(&maxPos)
	return maxPos, err
}

// GetTicketIDsByChain returns all ticket IDs for a chain.
func (r *ChainItemRepo) GetTicketIDsByChain(chainID string) ([]string, error) {
	rows, err := r.pool.Query(
		`SELECT ticket_id FROM chain_execution_items WHERE chain_id = ?`, chainID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var ids []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	return ids, rows.Err()
}

// DeletePendingByTicketIDs deletes pending items matching the given ticket IDs.
// Returns the number of rows actually deleted.
func (r *ChainItemRepo) DeletePendingByTicketIDs(chainID string, ticketIDs []string) (int64, error) {
	return r.DeletePendingByTicketIDsTx(r.pool, chainID, ticketIDs)
}

// DeletePendingByTicketIDsTx is the transactional variant of DeletePendingByTicketIDs.
func (r *ChainItemRepo) DeletePendingByTicketIDsTx(exec interface {
	Exec(query string, args ...interface{}) (sql.Result, error)
}, chainID string, ticketIDs []string) (int64, error) {
	if len(ticketIDs) == 0 {
		return 0, nil
	}

	placeholders := make([]string, len(ticketIDs))
	args := make([]interface{}, 0, len(ticketIDs)+1)
	args = append(args, chainID)
	for i, tid := range ticketIDs {
		placeholders[i] = "?"
		args = append(args, strings.ToLower(tid))
	}

	result, err := exec.Exec(`
		DELETE FROM chain_execution_items
		WHERE chain_id = ? AND LOWER(ticket_id) IN (`+strings.Join(placeholders, ",")+`) AND status = 'pending'`,
		args...)
	if err != nil {
		return 0, err
	}
	return result.RowsAffected()
}

// DeleteByChain deletes all items for a chain
func (r *ChainItemRepo) DeleteByChain(chainID string) error {
	_, err := r.pool.Exec(`DELETE FROM chain_execution_items WHERE chain_id = ?`, chainID)
	return err
}
