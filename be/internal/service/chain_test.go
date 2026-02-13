package service

import (
	"path/filepath"
	"strings"
	"testing"
	"time"

	"be/internal/db"
	"be/internal/model"
	"be/internal/repo"
	"be/internal/types"
)

func setupChainTestDB(t *testing.T) (*db.Pool, string) {
	t.Helper()
	dbPath := filepath.Join(t.TempDir(), "chain_test.db")
	pool, err := db.NewPoolPath(dbPath, db.DefaultPoolConfig())
	if err != nil {
		t.Fatalf("failed to create pool: %v", err)
	}

	projectID := "test-project"
	now := time.Now().UTC().Format(time.RFC3339)
	_, err = pool.Exec(`
		INSERT INTO projects (id, name, root_path, created_at, updated_at)
		VALUES (?, 'Test Project', '/tmp/test', ?, ?)`,
		strings.ToLower(projectID), now, now)
	if err != nil {
		t.Fatalf("failed to create test project: %v", err)
	}

	return pool, projectID
}

func createTestTickets(t *testing.T, pool *db.Pool, projectID string, tickets map[string]time.Time) {
	t.Helper()
	for tid, createdAt := range tickets {
		created := createdAt.UTC().Format(time.RFC3339)
		_, err := pool.Exec(`
			INSERT INTO tickets (id, project_id, title, status, issue_type, priority, created_at, updated_at, created_by)
			VALUES (?, ?, ?, 'open', 'feature', 2, ?, ?, 'test')`,
			strings.ToLower(tid), strings.ToLower(projectID), tid, created, created)
		if err != nil {
			t.Fatalf("failed to create ticket %s: %v", tid, err)
		}
	}
}

func createTestDependencies(t *testing.T, pool *db.Pool, projectID string, deps map[string][]string) {
	t.Helper()
	database, err := db.Open(pool.Path)
	if err != nil {
		t.Fatalf("failed to open DB: %v", err)
	}
	defer database.Close()
	depRepo := repo.NewDependencyRepo(database)

	for child, blockers := range deps {
		for _, blocker := range blockers {
			dep := &model.Dependency{
				ProjectID:   projectID,
				IssueID:     child,
				DependsOnID: blocker,
				Type:        "blocks",
				CreatedBy:   "test",
			}
			if err := depRepo.Create(dep); err != nil {
				t.Fatalf("failed to create dependency %s -> %s: %v", child, blocker, err)
			}
		}
	}
}

// TestExpandWithBlockers_NoDependencies verifies that expansion works with no dependencies
func TestExpandWithBlockers_NoDependencies(t *testing.T) {
	pool, projectID := setupChainTestDB(t)
	defer pool.Close()

	now := time.Now()
	createTestTickets(t, pool, projectID, map[string]time.Time{
		"A": now,
		"B": now.Add(time.Second),
	})

	svc := NewChainService(pool)
	allIDs, deps, err := svc.expandWithBlockers(projectID, []string{"A", "B"})
	if err != nil {
		t.Fatalf("expandWithBlockers failed: %v", err)
	}

	if len(allIDs) != 2 {
		t.Errorf("expected 2 tickets, got %d", len(allIDs))
	}
	if len(deps) != 0 {
		t.Errorf("expected 0 dependencies, got %d", len(deps))
	}
}

// TestExpandWithBlockers_DirectBlockers verifies single-level dependency expansion
func TestExpandWithBlockers_DirectBlockers(t *testing.T) {
	pool, projectID := setupChainTestDB(t)
	defer pool.Close()

	now := time.Now()
	createTestTickets(t, pool, projectID, map[string]time.Time{
		"A": now,
		"B": now.Add(time.Second),
		"C": now.Add(2 * time.Second),
	})
	createTestDependencies(t, pool, projectID, map[string][]string{
		"C": {"A", "B"}, // C depends on A and B
	})

	svc := NewChainService(pool)
	allIDs, deps, err := svc.expandWithBlockers(projectID, []string{"C"})
	if err != nil {
		t.Fatalf("expandWithBlockers failed: %v", err)
	}

	if len(allIDs) != 3 {
		t.Errorf("expected 3 tickets (A, B, C), got %d: %v", len(allIDs), allIDs)
	}
	if len(deps["c"]) != 2 {
		t.Errorf("expected C to have 2 blockers, got %d", len(deps["c"]))
	}
}

