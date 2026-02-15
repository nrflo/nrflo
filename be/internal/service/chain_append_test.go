package service

import (
	"strings"
	"testing"
	"time"

	"be/internal/clock"
	"be/internal/model"
	"be/internal/repo"
	"be/internal/types"
)

// TestAppendToChain_RunningChainSucceeds verifies appending to a running chain
func TestAppendToChain_RunningChainSucceeds(t *testing.T) {
	pool, projectID := setupChainTestDB(t)
	defer pool.Close()

	now := time.Now()
	createTestTickets(t, pool, projectID, map[string]time.Time{
		"A": now,
		"B": now.Add(time.Second),
		"C": now.Add(2 * time.Second),
	})

	// Create a running chain with ticket A
	svc := NewChainService(pool, clock.Real())
	chain, err := svc.CreateChain(projectID, &types.ChainCreateRequest{
		Name:         "Test Chain",
		WorkflowName: "test",
		TicketIDs:    []string{"A"},
	})
	if err != nil {
		t.Fatalf("CreateChain failed: %v", err)
	}

	// Mark chain as running
	chainRepo := repo.NewChainRepo(pool, clock.Real())
	if err := chainRepo.UpdateStatus(chain.ID, model.ChainStatusRunning); err != nil {
		t.Fatalf("failed to mark chain as running: %v", err)
	}

	// Append tickets B and C
	updated, err := svc.AppendToChain(chain.ID, &types.ChainAppendRequest{
		TicketIDs: []string{"B", "C"},
	})
	if err != nil {
		t.Fatalf("AppendToChain failed: %v", err)
	}

	// Verify 3 items total
	if len(updated.Items) != 3 {
		t.Fatalf("expected 3 items, got %d", len(updated.Items))
	}

	// Verify positions: A=0, B=1, C=2
	if updated.Items[0].TicketID != "a" || updated.Items[0].Position != 0 {
		t.Errorf("expected item 0: A at position 0, got %s at position %d", updated.Items[0].TicketID, updated.Items[0].Position)
	}
	if updated.Items[1].TicketID != "b" || updated.Items[1].Position != 1 {
		t.Errorf("expected item 1: B at position 1, got %s at position %d", updated.Items[1].TicketID, updated.Items[1].Position)
	}
	if updated.Items[2].TicketID != "c" || updated.Items[2].Position != 2 {
		t.Errorf("expected item 2: C at position 2, got %s at position %d", updated.Items[2].TicketID, updated.Items[2].Position)
	}

	// Verify new items are pending
	if updated.Items[1].Status != model.ChainItemPending {
		t.Errorf("expected B to be pending, got %s", updated.Items[1].Status)
	}
	if updated.Items[2].Status != model.ChainItemPending {
		t.Errorf("expected C to be pending, got %s", updated.Items[2].Status)
	}
}

// TestAppendToChain_PendingChainFails verifies appending to pending chain fails
func TestAppendToChain_PendingChainFails(t *testing.T) {
	pool, projectID := setupChainTestDB(t)
	defer pool.Close()

	now := time.Now()
	createTestTickets(t, pool, projectID, map[string]time.Time{
		"A": now,
		"B": now.Add(time.Second),
	})

	svc := NewChainService(pool, clock.Real())
	chain, err := svc.CreateChain(projectID, &types.ChainCreateRequest{
		Name:         "Pending Chain",
		WorkflowName: "test",
		TicketIDs:    []string{"A"},
	})
	if err != nil {
		t.Fatalf("CreateChain failed: %v", err)
	}

	// Chain is pending by default - try to append
	_, err = svc.AppendToChain(chain.ID, &types.ChainAppendRequest{
		TicketIDs: []string{"B"},
	})
	if err == nil {
		t.Fatal("expected error appending to pending chain, got nil")
	}
	if !strings.Contains(err.Error(), "can only append to running chains") {
		t.Errorf("expected 'can only append to running chains' error, got: %v", err)
	}
}

// TestAppendToChain_CompletedChainFails verifies appending to completed chain fails
func TestAppendToChain_CompletedChainFails(t *testing.T) {
	pool, projectID := setupChainTestDB(t)
	defer pool.Close()

	now := time.Now()
	createTestTickets(t, pool, projectID, map[string]time.Time{
		"A": now,
		"B": now.Add(time.Second),
	})

	svc := NewChainService(pool, clock.Real())
	chain, err := svc.CreateChain(projectID, &types.ChainCreateRequest{
		Name:         "Completed Chain",
		WorkflowName: "test",
		TicketIDs:    []string{"A"},
	})
	if err != nil {
		t.Fatalf("CreateChain failed: %v", err)
	}

	// Mark chain as completed
	chainRepo := repo.NewChainRepo(pool, clock.Real())
	if err := chainRepo.UpdateStatus(chain.ID, model.ChainStatusCompleted); err != nil {
		t.Fatalf("failed to mark chain as completed: %v", err)
	}

	_, err = svc.AppendToChain(chain.ID, &types.ChainAppendRequest{
		TicketIDs: []string{"B"},
	})
	if err == nil {
		t.Fatal("expected error appending to completed chain, got nil")
	}
	if !strings.Contains(err.Error(), "can only append to running chains") {
		t.Errorf("expected 'can only append to running chains' error, got: %v", err)
	}
}

