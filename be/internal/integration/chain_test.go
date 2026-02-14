package integration

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"be/internal/db"
	"be/internal/model"
	"be/internal/orchestrator"
	"be/internal/repo"
	"be/internal/service"
	"be/internal/types"
)

// createChainTickets creates test tickets for chain tests
func createChainTickets(t *testing.T, env *TestEnv, tickets map[string]time.Time) {
	t.Helper()
	for tid, createdAt := range tickets {
		created := createdAt.UTC().Format(time.RFC3339)
		_, err := env.Pool.Exec(`
			INSERT INTO tickets (id, project_id, title, status, issue_type, priority, created_at, updated_at, created_by)
			VALUES (?, ?, ?, 'open', 'feature', 2, ?, ?, 'test')`,
			strings.ToLower(tid), strings.ToLower(env.ProjectID), tid, created, created)
		if err != nil {
			t.Fatalf("failed to create ticket %s: %v", tid, err)
		}
	}
}

// createChainDependencies creates test dependencies
func createChainDependencies(t *testing.T, env *TestEnv, deps map[string][]string) {
	t.Helper()
	database, err := db.Open(env.Pool.Path)
	if err != nil {
		t.Fatalf("failed to open DB: %v", err)
	}
	defer database.Close()

	depRepo := repo.NewDependencyRepo(database)

	for child, blockers := range deps {
		for _, blocker := range blockers {
			dep := &model.Dependency{
				ProjectID:   env.ProjectID,
				IssueID:     child,
				DependsOnID: blocker,
				Type:        "blocks",
				CreatedBy:   "test",
			}
			err := depRepo.Create(dep)
			if err != nil {
				t.Fatalf("failed to create dependency %s -> %s: %v", child, blocker, err)
			}
		}
	}
}

// TestChainCreate_WithDependencies verifies chain creation expands dependencies
func TestChainCreate_WithDependencies(t *testing.T) {
	env := NewTestEnv(t)

	base := time.Now()
	createChainTickets(t, env, map[string]time.Time{
		"A": base,
		"B": base.Add(time.Second),
		"C": base.Add(2 * time.Second),
	})
	createChainDependencies(t, env, map[string][]string{
		"B": {"A"},
		"C": {"B"}, // C -> B -> A (transitive)
	})

	chainSvc := service.NewChainService(env.Pool)
	chain, err := chainSvc.CreateChain(env.ProjectID, &types.ChainCreateRequest{
		Name:         "Transitive Chain",
		WorkflowName: "test",
		TicketIDs:    []string{"C"},
	})
	if err != nil {
		t.Fatalf("CreateChain failed: %v", err)
	}

	// Should expand to A, B, C in topological order
	if len(chain.Items) != 3 {
		t.Fatalf("expected 3 items (expanded), got %d", len(chain.Items))
	}

	// Verify topological order: A, B, C
	if chain.Items[0].TicketID != "a" {
		t.Errorf("expected first item A, got %s", chain.Items[0].TicketID)
	}
	if chain.Items[1].TicketID != "b" {
		t.Errorf("expected second item B, got %s", chain.Items[1].TicketID)
	}
	if chain.Items[2].TicketID != "c" {
		t.Errorf("expected third item C, got %s", chain.Items[2].TicketID)
	}

	// Verify all items are pending
	for i, item := range chain.Items {
		if item.Status != model.ChainItemPending {
			t.Errorf("item %d: expected pending status, got %s", i, item.Status)
		}
		if item.Position != i {
			t.Errorf("item %d: expected position %d, got %d", i, i, item.Position)
		}
	}

	// Verify chain metadata
	if chain.Status != model.ChainStatusPending {
		t.Errorf("expected chain status pending, got %s", chain.Status)
	}
	if chain.WorkflowName != "test" {
		t.Errorf("expected workflow 'test', got %s", chain.WorkflowName)
	}
}

// TestChainCreate_LocksInserted verifies locks are created
func TestChainCreate_LocksInserted(t *testing.T) {
	env := NewTestEnv(t)

	base := time.Now()
	createChainTickets(t, env, map[string]time.Time{
		"A": base,
		"B": base.Add(time.Second),
	})
	createChainDependencies(t, env, map[string][]string{
		"B": {"A"},
	})

	chainSvc := service.NewChainService(env.Pool)
	chain, err := chainSvc.CreateChain(env.ProjectID, &types.ChainCreateRequest{
		Name:         "Lock Test",
		WorkflowName: "test",
		TicketIDs:    []string{"B"},
	})
	if err != nil {
		t.Fatalf("CreateChain failed: %v", err)
	}

	// Verify locks exist for both A and B
	lockRepo := repo.NewChainLockRepo(env.Pool)
	conflicts, err := lockRepo.CheckConflicts(env.ProjectID, []string{"A", "B"}, "")
	if err != nil {
		t.Fatalf("CheckConflicts failed: %v", err)
	}

	if len(conflicts) != 2 {
		t.Errorf("expected 2 locked tickets, got %d: %v", len(conflicts), conflicts)
	}

	// Verify locks belong to our chain
	var lockCount int
	err = env.Pool.QueryRow(`
		SELECT COUNT(*) FROM chain_execution_locks
		WHERE chain_id = ?`, chain.ID).Scan(&lockCount)
	if err != nil {
		t.Fatalf("failed to query locks: %v", err)
	}
	if lockCount != 2 {
		t.Errorf("expected 2 locks for chain, got %d", lockCount)
	}
}