// TestExpandWithBlockers_TransitiveBlockers verifies multi-level dependency expansion
func TestExpandWithBlockers_TransitiveBlockers(t *testing.T) {
	pool, projectID := setupChainTestDB(t)
	defer pool.Close()

	now := time.Now()
	createTestTickets(t, pool, projectID, map[string]time.Time{
		"A": now,
		"B": now.Add(time.Second),
		"C": now.Add(2 * time.Second),
	})
	createTestDependencies(t, pool, projectID, map[string][]string{
		"B": {"A"}, // B depends on A
		"C": {"B"}, // C depends on B (transitively on A)
	})

	svc := NewChainService(pool)
	allIDs, deps, err := svc.expandWithBlockers(projectID, []string{"C"})
	if err != nil {
		t.Fatalf("expandWithBlockers failed: %v", err)
	}

	if len(allIDs) != 3 {
		t.Errorf("expected 3 tickets (A, B, C), got %d: %v", len(allIDs), allIDs)
	}
	// C depends on B, B depends on A
	if len(deps["c"]) != 1 || deps["c"][0] != "b" {
		t.Errorf("expected C -> B, got %v", deps["c"])
	}
	if len(deps["b"]) != 1 || deps["b"][0] != "a" {
		t.Errorf("expected B -> A, got %v", deps["b"])
	}
}

// TestDetectCycles_NoCycle verifies that valid DAGs don't trigger cycle detection
func TestDetectCycles_NoCycle(t *testing.T) {
	// Linear: A -> B -> C
	deps := map[string][]string{
		"b": {"a"},
		"c": {"b"},
	}
	err := detectCycles([]string{"a", "b", "c"}, deps)
	if err != nil {
		t.Errorf("expected no cycle, got error: %v", err)
	}
}

// TestDetectCycles_SimpleCycle verifies detection of simple A -> B -> A cycle
func TestDetectCycles_SimpleCycle(t *testing.T) {
	deps := map[string][]string{
		"a": {"b"},
		"b": {"a"},
	}
	err := detectCycles([]string{"a", "b"}, deps)
	if err == nil {
		t.Error("expected cycle error, got nil")
	}
	if !strings.Contains(err.Error(), "cycle detected") {
		t.Errorf("expected cycle error, got: %v", err)
	}
}

// TestDetectCycles_DiamondNotCycle verifies diamond dependencies don't trigger false positive
func TestDetectCycles_DiamondNotCycle(t *testing.T) {
	// Diamond: D depends on B and C, both B and C depend on A
	deps := map[string][]string{
		"b": {"a"},
		"c": {"a"},
		"d": {"b", "c"},
	}
	err := detectCycles([]string{"a", "b", "c", "d"}, deps)
	if err != nil {
		t.Errorf("diamond is not a cycle, got error: %v", err)
	}
}

// TestDetectCycles_ComplexCycle verifies detection of cycle in larger graph
func TestDetectCycles_ComplexCycle(t *testing.T) {
	// A -> B -> C -> D -> A (cycle back to A)
	deps := map[string][]string{
		"b": {"a"},
		"c": {"b"},
		"d": {"c"},
		"a": {"d"}, // cycle back to A
	}
	err := detectCycles([]string{"a", "b", "c", "d"}, deps)
	if err == nil {
		t.Error("expected cycle error, got nil")
	}
}