// TestAppendToChain_FailedChainFails verifies appending to failed chain fails
func TestAppendToChain_FailedChainFails(t *testing.T) {
	pool, projectID := setupChainTestDB(t)
	defer pool.Close()

	now := time.Now()
	createTestTickets(t, pool, projectID, map[string]time.Time{
		"A": now,
		"B": now.Add(time.Second),
	})

	svc := NewChainService(pool, clock.Real())
	chain, err := svc.CreateChain(projectID, &types.ChainCreateRequest{
		Name:         "Failed Chain",
		WorkflowName: "test",
		TicketIDs:    []string{"A"},
	})
	if err != nil {
		t.Fatalf("CreateChain failed: %v", err)
	}

	// Mark chain as failed
	chainRepo := repo.NewChainRepo(pool, clock.Real())
	if err := chainRepo.UpdateStatus(chain.ID, model.ChainStatusFailed); err != nil {
		t.Fatalf("failed to mark chain as failed: %v", err)
	}

	_, err = svc.AppendToChain(chain.ID, &types.ChainAppendRequest{
		TicketIDs: []string{"B"},
	})
	if err == nil {
		t.Fatal("expected error appending to failed chain, got nil")
	}
	if !strings.Contains(err.Error(), "can only append to running chains") {
		t.Errorf("expected 'can only append to running chains' error, got: %v", err)
	}
}

// TestAppendToChain_DuplicateTicketExcluded verifies duplicate is silently excluded
func TestAppendToChain_DuplicateTicketExcluded(t *testing.T) {
	pool, projectID := setupChainTestDB(t)
	defer pool.Close()

	now := time.Now()
	createTestTickets(t, pool, projectID, map[string]time.Time{
		"A": now,
		"B": now.Add(time.Second),
	})

	svc := NewChainService(pool, clock.Real())
	chain, err := svc.CreateChain(projectID, &types.ChainCreateRequest{
		Name:         "Test Chain",
		WorkflowName: "test",
		TicketIDs:    []string{"A", "B"},
	})
	if err != nil {
		t.Fatalf("CreateChain failed: %v", err)
	}

	// Mark chain as running
	chainRepo := repo.NewChainRepo(pool, clock.Real())
	if err := chainRepo.UpdateStatus(chain.ID, model.ChainStatusRunning); err != nil {
		t.Fatalf("failed to mark chain as running: %v", err)
	}

	// Try to append ticket A again (already in chain)
	updated, err := svc.AppendToChain(chain.ID, &types.ChainAppendRequest{
		TicketIDs: []string{"A"},
	})
	if err != nil {
		t.Fatalf("AppendToChain failed: %v", err)
	}

	// Should still have 2 items, no duplicates
	if len(updated.Items) != 2 {
		t.Errorf("expected 2 items (no duplicate), got %d", len(updated.Items))
	}
}

// TestAppendToChain_LockedTicketFails verifies lock conflict detection
func TestAppendToChain_LockedTicketFails(t *testing.T) {
	pool, projectID := setupChainTestDB(t)
	defer pool.Close()

	now := time.Now()
	createTestTickets(t, pool, projectID, map[string]time.Time{
		"A": now,
		"B": now.Add(time.Second),
		"C": now.Add(2 * time.Second),
	})

	svc := NewChainService(pool, clock.Real())

	// Create chain 1 with ticket B
	_, err := svc.CreateChain(projectID, &types.ChainCreateRequest{
		Name:         "Chain 1",
		WorkflowName: "test",
		TicketIDs:    []string{"B"},
	})
	if err != nil {
		t.Fatalf("CreateChain 1 failed: %v", err)
	}

	// Create chain 2 with ticket A, mark as running
	chain2, err := svc.CreateChain(projectID, &types.ChainCreateRequest{
		Name:         "Chain 2",
		WorkflowName: "test",
		TicketIDs:    []string{"A"},
	})
	if err != nil {
		t.Fatalf("CreateChain 2 failed: %v", err)
	}

	chainRepo := repo.NewChainRepo(pool, clock.Real())
	if err := chainRepo.UpdateStatus(chain2.ID, model.ChainStatusRunning); err != nil {
		t.Fatalf("failed to mark chain 2 as running: %v", err)
	}

	// Try to append ticket B to chain 2 (locked by chain 1)
	_, err = svc.AppendToChain(chain2.ID, &types.ChainAppendRequest{
		TicketIDs: []string{"B"},
	})
	if err == nil {
		t.Fatal("expected error due to lock conflict, got nil")
	}
	if !strings.Contains(err.Error(), "already locked by another chain") {
		t.Errorf("expected 'already locked by another chain' error, got: %v", err)
	}
}

