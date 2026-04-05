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

// getChainItemCount returns item count by listing from the service.
func getChainItemCount(t *testing.T, svc *ChainService, chainID string) int {
	t.Helper()
	chain, err := svc.GetChainWithItems(chainID)
	if err != nil {
		t.Fatalf("GetChainWithItems failed: %v", err)
	}
	return len(chain.Items)
}

// TestRemoveFromChain_PendingItemRemoved verifies that a pending item is deleted
// and its lock is released.
func TestRemoveFromChain_PendingItemRemoved(t *testing.T) {
	pool, projectID := setupChainTestDB(t)
	defer pool.Close()

	now := time.Now()
	createTestTickets(t, pool, projectID, map[string]time.Time{
		"A": now,
		"B": now.Add(time.Second),
		"C": now.Add(2 * time.Second),
	})

	svc := NewChainService(pool, clock.Real())
	chain, err := svc.CreateChain(projectID, &types.ChainCreateRequest{
		Name:         "Test Chain",
		WorkflowName: "test",
		TicketIDs:    []string{"A", "B", "C"},
	})
	if err != nil {
		t.Fatalf("CreateChain failed: %v", err)
	}

	chainRepo := repo.NewChainRepo(pool, clock.Real())
	if err := chainRepo.UpdateStatus(chain.ID, model.ChainStatusRunning); err != nil {
		t.Fatalf("UpdateStatus failed: %v", err)
	}

	// Mark A as running so it is immutable; B and C remain pending
	itemRepo := repo.NewChainItemRepo(pool, clock.Real())
	items, err := itemRepo.ListByChain(chain.ID)
	if err != nil {
		t.Fatalf("ListByChain failed: %v", err)
	}
	var itemAID string
	for _, it := range items {
		if it.TicketID == "a" {
			itemAID = it.ID
		}
	}
	if err := itemRepo.UpdateItemStatus(itemAID, model.ChainItemRunning); err != nil {
		t.Fatalf("UpdateItemStatus failed: %v", err)
	}

	// Remove B (pending)
	updated, err := svc.RemoveFromChain(chain.ID, []string{"B"})
	if err != nil {
		t.Fatalf("RemoveFromChain failed: %v", err)
	}

	// Should have 2 items (A and C remain)
	if len(updated.Items) != 2 {
		t.Errorf("expected 2 items after removal, got %d", len(updated.Items))
	}
	for _, it := range updated.Items {
		if it.TicketID == "b" {
			t.Errorf("ticket B should have been removed but is still present")
		}
	}

	// Verify lock for B is released
	lockRepo := repo.NewChainLockRepo(pool)
	conflicts, err := lockRepo.CheckConflicts(projectID, []string{"b"}, "")
	if err != nil {
		t.Fatalf("CheckConflicts failed: %v", err)
	}
	if len(conflicts) != 0 {
		t.Errorf("expected lock for B to be released, but conflicts found: %v", conflicts)
	}
}

// TestRemoveFromChain_MultipleItemsRemoved verifies bulk removal of pending items.
func TestRemoveFromChain_MultipleItemsRemoved(t *testing.T) {
	pool, projectID := setupChainTestDB(t)
	defer pool.Close()

	now := time.Now()
	createTestTickets(t, pool, projectID, map[string]time.Time{
		"A": now,
		"B": now.Add(time.Second),
		"C": now.Add(2 * time.Second),
		"D": now.Add(3 * time.Second),
	})

	svc := NewChainService(pool, clock.Real())
	chain, err := svc.CreateChain(projectID, &types.ChainCreateRequest{
		Name:         "Test Chain",
		WorkflowName: "test",
		TicketIDs:    []string{"A", "B", "C", "D"},
	})
	if err != nil {
		t.Fatalf("CreateChain failed: %v", err)
	}

	chainRepo := repo.NewChainRepo(pool, clock.Real())
	if err := chainRepo.UpdateStatus(chain.ID, model.ChainStatusRunning); err != nil {
		t.Fatalf("UpdateStatus failed: %v", err)
	}

	// Remove B and C (both pending)
	updated, err := svc.RemoveFromChain(chain.ID, []string{"B", "C"})
	if err != nil {
		t.Fatalf("RemoveFromChain failed: %v", err)
	}

	if len(updated.Items) != 2 {
		t.Errorf("expected 2 items (A, D), got %d", len(updated.Items))
	}

	// Locks for B and C should be released
	lockRepo := repo.NewChainLockRepo(pool)
	conflicts, err := lockRepo.CheckConflicts(projectID, []string{"b", "c"}, "")
	if err != nil {
		t.Fatalf("CheckConflicts failed: %v", err)
	}
	if len(conflicts) != 0 {
		t.Errorf("expected locks for B and C released, conflicts: %v", conflicts)
	}
}

