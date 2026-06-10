package service

import (
	"fmt"
	"strings"

	"be/internal/db"
	"be/internal/model"
	"be/internal/repo"
)

// RemoveFromChain removes pending items from a running chain.
// All requested ticket IDs must be pending — if any are not, no items are removed (atomic).
func (s *ChainService) RemoveFromChain(chainID string, ticketIDs []string) (*model.ChainExecution, error) {
	if len(ticketIDs) == 0 {
		return nil, fmt.Errorf("at least one ticket_id is required")
	}

	chainRepo := repo.NewChainRepo(s.pool, s.clock)
	chain, err := chainRepo.Get(chainID)
	if err != nil {
		return nil, err
	}
	if chain.Status != model.ChainStatusRunning {
		return nil, fmt.Errorf("can only remove items from running chains (current: %s)", chain.Status)
	}

	// Deduplicate ticket IDs (case-insensitive)
	seen := make(map[string]bool, len(ticketIDs))
	var deduped []string
	for _, tid := range ticketIDs {
		lower := strings.ToLower(tid)
		if !seen[lower] {
			seen[lower] = true
			deduped = append(deduped, tid)
		}
	}

	if err := db.WithBusyRetry(func() error {
		return s.removeFromChainOnce(chainID, deduped)
	}); err != nil {
		return nil, err
	}

	return s.GetChainWithItems(chainID)
}

func (s *ChainService) removeFromChainOnce(chainID string, deduped []string) error {
	tx, err := s.pool.Begin()
	if err != nil {
		return fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer tx.Rollback() //nolint:errcheck

	itemRepo := repo.NewChainItemRepo(s.pool, s.clock)
	deleted, err := itemRepo.DeletePendingByTicketIDsTx(tx, chainID, deduped)
	if err != nil {
		return fmt.Errorf("failed to delete pending items: %w", err)
	}

	if deleted < int64(len(deduped)) {
		return fmt.Errorf("could not remove %d of %d items (not pending or not in chain)", int64(len(deduped))-deleted, len(deduped))
	}

	lockRepo := repo.NewChainLockRepo(s.pool)
	if err := lockRepo.DeleteLocksByTicketIDsTx(tx, chainID, deduped); err != nil {
		return fmt.Errorf("failed to release locks: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("failed to commit removal: %w", err)
	}
	return nil
}