// TestTopologicalSort_Linear verifies simple linear chain ordering
func TestTopologicalSort_Linear(t *testing.T) {
	pool, projectID := setupChainTestDB(t)
	defer pool.Close()

	base := time.Now()
	createTestTickets(t, pool, projectID, map[string]time.Time{
		"A": base,
		"B": base.Add(time.Second),
		"C": base.Add(2 * time.Second),
	})

	// A -> B -> C
	deps := map[string][]string{
		"b": {"a"},
		"c": {"b"},
	}

	svc := NewChainService(pool)
	sorted, err := svc.topologicalSort(projectID, []string{"a", "b", "c"}, deps)
	if err != nil {
		t.Fatalf("topologicalSort failed: %v", err)
	}

	// Expected: A, B, C
	if len(sorted) != 3 {
		t.Fatalf("expected 3 tickets, got %d", len(sorted))
	}
	if sorted[0] != "a" || sorted[1] != "b" || sorted[2] != "c" {
		t.Errorf("expected [a, b, c], got %v", sorted)
	}
}

// TestTopologicalSort_Diamond verifies diamond dependency ordering
func TestTopologicalSort_Diamond(t *testing.T) {
	pool, projectID := setupChainTestDB(t)
	defer pool.Close()

	base := time.Now()
	createTestTickets(t, pool, projectID, map[string]time.Time{
		"A": base,
		"B": base.Add(time.Second),
		"C": base.Add(2 * time.Second),
		"D": base.Add(3 * time.Second),
	})

	// Diamond: D depends on B and C, both depend on A
	deps := map[string][]string{
		"b": {"a"},
		"c": {"a"},
		"d": {"b", "c"},
	}

	svc := NewChainService(pool)
	sorted, err := svc.topologicalSort(projectID, []string{"a", "b", "c", "d"}, deps)
	if err != nil {
		t.Fatalf("topologicalSort failed: %v", err)
	}

	if len(sorted) != 4 {
		t.Fatalf("expected 4 tickets, got %d", len(sorted))
	}
	// A must be first, D must be last
	if sorted[0] != "a" {
		t.Errorf("expected A first, got %s", sorted[0])
	}
	if sorted[3] != "d" {
		t.Errorf("expected D last, got %s", sorted[3])
	}
	// B and C can be in either order, but both must be after A and before D
	if (sorted[1] != "b" && sorted[1] != "c") || (sorted[2] != "b" && sorted[2] != "c") {
		t.Errorf("expected B and C in middle positions, got %v", sorted)
	}
}

// TestTopologicalSort_TieBreak verifies created_at then ID tie-breaking
func TestTopologicalSort_TieBreak(t *testing.T) {
	pool, projectID := setupChainTestDB(t)
	defer pool.Close()

	base := time.Now()
	// B and C have same created_at, should be sorted by ID
	createTestTickets(t, pool, projectID, map[string]time.Time{
		"C": base,
		"B": base, // same time as C
		"A": base.Add(-time.Second),
	})

	// No dependencies
	deps := map[string][]string{}

	svc := NewChainService(pool)
	sorted, err := svc.topologicalSort(projectID, []string{"a", "b", "c"}, deps)
	if err != nil {
		t.Fatalf("topologicalSort failed: %v", err)
	}

	// Expected: A (oldest), then B and C sorted by ID (B < C)
	if len(sorted) != 3 {
		t.Fatalf("expected 3 tickets, got %d", len(sorted))
	}
	if sorted[0] != "a" {
		t.Errorf("expected A first (oldest), got %s", sorted[0])
	}
	if sorted[1] != "b" || sorted[2] != "c" {
		t.Errorf("expected B, C (ID order), got %v", sorted)
	}
}