// TestChainCreate_OverlapConflict verifies overlapping chains are rejected
func TestChainCreate_OverlapConflict(t *testing.T) {
	env := NewTestEnv(t)

	base := time.Now()
	createChainTickets(t, env, map[string]time.Time{
		"A": base,
		"B": base.Add(time.Second),
	})

	chainSvc := service.NewChainService(env.Pool)

	// Create first chain with ticket A
	chain1, err := chainSvc.CreateChain(env.ProjectID, &types.ChainCreateRequest{
		Name:         "Chain 1",
		WorkflowName: "test",
		TicketIDs:    []string{"A"},
	})
	if err != nil {
		t.Fatalf("first chain creation failed: %v", err)
	}

	// Try to create second chain with same ticket - should fail
	_, err = chainSvc.CreateChain(env.ProjectID, &types.ChainCreateRequest{
		Name:         "Chain 2",
		WorkflowName: "test",
		TicketIDs:    []string{"A"},
	})
	if err == nil {
		t.Fatal("expected lock conflict error, got nil")
	}
	if !strings.Contains(err.Error(), "already locked") {
		t.Errorf("expected 'already locked' error, got: %v", err)
	}

	// Release locks from chain1
	lockRepo := repo.NewChainLockRepo(env.Pool)
	err = lockRepo.DeleteLocksByChain(chain1.ID)
	if err != nil {
		t.Fatalf("failed to delete locks: %v", err)
	}

	// Now second chain should succeed
	_, err = chainSvc.CreateChain(env.ProjectID, &types.ChainCreateRequest{
		Name:         "Chain 2",
		WorkflowName: "test",
		TicketIDs:    []string{"A"},
	})
	if err != nil {
		t.Errorf("second chain should succeed after locks released: %v", err)
	}
}

// TestChainUpdate_PendingOnly verifies only pending chains can be edited
func TestChainUpdate_PendingOnly(t *testing.T) {
	env := NewTestEnv(t)

	createChainTickets(t, env, map[string]time.Time{
		"A": time.Now(),
	})

	chainSvc := service.NewChainService(env.Pool)
	chain, err := chainSvc.CreateChain(env.ProjectID, &types.ChainCreateRequest{
		Name:         "Test",
		WorkflowName: "test",
		TicketIDs:    []string{"A"},
	})
	if err != nil {
		t.Fatalf("CreateChain failed: %v", err)
	}

	// Update chain to running status
	chainRepo := repo.NewChainRepo(env.Pool)
	err = chainRepo.UpdateStatus(chain.ID, model.ChainStatusRunning)
	if err != nil {
		t.Fatalf("failed to update chain status: %v", err)
	}

	// Try to edit - should fail
	newName := "Updated Name"
	_, err = chainSvc.UpdateChain(chain.ID, &types.ChainUpdateRequest{
		Name: &newName,
	})
	if err == nil {
		t.Fatal("expected error editing running chain, got nil")
	}
	if !strings.Contains(err.Error(), "pending") {
		t.Errorf("expected pending-only error, got: %v", err)
	}
}

// TestChainUpdate_NameOnly verifies name-only updates work
func TestChainUpdate_NameOnly(t *testing.T) {
	env := NewTestEnv(t)

	createChainTickets(t, env, map[string]time.Time{
		"A": time.Now(),
	})

	chainSvc := service.NewChainService(env.Pool)
	chain, err := chainSvc.CreateChain(env.ProjectID, &types.ChainCreateRequest{
		Name:         "Original",
		WorkflowName: "test",
		TicketIDs:    []string{"A"},
	})
	if err != nil {
		t.Fatalf("CreateChain failed: %v", err)
	}

	newName := "Updated"
	_, err = chainSvc.UpdateChain(chain.ID, &types.ChainUpdateRequest{
		Name: &newName,
	})
	if err != nil {
		t.Fatalf("UpdateChain failed: %v", err)
	}

	// Get updated chain with items
	updated, err := chainSvc.GetChainWithItems(chain.ID)
	if err != nil {
		t.Fatalf("GetChainWithItems failed: %v", err)
	}

	if updated.Name != "Updated" {
		t.Errorf("expected name 'Updated', got %s", updated.Name)
	}

	// Items should remain unchanged
	if len(updated.Items) != 1 || updated.Items[0].TicketID != "a" {
		t.Errorf("expected items unchanged, got %d items", len(updated.Items))
	}
}

// TestChainUpdate_TicketReexpansion verifies ticket updates trigger re-expansion
func TestChainUpdate_TicketReexpansion(t *testing.T) {
	env := NewTestEnv(t)

	base := time.Now()
	createChainTickets(t, env, map[string]time.Time{
		"A": base,
		"B": base.Add(time.Second),
		"C": base.Add(2 * time.Second),
	})
	createChainDependencies(t, env, map[string][]string{
		"B": {"A"},
		"C": {"B"},
	})

	chainSvc := service.NewChainService(env.Pool)
	chain, err := chainSvc.CreateChain(env.ProjectID, &types.ChainCreateRequest{
		Name:         "Test",
		WorkflowName: "test",
		TicketIDs:    []string{"B"},
	})
	if err != nil {
		t.Fatalf("CreateChain failed: %v", err)
	}

	// Initially: B + A (blocker) = 2 items
	if len(chain.Items) != 2 {
		t.Fatalf("expected 2 items initially, got %d", len(chain.Items))
	}

	// Update to include C (which depends on B, transitively on A)
	_, err = chainSvc.UpdateChain(chain.ID, &types.ChainUpdateRequest{
		TicketIDs: []string{"C"},
	})
	if err != nil {
		t.Fatalf("UpdateChain failed: %v", err)
	}

	// Get updated chain with items
	updated, err := chainSvc.GetChainWithItems(chain.ID)
	if err != nil {
		t.Fatalf("GetChainWithItems failed: %v", err)
	}

	// Should now have A, B, C
	if len(updated.Items) != 3 {
		t.Fatalf("expected 3 items after update, got %d", len(updated.Items))
	}

	// Verify order: A, B, C
	if updated.Items[0].TicketID != "a" || updated.Items[1].TicketID != "b" || updated.Items[2].TicketID != "c" {
		t.Errorf("expected [a, b, c], got %v", []string{updated.Items[0].TicketID, updated.Items[1].TicketID, updated.Items[2].TicketID})
	}
}

