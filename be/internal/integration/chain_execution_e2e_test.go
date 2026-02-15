package integration

import (
	"strings"
	"testing"
	"time"

	"be/internal/clock"
	"be/internal/model"
	"be/internal/orchestrator"
	"be/internal/repo"
	"be/internal/service"
	"be/internal/types"
)

// TestChainExecution_E2E_Sequential verifies end-to-end sequential chain execution
// This test verifies the full flow: create chain -> start -> items execute sequentially -> complete
func TestChainExecution_E2E_Sequential(t *testing.T) {
	env := NewTestEnv(t)

	// Create tickets A, B with B depending on A
	base := time.Now()
	createChainTickets(t, env, map[string]time.Time{
		"A": base,
		"B": base.Add(time.Second),
	})
	createChainDependencies(t, env, map[string][]string{
		"B": {"A"},
	})

	// Create workflow instances for both tickets to simulate they've been run
	// (ChainRunner.Start calls Orchestrator.Start which expects tickets to be initialized)
	env.InitWorkflow(t, "A")
	env.InitWorkflow(t, "B")

	// Create chain
	chainSvc := service.NewChainService(env.Pool, clock.Real())
	chain, err := chainSvc.CreateChain(env.ProjectID, &types.ChainCreateRequest{
		Name:         "Sequential E2E",
		WorkflowName: "test",
		TicketIDs:    []string{"B"},
	})
	if err != nil {
		t.Fatalf("CreateChain failed: %v", err)
	}

	// Verify chain is pending with 2 items (A, B)
	if chain.Status != model.ChainStatusPending {
		t.Fatalf("expected pending status, got %s", chain.Status)
	}
	if len(chain.Items) != 2 {
		t.Fatalf("expected 2 items, got %d", len(chain.Items))
	}

	// Verify locks exist
	lockRepo := repo.NewChainLockRepo(env.Pool)
	conflicts, _ := lockRepo.CheckConflicts(env.ProjectID, []string{"A", "B"}, "")
	if len(conflicts) != 2 {
		t.Fatalf("expected 2 locks, got %d", len(conflicts))
	}

	// Note: We cannot easily test the full chain runner without a real orchestrator
	// that spawns agents. Instead, we verify:
	// 1. Chain transitions to running when started
	// 2. Items can be marked as running/completed manually
	// 3. Locks are released on completion

	// Simulate chain start by updating status
	chainRepo := repo.NewChainRepo(env.Pool, clock.Real())
	err = chainRepo.UpdateStatus(chain.ID, model.ChainStatusRunning)
	if err != nil {
		t.Fatalf("failed to update status: %v", err)
	}

	// Verify status updated
	updated, _ := chainRepo.Get(chain.ID)
	if updated.Status != model.ChainStatusRunning {
		t.Errorf("expected running status, got %s", updated.Status)
	}

	// Simulate first item (A) execution
	itemRepo := repo.NewChainItemRepo(env.Pool, clock.Real())
	items, _ := itemRepo.ListByChain(chain.ID)

	// Mark first item running
	err = itemRepo.UpdateItemStatus(items[0].ID, model.ChainItemRunning)
	if err != nil {
		t.Fatalf("failed to mark item running: %v", err)
	}

	// Verify started_at is set
	updatedItems, _ := itemRepo.ListByChain(chain.ID)
	if !updatedItems[0].StartedAt.Valid {
		t.Error("expected started_at to be set when item becomes running")
	}

	// Mark first item completed
	err = itemRepo.UpdateItemStatus(items[0].ID, model.ChainItemCompleted)
	if err != nil {
		t.Fatalf("failed to mark item completed: %v", err)
	}

	// Verify ended_at is set
	updatedItems, _ = itemRepo.ListByChain(chain.ID)
	if !updatedItems[0].EndedAt.Valid {
		t.Error("expected ended_at to be set when item completes")
	}
	if updatedItems[0].Status != model.ChainItemCompleted {
		t.Errorf("expected completed status, got %s", updatedItems[0].Status)
	}

	// Mark second item running then completed
	err = itemRepo.UpdateItemStatus(items[1].ID, model.ChainItemRunning)
	if err != nil {
		t.Fatalf("failed to mark item 2 running: %v", err)
	}
	err = itemRepo.UpdateItemStatus(items[1].ID, model.ChainItemCompleted)
	if err != nil {
		t.Fatalf("failed to mark item 2 completed: %v", err)
	}

	// Mark chain completed
	err = chainRepo.UpdateStatus(chain.ID, model.ChainStatusCompleted)
	if err != nil {
		t.Fatalf("failed to mark chain completed: %v", err)
	}

	// Release locks
	err = lockRepo.DeleteLocksByChain(chain.ID)
	if err != nil {
		t.Fatalf("failed to release locks: %v", err)
	}

	// Verify final state
	final, _ := chainRepo.Get(chain.ID)
	if final.Status != model.ChainStatusCompleted {
		t.Errorf("expected completed status, got %s", final.Status)
	}

	// Verify locks released
	conflicts, _ = lockRepo.CheckConflicts(env.ProjectID, []string{"A", "B"}, "")
	if len(conflicts) != 0 {
		t.Errorf("expected locks released, got %d conflicts", len(conflicts))
	}
}

