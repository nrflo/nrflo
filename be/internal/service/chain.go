package service

import (
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/google/uuid"

	"be/internal/clock"
	"be/internal/db"
	"be/internal/model"
	"be/internal/repo"
	"be/internal/types"
)

// ChainService handles chain execution business logic
type ChainService struct {
	clock clock.Clock
	pool  *db.Pool
}

// NewChainService creates a new chain service
func NewChainService(pool *db.Pool, clk clock.Clock) *ChainService {
	return &ChainService{pool: pool, clock: clk}
}

// CreateChain builds and persists a chain from selected tickets.
// It expands transitive blockers, detects cycles, topologically sorts,
// validates lock conflicts, and persists chain+items+locks atomically.
func (s *ChainService) CreateChain(projectID string, req *types.ChainCreateRequest) (*model.ChainExecution, error) {
	if req.Name == "" {
		return nil, fmt.Errorf("chain name is required")
	}
	if req.WorkflowName == "" {
		return nil, fmt.Errorf("workflow name is required")
	}
	if len(req.TicketIDs) == 0 {
		return nil, fmt.Errorf("at least one ticket is required")
	}

	// Expand with transitive blockers
	allTicketIDs, deps, err := s.expandWithBlockers(projectID, req.TicketIDs)
	if err != nil {
		return nil, fmt.Errorf("failed to expand dependencies: %w", err)
	}

	// Detect cycles
	if err := detectCycles(allTicketIDs, deps); err != nil {
		return nil, err
	}

	// Topological sort with deterministic tie-break
	sorted, err := s.topologicalSort(projectID, allTicketIDs, deps)
	if err != nil {
		return nil, fmt.Errorf("failed to sort tickets: %w", err)
	}

	// If custom ordering provided, validate and use it
	if len(req.OrderedTicketIDs) > 0 {
		normalized := make([]string, len(req.OrderedTicketIDs))
		for i, id := range req.OrderedTicketIDs {
			normalized[i] = strings.ToLower(id)
		}
		// Validate same ticket set
		if err := validateSameSet(normalized, sorted); err != nil {
			return nil, err
		}
		// Validate dependency constraints
		if err := validateCustomOrder(normalized, deps); err != nil {
			return nil, err
		}
		sorted = normalized
	}

	// Check lock conflicts
	lockRepo := repo.NewChainLockRepo(s.pool)
	conflicts, err := lockRepo.CheckConflicts(projectID, sorted, "")
	if err != nil {
		return nil, fmt.Errorf("failed to check lock conflicts: %w", err)
	}
	if len(conflicts) > 0 {
		return nil, fmt.Errorf("tickets already locked by another chain: %s", strings.Join(conflicts, ", "))
	}

	// Create chain
	chainID := uuid.New().String()
	chain := &model.ChainExecution{
		ID:           chainID,
		ProjectID:    projectID,
		Name:         req.Name,
		Status:       model.ChainStatusPending,
		WorkflowName: req.WorkflowName,
		EpicTicketID: req.EpicTicketID,
		Deps:         deps,
	}

	chainRepo := repo.NewChainRepo(s.pool, s.clock)
	if err := chainRepo.Create(chain); err != nil {
		return nil, fmt.Errorf("failed to create chain: %w", err)
	}

	// Create items
	items := make([]*model.ChainExecutionItem, len(sorted))
	for i, tid := range sorted {
		items[i] = &model.ChainExecutionItem{
			ID:       uuid.New().String(),
			ChainID:  chainID,
			TicketID: tid,
			Position: i,
			Status:   model.ChainItemPending,
		}
	}
	itemRepo := repo.NewChainItemRepo(s.pool, s.clock)
	if err := itemRepo.BatchInsert(items); err != nil {
		chainRepo.Delete(chainID) // best-effort cleanup
		return nil, fmt.Errorf("failed to create chain items: %w", err)
	}

	// Insert locks
	if err := lockRepo.InsertLocks(projectID, chainID, sorted); err != nil {
		chainRepo.Delete(chainID) // cascades to items
		return nil, fmt.Errorf("failed to acquire locks: %w", err)
	}

	// Reload items with ticket titles from JOIN
	chain.Items, err = itemRepo.ListByChain(chainID)
	if err != nil {
		return nil, fmt.Errorf("failed to load chain items: %w", err)
	}
	return chain, nil
}

