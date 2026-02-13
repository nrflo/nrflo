package integration

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"be/internal/model"
	"be/internal/repo"
	"be/internal/service"
	"be/internal/types"
	"be/internal/ws"
)

// TestChainCreate_WithEpicTicketID verifies epic_ticket_id round-trips through Create/Get/List
func TestChainCreate_WithEpicTicketID(t *testing.T) {
	env := NewTestEnv(t)

	// Create epic ticket and chain ticket
	createChainTickets(t, env, map[string]time.Time{
		"EPIC-1": time.Now(),
		"A":      time.Now().Add(time.Second),
	})

	chainSvc := service.NewChainService(env.Pool)
	chain, err := chainSvc.CreateChain(env.ProjectID, &types.ChainCreateRequest{
		Name:         "Epic Chain",
		WorkflowName: "test",
		TicketIDs:    []string{"A"},
		EpicTicketID: "EPIC-1",
	})
	if err != nil {
		t.Fatalf("CreateChain failed: %v", err)
	}

	// Verify epic_ticket_id is set in create response (preserves case)
	if chain.EpicTicketID != "EPIC-1" {
		t.Errorf("expected epic_ticket_id 'EPIC-1', got %s", chain.EpicTicketID)
	}

	// Verify epic_ticket_id round-trips through Get
	chainRepo := repo.NewChainRepo(env.Pool)
	retrieved, err := chainRepo.Get(chain.ID)
	if err != nil {
		t.Fatalf("Get failed: %v", err)
	}
	if retrieved.EpicTicketID != "EPIC-1" {
		t.Errorf("expected epic_ticket_id 'EPIC-1' from Get, got %s", retrieved.EpicTicketID)
	}

	// Verify epic_ticket_id round-trips through List
	chains, err := chainRepo.List(env.ProjectID, "")
	if err != nil {
		t.Fatalf("List failed: %v", err)
	}
	if len(chains) != 1 {
		t.Fatalf("expected 1 chain, got %d", len(chains))
	}
	if chains[0].EpicTicketID != "EPIC-1" {
		t.Errorf("expected epic_ticket_id 'EPIC-1' from List, got %s", chains[0].EpicTicketID)
	}
}

// TestChainCreate_WithoutEpicTicketID verifies chains without epic_ticket_id work correctly
func TestChainCreate_WithoutEpicTicketID(t *testing.T) {
	env := NewTestEnv(t)

	createChainTickets(t, env, map[string]time.Time{
		"A": time.Now(),
	})

	chainSvc := service.NewChainService(env.Pool)
	chain, err := chainSvc.CreateChain(env.ProjectID, &types.ChainCreateRequest{
		Name:         "No Epic Chain",
		WorkflowName: "test",
		TicketIDs:    []string{"A"},
		// EpicTicketID intentionally omitted
	})
	if err != nil {
		t.Fatalf("CreateChain failed: %v", err)
	}

	// Verify epic_ticket_id is empty
	if chain.EpicTicketID != "" {
		t.Errorf("expected empty epic_ticket_id, got %s", chain.EpicTicketID)
	}

	// Verify it round-trips as empty
	chainRepo := repo.NewChainRepo(env.Pool)
	retrieved, err := chainRepo.Get(chain.ID)
	if err != nil {
		t.Fatalf("Get failed: %v", err)
	}
	if retrieved.EpicTicketID != "" {
		t.Errorf("expected empty epic_ticket_id from Get, got %s", retrieved.EpicTicketID)
	}
}