// TestRemoveFromChain_EmptyTicketListFails verifies that an empty request fails.
func TestRemoveFromChain_EmptyTicketListFails(t *testing.T) {
	pool, projectID := setupChainTestDB(t)
	defer pool.Close()

	now := time.Now()
	createTestTickets(t, pool, projectID, map[string]time.Time{"A": now})

	svc := NewChainService(pool, clock.Real())
	chain, err := svc.CreateChain(projectID, &types.ChainCreateRequest{
		Name: "Test Chain", WorkflowName: "test", TicketIDs: []string{"A"},
	})
	if err != nil {
		t.Fatalf("CreateChain failed: %v", err)
	}

	chainRepo := repo.NewChainRepo(pool, clock.Real())
	if err := chainRepo.UpdateStatus(chain.ID, model.ChainStatusRunning); err != nil {
		t.Fatalf("UpdateStatus failed: %v", err)
	}

	_, err = svc.RemoveFromChain(chain.ID, []string{})
	if err == nil {
		t.Fatal("expected error for empty ticket list, got nil")
	}
	if !strings.Contains(err.Error(), "at least one ticket_id is required") {
		t.Errorf("expected 'at least one ticket_id is required', got: %v", err)
	}
}

// TestRemoveFromChain_NonRunningChainFails verifies rejection when chain is not running.
func TestRemoveFromChain_NonRunningChainFails(t *testing.T) {
	statuses := []model.ChainStatus{
		model.ChainStatusPending,
		model.ChainStatusCompleted,
		model.ChainStatusFailed,
		model.ChainStatusCanceled,
	}

	for _, status := range statuses {
		status := status
		t.Run(string(status), func(t *testing.T) {
			pool, projectID := setupChainTestDB(t)
			defer pool.Close()

			now := time.Now()
			createTestTickets(t, pool, projectID, map[string]time.Time{
				"A": now,
				"B": now.Add(time.Second),
			})

			svc := NewChainService(pool, clock.Real())
			chain, err := svc.CreateChain(projectID, &types.ChainCreateRequest{
				Name: "Test Chain", WorkflowName: "test", TicketIDs: []string{"A", "B"},
			})
			if err != nil {
				t.Fatalf("CreateChain failed: %v", err)
			}

			if status != model.ChainStatusPending {
				chainRepo := repo.NewChainRepo(pool, clock.Real())
				if err := chainRepo.UpdateStatus(chain.ID, status); err != nil {
					t.Fatalf("UpdateStatus failed: %v", err)
				}
			}

			_, err = svc.RemoveFromChain(chain.ID, []string{"B"})
			if err == nil {
				t.Fatalf("expected error for chain status %s, got nil", status)
			}
			if !strings.Contains(err.Error(), "can only remove items from running chains") {
				t.Errorf("expected 'can only remove items from running chains', got: %v", err)
			}
		})
	}
}

// TestRemoveFromChain_NonPendingItemFails verifies that non-pending items cannot be removed.
func TestRemoveFromChain_NonPendingItemFails(t *testing.T) {
	nonPendingStatuses := []model.ChainItemStatus{
		model.ChainItemRunning,
		model.ChainItemCompleted,
		model.ChainItemFailed,
	}

	for _, itemStatus := range nonPendingStatuses {
		itemStatus := itemStatus
		t.Run(string(itemStatus), func(t *testing.T) {
			pool, projectID := setupChainTestDB(t)
			defer pool.Close()

			now := time.Now()
			createTestTickets(t, pool, projectID, map[string]time.Time{
				"A": now,
				"B": now.Add(time.Second),
			})

			svc := NewChainService(pool, clock.Real())
			chain, err := svc.CreateChain(projectID, &types.ChainCreateRequest{
				Name: "Test Chain", WorkflowName: "test", TicketIDs: []string{"A", "B"},
			})
			if err != nil {
				t.Fatalf("CreateChain failed: %v", err)
			}

			chainRepo := repo.NewChainRepo(pool, clock.Real())
			if err := chainRepo.UpdateStatus(chain.ID, model.ChainStatusRunning); err != nil {
				t.Fatalf("UpdateStatus failed: %v", err)
			}

			// Mark A with the non-pending status
			itemRepo := repo.NewChainItemRepo(pool, clock.Real())
			items, err := itemRepo.ListByChain(chain.ID)
			if err != nil {
				t.Fatalf("ListByChain failed: %v", err)
			}
			var itemAID string
			for _, it := range items {
				if it.TicketID == "a" {
					itemAID = it.ID
				}
			}
			if err := itemRepo.UpdateItemStatus(itemAID, itemStatus); err != nil {
				t.Fatalf("UpdateItemStatus failed: %v", err)
			}

			// Try to remove A (non-pending)
			_, err = svc.RemoveFromChain(chain.ID, []string{"A"})
			if err == nil {
				t.Fatalf("expected error removing %s item, got nil", itemStatus)
			}
			if !strings.Contains(err.Error(), "not pending or not in chain") {
				t.Errorf("expected 'not pending or not in chain', got: %v", err)
			}

			// Verify B is still present (no partial removal)
			remaining := getChainItemCount(t, svc, chain.ID)
			if remaining != 2 {
				t.Errorf("expected 2 items unchanged, got %d", remaining)
			}
		})
	}
}