// TestAppendToChain_TransitiveBlockersExpanded verifies blocker expansion
func TestAppendToChain_TransitiveBlockersExpanded(t *testing.T) {
	pool, projectID := setupChainTestDB(t)
	defer pool.Close()

	now := time.Now()
	createTestTickets(t, pool, projectID, map[string]time.Time{
		"A": now,
		"B": now.Add(time.Second),
		"C": now.Add(2 * time.Second),
		"D": now.Add(3 * time.Second),
	})

	// Create dependencies: D -> C -> B -> A (transitive chain)
	createTestDependencies(t, pool, projectID, map[string][]string{
		"B": {"A"},
		"C": {"B"},
		"D": {"C"},
	})

	svc := NewChainService(pool, clock.Real())

	// Create chain with just A, mark as running
	chain, err := svc.CreateChain(projectID, &types.ChainCreateRequest{
		Name:         "Test Chain",
		WorkflowName: "test",
		TicketIDs:    []string{"A"},
	})
	if err != nil {
		t.Fatalf("CreateChain failed: %v", err)
	}

	chainRepo := repo.NewChainRepo(pool, clock.Real())
	if err := chainRepo.UpdateStatus(chain.ID, model.ChainStatusRunning); err != nil {
		t.Fatalf("failed to mark chain as running: %v", err)
	}

	// Append D - should expand to include C, B (A already in chain)
	updated, err := svc.AppendToChain(chain.ID, &types.ChainAppendRequest{
		TicketIDs: []string{"D"},
	})
	if err != nil {
		t.Fatalf("AppendToChain failed: %v", err)
	}

	// Should have 4 items: A, B, C, D in topological order
	if len(updated.Items) != 4 {
		t.Fatalf("expected 4 items (A + expanded B,C,D), got %d", len(updated.Items))
	}

	// Verify topological order: A -> B -> C -> D
	if updated.Items[0].TicketID != "a" {
		t.Errorf("expected item 0 to be A, got %s", updated.Items[0].TicketID)
	}
	if updated.Items[1].TicketID != "b" {
		t.Errorf("expected item 1 to be B, got %s", updated.Items[1].TicketID)
	}
	if updated.Items[2].TicketID != "c" {
		t.Errorf("expected item 2 to be C, got %s", updated.Items[2].TicketID)
	}
	if updated.Items[3].TicketID != "d" {
		t.Errorf("expected item 3 to be D, got %s", updated.Items[3].TicketID)
	}

	// Verify positions
	if updated.Items[1].Position != 1 || updated.Items[2].Position != 2 || updated.Items[3].Position != 3 {
		t.Errorf("expected positions 1,2,3 for new items, got %d,%d,%d",
			updated.Items[1].Position, updated.Items[2].Position, updated.Items[3].Position)
	}
}

// TestAppendToChain_BlockerAlreadyInChainExcluded verifies blockers already in chain are filtered out
func TestAppendToChain_BlockerAlreadyInChainExcluded(t *testing.T) {
	pool, projectID := setupChainTestDB(t)
	defer pool.Close()

	now := time.Now()
	createTestTickets(t, pool, projectID, map[string]time.Time{
		"A": now,
		"B": now.Add(time.Second),
		"C": now.Add(2 * time.Second),
	})

	// C -> B -> A
	createTestDependencies(t, pool, projectID, map[string][]string{
		"B": {"A"},
		"C": {"B"},
	})

	svc := NewChainService(pool, clock.Real())

	// Create chain with A and B (B's blocker A is already in)
	chain, err := svc.CreateChain(projectID, &types.ChainCreateRequest{
		Name:         "Test Chain",
		WorkflowName: "test",
		TicketIDs:    []string{"A", "B"},
	})
	if err != nil {
		t.Fatalf("CreateChain failed: %v", err)
	}

	chainRepo := repo.NewChainRepo(pool, clock.Real())
	if err := chainRepo.UpdateStatus(chain.ID, model.ChainStatusRunning); err != nil {
		t.Fatalf("failed to mark chain as running: %v", err)
	}

	// Append C - blocker B is already in chain, should not duplicate
	updated, err := svc.AppendToChain(chain.ID, &types.ChainAppendRequest{
		TicketIDs: []string{"C"},
	})
	if err != nil {
		t.Fatalf("AppendToChain failed: %v", err)
	}

	// Should have 3 items total: A, B, C (B not duplicated)
	if len(updated.Items) != 3 {
		t.Fatalf("expected 3 items (no duplicate B), got %d", len(updated.Items))
	}

	// Verify order: A, B, C
	if updated.Items[0].TicketID != "a" || updated.Items[1].TicketID != "b" || updated.Items[2].TicketID != "c" {
		t.Errorf("expected A,B,C order, got %s,%s,%s",
			updated.Items[0].TicketID, updated.Items[1].TicketID, updated.Items[2].TicketID)
	}
}