// UpdateChain updates a pending chain's tickets and/or name.
func (s *ChainService) UpdateChain(chainID string, req *types.ChainUpdateRequest) (*model.ChainExecution, error) {
	chainRepo := repo.NewChainRepo(s.pool, s.clock)
	chain, err := chainRepo.Get(chainID)
	if err != nil {
		return nil, err
	}
	if chain.Status != model.ChainStatusPending {
		return nil, fmt.Errorf("can only edit pending chains (current: %s)", chain.Status)
	}

	if req.Name != nil {
		if err := chainRepo.UpdateName(chainID, *req.Name); err != nil {
			return nil, err
		}
	}

	if len(req.TicketIDs) > 0 {
		// Re-expand, re-sort, re-lock
		allTicketIDs, deps, err := s.expandWithBlockers(chain.ProjectID, req.TicketIDs)
		if err != nil {
			return nil, fmt.Errorf("failed to expand dependencies: %w", err)
		}
		if err := detectCycles(allTicketIDs, deps); err != nil {
			return nil, err
		}
		sorted, err := s.topologicalSort(chain.ProjectID, allTicketIDs, deps)
		if err != nil {
			return nil, err
		}

		// If custom ordering provided, validate and use it
		if len(req.OrderedTicketIDs) > 0 {
			normalized := make([]string, len(req.OrderedTicketIDs))
			for i, id := range req.OrderedTicketIDs {
				normalized[i] = strings.ToLower(id)
			}
			if err := validateSameSet(normalized, sorted); err != nil {
				return nil, err
			}
			if err := validateCustomOrder(normalized, deps); err != nil {
				return nil, err
			}
			sorted = normalized
		}

		lockRepo := repo.NewChainLockRepo(s.pool)
		conflicts, err := lockRepo.CheckConflicts(chain.ProjectID, sorted, chainID)
		if err != nil {
			return nil, err
		}
		if len(conflicts) > 0 {
			return nil, fmt.Errorf("tickets already locked by another chain: %s", strings.Join(conflicts, ", "))
		}

		// Delete old items and locks, insert new ones
		itemRepo := repo.NewChainItemRepo(s.pool, s.clock)
		if err := itemRepo.DeleteByChain(chainID); err != nil {
			return nil, err
		}
		if err := lockRepo.DeleteLocksByChain(chainID); err != nil {
			return nil, err
		}

		items := make([]*model.ChainExecutionItem, len(sorted))
		for i, tid := range sorted {
			items[i] = &model.ChainExecutionItem{
				ID:       uuid.New().String(),
				ChainID:  chainID,
				TicketID: tid,
				Position: i,
				Status:   model.ChainItemPending,
			}
		}
		if err := itemRepo.BatchInsert(items); err != nil {
			return nil, err
		}
		if err := lockRepo.InsertLocks(chain.ProjectID, chainID, sorted); err != nil {
			return nil, err
		}
	}

	chain, err = chainRepo.Get(chainID)
	if err != nil {
		return nil, err
	}
	itemRepo := repo.NewChainItemRepo(s.pool, s.clock)
	chain.Items, err = itemRepo.ListByChain(chainID)
	if err != nil {
		return nil, err
	}

	// Populate deps for response
	ticketIDs := make([]string, len(chain.Items))
	for i, item := range chain.Items {
		ticketIDs[i] = item.TicketID
	}
	chain.Deps, _ = s.computeDeps(chain.ProjectID, ticketIDs)

	return chain, nil
}

// GetChainWithItems returns a chain with its items loaded
func (s *ChainService) GetChainWithItems(chainID string) (*model.ChainExecution, error) {
	chainRepo := repo.NewChainRepo(s.pool, s.clock)
	chain, err := chainRepo.Get(chainID)
	if err != nil {
		return nil, err
	}

	itemRepo := repo.NewChainItemRepo(s.pool, s.clock)
	items, err := itemRepo.ListByChain(chainID)
	if err != nil {
		return nil, err
	}
	chain.Items = items

	// Populate deps for response
	ticketIDs := make([]string, len(items))
	for i, item := range items {
		ticketIDs[i] = item.TicketID
	}
	chain.Deps, _ = s.computeDeps(chain.ProjectID, ticketIDs)

	return chain, nil
}