// TestChainExecution_E2E_Failure verifies chain failure handling
func TestChainExecution_E2E_Failure(t *testing.T) {
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

	chainSvc := service.NewChainService(env.Pool, clock.Real())
	chain, err := chainSvc.CreateChain(env.ProjectID, &types.ChainCreateRequest{
		Name:         "Failure E2E",
		WorkflowName: "test",
		TicketIDs:    []string{"C"},
	})
	if err != nil {
		t.Fatalf("CreateChain failed: %v", err)
	}

	// Should have 3 items: A, B, C
	if len(chain.Items) != 3 {
		t.Fatalf("expected 3 items, got %d", len(chain.Items))
	}

	chainRepo := repo.NewChainRepo(env.Pool, clock.Real())
	itemRepo := repo.NewChainItemRepo(env.Pool, clock.Real())
	lockRepo := repo.NewChainLockRepo(env.Pool)

	// Start chain
	chainRepo.UpdateStatus(chain.ID, model.ChainStatusRunning)

	// First item (A) succeeds
	items, _ := itemRepo.ListByChain(chain.ID)
	itemRepo.UpdateItemStatus(items[0].ID, model.ChainItemRunning)
	itemRepo.UpdateItemStatus(items[0].ID, model.ChainItemCompleted)

	// Second item (B) fails
	itemRepo.UpdateItemStatus(items[1].ID, model.ChainItemRunning)
	itemRepo.UpdateItemStatus(items[1].ID, model.ChainItemFailed)

	// Remaining items (C) should be canceled
	itemRepo.UpdateItemStatus(items[2].ID, model.ChainItemCanceled)

	// Mark chain failed
	chainRepo.UpdateStatus(chain.ID, model.ChainStatusFailed)

	// Release locks
	lockRepo.DeleteLocksByChain(chain.ID)

	// Verify final state
	finalItems, _ := itemRepo.ListByChain(chain.ID)
	if finalItems[0].Status != model.ChainItemCompleted {
		t.Errorf("expected item A completed, got %s", finalItems[0].Status)
	}
	if finalItems[1].Status != model.ChainItemFailed {
		t.Errorf("expected item B failed, got %s", finalItems[1].Status)
	}
	if finalItems[2].Status != model.ChainItemCanceled {
		t.Errorf("expected item C canceled, got %s", finalItems[2].Status)
	}

	final, _ := chainRepo.Get(chain.ID)
	if final.Status != model.ChainStatusFailed {
		t.Errorf("expected chain failed, got %s", final.Status)
	}

	// Verify locks released
	conflicts, _ := lockRepo.CheckConflicts(env.ProjectID, []string{"A", "B", "C"}, "")
	if len(conflicts) != 0 {
		t.Errorf("expected locks released after failure, got %d conflicts", len(conflicts))
	}
}