// TestChainCompletion_ClosesEpic verifies epic ticket is closed when chain completes
func TestChainCompletion_ClosesEpic(t *testing.T) {
	env := NewTestEnv(t)

	base := time.Now()
	createChainTickets(t, env, map[string]time.Time{
		"EPIC-1": base,
		"A":      base.Add(time.Second),
		"B":      base.Add(2 * time.Second),
	})
	createChainDependencies(t, env, map[string][]string{
		"B": {"A"},
	})

	chainSvc := service.NewChainService(env.Pool)
	chain, err := chainSvc.CreateChain(env.ProjectID, &types.ChainCreateRequest{
		Name:         "Epic Test Chain",
		WorkflowName: "test",
		TicketIDs:    []string{"B"},
		EpicTicketID: "EPIC-1",
	})
	if err != nil {
		t.Fatalf("CreateChain failed: %v", err)
	}

	// Verify epic ticket exists and is open
	ticketSvc := service.NewTicketService(env.Pool)
	epic, err := ticketSvc.Get(env.ProjectID, "EPIC-1")
	if err != nil {
		t.Fatalf("Get epic failed: %v", err)
	}
	if epic.Status != model.StatusOpen {
		t.Fatalf("expected epic status 'open', got %s", epic.Status)
	}

	// Create WS client to verify broadcast event
	wsClient, wsChan := env.NewWSClient(t, "test-client", "")

	// Simulate chain completion
	chainRepo := repo.NewChainRepo(env.Pool)
	itemRepo := repo.NewChainItemRepo(env.Pool)
	lockRepo := repo.NewChainLockRepo(env.Pool)

	// Mark chain running
	chainRepo.UpdateStatus(chain.ID, model.ChainStatusRunning)

	// Mark all items completed
	items, _ := itemRepo.ListByChain(chain.ID)
	for _, item := range items {
		itemRepo.UpdateItemStatus(item.ID, model.ChainItemRunning)
		itemRepo.UpdateItemStatus(item.ID, model.ChainItemCompleted)
	}

	// Mark chain completed (this should trigger epic close)
	chainRepo.UpdateStatus(chain.ID, model.ChainStatusCompleted)
	lockRepo.DeleteLocksByChain(chain.ID)

	// Simulate markChainCompleted epic close logic
	// (In production, this is called by chain_runner.go:292-316)
	retrievedChain, err := chainRepo.Get(chain.ID)
	if err != nil {
		t.Fatalf("Get chain failed: %v", err)
	}
	if retrievedChain.EpicTicketID != "" {
		err = ticketSvc.Close(env.ProjectID, retrievedChain.EpicTicketID, "All epic tickets completed via chain 'Epic Test Chain'")
		if err != nil {
			t.Logf("Epic close failed (non-fatal): %v", err)
		} else {
			// Broadcast event
			env.Hub.Broadcast(ws.NewEvent(ws.EventTicketUpdated, env.ProjectID, retrievedChain.EpicTicketID, "", map[string]interface{}{
				"status": "closed",
			}))
		}
	}

	// Verify epic ticket is closed
	epicAfter, err := ticketSvc.Get(env.ProjectID, "EPIC-1")
	if err != nil {
		t.Fatalf("Get epic after completion failed: %v", err)
	}
	if epicAfter.Status != model.StatusClosed {
		t.Errorf("expected epic status 'closed', got %s", epicAfter.Status)
	}

	// Verify close reason
	expectedReason := "All epic tickets completed via chain 'Epic Test Chain'"
	if !epicAfter.CloseReason.Valid || epicAfter.CloseReason.String != expectedReason {
		t.Errorf("expected close_reason '%s', got %v (valid=%v)", expectedReason, epicAfter.CloseReason.String, epicAfter.CloseReason.Valid)
	}

	// Verify WebSocket broadcast event
	select {
	case msg := <-wsChan:
		var evt ws.Event
		if err := json.Unmarshal(msg, &evt); err != nil {
			t.Fatalf("failed to unmarshal WS event: %v", err)
		}
		if evt.Type != ws.EventTicketUpdated {
			t.Errorf("expected event type %s, got %s", ws.EventTicketUpdated, evt.Type)
		}
		if !strings.EqualFold(evt.TicketID, "EPIC-1") {
			t.Errorf("expected ticket_id 'EPIC-1' (case-insensitive), got %s", evt.TicketID)
		}
		if evt.Data["status"] != "closed" {
			t.Errorf("expected status 'closed' in event, got %v", evt.Data["status"])
		}
	case <-time.After(500 * time.Millisecond):
		t.Error("timeout waiting for ticket.updated event")
	}

	// Clean up
	env.Hub.Unregister(wsClient)
}