// TestChainList_StatusFilter verifies listing chains with status filter
func TestChainList_StatusFilter(t *testing.T) {
	env := NewTestEnv(t)

	createChainTickets(t, env, map[string]time.Time{
		"A": time.Now(),
		"B": time.Now().Add(time.Second),
	})

	chainSvc := service.NewChainService(env.Pool)

	// Create pending chain
	chain1, err := chainSvc.CreateChain(env.ProjectID, &types.ChainCreateRequest{
		Name:         "Pending Chain",
		WorkflowName: "test",
		TicketIDs:    []string{"A"},
	})
	if err != nil {
		t.Fatalf("CreateChain 1 failed: %v", err)
	}

	// Create second chain and mark it completed
	chain2, err := chainSvc.CreateChain(env.ProjectID, &types.ChainCreateRequest{
		Name:         "Completed Chain",
		WorkflowName: "test",
		TicketIDs:    []string{"B"},
	})
	if err != nil {
		t.Fatalf("CreateChain 2 failed: %v", err)
	}

	chainRepo := repo.NewChainRepo(env.Pool)
	chainRepo.UpdateStatus(chain2.ID, model.ChainStatusCompleted)

	// List pending chains
	pendingChains, err := chainRepo.List(env.ProjectID, string(model.ChainStatusPending), "")
	if err != nil {
		t.Fatalf("List pending failed: %v", err)
	}

	if len(pendingChains) != 1 {
		t.Fatalf("expected 1 pending chain, got %d", len(pendingChains))
	}
	if pendingChains[0].ID != chain1.ID {
		t.Errorf("expected chain1, got %s", pendingChains[0].ID)
	}

	// List all chains
	allChains, err := chainRepo.List(env.ProjectID, "", "")
	if err != nil {
		t.Fatalf("List all failed: %v", err)
	}

	if len(allChains) != 2 {
		t.Fatalf("expected 2 chains total, got %d", len(allChains))
	}
}

// TestChainCancel_Pending verifies canceling a pending chain releases locks
func TestChainCancel_Pending(t *testing.T) {
	env := NewTestEnv(t)

	createChainTickets(t, env, map[string]time.Time{
		"A": time.Now(),
	})

	chainSvc := service.NewChainService(env.Pool)
	chain, err := chainSvc.CreateChain(env.ProjectID, &types.ChainCreateRequest{
		Name:         "Cancel Test",
		WorkflowName: "test",
		TicketIDs:    []string{"A"},
	})
	if err != nil {
		t.Fatalf("CreateChain failed: %v", err)
	}

	// Verify lock exists
	lockRepo := repo.NewChainLockRepo(env.Pool)
	conflicts, _ := lockRepo.CheckConflicts(env.ProjectID, []string{"A"}, "")
	if len(conflicts) != 1 {
		t.Fatalf("expected lock on A, got %d conflicts", len(conflicts))
	}

	// Create chain runner and cancel
	runner := orchestrator.NewChainRunner(nil, env.Pool.Path, env.Hub)
	err = runner.Cancel(chain.ID)
	if err != nil {
		t.Fatalf("Cancel failed: %v", err)
	}

	// Verify status updated
	chainRepo := repo.NewChainRepo(env.Pool)
	updated, err := chainRepo.Get(chain.ID)
	if err != nil {
		t.Fatalf("Get chain failed: %v", err)
	}
	if updated.Status != model.ChainStatusCanceled {
		t.Errorf("expected status canceled, got %s", updated.Status)
	}

	// Verify locks released
	conflicts, _ = lockRepo.CheckConflicts(env.ProjectID, []string{"A"}, "")
	if len(conflicts) != 0 {
		t.Errorf("expected locks released, got %d conflicts", len(conflicts))
	}
}

// TestChainRunner_ZombieRecovery verifies zombie chain recovery on startup
func TestChainRunner_ZombieRecovery(t *testing.T) {
	env := NewTestEnv(t)

	base := time.Now()
	createChainTickets(t, env, map[string]time.Time{
		"A": base,
		"B": base.Add(time.Second),
	})

	chainSvc := service.NewChainService(env.Pool)
	chain, err := chainSvc.CreateChain(env.ProjectID, &types.ChainCreateRequest{
		Name:         "Zombie Test",
		WorkflowName: "test",
		TicketIDs:    []string{"A", "B"},
	})
	if err != nil {
		t.Fatalf("CreateChain failed: %v", err)
	}

	// Manually set chain to running (simulating crash during execution)
	chainRepo := repo.NewChainRepo(env.Pool)
	chainRepo.UpdateStatus(chain.ID, model.ChainStatusRunning)

	// Set first item to running
	itemRepo := repo.NewChainItemRepo(env.Pool)
	items, _ := itemRepo.ListByChain(chain.ID)
	itemRepo.UpdateItemStatus(items[0].ID, model.ChainItemRunning)

	// Run zombie recovery
	runner := orchestrator.NewChainRunner(nil, env.Pool.Path, env.Hub)
	runner.RecoverZombieChains()

	// Verify chain marked as failed
	recovered, err := chainRepo.Get(chain.ID)
	if err != nil {
		t.Fatalf("Get chain failed: %v", err)
	}
	if recovered.Status != model.ChainStatusFailed {
		t.Errorf("expected status failed after recovery, got %s", recovered.Status)
	}

	// Verify locks released
	lockRepo := repo.NewChainLockRepo(env.Pool)
	conflicts, _ := lockRepo.CheckConflicts(env.ProjectID, []string{"A", "B"}, "")
	if len(conflicts) != 0 {
		t.Errorf("expected locks released after recovery, got %d conflicts", len(conflicts))
	}

	// Verify items marked canceled
	items, _ = itemRepo.ListByChain(chain.ID)
	for _, item := range items {
		if item.Status != model.ChainItemCanceled {
			t.Errorf("expected item %s to be canceled, got %s", item.TicketID, item.Status)
		}
	}
}