// TestRemoveFromChain_TicketNotInChainFails verifies that a ticket not in the chain
// returns an error.
func TestRemoveFromChain_TicketNotInChainFails(t *testing.T) {
	pool, projectID := setupChainTestDB(t)
	defer pool.Close()

	now := time.Now()
	createTestTickets(t, pool, projectID, map[string]time.Time{
		"A": now,
		"X": now.Add(time.Second), // exists as ticket but not in chain
	})

	svc := NewChainService(pool, clock.Real())
	chain, err := svc.CreateChain(projectID, &types.ChainCreateRequest{
		Name: "Test Chain", WorkflowName: "test", TicketIDs: []string{"A"},
	})
	if err != nil {
		t.Fatalf("CreateChain failed: %v", err)
	}

	chainRepo := repo.NewChainRepo(pool, clock.Real())
	if err := chainRepo.UpdateStatus(chain.ID, model.ChainStatusRunning); err != nil {
		t.Fatalf("UpdateStatus failed: %v", err)
	}

	_, err = svc.RemoveFromChain(chain.ID, []string{"X"})
	if err == nil {
		t.Fatal("expected error for ticket not in chain, got nil")
	}
	if !strings.Contains(err.Error(), "not pending or not in chain") {
		t.Errorf("expected 'not pending or not in chain', got: %v", err)
	}
}

// TestRemoveFromChain_SingleNonPendingFailIsAtomic verifies that requesting removal
// of a single non-pending item returns an error and leaves all items intact.
// The delete + lock release are wrapped in a transaction, so mixed batches
// (some pending + some non-pending) are rolled back atomically.
func TestRemoveFromChain_SingleNonPendingFailIsAtomic(t *testing.T) {
	pool, projectID := setupChainTestDB(t)
	defer pool.Close()

	now := time.Now()
	createTestTickets(t, pool, projectID, map[string]time.Time{
		"A": now,
		"B": now.Add(time.Second),
		"C": now.Add(2 * time.Second),
	})

	svc := NewChainService(pool, clock.Real())
	chain, err := svc.CreateChain(projectID, &types.ChainCreateRequest{
		Name: "Test Chain", WorkflowName: "test", TicketIDs: []string{"A", "B", "C"},
	})
	if err != nil {
		t.Fatalf("CreateChain failed: %v", err)
	}

	chainRepo := repo.NewChainRepo(pool, clock.Real())
	if err := chainRepo.UpdateStatus(chain.ID, model.ChainStatusRunning); err != nil {
		t.Fatalf("UpdateStatus failed: %v", err)
	}

	// Mark A as running (non-pending)
	itemRepo := repo.NewChainItemRepo(pool, clock.Real())
	items, err := itemRepo.ListByChain(chain.ID)
	if err != nil {
		t.Fatalf("ListByChain failed: %v", err)
	}
	for _, it := range items {
		if it.TicketID == "a" {
			if err := itemRepo.UpdateItemStatus(it.ID, model.ChainItemRunning); err != nil {
				t.Fatalf("UpdateItemStatus failed: %v", err)
			}
		}
	}

	// Try to remove only A (running) — DELETE matches 0 rows, so no item is removed
	_, err = svc.RemoveFromChain(chain.ID, []string{"A"})
	if err == nil {
		t.Fatal("expected error removing running item, got nil")
	}
	if !strings.Contains(err.Error(), "not pending or not in chain") {
		t.Errorf("expected 'not pending or not in chain', got: %v", err)
	}

	// Verify all 3 items remain untouched
	remaining := getChainItemCount(t, svc, chain.ID)
	if remaining != 3 {
		t.Errorf("expected 3 items unchanged, got %d", remaining)
	}
}

// TestRemoveFromChain_CaseInsensitiveDeduplicated verifies that ticket IDs are
// matched and deduplicated case-insensitively.
func TestRemoveFromChain_CaseInsensitiveDeduplicated(t *testing.T) {
	pool, projectID := setupChainTestDB(t)
	defer pool.Close()

	now := time.Now()
	createTestTickets(t, pool, projectID, map[string]time.Time{
		"A": now,
		"B": now.Add(time.Second),
	})

	svc := NewChainService(pool, clock.Real())
	chain, err := svc.CreateChain(projectID, &types.ChainCreateRequest{
		Name: "Test Chain", WorkflowName: "test", TicketIDs: []string{"A", "B"},
	})
	if err != nil {
		t.Fatalf("CreateChain failed: %v", err)
	}

	chainRepo := repo.NewChainRepo(pool, clock.Real())
	if err := chainRepo.UpdateStatus(chain.ID, model.ChainStatusRunning); err != nil {
		t.Fatalf("UpdateStatus failed: %v", err)
	}

	// Remove "B" with mixed case + duplicate — should count as single removal
	updated, err := svc.RemoveFromChain(chain.ID, []string{"B", "b", "B"})
	if err != nil {
		t.Fatalf("RemoveFromChain failed: %v", err)
	}

	if len(updated.Items) != 1 {
		t.Errorf("expected 1 item (only A), got %d", len(updated.Items))
	}
	if updated.Items[0].TicketID != "a" {
		t.Errorf("expected remaining item to be A, got %s", updated.Items[0].TicketID)
	}
}