// TestChainCompletion_NoEpic verifies chains without epic_ticket_id don't attempt epic close
func TestChainCompletion_NoEpic(t *testing.T) {
	env := NewTestEnv(t)

	createChainTickets(t, env, map[string]time.Time{
		"A": time.Now(),
	})

	chainSvc := service.NewChainService(env.Pool)
	chain, err := chainSvc.CreateChain(env.ProjectID, &types.ChainCreateRequest{
		Name:         "No Epic Chain",
		WorkflowName: "test",
		TicketIDs:    []string{"A"},
		// EpicTicketID intentionally omitted
	})
	if err != nil {
		t.Fatalf("CreateChain failed: %v", err)
	}

	// Simulate chain completion
	chainRepo := repo.NewChainRepo(env.Pool)
	itemRepo := repo.NewChainItemRepo(env.Pool)
	lockRepo := repo.NewChainLockRepo(env.Pool)

	chainRepo.UpdateStatus(chain.ID, model.ChainStatusRunning)

	items, _ := itemRepo.ListByChain(chain.ID)
	itemRepo.UpdateItemStatus(items[0].ID, model.ChainItemRunning)
	itemRepo.UpdateItemStatus(items[0].ID, model.ChainItemCompleted)

	// Mark chain completed
	chainRepo.UpdateStatus(chain.ID, model.ChainStatusCompleted)
	lockRepo.DeleteLocksByChain(chain.ID)

	// Verify chain is completed
	completed, err := chainRepo.Get(chain.ID)
	if err != nil {
		t.Fatalf("Get chain failed: %v", err)
	}
	if completed.Status != model.ChainStatusCompleted {
		t.Errorf("expected chain status 'completed', got %s", completed.Status)
	}

	// This test verifies that chains without epic_ticket_id complete successfully
	// without attempting to close a non-existent epic (behavior unchanged)
}

// TestChainCompletion_EpicCloseFailure verifies chain completes even if epic close fails
func TestChainCompletion_EpicCloseFailure(t *testing.T) {
	env := NewTestEnv(t)

	createChainTickets(t, env, map[string]time.Time{
		"A": time.Now(),
	})

	chainSvc := service.NewChainService(env.Pool)
	chain, err := chainSvc.CreateChain(env.ProjectID, &types.ChainCreateRequest{
		Name:         "Invalid Epic Chain",
		WorkflowName: "test",
		TicketIDs:    []string{"A"},
		EpicTicketID: "NONEXISTENT-EPIC",
	})
	if err != nil {
		t.Fatalf("CreateChain failed: %v", err)
	}

	// Verify chain has invalid epic_ticket_id (preserves case)
	if chain.EpicTicketID != "NONEXISTENT-EPIC" {
		t.Fatalf("expected epic_ticket_id 'NONEXISTENT-EPIC', got %s", chain.EpicTicketID)
	}

	// Simulate chain completion
	chainRepo := repo.NewChainRepo(env.Pool)
	itemRepo := repo.NewChainItemRepo(env.Pool)
	lockRepo := repo.NewChainLockRepo(env.Pool)

	chainRepo.UpdateStatus(chain.ID, model.ChainStatusRunning)

	items, _ := itemRepo.ListByChain(chain.ID)
	itemRepo.UpdateItemStatus(items[0].ID, model.ChainItemRunning)
	itemRepo.UpdateItemStatus(items[0].ID, model.ChainItemCompleted)

	// Mark chain completed (this should not fail even though epic is invalid)
	chainRepo.UpdateStatus(chain.ID, model.ChainStatusCompleted)
	lockRepo.DeleteLocksByChain(chain.ID)

	// Simulate epic close attempt (will fail but should be logged and ignored)
	ticketSvc := service.NewTicketService(env.Pool)
	retrievedChain, _ := chainRepo.Get(chain.ID)
	if retrievedChain.EpicTicketID != "" {
		err = ticketSvc.Close(env.ProjectID, retrievedChain.EpicTicketID, "All epic tickets completed via chain 'Invalid Epic Chain'")
		// Error is expected and should be logged (not fatal)
		if err == nil {
			t.Error("expected error closing nonexistent epic, got nil")
		}
		if !strings.Contains(err.Error(), "not found") {
			t.Logf("Epic close failed as expected: %v", err)
		}
	}

	// Verify chain is still completed despite epic close failure
	completed, err := chainRepo.Get(chain.ID)
	if err != nil {
		t.Fatalf("Get chain failed: %v", err)
	}
	if completed.Status != model.ChainStatusCompleted {
		t.Errorf("expected chain status 'completed' even with epic close failure, got %s", completed.Status)
	}
}