// TestCreateChain_HappyPath verifies basic chain creation
func TestCreateChain_HappyPath(t *testing.T) {
	pool, projectID := setupChainTestDB(t)
	defer pool.Close()

	base := time.Now()
	createTestTickets(t, pool, projectID, map[string]time.Time{
		"A": base,
		"B": base.Add(time.Second),
	})
	createTestDependencies(t, pool, projectID, map[string][]string{
		"B": {"A"},
	})

	svc := NewChainService(pool)
	chain, err := svc.CreateChain(projectID, &types.ChainCreateRequest{
		Name:         "Test Chain",
		WorkflowName: "test",
		Category:     "full",
		TicketIDs:    []string{"B"},
	})
	if err != nil {
		t.Fatalf("CreateChain failed: %v", err)
	}

	if chain.Name != "Test Chain" {
		t.Errorf("expected name 'Test Chain', got %s", chain.Name)
	}
	if chain.Status != model.ChainStatusPending {
		t.Errorf("expected status pending, got %s", chain.Status)
	}
	// Should expand to include A (blocker)
	if len(chain.Items) != 2 {
		t.Fatalf("expected 2 items (A expanded), got %d", len(chain.Items))
	}
	// A should be first (blocker)
	if chain.Items[0].TicketID != "a" {
		t.Errorf("expected first item to be A, got %s", chain.Items[0].TicketID)
	}
	if chain.Items[1].TicketID != "b" {
		t.Errorf("expected second item to be B, got %s", chain.Items[1].TicketID)
	}
	if chain.Items[0].Position != 0 || chain.Items[1].Position != 1 {
		t.Errorf("expected positions 0, 1, got %d, %d", chain.Items[0].Position, chain.Items[1].Position)
	}
	// Verify ticket titles are populated
	if chain.Items[0].TicketTitle != "A" {
		t.Errorf("expected ticket title 'A', got %s", chain.Items[0].TicketTitle)
	}
	if chain.Items[1].TicketTitle != "B" {
		t.Errorf("expected ticket title 'B', got %s", chain.Items[1].TicketTitle)
	}
}

// TestCreateChain_CycleDetection verifies cycle detection during creation
func TestCreateChain_CycleDetection(t *testing.T) {
	pool, projectID := setupChainTestDB(t)
	defer pool.Close()

	base := time.Now()
	createTestTickets(t, pool, projectID, map[string]time.Time{
		"A": base,
		"B": base.Add(time.Second),
	})
	createTestDependencies(t, pool, projectID, map[string][]string{
		"A": {"B"},
		"B": {"A"}, // cycle
	})

	svc := NewChainService(pool)
	_, err := svc.CreateChain(projectID, &types.ChainCreateRequest{
		Name:         "Cycle Chain",
		WorkflowName: "test",
		TicketIDs:    []string{"A"},
	})
	if err == nil {
		t.Fatal("expected cycle error, got nil")
	}
	if !strings.Contains(err.Error(), "cycle detected") {
		t.Errorf("expected cycle error, got: %v", err)
	}
}

// TestCreateChain_LockConflict verifies lock conflict detection
func TestCreateChain_LockConflict(t *testing.T) {
	pool, projectID := setupChainTestDB(t)
	defer pool.Close()

	base := time.Now()
	createTestTickets(t, pool, projectID, map[string]time.Time{
		"A": base,
	})

	svc := NewChainService(pool)
	// Create first chain
	chain1, err := svc.CreateChain(projectID, &types.ChainCreateRequest{
		Name:         "Chain 1",
		WorkflowName: "test",
		TicketIDs:    []string{"A"},
	})
	if err != nil {
		t.Fatalf("first chain creation failed: %v", err)
	}

	// Try to create second chain with overlapping ticket
	_, err = svc.CreateChain(projectID, &types.ChainCreateRequest{
		Name:         "Chain 2",
		WorkflowName: "test",
		TicketIDs:    []string{"A"},
	})
	if err == nil {
		t.Fatal("expected lock conflict error, got nil")
	}
	if !strings.Contains(err.Error(), "already locked") {
		t.Errorf("expected lock conflict error, got: %v", err)
	}

	// Cleanup first chain's locks
	lockRepo := repo.NewChainLockRepo(pool)
	lockRepo.DeleteLocksByChain(chain1.ID)

	// Now second chain should succeed
	_, err = svc.CreateChain(projectID, &types.ChainCreateRequest{
		Name:         "Chain 2",
		WorkflowName: "test",
		TicketIDs:    []string{"A"},
	})
	if err != nil {
		t.Errorf("second chain should succeed after locks released: %v", err)
	}
}

