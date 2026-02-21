package service

import (
	"fmt"
	"strings"

	"be/internal/types"
)

// validateSameSet checks that two slices contain exactly the same set of elements.
func validateSameSet(a, b []string) error {
	if len(a) != len(b) {
		return fmt.Errorf("ordered_ticket_ids has %d tickets but expected %d", len(a), len(b))
	}
	setA := make(map[string]bool, len(a))
	for _, id := range a {
		setA[strings.ToLower(id)] = true
	}
	setB := make(map[string]bool, len(b))
	for _, id := range b {
		setB[strings.ToLower(id)] = true
	}
	for id := range setA {
		if !setB[id] {
			return fmt.Errorf("ordered_ticket_ids contains unexpected ticket: %s", id)
		}
	}
	for id := range setB {
		if !setA[id] {
			return fmt.Errorf("ordered_ticket_ids is missing ticket: %s", id)
		}
	}
	return nil
}

// validateCustomOrder checks that the provided ordering respects all dependency constraints.
// Every blocker must appear before its dependent in the ordered list.
func validateCustomOrder(orderedIDs []string, deps map[string][]string) error {
	position := make(map[string]int, len(orderedIDs))
	for i, id := range orderedIDs {
		position[strings.ToLower(id)] = i
	}

	for ticket, blockers := range deps {
		ticketPos, ok := position[strings.ToLower(ticket)]
		if !ok {
			continue
		}
		for _, blocker := range blockers {
			blockerPos, ok := position[strings.ToLower(blocker)]
			if !ok {
				continue
			}
			if blockerPos >= ticketPos {
				return fmt.Errorf("invalid order: %s (blocker) must come before %s (dependent)", blocker, ticket)
			}
		}
	}
	return nil
}

// computeDeps calls expandWithBlockers and returns only the deps map.
func (s *ChainService) computeDeps(projectID string, ticketIDs []string) (map[string][]string, error) {
	_, deps, err := s.expandWithBlockers(projectID, ticketIDs)
	if err != nil {
		return nil, err
	}
	return deps, nil
}

// PreviewChain returns expanded tickets, dependency map, and auto-added tickets
// without persisting anything.
func (s *ChainService) PreviewChain(projectID string, req *types.ChainPreviewRequest) (*types.ChainPreviewResponse, error) {
	if len(req.TicketIDs) == 0 {
		return nil, fmt.Errorf("at least one ticket is required")
	}

	allTicketIDs, deps, err := s.expandWithBlockers(projectID, req.TicketIDs)
	if err != nil {
		return nil, fmt.Errorf("failed to expand dependencies: %w", err)
	}

	if err := detectCycles(allTicketIDs, deps); err != nil {
		return nil, err
	}

	sorted, err := s.topologicalSort(projectID, allTicketIDs, deps)
	if err != nil {
		return nil, fmt.Errorf("failed to sort tickets: %w", err)
	}

	// Compute added_by_deps: tickets in expanded set but not in original request
	originalSet := make(map[string]bool, len(req.TicketIDs))
	for _, tid := range req.TicketIDs {
		originalSet[strings.ToLower(tid)] = true
	}
	var addedByDeps []string
	for _, tid := range sorted {
		if !originalSet[tid] {
			addedByDeps = append(addedByDeps, tid)
		}
	}

	if addedByDeps == nil {
		addedByDeps = []string{}
	}
	if deps == nil {
		deps = make(map[string][]string)
	}

	return &types.ChainPreviewResponse{
		TicketIDs:   sorted,
		Deps:        deps,
		AddedByDeps: addedByDeps,
	}, nil
}