// TestAppendToChain_CycleDetection verifies cycle detection among new tickets
func TestAppendToChain_CycleDetection(t *testing.T) {
	pool, projectID := setupChainTestDB(t)
	defer pool.Close()

	now := time.Now()
	createTestTickets(t, pool, projectID, map[string]time.Time{
		"A": now,
		"B": now.Add(time.Second),
		"C": now.Add(2 * time.Second),
	})

	// Create a cycle: B -> C -> B
	createTestDependencies(t, pool, projectID, map[string][]string{
		"B": {"C"},
		"C": {"B"},
	})

	svc := NewChainService(pool, clock.Real())

	// Create chain with A, mark as running
	chain, err := svc.CreateChain(projectID, &types.ChainCreateRequest{
		Name:         "Test Chain",
		WorkflowName: "test",
		TicketIDs:    []string{"A"},
	})
	if err != nil {
		t.Fatalf("CreateChain failed: %v", err)
	}

	chainRepo := repo.NewChainRepo(pool, clock.Real())
	if err := chainRepo.UpdateStatus(chain.ID, model.ChainStatusRunning); err != nil {
		t.Fatalf("failed to mark chain as running: %v", err)
	}

	// Try to append B (which has cycle with C)
	_, err = svc.AppendToChain(chain.ID, &types.ChainAppendRequest{
		TicketIDs: []string{"B"},
	})
	if err == nil {
		t.Fatal("expected error due to cycle, got nil")
	}
	if !strings.Contains(err.Error(), "cycle detected") {
		t.Errorf("expected 'cycle detected' error, got: %v", err)
	}
}

// TestAppendToChain_EmptyAfterDedup verifies empty ticket list after dedup returns chain as-is
func TestAppendToChain_EmptyAfterDedup(t *testing.T) {
	pool, projectID := setupChainTestDB(t)
	defer pool.Close()

	now := time.Now()
	createTestTickets(t, pool, projectID, map[string]time.Time{
		"A": now,
		"B": now.Add(time.Second),
	})

	svc := NewChainService(pool, clock.Real())
	chain, err := svc.CreateChain(projectID, &types.ChainCreateRequest{
		Name:         "Test Chain",
		WorkflowName: "test",
		TicketIDs:    []string{"A", "B"},
	})
	if err != nil {
		t.Fatalf("CreateChain failed: %v", err)
	}

	chainRepo := repo.NewChainRepo(pool, clock.Real())
	if err := chainRepo.UpdateStatus(chain.ID, model.ChainStatusRunning); err != nil {
		t.Fatalf("failed to mark chain as running: %v", err)
	}

	// Append tickets already in chain
	updated, err := svc.AppendToChain(chain.ID, &types.ChainAppendRequest{
		TicketIDs: []string{"A", "B"},
	})
	if err != nil {
		t.Fatalf("AppendToChain failed: %v", err)
	}

	// Should return chain as-is with no changes
	if len(updated.Items) != 2 {
		t.Errorf("expected 2 items unchanged, got %d", len(updated.Items))
	}
}

// TestAppendToChain_EmptyTicketListFails verifies empty request fails
func TestAppendToChain_EmptyTicketListFails(t *testing.T) {
	pool, projectID := setupChainTestDB(t)
	defer pool.Close()

	now := time.Now()
	createTestTickets(t, pool, projectID, map[string]time.Time{
		"A": now,
	})

	svc := NewChainService(pool, clock.Real())
	chain, err := svc.CreateChain(projectID, &types.ChainCreateRequest{
		Name:         "Test Chain",
		WorkflowName: "test",
		TicketIDs:    []string{"A"},
	})
	if err != nil {
		t.Fatalf("CreateChain failed: %v", err)
	}

	chainRepo := repo.NewChainRepo(pool, clock.Real())
	if err := chainRepo.UpdateStatus(chain.ID, model.ChainStatusRunning); err != nil {
		t.Fatalf("failed to mark chain as running: %v", err)
	}

	// Try to append with empty list
	_, err = svc.AppendToChain(chain.ID, &types.ChainAppendRequest{
		TicketIDs: []string{},
	})
	if err == nil {
		t.Fatal("expected error with empty ticket list, got nil")
	}
	if !strings.Contains(err.Error(), "at least one ticket_id is required") {
		t.Errorf("expected 'at least one ticket_id is required' error, got: %v", err)
	}
}