// TestUpdateChain_PendingOnly verifies only pending chains can be edited
func TestUpdateChain_PendingOnly(t *testing.T) {
	pool, projectID := setupChainTestDB(t)
	defer pool.Close()

	base := time.Now()
	createTestTickets(t, pool, projectID, map[string]time.Time{
		"A": base,
	})

	svc := NewChainService(pool)
	chain, err := svc.CreateChain(projectID, &types.ChainCreateRequest{
		Name:         "Test",
		WorkflowName: "test",
		TicketIDs:    []string{"A"},
	})
	if err != nil {
		t.Fatalf("CreateChain failed: %v", err)
	}

	// Update to running
	chainRepo := repo.NewChainRepo(pool)
	chainRepo.UpdateStatus(chain.ID, model.ChainStatusRunning)

	// Try to edit running chain
	newName := "Updated"
	_, err = svc.UpdateChain(chain.ID, &types.ChainUpdateRequest{
		Name: &newName,
	})
	if err == nil {
		t.Fatal("expected error editing running chain, got nil")
	}
	if !strings.Contains(err.Error(), "pending") {
		t.Errorf("expected pending-only error, got: %v", err)
	}
}

// TestUpdateChain_TicketReexpansion verifies ticket list updates trigger re-expansion
func TestUpdateChain_TicketReexpansion(t *testing.T) {
	pool, projectID := setupChainTestDB(t)
	defer pool.Close()

	base := time.Now()
	createTestTickets(t, pool, projectID, map[string]time.Time{
		"A": base,
		"B": base.Add(time.Second),
		"C": base.Add(2 * time.Second),
	})
	createTestDependencies(t, pool, projectID, map[string][]string{
		"B": {"A"},
		"C": {"A"},
	})

	svc := NewChainService(pool)
	chain, err := svc.CreateChain(projectID, &types.ChainCreateRequest{
		Name:         "Test",
		WorkflowName: "test",
		TicketIDs:    []string{"B"},
	})
	if err != nil {
		t.Fatalf("CreateChain failed: %v", err)
	}

	// Initial: B + A (blocker) = 2 items
	if len(chain.Items) != 2 {
		t.Fatalf("expected 2 items initially, got %d", len(chain.Items))
	}

	// Update to include C (also depends on A)
	updated, err := svc.UpdateChain(chain.ID, &types.ChainUpdateRequest{
		TicketIDs: []string{"B", "C"},
	})
	if err != nil {
		t.Fatalf("UpdateChain failed: %v", err)
	}

	// Should now have A, B, C (A still first as shared blocker)
	if len(updated.Items) != 3 {
		t.Fatalf("expected 3 items after update, got %d", len(updated.Items))
	}

	// Verify ticket titles are populated in UpdateChain response
	for _, item := range updated.Items {
		if item.TicketTitle == "" {
			t.Errorf("expected ticket title for %s to be non-empty", item.TicketID)
		}
	}
}

// TestCreateChain_EmptyTicketList verifies error on empty ticket list
func TestCreateChain_EmptyTicketList(t *testing.T) {
	pool, projectID := setupChainTestDB(t)
	defer pool.Close()

	svc := NewChainService(pool)
	_, err := svc.CreateChain(projectID, &types.ChainCreateRequest{
		Name:         "Empty",
		WorkflowName: "test",
		TicketIDs:    []string{},
	})
	if err == nil {
		t.Fatal("expected error for empty ticket list, got nil")
	}
	if !strings.Contains(err.Error(), "at least one ticket") {
		t.Errorf("expected 'at least one ticket' error, got: %v", err)
	}
}

// TestCreateChain_SingleTicketNoDeps verifies single ticket with no dependencies
func TestCreateChain_SingleTicketNoDeps(t *testing.T) {
	pool, projectID := setupChainTestDB(t)
	defer pool.Close()

	createTestTickets(t, pool, projectID, map[string]time.Time{
		"A": time.Now(),
	})

	svc := NewChainService(pool)
	chain, err := svc.CreateChain(projectID, &types.ChainCreateRequest{
		Name:         "Single",
		WorkflowName: "test",
		TicketIDs:    []string{"A"},
	})
	if err != nil {
		t.Fatalf("CreateChain failed: %v", err)
	}

	if len(chain.Items) != 1 {
		t.Fatalf("expected 1 item, got %d", len(chain.Items))
	}
	if chain.Items[0].TicketID != "a" {
		t.Errorf("expected ticket A, got %s", chain.Items[0].TicketID)
	}
}