// TestChainCreate_CycleRejection verifies cycle detection
func TestChainCreate_CycleRejection(t *testing.T) {
	env := NewTestEnv(t)

	base := time.Now()
	createChainTickets(t, env, map[string]time.Time{
		"A": base,
		"B": base.Add(time.Second),
	})
	createChainDependencies(t, env, map[string][]string{
		"A": {"B"},
		"B": {"A"}, // A -> B -> A cycle
	})

	chainSvc := service.NewChainService(env.Pool)
	_, err := chainSvc.CreateChain(env.ProjectID, &types.ChainCreateRequest{
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

// TestChainCreate_EmptyTickets verifies error on empty ticket list
func TestChainCreate_EmptyTickets(t *testing.T) {
	env := NewTestEnv(t)

	chainSvc := service.NewChainService(env.Pool)
	_, err := chainSvc.CreateChain(env.ProjectID, &types.ChainCreateRequest{
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

// TestChainCreate_MissingName verifies error on missing name
func TestChainCreate_MissingName(t *testing.T) {
	env := NewTestEnv(t)

	createChainTickets(t, env, map[string]time.Time{
		"A": time.Now(),
	})

	chainSvc := service.NewChainService(env.Pool)
	_, err := chainSvc.CreateChain(env.ProjectID, &types.ChainCreateRequest{
		Name:         "",
		WorkflowName: "test",
		TicketIDs:    []string{"A"},
	})
	if err == nil {
		t.Fatal("expected error for missing name, got nil")
	}
	if !strings.Contains(err.Error(), "name is required") {
		t.Errorf("expected 'name is required' error, got: %v", err)
	}
}

// TestChainCreate_MissingWorkflow verifies error on missing workflow
func TestChainCreate_MissingWorkflow(t *testing.T) {
	env := NewTestEnv(t)

	createChainTickets(t, env, map[string]time.Time{
		"A": time.Now(),
	})

	chainSvc := service.NewChainService(env.Pool)
	_, err := chainSvc.CreateChain(env.ProjectID, &types.ChainCreateRequest{
		Name:         "Test",
		WorkflowName: "",
		TicketIDs:    []string{"A"},
	})
	if err == nil {
		t.Fatal("expected error for missing workflow, got nil")
	}
	if !strings.Contains(err.Error(), "workflow name is required") {
		t.Errorf("expected 'workflow name is required' error, got: %v", err)
	}
}

// TestChainGetWithItems verifies retrieving chain with items
func TestChainGetWithItems(t *testing.T) {
	env := NewTestEnv(t)

	base := time.Now()
	createChainTickets(t, env, map[string]time.Time{
		"A": base,
		"B": base.Add(time.Second),
	})
	createChainDependencies(t, env, map[string][]string{
		"B": {"A"},
	})

	chainSvc := service.NewChainService(env.Pool)
	chain, err := chainSvc.CreateChain(env.ProjectID, &types.ChainCreateRequest{
		Name:         "Get Test",
		WorkflowName: "test",
		TicketIDs:    []string{"B"},
	})
	if err != nil {
		t.Fatalf("CreateChain failed: %v", err)
	}

	// Get with items
	retrieved, err := chainSvc.GetChainWithItems(chain.ID)
	if err != nil {
		t.Fatalf("GetChainWithItems failed: %v", err)
	}

	if retrieved.ID != chain.ID {
		t.Errorf("expected chain ID %s, got %s", chain.ID, retrieved.ID)
	}
	if len(retrieved.Items) != 2 {
		t.Fatalf("expected 2 items, got %d", len(retrieved.Items))
	}

	// Verify items are ordered by position
	if retrieved.Items[0].Position != 0 || retrieved.Items[1].Position != 1 {
		t.Errorf("expected positions 0, 1, got %d, %d", retrieved.Items[0].Position, retrieved.Items[1].Position)
	}
}

// TestChainRunner_IsRunning verifies runner tracking of active chains
func TestChainRunner_IsRunning(t *testing.T) {
	env := NewTestEnv(t)

	createChainTickets(t, env, map[string]time.Time{
		"A": time.Now(),
	})

	chainSvc := service.NewChainService(env.Pool)
	chain, err := chainSvc.CreateChain(env.ProjectID, &types.ChainCreateRequest{
		Name:         "Running Test",
		WorkflowName: "test",
		TicketIDs:    []string{"A"},
	})
	if err != nil {
		t.Fatalf("CreateChain failed: %v", err)
	}

	runner := orchestrator.NewChainRunner(nil, env.Pool.Path, env.Hub)

	// Initially not running
	if runner.IsRunning(chain.ID) {
		t.Error("chain should not be running initially")
	}

	// Note: We can't easily test Start here without a full orchestrator,
	// so we just verify the IsRunning method works with no registered chains
}

// TestChainCreate_TicketTitlesInResponse verifies ticket titles are included in create response
func TestChainCreate_TicketTitlesInResponse(t *testing.T) {
	env := NewTestEnv(t)

	base := time.Now()
	createChainTickets(t, env, map[string]time.Time{
		"TICKET-A": base,
		"TICKET-B": base.Add(time.Second),
		"TICKET-C": base.Add(2 * time.Second),
	})
	createChainDependencies(t, env, map[string][]string{
		"TICKET-B": {"TICKET-A"},
		"TICKET-C": {"TICKET-A", "TICKET-B"},
	})

	chainSvc := service.NewChainService(env.Pool)
	chain, err := chainSvc.CreateChain(env.ProjectID, &types.ChainCreateRequest{
		Name:         "Title Test Chain",
		WorkflowName: "test",
		TicketIDs:    []string{"TICKET-C"},
	})
	if err != nil {
		t.Fatalf("CreateChain failed: %v", err)
	}

	// Should have 3 items: A, B, C (topologically sorted)
	if len(chain.Items) != 3 {
		t.Fatalf("expected 3 items, got %d", len(chain.Items))
	}

	// Verify all items have ticket titles populated
	for _, item := range chain.Items {
		if item.TicketTitle == "" {
			t.Errorf("expected non-empty ticket title for %s", item.TicketID)
		}
	}

	// Verify specific titles match ticket IDs
	expectedTitles := map[string]string{
		"ticket-a": "TICKET-A",
		"ticket-b": "TICKET-B",
		"ticket-c": "TICKET-C",
	}

	for _, item := range chain.Items {
		expectedTitle, ok := expectedTitles[item.TicketID]
		if !ok {
			t.Errorf("unexpected ticket ID: %s", item.TicketID)
			continue
		}
		if item.TicketTitle != expectedTitle {
			t.Errorf("ticket %s: expected title %s, got %s", item.TicketID, expectedTitle, item.TicketTitle)
		}
	}
}

// TestChainUpdate_TicketTitlesInResponse verifies ticket titles are included in update response
func TestChainUpdate_TicketTitlesInResponse(t *testing.T) {
	env := NewTestEnv(t)

	base := time.Now()
	createChainTickets(t, env, map[string]time.Time{
		"T1": base,
		"T2": base.Add(time.Second),
		"T3": base.Add(2 * time.Second),
	})
	createChainDependencies(t, env, map[string][]string{
		"T2": {"T1"},
		"T3": {"T1"},
	})

	chainSvc := service.NewChainService(env.Pool)
	chain, err := chainSvc.CreateChain(env.ProjectID, &types.ChainCreateRequest{
		Name:         "Update Test",
		WorkflowName: "test",
		TicketIDs:    []string{"T2"},
	})
	if err != nil {
		t.Fatalf("CreateChain failed: %v", err)
	}

	// Initial chain should have titles
	for _, item := range chain.Items {
		if item.TicketTitle == "" {
			t.Errorf("expected non-empty title in create response for %s", item.TicketID)
		}
	}

	// Update chain to include T3
	updated, err := chainSvc.UpdateChain(chain.ID, &types.ChainUpdateRequest{
		TicketIDs: []string{"T2", "T3"},
	})
	if err != nil {
		t.Fatalf("UpdateChain failed: %v", err)
	}

	// Updated chain should have 3 items with titles
	if len(updated.Items) != 3 {
		t.Fatalf("expected 3 items after update, got %d", len(updated.Items))
	}

	for _, item := range updated.Items {
		if item.TicketTitle == "" {
			t.Errorf("expected non-empty title in update response for %s", item.TicketID)
		}
	}
}

// TestChainAppend_SuccessWithTitles verifies appending tickets to running chain returns updated chain with titles
func TestChainAppend_SuccessWithTitles(t *testing.T) {
	env := NewTestEnv(t)

	base := time.Now()
	createChainTickets(t, env, map[string]time.Time{
		"A": base,
		"B": base.Add(time.Second),
		"C": base.Add(2 * time.Second),
		"D": base.Add(3 * time.Second),
	})
	createChainDependencies(t, env, map[string][]string{
		"B": {"A"},
		"C": {"B"},
	})

	chainSvc := service.NewChainService(env.Pool)

	// Create chain with A and B, mark as running
	chain, err := chainSvc.CreateChain(env.ProjectID, &types.ChainCreateRequest{
		Name:         "Append Test",
		WorkflowName: "test",
		TicketIDs:    []string{"A", "B"},
	})
	if err != nil {
		t.Fatalf("CreateChain failed: %v", err)
	}

	chainRepo := repo.NewChainRepo(env.Pool)
	if err := chainRepo.UpdateStatus(chain.ID, model.ChainStatusRunning); err != nil {
		t.Fatalf("failed to mark chain as running: %v", err)
	}

	// Append C (depends on B, already in chain) and D (no deps)
	updated, err := chainSvc.AppendToChain(chain.ID, &types.ChainAppendRequest{
		TicketIDs: []string{"C", "D"},
	})
	if err != nil {
		t.Fatalf("AppendToChain failed: %v", err)
	}

	// Should have 4 items: A, B, C, D
	if len(updated.Items) != 4 {
		t.Fatalf("expected 4 items after append, got %d", len(updated.Items))
	}

	// Verify order: A (0), B (1), C (2), D (3)
	expectedOrder := []string{"a", "b", "c", "d"}
	for i, expected := range expectedOrder {
		if updated.Items[i].TicketID != expected {
			t.Errorf("item %d: expected %s, got %s", i, expected, updated.Items[i].TicketID)
		}
		if updated.Items[i].Position != i {
			t.Errorf("item %d: expected position %d, got %d", i, i, updated.Items[i].Position)
		}
	}

	// Verify all items have titles
	for _, item := range updated.Items {
		if item.TicketTitle == "" {
			t.Errorf("expected non-empty title for %s", item.TicketID)
		}
	}

	// Verify new items are pending
	if updated.Items[2].Status != model.ChainItemPending {
		t.Errorf("expected C to be pending, got %s", updated.Items[2].Status)
	}
	if updated.Items[3].Status != model.ChainItemPending {
		t.Errorf("expected D to be pending, got %s", updated.Items[3].Status)
	}

	// Verify locks were acquired for new tickets
	lockRepo := repo.NewChainLockRepo(env.Pool)
	conflicts, err := lockRepo.CheckConflicts(env.ProjectID, []string{"C", "D"}, "")
	if err != nil {
		t.Fatalf("CheckConflicts failed: %v", err)
	}
	if len(conflicts) != 2 {
		t.Errorf("expected C and D to be locked, got %d conflicts: %v", len(conflicts), conflicts)
	}
}

// TestChainItemTokensUsed_WithCompletedWorkflow verifies that chain items with
// completed workflows include total_tokens_used in the response.
func TestChainItemTokensUsed_WithCompletedWorkflow(t *testing.T) {
	env := NewTestEnv(t)

	base := time.Now()
	createChainTickets(t, env, map[string]time.Time{
		"TK-A": base,
		"TK-B": base.Add(time.Second),
	})

	chainSvc := service.NewChainService(env.Pool)
	chain, err := chainSvc.CreateChain(env.ProjectID, &types.ChainCreateRequest{
		Name:         "Token Test Chain",
		WorkflowName: "test",
		TicketIDs:    []string{"TK-A", "TK-B"},
	})
	if err != nil {
		t.Fatalf("CreateChain failed: %v", err)
	}

	// Create workflow instances and agent sessions for each ticket
	// TK-A: 2 agents with context_left 60 and 40
	wfiA := "wfi-tka-001"
	env.InitWorkflowWithID(t, "TK-A", wfiA)
	insertSessionWithContextLeft(t, env, "sess-tka-1", "TK-A", wfiA,
		"analyzer", "setup-analyzer", "claude:sonnet", "completed", "pass", 60)
	insertSessionWithContextLeft(t, env, "sess-tka-2", "TK-A", wfiA,
		"builder", "implementor", "claude:opus", "completed", "pass", 40)

	// TK-B: 1 agent with context_left 75
	wfiB := "wfi-tkb-001"
	env.InitWorkflowWithID(t, "TK-B", wfiB)
	insertSessionWithContextLeft(t, env, "sess-tkb-1", "TK-B", wfiB,
		"analyzer", "setup-analyzer", "claude:sonnet", "completed", "pass", 75)

	// Link workflow instances to chain items
	itemRepo := repo.NewChainItemRepo(env.Pool)
	items, err := itemRepo.ListByChain(chain.ID)
	if err != nil {
		t.Fatalf("failed to list chain items: %v", err)
	}

	for _, item := range items {
		if item.TicketID == "tk-a" {
			if err := itemRepo.SetWorkflowInstanceID(item.ID, wfiA); err != nil {
				t.Fatalf("failed to set workflow instance for TK-A: %v", err)
			}
		} else if item.TicketID == "tk-b" {
			if err := itemRepo.SetWorkflowInstanceID(item.ID, wfiB); err != nil {
				t.Fatalf("failed to set workflow instance for TK-B: %v", err)
			}
		}
	}

	// Retrieve chain with items
	retrieved, err := chainSvc.GetChainWithItems(chain.ID)
	if err != nil {
		t.Fatalf("GetChainWithItems failed: %v", err)
	}

	if len(retrieved.Items) != 2 {
		t.Fatalf("expected 2 items, got %d", len(retrieved.Items))
	}

	// Verify token calculations
	// TK-A: 200000*(100-60)/100 + 200000*(100-40)/100 = 80000 + 120000 = 200000
	// TK-B: 200000*(100-75)/100 = 50000
	expectedTokens := map[string]int64{
		"tk-a": 200000,
		"tk-b": 50000,
	}

	for _, item := range retrieved.Items {
		expected, ok := expectedTokens[item.TicketID]
		if !ok {
			t.Errorf("unexpected ticket ID: %s", item.TicketID)
			continue
		}
		if item.TotalTokensUsed != expected {
			t.Errorf("ticket %s: expected total_tokens_used %d, got %d",
				item.TicketID, expected, item.TotalTokensUsed)
		}
	}
}

// TestChainItemTokensUsed_WithoutWorkflow verifies that chain items without
// workflow instances return 0 for total_tokens_used.
func TestChainItemTokensUsed_WithoutWorkflow(t *testing.T) {
	env := NewTestEnv(t)

	createChainTickets(t, env, map[string]time.Time{
		"TK-C": time.Now(),
	})

	chainSvc := service.NewChainService(env.Pool)
	chain, err := chainSvc.CreateChain(env.ProjectID, &types.ChainCreateRequest{
		Name:         "No Workflow Chain",
		WorkflowName: "test",
		TicketIDs:    []string{"TK-C"},
	})
	if err != nil {
		t.Fatalf("CreateChain failed: %v", err)
	}

	// Don't link any workflow instance - item should have no workflow_instance_id

	// Retrieve chain with items
	retrieved, err := chainSvc.GetChainWithItems(chain.ID)
	if err != nil {
		t.Fatalf("GetChainWithItems failed: %v", err)
	}

	if len(retrieved.Items) != 1 {
		t.Fatalf("expected 1 item, got %d", len(retrieved.Items))
	}

	item := retrieved.Items[0]
	if item.TotalTokensUsed != 0 {
		t.Errorf("expected total_tokens_used 0 for item without workflow, got %d", item.TotalTokensUsed)
	}
}

// TestChainItemTokensUsed_RunningAgentsExcluded verifies that running and
// continued agent sessions are excluded from the token calculation.
func TestChainItemTokensUsed_RunningAgentsExcluded(t *testing.T) {
	env := NewTestEnv(t)

	createChainTickets(t, env, map[string]time.Time{
		"TK-D": time.Now(),
	})

	chainSvc := service.NewChainService(env.Pool)
	chain, err := chainSvc.CreateChain(env.ProjectID, &types.ChainCreateRequest{
		Name:         "Excluding Running Agents",
		WorkflowName: "test",
		TicketIDs:    []string{"TK-D"},
	})
	if err != nil {
		t.Fatalf("CreateChain failed: %v", err)
	}

	wfiD := "wfi-tkd-001"
	env.InitWorkflowWithID(t, "TK-D", wfiD)

	// Insert 1 completed agent with context_left=50
	insertSessionWithContextLeft(t, env, "sess-tkd-1", "TK-D", wfiD,
		"analyzer", "setup-analyzer", "claude:sonnet", "completed", "pass", 50)

	// Insert 1 running agent with context_left=30 (should be excluded)
	insertSessionWithContextLeft(t, env, "sess-tkd-2", "TK-D", wfiD,
		"builder", "implementor", "claude:opus", "running", "", 30)

	// Insert 1 continued agent with context_left=20 (should be excluded)
	insertSessionWithContextLeft(t, env, "sess-tkd-3", "TK-D", wfiD,
		"verifier", "qa-verifier", "claude:sonnet", "continued", "", 20)

	// Link workflow instance to chain item
	itemRepo := repo.NewChainItemRepo(env.Pool)
	items, err := itemRepo.ListByChain(chain.ID)
	if err != nil {
		t.Fatalf("failed to list chain items: %v", err)
	}
	if err := itemRepo.SetWorkflowInstanceID(items[0].ID, wfiD); err != nil {
		t.Fatalf("failed to set workflow instance: %v", err)
	}

	// Retrieve chain with items
	retrieved, err := chainSvc.GetChainWithItems(chain.ID)
	if err != nil {
		t.Fatalf("GetChainWithItems failed: %v", err)
	}

	// Only the completed agent should be counted: 200000*(100-50)/100 = 100000
	expectedTokens := int64(100000)
	if retrieved.Items[0].TotalTokensUsed != expectedTokens {
		t.Errorf("expected total_tokens_used %d (excluding running/continued), got %d",
			expectedTokens, retrieved.Items[0].TotalTokensUsed)
	}
}

// TestChainItemTokensUsed_NullContextLeftExcluded verifies that agent sessions
// with NULL context_left are excluded from the token calculation.
func TestChainItemTokensUsed_NullContextLeftExcluded(t *testing.T) {
	env := NewTestEnv(t)

	createChainTickets(t, env, map[string]time.Time{
		"TK-E": time.Now(),
	})

	chainSvc := service.NewChainService(env.Pool)
	chain, err := chainSvc.CreateChain(env.ProjectID, &types.ChainCreateRequest{
		Name:         "Null Context Chain",
		WorkflowName: "test",
		TicketIDs:    []string{"TK-E"},
	})
	if err != nil {
		t.Fatalf("CreateChain failed: %v", err)
	}

	wfiE := "wfi-tke-001"
	env.InitWorkflowWithID(t, "TK-E", wfiE)

	// Insert 1 completed agent with context_left=80
	insertSessionWithContextLeft(t, env, "sess-tke-1", "TK-E", wfiE,
		"analyzer", "setup-analyzer", "claude:sonnet", "completed", "pass", 80)

	// Insert 1 completed agent with NULL context_left (should be excluded)
	insertCompletedSession(t, env, "sess-tke-2", "TK-E", wfiE,
		"builder", "implementor", "claude:opus", "completed", "pass")

	// Link workflow instance to chain item
	itemRepo := repo.NewChainItemRepo(env.Pool)
	items, err := itemRepo.ListByChain(chain.ID)
	if err != nil {
		t.Fatalf("failed to list chain items: %v", err)
	}
	if err := itemRepo.SetWorkflowInstanceID(items[0].ID, wfiE); err != nil {
		t.Fatalf("failed to set workflow instance: %v", err)
	}

	// Retrieve chain with items
	retrieved, err := chainSvc.GetChainWithItems(chain.ID)
	if err != nil {
		t.Fatalf("GetChainWithItems failed: %v", err)
	}

	// Only the agent with context_left=80 should be counted: 200000*(100-80)/100 = 40000
	expectedTokens := int64(40000)
	if retrieved.Items[0].TotalTokensUsed != expectedTokens {
		t.Errorf("expected total_tokens_used %d (excluding NULL context_left), got %d",
			expectedTokens, retrieved.Items[0].TotalTokensUsed)
	}
}

// TestChainItemTokensUsed_BoundaryValues verifies token calculation at boundary
// values: context_left=0 (fully consumed) and context_left=100 (no consumption).
func TestChainItemTokensUsed_BoundaryValues(t *testing.T) {
	env := NewTestEnv(t)

	createChainTickets(t, env, map[string]time.Time{
		"TK-F": time.Now(),
		"TK-G": time.Now().Add(time.Second),
	})

	chainSvc := service.NewChainService(env.Pool)
	chain, err := chainSvc.CreateChain(env.ProjectID, &types.ChainCreateRequest{
		Name:         "Boundary Values Chain",
		WorkflowName: "test",
		TicketIDs:    []string{"TK-F", "TK-G"},
	})
	if err != nil {
		t.Fatalf("CreateChain failed: %v", err)
	}

	// TK-F: context_left=0 (fully consumed) → 200000 tokens
	wfiF := "wfi-tkf-001"
	env.InitWorkflowWithID(t, "TK-F", wfiF)
	insertSessionWithContextLeft(t, env, "sess-tkf-1", "TK-F", wfiF,
		"analyzer", "setup-analyzer", "claude:sonnet", "completed", "pass", 0)

	// TK-G: context_left=100 (no consumption) → 0 tokens
	wfiG := "wfi-tkg-001"
	env.InitWorkflowWithID(t, "TK-G", wfiG)
	insertSessionWithContextLeft(t, env, "sess-tkg-1", "TK-G", wfiG,
		"analyzer", "setup-analyzer", "claude:sonnet", "completed", "pass", 100)

	// Link workflow instances to chain items
	itemRepo := repo.NewChainItemRepo(env.Pool)
	items, err := itemRepo.ListByChain(chain.ID)
	if err != nil {
		t.Fatalf("failed to list chain items: %v", err)
	}

	for _, item := range items {
		if item.TicketID == "tk-f" {
			if err := itemRepo.SetWorkflowInstanceID(item.ID, wfiF); err != nil {
				t.Fatalf("failed to set workflow instance for TK-F: %v", err)
			}
		} else if item.TicketID == "tk-g" {
			if err := itemRepo.SetWorkflowInstanceID(item.ID, wfiG); err != nil {
				t.Fatalf("failed to set workflow instance for TK-G: %v", err)
			}
		}
	}

	// Retrieve chain with items
	retrieved, err := chainSvc.GetChainWithItems(chain.ID)
	if err != nil {
		t.Fatalf("GetChainWithItems failed: %v", err)
	}

	expectedTokens := map[string]int64{
		"tk-f": 200000, // context_left=0 → 200000*(100-0)/100 = 200000
		"tk-g": 0,      // context_left=100 → 200000*(100-100)/100 = 0
	}

	for _, item := range retrieved.Items {
		expected, ok := expectedTokens[item.TicketID]
		if !ok {
			t.Errorf("unexpected ticket ID: %s", item.TicketID)
			continue
		}
		if item.TotalTokensUsed != expected {
			t.Errorf("ticket %s: expected total_tokens_used %d, got %d",
				item.TicketID, expected, item.TotalTokensUsed)
		}
	}
}

// TestChainItemTokensUsed_JSONOmitsZero verifies that MarshalJSON includes
// total_tokens_used in the JSON output with omitempty behavior.
func TestChainItemTokensUsed_JSONOmitsZero(t *testing.T) {
	env := NewTestEnv(t)

	createChainTickets(t, env, map[string]time.Time{
		"TK-H": time.Now(),
	})

	chainSvc := service.NewChainService(env.Pool)
	chain, err := chainSvc.CreateChain(env.ProjectID, &types.ChainCreateRequest{
		Name:         "JSON Test Chain",
		WorkflowName: "test",
		TicketIDs:    []string{"TK-H"},
	})
	if err != nil {
		t.Fatalf("CreateChain failed: %v", err)
	}

	wfiH := "wfi-tkh-001"
	env.InitWorkflowWithID(t, "TK-H", wfiH)
	insertSessionWithContextLeft(t, env, "sess-tkh-1", "TK-H", wfiH,
		"analyzer", "setup-analyzer", "claude:sonnet", "completed", "pass", 50)

	// Link workflow instance to chain item
	itemRepo := repo.NewChainItemRepo(env.Pool)
	items, err := itemRepo.ListByChain(chain.ID)
	if err != nil {
		t.Fatalf("failed to list chain items: %v", err)
	}
	if err := itemRepo.SetWorkflowInstanceID(items[0].ID, wfiH); err != nil {
		t.Fatalf("failed to set workflow instance: %v", err)
	}

	// Retrieve chain with items
	retrieved, err := chainSvc.GetChainWithItems(chain.ID)
	if err != nil {
		t.Fatalf("GetChainWithItems failed: %v", err)
	}

	// Marshal to JSON and verify total_tokens_used is present
	data, err := json.Marshal(retrieved.Items[0])
	if err != nil {
		t.Fatalf("failed to marshal item: %v", err)
	}

	var jsonMap map[string]interface{}
	if err := json.Unmarshal(data, &jsonMap); err != nil {
		t.Fatalf("failed to unmarshal JSON: %v", err)
	}

	// Should include total_tokens_used (100000) since it's non-zero
	tokensUsed, ok := jsonMap["total_tokens_used"].(float64)
	if !ok {
		t.Fatalf("expected total_tokens_used in JSON, got %v", jsonMap)
	}
	if int64(tokensUsed) != 100000 {
		t.Errorf("expected total_tokens_used 100000 in JSON, got %v", tokensUsed)
	}
}