// expandWithBlockers walks transitive blockers for the given tickets
// and returns the full set of ticket IDs plus the dependency edges.
func (s *ChainService) expandWithBlockers(projectID string, ticketIDs []string) ([]string, map[string][]string, error) {
	database, err := db.Open(s.pool.Path)
	if err != nil {
		return nil, nil, err
	}
	defer database.Close()

	depRepo := repo.NewDependencyRepo(database, s.clock)
	ticketRepo := repo.NewTicketRepo(database, s.clock)

	// Cache ticket statuses to avoid redundant queries
	statusCache := make(map[string]model.Status)
	getStatus := func(tid string) model.Status {
		if st, ok := statusCache[tid]; ok {
			return st
		}
		ticket, err := ticketRepo.Get(projectID, tid)
		if err != nil {
			// Treat missing/errored tickets as open (preserve existing LEFT JOIN behavior)
			return model.StatusOpen
		}
		statusCache[tid] = ticket.Status
		return ticket.Status
	}

	visited := make(map[string]bool)
	deps := make(map[string][]string) // ticket -> blockers
	queue := make([]string, len(ticketIDs))
	for i, tid := range ticketIDs {
		queue[i] = strings.ToLower(tid)
	}

	for len(queue) > 0 {
		tid := queue[0]
		queue = queue[1:]

		if visited[tid] {
			continue
		}
		visited[tid] = true

		blockers, err := depRepo.GetBlockers(projectID, tid)
		if err != nil {
			return nil, nil, fmt.Errorf("failed to get blockers for %s: %w", tid, err)
		}

		for _, blocker := range blockers {
			blockerID := strings.ToLower(blocker.DependsOnID)
			// Skip closed blockers — they represent completed work
			if getStatus(blockerID) == model.StatusClosed {
				continue
			}
			deps[tid] = append(deps[tid], blockerID)
			if !visited[blockerID] {
				queue = append(queue, blockerID)
			}
		}
	}

	allIDs := make([]string, 0, len(visited))
	for tid := range visited {
		allIDs = append(allIDs, tid)
	}
	return allIDs, deps, nil
}

// detectCycles uses DFS to find cycles in the dependency graph.
func detectCycles(ticketIDs []string, deps map[string][]string) error {
	const (
		white = 0 // not visited
		gray  = 1 // in current path
		black = 2 // fully processed
	)

	color := make(map[string]int)
	for _, tid := range ticketIDs {
		color[tid] = white
	}

	var dfs func(node string) error
	dfs = func(node string) error {
		color[node] = gray
		for _, dep := range deps[node] {
			if color[dep] == gray {
				return fmt.Errorf("cycle detected involving ticket %s -> %s", node, dep)
			}
			if color[dep] == white {
				if err := dfs(dep); err != nil {
					return err
				}
			}
		}
		color[node] = black
		return nil
	}

	for _, tid := range ticketIDs {
		if color[tid] == white {
			if err := dfs(tid); err != nil {
				return err
			}
		}
	}
	return nil
}

// topologicalSort performs Kahn's algorithm with deterministic tie-break
// (created_at ASC then ticket ID ASC).
func (s *ChainService) topologicalSort(projectID string, ticketIDs []string, deps map[string][]string) ([]string, error) {
	// Build in-degree map
	inDegree := make(map[string]int)
	for _, tid := range ticketIDs {
		inDegree[tid] = 0
	}
	// deps[ticket] = blockers means: ticket depends on blockers
	// so blocker -> ticket is the edge direction for topo sort
	// In-degree counts how many blockers a ticket has
	for tid, blockers := range deps {
		inDegree[tid] += len(blockers)
	}

	// Load ticket created_at for tie-breaking
	createdAt := make(map[string]time.Time)
	database, err := db.Open(s.pool.Path)
	if err != nil {
		return nil, err
	}
	defer database.Close()

	for _, tid := range ticketIDs {
		var ca string
		err := database.QueryRow(
			`SELECT created_at FROM tickets WHERE LOWER(project_id) = LOWER(?) AND LOWER(id) = LOWER(?)`,
			projectID, tid).Scan(&ca)
		if err != nil {
			return nil, fmt.Errorf("ticket not found: %s", tid)
		}
		createdAt[tid], _ = time.Parse(time.RFC3339Nano, ca)
	}

	// Build reverse mapping: blocker -> tickets that depend on it
	reverse := make(map[string][]string)
	for tid, blockers := range deps {
		for _, b := range blockers {
			reverse[b] = append(reverse[b], tid)
		}
	}

	// Collect zero in-degree nodes (sorted for determinism)
	var queue []string
	for _, tid := range ticketIDs {
		if inDegree[tid] == 0 {
			queue = append(queue, tid)
		}
	}
	sortByCreatedThenID(queue, createdAt)

	var result []string
	for len(queue) > 0 {
		node := queue[0]
		queue = queue[1:]
		result = append(result, node)

		for _, dependent := range reverse[node] {
			inDegree[dependent]--
			if inDegree[dependent] == 0 {
				queue = append(queue, dependent)
				sortByCreatedThenID(queue, createdAt)
			}
		}
	}

	if len(result) != len(ticketIDs) {
		return nil, fmt.Errorf("cycle detected: could not sort all tickets")
	}
	return result, nil
}

func sortByCreatedThenID(ids []string, createdAt map[string]time.Time) {
	sort.Slice(ids, func(i, j int) bool {
		ci, cj := createdAt[ids[i]], createdAt[ids[j]]
		if !ci.Equal(cj) {
			return ci.Before(cj)
		}
		return ids[i] < ids[j]
	})
}