// TestChainItemTicketTitle_DeletedTicket verifies ticket title is empty string when ticket is deleted
func TestChainItemTicketTitle_DeletedTicket(t *testing.T) {
	pool, projectID := setupChainTestDB(t)
	defer pool.Close()

	base := time.Now()
	createTestTickets(t, pool, projectID, map[string]time.Time{
		"A": base,
		"B": base.Add(time.Second),
	})

	svc := NewChainService(pool)
	chain, err := svc.CreateChain(projectID, &types.ChainCreateRequest{
		Name:         "Test",
		WorkflowName: "test",
		TicketIDs:    []string{"A", "B"},
	})
	if err != nil {
		t.Fatalf("CreateChain failed: %v", err)
	}

	// Verify initial titles are populated
	if chain.Items[0].TicketTitle != "A" {
		t.Errorf("expected title 'A', got %s", chain.Items[0].TicketTitle)
	}
	if chain.Items[1].TicketTitle != "B" {
		t.Errorf("expected title 'B', got %s", chain.Items[1].TicketTitle)
	}

	// Delete ticket A
	_, err = pool.Exec(`DELETE FROM tickets WHERE LOWER(id) = LOWER(?) AND LOWER(project_id) = LOWER(?)`, "A", projectID)
	if err != nil {
		t.Fatalf("failed to delete ticket: %v", err)
	}

	// Retrieve chain again
	retrieved, err := svc.GetChainWithItems(chain.ID)
	if err != nil {
		t.Fatalf("GetChainWithItems failed: %v", err)
	}

	// Verify ticket A has empty title (LEFT JOIN returns NULL, COALESCE makes it empty string)
	if retrieved.Items[0].TicketTitle != "" {
		t.Errorf("expected empty title for deleted ticket, got %s", retrieved.Items[0].TicketTitle)
	}

	// Ticket B should still have its title
	if retrieved.Items[1].TicketTitle != "B" {
		t.Errorf("expected title 'B', got %s", retrieved.Items[1].TicketTitle)
	}
}

// TestGetChainWithItems_TicketTitlesPopulated verifies GetChainWithItems includes ticket titles
func TestGetChainWithItems_TicketTitlesPopulated(t *testing.T) {
	pool, projectID := setupChainTestDB(t)
	defer pool.Close()

	base := time.Now()
	createTestTickets(t, pool, projectID, map[string]time.Time{
		"TICKET-1": base,
		"TICKET-2": base.Add(time.Second),
		"TICKET-3": base.Add(2 * time.Second),
	})
	createTestDependencies(t, pool, projectID, map[string][]string{
		"TICKET-2": {"TICKET-1"},
		"TICKET-3": {"TICKET-2"},
	})

	svc := NewChainService(pool)
	chain, err := svc.CreateChain(projectID, &types.ChainCreateRequest{
		Name:         "Multi-ticket",
		WorkflowName: "test",
		TicketIDs:    []string{"TICKET-3"},
	})
	if err != nil {
		t.Fatalf("CreateChain failed: %v", err)
	}

	// Get with items
	retrieved, err := svc.GetChainWithItems(chain.ID)
	if err != nil {
		t.Fatalf("GetChainWithItems failed: %v", err)
	}

	if len(retrieved.Items) != 3 {
		t.Fatalf("expected 3 items, got %d", len(retrieved.Items))
	}

	// Verify all items have populated ticket titles
	expectedTitles := map[string]string{
		"ticket-1": "TICKET-1",
		"ticket-2": "TICKET-2",
		"ticket-3": "TICKET-3",
	}

	for _, item := range retrieved.Items {
		expectedTitle, ok := expectedTitles[item.TicketID]
		if !ok {
			t.Errorf("unexpected ticket ID: %s", item.TicketID)
			continue
		}
		if item.TicketTitle != expectedTitle {
			t.Errorf("expected title %s for ticket %s, got %s", expectedTitle, item.TicketID, item.TicketTitle)
		}
	}
}
