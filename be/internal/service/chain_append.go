package service

import (
	"fmt"
	"strings"

	"github.com/google/uuid"

	"be/internal/model"
	"be/internal/repo"
	"be/internal/types"
)

// AppendToChain appends new tickets to a running chain.
// New tickets are expanded with transitive blockers, deduplicated against
// existing chain items, checked for lock conflicts, cycle-checked against
// the combined graph, topologically sorted, and inserted after the current
// max position.
func (s *ChainService) AppendToChain(chainID string, req *types.ChainAppendRequest) (*model.ChainExecution, error) {
	chainRepo := repo.NewChainRepo(s.pool, s.clock)
	chain, err := chainRepo.Get(chainID)
	if err != nil {
		return nil, err
	}
	if chain.Status != model.ChainStatusRunning {
		return nil, fmt.Errorf("can only append to running chains (current: %s)", chain.Status)
	}
	if len(req.TicketIDs) == 0 {
		return nil, fmt.Errorf("at least one ticket_id is required")
	}

	itemRepo := repo.NewChainItemRepo(s.pool, s.clock)

	// Get existing ticket IDs in the chain
	existingIDs, err := itemRepo.GetTicketIDsByChain(chainID)
	if err != nil {
		return nil, fmt.Errorf("failed to get existing chain items: %w", err)
	}
	existingSet := make(map[string]bool, len(existingIDs))
	for _, id := range existingIDs {
		existingSet[strings.ToLower(id)] = true
	}

	// Filter out tickets already in the chain
	var newTicketIDs []string
	for _, tid := range req.TicketIDs {
		if !existingSet[strings.ToLower(tid)] {
			newTicketIDs = append(newTicketIDs, tid)
		}
	}
	if len(newTicketIDs) == 0 {
		// All tickets already in chain — return as-is
		return s.GetChainWithItems(chainID)
	}

	// Expand new tickets with transitive blockers
	expandedIDs, newDeps, err := s.expandWithBlockers(chain.ProjectID, newTicketIDs)
	if err != nil {
		return nil, fmt.Errorf("failed to expand dependencies: %w", err)
	}

	// Remove expanded tickets that are already in the chain
	var filteredIDs []string
	filteredDeps := make(map[string][]string)
	for _, tid := range expandedIDs {
		if !existingSet[strings.ToLower(tid)] {
			filteredIDs = append(filteredIDs, tid)
			if blockers, ok := newDeps[tid]; ok {
				// Only keep deps on other new tickets (existing ones are already satisfied)
				var newBlockers []string
				for _, b := range blockers {
					if !existingSet[strings.ToLower(b)] {
						newBlockers = append(newBlockers, b)
					}
				}
				if len(newBlockers) > 0 {
					filteredDeps[tid] = newBlockers
				}
			}
		}
	}
	if len(filteredIDs) == 0 {
		return s.GetChainWithItems(chainID)
	}

	// Detect cycles among new tickets
	if err := detectCycles(filteredIDs, filteredDeps); err != nil {
		return nil, err
	}

	// Topological sort only the new tickets
	sorted, err := s.topologicalSort(chain.ProjectID, filteredIDs, filteredDeps)
	if err != nil {
		return nil, fmt.Errorf("failed to sort tickets: %w", err)
	}

	// Check lock conflicts (exclude self chain)
	lockRepo := repo.NewChainLockRepo(s.pool)
	conflicts, err := lockRepo.CheckConflicts(chain.ProjectID, sorted, chainID)
	if err != nil {
		return nil, fmt.Errorf("failed to check lock conflicts: %w", err)
	}
	if len(conflicts) > 0 {
		return nil, fmt.Errorf("tickets already locked by another chain: %s", strings.Join(conflicts, ", "))
	}

	// Get max position
	maxPos, err := itemRepo.GetMaxPosition(chainID)
	if err != nil {
		return nil, fmt.Errorf("failed to get max position: %w", err)
	}

	// Create new items starting after current max position
	items := make([]*model.ChainExecutionItem, len(sorted))
	for i, tid := range sorted {
		items[i] = &model.ChainExecutionItem{
			ID:       uuid.New().String(),
			ChainID:  chainID,
			TicketID: tid,
			Position: maxPos + 1 + i,
			Status:   model.ChainItemPending,
		}
	}
	if err := itemRepo.BatchInsert(items); err != nil {
		return nil, fmt.Errorf("failed to insert appended items: %w", err)
	}

	// Insert locks for new tickets
	if err := lockRepo.InsertLocks(chain.ProjectID, chainID, sorted); err != nil {
		return nil, fmt.Errorf("failed to acquire locks: %w", err)
	}

	return s.GetChainWithItems(chainID)
}