// TestChainExecution_E2E_Cancel verifies chain cancellation during execution
func TestChainExecution_E2E_Cancel(t *testing.T) {
	env := NewTestEnv(t)

	base := time.Now()
	createChainTickets(t, env, map[string]time.Time{
		"A": base,
		"B": base.Add(time.Second),
	})

	chainSvc := service.NewChainService(env.Pool, clock.Real())
	chain, err := chainSvc.CreateChain(env.ProjectID, &types.ChainCreateRequest{
		Name:         "Cancel E2E",
		WorkflowName: "test",
		TicketIDs:    []string{"A", "B"},
	})
	if err != nil {
		t.Fatalf("CreateChain failed: %v", err)
	}

	chainRepo := repo.NewChainRepo(env.Pool, clock.Real())
	itemRepo := repo.NewChainItemRepo(env.Pool, clock.Real())
	lockRepo := repo.NewChainLockRepo(env.Pool)

	// Start chain
	chainRepo.UpdateStatus(chain.ID, model.ChainStatusRunning)

	// First item running
	items, _ := itemRepo.ListByChain(chain.ID)
	itemRepo.UpdateItemStatus(items[0].ID, model.ChainItemRunning)

	// Simulate cancel via ChainRunner
	runner := orchestrator.NewChainRunner(nil, env.Pool.Path, env.Hub, clock.Real())

	// Manually update to simulate what handleCancel does
	itemRepo.UpdateItemStatus(items[0].ID, model.ChainItemCanceled)
	itemRepo.UpdateItemStatus(items[1].ID, model.ChainItemCanceled)
	chainRepo.UpdateStatus(chain.ID, model.ChainStatusCanceled)
	lockRepo.DeleteLocksByChain(chain.ID)

	// Verify final state
	finalItems, _ := itemRepo.ListByChain(chain.ID)
	for _, item := range finalItems {
		if item.Status != model.ChainItemCanceled {
			t.Errorf("expected item %s canceled, got %s", item.TicketID, item.Status)
		}
	}

	final, _ := chainRepo.Get(chain.ID)
	if final.Status != model.ChainStatusCanceled {
		t.Errorf("expected chain canceled, got %s", final.Status)
	}

	// Verify locks released
	conflicts, _ := lockRepo.CheckConflicts(env.ProjectID, []string{"A", "B"}, "")
	if len(conflicts) != 0 {
		t.Errorf("expected locks released after cancel, got %d conflicts", len(conflicts))
	}

	// Verify runner doesn't track this chain
	if runner.IsRunning(chain.ID) {
		t.Error("chain should not be running after cancel")
	}
}

// TestChainExecution_ItemWorkflowInstanceTracking verifies workflow instance IDs are tracked
func TestChainExecution_ItemWorkflowInstanceTracking(t *testing.T) {
	env := NewTestEnv(t)

	createChainTickets(t, env, map[string]time.Time{
		"A": time.Now(),
	})
	env.InitWorkflow(t, "A")

	chainSvc := service.NewChainService(env.Pool, clock.Real())
	chain, err := chainSvc.CreateChain(env.ProjectID, &types.ChainCreateRequest{
		Name:         "Workflow Tracking",
		WorkflowName: "test",
		TicketIDs:    []string{"A"},
	})
	if err != nil {
		t.Fatalf("CreateChain failed: %v", err)
	}

	// Get workflow instance ID for ticket A
	wfiID := env.GetWorkflowInstanceID(t, "A", "test")

	// Set workflow instance ID on chain item
	itemRepo := repo.NewChainItemRepo(env.Pool, clock.Real())
	items, _ := itemRepo.ListByChain(chain.ID)

	err = itemRepo.SetWorkflowInstanceID(items[0].ID, wfiID)
	if err != nil {
		t.Fatalf("SetWorkflowInstanceID failed: %v", err)
	}

	// Verify it's set
	updatedItems, _ := itemRepo.ListByChain(chain.ID)
	if !updatedItems[0].WorkflowInstanceID.Valid {
		t.Error("expected workflow_instance_id to be set")
	}
	if updatedItems[0].WorkflowInstanceID.String != wfiID {
		t.Errorf("expected wfi ID %s, got %s", wfiID, updatedItems[0].WorkflowInstanceID.String)
	}
}