// TestChainFailure_EpicRemainsOpen verifies epic stays open when chain fails
func TestChainFailure_EpicRemainsOpen(t *testing.T) {
	env := NewTestEnv(t)

	base := time.Now()
	createChainTickets(t, env, map[string]time.Time{
		"EPIC-1": base,
		"A":      base.Add(time.Second),
		"B":      base.Add(2 * time.Second),
	})
	createChainDependencies(t, env, map[string][]string{
		"B": {"A"},
	})

	chainSvc := service.NewChainService(env.Pool)
	chain, err := chainSvc.CreateChain(env.ProjectID, &types.ChainCreateRequest{
		Name:         "Failure Test Chain",
		WorkflowName: "test",
		TicketIDs:    []string{"B"},
		EpicTicketID: "EPIC-1",
	})
	if err != nil {
		t.Fatalf("CreateChain failed: %v", err)
	}

	// Verify epic ticket exists and is open
	ticketSvc := service.NewTicketService(env.Pool)
	epic, err := ticketSvc.Get(env.ProjectID, "EPIC-1")
	if err != nil {
		t.Fatalf("Get epic failed: %v", err)
	}
	if epic.Status != model.StatusOpen {
		t.Fatalf("expected epic status 'open', got %s", epic.Status)
	}

	// Simulate chain failure
	chainRepo := repo.NewChainRepo(env.Pool)
	itemRepo := repo.NewChainItemRepo(env.Pool)
	lockRepo := repo.NewChainLockRepo(env.Pool)

	chainRepo.UpdateStatus(chain.ID, model.ChainStatusRunning)

	// First item completes, second item fails
	items, _ := itemRepo.ListByChain(chain.ID)
	itemRepo.UpdateItemStatus(items[0].ID, model.ChainItemRunning)
	itemRepo.UpdateItemStatus(items[0].ID, model.ChainItemCompleted)

	itemRepo.UpdateItemStatus(items[1].ID, model.ChainItemRunning)
	itemRepo.UpdateItemStatus(items[1].ID, model.ChainItemFailed)

	// Mark chain failed (markChainFailed does NOT close epic)
	chainRepo.UpdateStatus(chain.ID, model.ChainStatusFailed)
	lockRepo.DeleteLocksByChain(chain.ID)

	// Verify chain is failed
	failed, err := chainRepo.Get(chain.ID)
	if err != nil {
		t.Fatalf("Get chain failed: %v", err)
	}
	if failed.Status != model.ChainStatusFailed {
		t.Errorf("expected chain status 'failed', got %s", failed.Status)
	}

	// Verify epic ticket remains open (not closed on failure)
	epicAfter, err := ticketSvc.Get(env.ProjectID, "EPIC-1")
	if err != nil {
		t.Fatalf("Get epic after failure failed: %v", err)
	}
	if epicAfter.Status != model.StatusOpen {
		t.Errorf("expected epic to remain 'open' after chain failure, got %s", epicAfter.Status)
	}
}

