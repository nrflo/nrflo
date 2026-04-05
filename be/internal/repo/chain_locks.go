package repo

import (
	"database/sql"
	"fmt"
	"strings"

	"be/internal/db"
)

// ChainLockRepo handles chain execution lock operations
type ChainLockRepo struct {
	pool *db.Pool
}

// NewChainLockRepo creates a new chain lock repository
func NewChainLockRepo(pool *db.Pool) *ChainLockRepo {
	return &ChainLockRepo{pool: pool}
}

// InsertLocks inserts locks for a set of tickets in a chain
func (r *ChainLockRepo) InsertLocks(projectID, chainID string, ticketIDs []string) error {
	for _, tid := range ticketIDs {
		_, err := r.pool.Exec(`
			INSERT INTO chain_execution_locks (project_id, ticket_id, chain_id)
			VALUES (?, ?, ?)`,
			strings.ToLower(projectID), strings.ToLower(tid), chainID)
		if err != nil {
			if strings.Contains(err.Error(), "UNIQUE constraint") {
				return fmt.Errorf("ticket %s is already locked by another chain", tid)
			}
			return err
		}
	}
	return nil
}

// DeleteLocksByChain removes all locks for a chain
func (r *ChainLockRepo) DeleteLocksByChain(chainID string) error {
	_, err := r.pool.Exec(`DELETE FROM chain_execution_locks WHERE chain_id = ?`, chainID)
	return err
}

// DeleteLocksByTicketIDs removes locks for specific tickets in a chain
func (r *ChainLockRepo) DeleteLocksByTicketIDs(chainID string, ticketIDs []string) error {
	return r.DeleteLocksByTicketIDsTx(r.pool, chainID, ticketIDs)
}

// DeleteLocksByTicketIDsTx is the transactional variant of DeleteLocksByTicketIDs.
func (r *ChainLockRepo) DeleteLocksByTicketIDsTx(exec interface {
	Exec(query string, args ...interface{}) (sql.Result, error)
}, chainID string, ticketIDs []string) error {
	if len(ticketIDs) == 0 {
		return nil
	}

	placeholders := make([]string, len(ticketIDs))
	args := make([]interface{}, 0, len(ticketIDs)+1)
	args = append(args, chainID)
	for i, tid := range ticketIDs {
		placeholders[i] = "?"
		args = append(args, strings.ToLower(tid))
	}

	_, err := exec.Exec(`
		DELETE FROM chain_execution_locks
		WHERE chain_id = ? AND LOWER(ticket_id) IN (`+strings.Join(placeholders, ",")+`)`,
		args...)
	return err
}

// CheckConflicts returns ticket IDs that are already locked by other chains
func (r *ChainLockRepo) CheckConflicts(projectID string, ticketIDs []string, excludeChainID string) ([]string, error) {
	if len(ticketIDs) == 0 {
		return nil, nil
	}

	placeholders := make([]string, len(ticketIDs))
	args := make([]interface{}, 0, len(ticketIDs)+2)
	args = append(args, strings.ToLower(projectID))
	for i, tid := range ticketIDs {
		placeholders[i] = "?"
		args = append(args, strings.ToLower(tid))
	}
	args = append(args, excludeChainID)

	rows, err := r.pool.Query(`
		SELECT ticket_id FROM chain_execution_locks
		WHERE LOWER(project_id) = ? AND LOWER(ticket_id) IN (`+strings.Join(placeholders, ",")+`)
		AND chain_id != ?`,
		args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var conflicts []string
	for rows.Next() {
		var tid string
		if err := rows.Scan(&tid); err != nil {
			return nil, err
		}
		conflicts = append(conflicts, tid)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return conflicts, nil
}