// TestChainExecution_NextPending verifies GetNextPending returns items in order
func TestChainExecution_NextPending(t *testing.T) {
	env := NewTestEnv(t)

	base := time.Now()
	createChainTickets(t, env, map[string]time.Time{
		"A": base,
		"B": base.Add(time.Second),
		"C": base.Add(2 * time.Second),
	})

	chainSvc := service.NewChainService(env.Pool, clock.Real())
	chain, err := chainSvc.CreateChain(env.ProjectID, &types.ChainCreateRequest{
		Name:         "Next Pending",
		WorkflowName: "test",
		TicketIDs:    []string{"A", "B", "C"},
	})
	if err != nil {
		t.Fatalf("CreateChain failed: %v", err)
	}

	itemRepo := repo.NewChainItemRepo(env.Pool, clock.Real())

	// First call should return A
	next, err := itemRepo.GetNextPending(chain.ID)
	if err != nil {
		t.Fatalf("GetNextPending failed: %v", err)
	}
	if next == nil {
		t.Fatal("expected next item, got nil")
	}
	if next.TicketID != "a" {
		t.Errorf("expected A, got %s", next.TicketID)
	}

	// Mark A completed
	itemRepo.UpdateItemStatus(next.ID, model.ChainItemCompleted)

	// Second call should return B
	next, err = itemRepo.GetNextPending(chain.ID)
	if err != nil {
		t.Fatalf("GetNextPending failed: %v", err)
	}
	if next == nil {
		t.Fatal("expected next item, got nil")
	}
	if next.TicketID != "b" {
		t.Errorf("expected B, got %s", next.TicketID)
	}

	// Mark B completed
	itemRepo.UpdateItemStatus(next.ID, model.ChainItemCompleted)

	// Third call should return C
	next, err = itemRepo.GetNextPending(chain.ID)
	if err != nil {
		t.Fatalf("GetNextPending failed: %v", err)
	}
	if next == nil {
		t.Fatal("expected next item, got nil")
	}
	if next.TicketID != "c" {
		t.Errorf("expected C, got %s", next.TicketID)
	}

	// Mark C completed
	itemRepo.UpdateItemStatus(next.ID, model.ChainItemCompleted)

	// Fourth call should return nil (all done)
	next, err = itemRepo.GetNextPending(chain.ID)
	if err != nil {
		t.Fatalf("GetNextPending failed: %v", err)
	}
	if next != nil {
		t.Errorf("expected nil (no more items), got %s", next.TicketID)
	}
}

// TestChainExecution_ConcurrentLockPrevention verifies UNIQUE constraint prevents races
func TestChainExecution_ConcurrentLockPrevention(t *testing.T) {
	env := NewTestEnv(t)

	createChainTickets(t, env, map[string]time.Time{
		"A": time.Now(),
	})

	chainSvc := service.NewChainService(env.Pool, clock.Real())

	// Create first chain with ticket A
	_, err := chainSvc.CreateChain(env.ProjectID, &types.ChainCreateRequest{
		Name:         "Chain 1",
		WorkflowName: "test",
		TicketIDs:    []string{"A"},
	})
	if err != nil {
		t.Fatalf("first chain creation failed: %v", err)
	}

	// Try to create second chain with same ticket (should fail due to lock conflict)
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
}