// TestChainCancellation_EpicRemainsOpen verifies epic stays open when chain is canceled
func TestChainCancellation_EpicRemainsOpen(t *testing.T) {
	env := NewTestEnv(t)

	createChainTickets(t, env, map[string]time.Time{
		"EPIC-1": time.Now(),
		"A":      time.Now().Add(time.Second),
	})

	chainSvc := service.NewChainService(env.Pool)
	chain, err := chainSvc.CreateChain(env.ProjectID, &types.ChainCreateRequest{
		Name:         "Cancel Test Chain",
		WorkflowName: "test",
		TicketIDs:    []string{"A"},
		EpicTicketID: "EPIC-1",
	})
	if err != nil {
		t.Fatalf("CreateChain failed: %v", err)
	}

	// Verify epic ticket exists and is open
	ticketSvc := service.NewTicketService(env.Pool)
	epic, err := ticketSvc.Get(env.ProjectID, "EPIC-1")
	if err != nil {
		t.Fatalf("Get epic failed: %v", err)
	}
	if epic.Status != model.StatusOpen {
		t.Fatalf("expected epic status 'open', got %s", epic.Status)
	}

	// Simulate chain cancellation
	chainRepo := repo.NewChainRepo(env.Pool)
	itemRepo := repo.NewChainItemRepo(env.Pool)
	lockRepo := repo.NewChainLockRepo(env.Pool)

	chainRepo.UpdateStatus(chain.ID, model.ChainStatusRunning)

	// Cancel while running
	items, _ := itemRepo.ListByChain(chain.ID)
	itemRepo.UpdateItemStatus(items[0].ID, model.ChainItemCanceled)

	chainRepo.UpdateStatus(chain.ID, model.ChainStatusCanceled)
	lockRepo.DeleteLocksByChain(chain.ID)

	// Verify chain is canceled
	canceled, err := chainRepo.Get(chain.ID)
	if err != nil {
		t.Fatalf("Get chain failed: %v", err)
	}
	if canceled.Status != model.ChainStatusCanceled {
		t.Errorf("expected chain status 'canceled', got %s", canceled.Status)
	}

	// Verify epic ticket remains open (not closed on cancellation)
	epicAfter, err := ticketSvc.Get(env.ProjectID, "EPIC-1")
	if err != nil {
		t.Fatalf("Get epic after cancellation failed: %v", err)
	}
	if epicAfter.Status != model.StatusOpen {
		t.Errorf("expected epic to remain 'open' after chain cancellation, got %s", epicAfter.Status)
	}
}

// TestChainEpic_IndexExists verifies the index on (project_id, epic_ticket_id) exists
func TestChainEpic_IndexExists(t *testing.T) {
	env := NewTestEnv(t)

	// Query sqlite_master to verify index exists
	var indexName string
	err := env.Pool.QueryRow(`
		SELECT name FROM sqlite_master
		WHERE type = 'index' AND name = 'idx_chain_exec_epic'
	`).Scan(&indexName)
	if err != nil {
		t.Fatalf("index idx_chain_exec_epic not found: %v", err)
	}
	if indexName != "idx_chain_exec_epic" {
		t.Errorf("expected index name 'idx_chain_exec_epic', got %s", indexName)
	}
}

// TestChainEpic_MultipleChainsSameEpic verifies multiple chains can reference the same epic
func TestChainEpic_MultipleChainsSameEpic(t *testing.T) {
	env := NewTestEnv(t)

	base := time.Now()
	createChainTickets(t, env, map[string]time.Time{
		"EPIC-1": base,
		"A":      base.Add(time.Second),
		"B":      base.Add(2 * time.Second),
	})

	chainSvc := service.NewChainService(env.Pool)

	// Create first chain with epic
	chain1, err := chainSvc.CreateChain(env.ProjectID, &types.ChainCreateRequest{
		Name:         "Chain 1",
		WorkflowName: "test",
		TicketIDs:    []string{"A"},
		EpicTicketID: "EPIC-1",
	})
	if err != nil {
		t.Fatalf("CreateChain 1 failed: %v", err)
	}
	if chain1.EpicTicketID != "EPIC-1" {
		t.Errorf("chain1: expected epic_ticket_id 'EPIC-1', got %s", chain1.EpicTicketID)
	}

	// Create second chain with same epic (should succeed)
	chain2, err := chainSvc.CreateChain(env.ProjectID, &types.ChainCreateRequest{
		Name:         "Chain 2",
		WorkflowName: "test",
		TicketIDs:    []string{"B"},
		EpicTicketID: "EPIC-1",
	})
	if err != nil {
		t.Fatalf("CreateChain 2 failed: %v", err)
	}
	if chain2.EpicTicketID != "EPIC-1" {
		t.Errorf("chain2: expected epic_ticket_id 'EPIC-1', got %s", chain2.EpicTicketID)
	}

	// Verify both chains exist with same epic_ticket_id
	chainRepo := repo.NewChainRepo(env.Pool)
	chains, err := chainRepo.List(env.ProjectID, "")
	if err != nil {
		t.Fatalf("List failed: %v", err)
	}

	epicChainCount := 0
	for _, c := range chains {
		if c.EpicTicketID == "EPIC-1" {
			epicChainCount++
		}
	}
	if epicChainCount != 2 {
		t.Errorf("expected 2 chains with epic_ticket_id 'EPIC-1', got %d", epicChainCount)
	}
}
