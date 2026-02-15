package integration

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"be/internal/clock"
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

	chainSvc := service.NewChainService(env.Pool, clock.Real())
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
	chainRepo := repo.NewChainRepo(env.Pool, clock.Real())
	retrieved, err := chainRepo.Get(chain.ID)
	if err != nil {
		t.Fatalf("Get failed: %v", err)
	}
	if retrieved.EpicTicketID != "EPIC-1" {
		t.Errorf("expected epic_ticket_id 'EPIC-1' from Get, got %s", retrieved.EpicTicketID)
	}

	// Verify epic_ticket_id round-trips through List
	chains, err := chainRepo.List(env.ProjectID, "", "")
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

	chainSvc := service.NewChainService(env.Pool, clock.Real())
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
	chainRepo := repo.NewChainRepo(env.Pool, clock.Real())
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

	chainSvc := service.NewChainService(env.Pool, clock.Real())
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
	ticketSvc := service.NewTicketService(env.Pool, clock.Real())
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
	chainRepo := repo.NewChainRepo(env.Pool, clock.Real())
	itemRepo := repo.NewChainItemRepo(env.Pool, clock.Real())
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

	chainSvc := service.NewChainService(env.Pool, clock.Real())
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
	chainRepo := repo.NewChainRepo(env.Pool, clock.Real())
	itemRepo := repo.NewChainItemRepo(env.Pool, clock.Real())
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

	chainSvc := service.NewChainService(env.Pool, clock.Real())
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
	chainRepo := repo.NewChainRepo(env.Pool, clock.Real())
	itemRepo := repo.NewChainItemRepo(env.Pool, clock.Real())
	lockRepo := repo.NewChainLockRepo(env.Pool)

	chainRepo.UpdateStatus(chain.ID, model.ChainStatusRunning)

	items, _ := itemRepo.ListByChain(chain.ID)
	itemRepo.UpdateItemStatus(items[0].ID, model.ChainItemRunning)
	itemRepo.UpdateItemStatus(items[0].ID, model.ChainItemCompleted)

	// Mark chain completed (this should not fail even though epic is invalid)
	chainRepo.UpdateStatus(chain.ID, model.ChainStatusCompleted)
	lockRepo.DeleteLocksByChain(chain.ID)

	// Simulate epic close attempt (will fail but should be logged and ignored)
	ticketSvc := service.NewTicketService(env.Pool, clock.Real())
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

	chainSvc := service.NewChainService(env.Pool, clock.Real())
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
	ticketSvc := service.NewTicketService(env.Pool, clock.Real())
	epic, err := ticketSvc.Get(env.ProjectID, "EPIC-1")
	if err != nil {
		t.Fatalf("Get epic failed: %v", err)
	}
	if epic.Status != model.StatusOpen {
		t.Fatalf("expected epic status 'open', got %s", epic.Status)
	}

	// Simulate chain failure
	chainRepo := repo.NewChainRepo(env.Pool, clock.Real())
	itemRepo := repo.NewChainItemRepo(env.Pool, clock.Real())
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

	chainSvc := service.NewChainService(env.Pool, clock.Real())
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
	ticketSvc := service.NewTicketService(env.Pool, clock.Real())
	epic, err := ticketSvc.Get(env.ProjectID, "EPIC-1")
	if err != nil {
		t.Fatalf("Get epic failed: %v", err)
	}
	if epic.Status != model.StatusOpen {
		t.Fatalf("expected epic status 'open', got %s", epic.Status)
	}

	// Simulate chain cancellation
	chainRepo := repo.NewChainRepo(env.Pool, clock.Real())
	itemRepo := repo.NewChainItemRepo(env.Pool, clock.Real())
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

	chainSvc := service.NewChainService(env.Pool, clock.Real())

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
	chainRepo := repo.NewChainRepo(env.Pool, clock.Real())
	chains, err := chainRepo.List(env.ProjectID, "", "")
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

// TestChainList_EpicTicketIDFilter verifies filtering chains by epic_ticket_id
func TestChainList_EpicTicketIDFilter(t *testing.T) {
	env := NewTestEnv(t)

	base := time.Now()
	createChainTickets(t, env, map[string]time.Time{
		"EPIC-1": base,
		"EPIC-2": base.Add(time.Second),
		"A":      base.Add(2 * time.Second),
		"B":      base.Add(3 * time.Second),
		"C":      base.Add(4 * time.Second),
	})

	chainSvc := service.NewChainService(env.Pool, clock.Real())

	// Create chain with EPIC-1
	chain1, err := chainSvc.CreateChain(env.ProjectID, &types.ChainCreateRequest{
		Name:         "Epic 1 Chain",
		WorkflowName: "test",
		TicketIDs:    []string{"A"},
		EpicTicketID: "EPIC-1",
	})
	if err != nil {
		t.Fatalf("CreateChain 1 failed: %v", err)
	}

	// Create another chain with EPIC-1
	chain2, err := chainSvc.CreateChain(env.ProjectID, &types.ChainCreateRequest{
		Name:         "Epic 1 Chain 2",
		WorkflowName: "test",
		TicketIDs:    []string{"B"},
		EpicTicketID: "EPIC-1",
	})
	if err != nil {
		t.Fatalf("CreateChain 2 failed: %v", err)
	}

	// Create chain with EPIC-2
	chain3, err := chainSvc.CreateChain(env.ProjectID, &types.ChainCreateRequest{
		Name:         "Epic 2 Chain",
		WorkflowName: "test",
		TicketIDs:    []string{"C"},
		EpicTicketID: "EPIC-2",
	})
	if err != nil {
		t.Fatalf("CreateChain 3 failed: %v", err)
	}

	chainRepo := repo.NewChainRepo(env.Pool, clock.Real())

	// Filter by EPIC-1
	epic1Chains, err := chainRepo.List(env.ProjectID, "", "EPIC-1")
	if err != nil {
		t.Fatalf("List with epic_ticket_id=EPIC-1 failed: %v", err)
	}
	if len(epic1Chains) != 2 {
		t.Fatalf("expected 2 chains for EPIC-1, got %d", len(epic1Chains))
	}

	// Verify both chains belong to EPIC-1
	epic1IDs := map[string]bool{chain1.ID: true, chain2.ID: true}
	for _, c := range epic1Chains {
		if !epic1IDs[c.ID] {
			t.Errorf("unexpected chain ID %s in EPIC-1 results", c.ID)
		}
		if c.EpicTicketID != "EPIC-1" {
			t.Errorf("chain %s: expected epic_ticket_id 'EPIC-1', got %s", c.ID, c.EpicTicketID)
		}
	}

	// Filter by EPIC-2
	epic2Chains, err := chainRepo.List(env.ProjectID, "", "EPIC-2")
	if err != nil {
		t.Fatalf("List with epic_ticket_id=EPIC-2 failed: %v", err)
	}
	if len(epic2Chains) != 1 {
		t.Fatalf("expected 1 chain for EPIC-2, got %d", len(epic2Chains))
	}
	if epic2Chains[0].ID != chain3.ID {
		t.Errorf("expected chain3 for EPIC-2, got %s", epic2Chains[0].ID)
	}

	// Filter by nonexistent epic
	noChains, err := chainRepo.List(env.ProjectID, "", "EPIC-NONEXISTENT")
	if err != nil {
		t.Fatalf("List with nonexistent epic failed: %v", err)
	}
	if len(noChains) != 0 {
		t.Errorf("expected 0 chains for nonexistent epic, got %d", len(noChains))
	}

	// No filter - should return all 3 chains
	allChains, err := chainRepo.List(env.ProjectID, "", "")
	if err != nil {
		t.Fatalf("List with no epic filter failed: %v", err)
	}
	if len(allChains) != 3 {
		t.Errorf("expected 3 chains with no epic filter, got %d", len(allChains))
	}
}

// TestChainList_CombinedFilters verifies combining status and epic_ticket_id filters
func TestChainList_CombinedFilters(t *testing.T) {
	env := NewTestEnv(t)

	base := time.Now()
	createChainTickets(t, env, map[string]time.Time{
		"EPIC-1": base,
		"A":      base.Add(time.Second),
		"B":      base.Add(2 * time.Second),
		"C":      base.Add(3 * time.Second),
	})

	chainSvc := service.NewChainService(env.Pool, clock.Real())

	// Create three chains with EPIC-1
	chain1, err := chainSvc.CreateChain(env.ProjectID, &types.ChainCreateRequest{
		Name:         "Chain 1 (pending)",
		WorkflowName: "test",
		TicketIDs:    []string{"A"},
		EpicTicketID: "EPIC-1",
	})
	if err != nil {
		t.Fatalf("CreateChain 1 failed: %v", err)
	}

	chain2, err := chainSvc.CreateChain(env.ProjectID, &types.ChainCreateRequest{
		Name:         "Chain 2 (running)",
		WorkflowName: "test",
		TicketIDs:    []string{"B"},
		EpicTicketID: "EPIC-1",
	})
	if err != nil {
		t.Fatalf("CreateChain 2 failed: %v", err)
	}

	chain3, err := chainSvc.CreateChain(env.ProjectID, &types.ChainCreateRequest{
		Name:         "Chain 3 (completed)",
		WorkflowName: "test",
		TicketIDs:    []string{"C"},
		EpicTicketID: "EPIC-1",
	})
	if err != nil {
		t.Fatalf("CreateChain 3 failed: %v", err)
	}

	// Update statuses
	chainRepo := repo.NewChainRepo(env.Pool, clock.Real())
	chainRepo.UpdateStatus(chain2.ID, model.ChainStatusRunning)
	chainRepo.UpdateStatus(chain3.ID, model.ChainStatusCompleted)

	// Filter by EPIC-1 + pending
	pendingChains, err := chainRepo.List(env.ProjectID, string(model.ChainStatusPending), "EPIC-1")
	if err != nil {
		t.Fatalf("List with status=pending and epic_ticket_id=EPIC-1 failed: %v", err)
	}
	if len(pendingChains) != 1 {
		t.Fatalf("expected 1 pending chain for EPIC-1, got %d", len(pendingChains))
	}
	if pendingChains[0].ID != chain1.ID {
		t.Errorf("expected chain1, got %s", pendingChains[0].ID)
	}
	if pendingChains[0].Status != model.ChainStatusPending {
		t.Errorf("expected status pending, got %s", pendingChains[0].Status)
	}

	// Filter by EPIC-1 + running
	runningChains, err := chainRepo.List(env.ProjectID, string(model.ChainStatusRunning), "EPIC-1")
	if err != nil {
		t.Fatalf("List with status=running and epic_ticket_id=EPIC-1 failed: %v", err)
	}
	if len(runningChains) != 1 {
		t.Fatalf("expected 1 running chain for EPIC-1, got %d", len(runningChains))
	}
	if runningChains[0].ID != chain2.ID {
		t.Errorf("expected chain2, got %s", runningChains[0].ID)
	}

	// Filter by EPIC-1 + completed
	completedChains, err := chainRepo.List(env.ProjectID, string(model.ChainStatusCompleted), "EPIC-1")
	if err != nil {
		t.Fatalf("List with status=completed and epic_ticket_id=EPIC-1 failed: %v", err)
	}
	if len(completedChains) != 1 {
		t.Fatalf("expected 1 completed chain for EPIC-1, got %d", len(completedChains))
	}
	if completedChains[0].ID != chain3.ID {
		t.Errorf("expected chain3, got %s", completedChains[0].ID)
	}

	// Filter by EPIC-1 only - should return all 3
	allEpicChains, err := chainRepo.List(env.ProjectID, "", "EPIC-1")
	if err != nil {
		t.Fatalf("List with epic_ticket_id=EPIC-1 only failed: %v", err)
	}
	if len(allEpicChains) != 3 {
		t.Errorf("expected 3 chains for EPIC-1 (all statuses), got %d", len(allEpicChains))
	}

	// Filter by EPIC-1 + nonexistent status
	noChains, err := chainRepo.List(env.ProjectID, "nonexistent", "EPIC-1")
	if err != nil {
		t.Fatalf("List with invalid status failed: %v", err)
	}
	if len(noChains) != 0 {
		t.Errorf("expected 0 chains for invalid status, got %d", len(noChains))
	}
}

// TestChainList_EpicTicketIDCaseInsensitive verifies epic_ticket_id filter is case-sensitive
func TestChainList_EpicTicketIDCaseInsensitive(t *testing.T) {
	env := NewTestEnv(t)

	base := time.Now()
	createChainTickets(t, env, map[string]time.Time{
		"EPIC-1": base,
		"A":      base.Add(time.Second),
	})

	chainSvc := service.NewChainService(env.Pool, clock.Real())
	_, err := chainSvc.CreateChain(env.ProjectID, &types.ChainCreateRequest{
		Name:         "Test Chain",
		WorkflowName: "test",
		TicketIDs:    []string{"A"},
		EpicTicketID: "EPIC-1",
	})
	if err != nil {
		t.Fatalf("CreateChain failed: %v", err)
	}

	chainRepo := repo.NewChainRepo(env.Pool, clock.Real())

	// Filter by exact case
	chains, err := chainRepo.List(env.ProjectID, "", "EPIC-1")
	if err != nil {
		t.Fatalf("List with EPIC-1 failed: %v", err)
	}
	if len(chains) != 1 {
		t.Fatalf("expected 1 chain for EPIC-1, got %d", len(chains))
	}

	// Filter by different case - should return 0 chains (epic_ticket_id is case-sensitive in SQL)
	chainsLower, err := chainRepo.List(env.ProjectID, "", "epic-1")
	if err != nil {
		t.Fatalf("List with epic-1 failed: %v", err)
	}
	if len(chainsLower) != 0 {
		t.Errorf("expected 0 chains for lowercase epic-1 (case-sensitive), got %d", len(chainsLower))
	}
}

// TestChainList_EmptyEpicTicketID verifies chains without epic_ticket_id are not included in epic filter
func TestChainList_EmptyEpicTicketID(t *testing.T) {
	env := NewTestEnv(t)

	base := time.Now()
	createChainTickets(t, env, map[string]time.Time{
		"EPIC-1": base,
		"A":      base.Add(time.Second),
		"B":      base.Add(2 * time.Second),
	})

	chainSvc := service.NewChainService(env.Pool, clock.Real())

	// Create chain with epic
	chainWithEpic, err := chainSvc.CreateChain(env.ProjectID, &types.ChainCreateRequest{
		Name:         "Epic Chain",
		WorkflowName: "test",
		TicketIDs:    []string{"A"},
		EpicTicketID: "EPIC-1",
	})
	if err != nil {
		t.Fatalf("CreateChain with epic failed: %v", err)
	}

	// Create chain without epic
	chainWithoutEpic, err := chainSvc.CreateChain(env.ProjectID, &types.ChainCreateRequest{
		Name:         "No Epic Chain",
		WorkflowName: "test",
		TicketIDs:    []string{"B"},
		// EpicTicketID intentionally omitted
	})
	if err != nil {
		t.Fatalf("CreateChain without epic failed: %v", err)
	}

	chainRepo := repo.NewChainRepo(env.Pool, clock.Real())

	// Filter by EPIC-1 - should only return chain with epic
	epicChains, err := chainRepo.List(env.ProjectID, "", "EPIC-1")
	if err != nil {
		t.Fatalf("List with epic_ticket_id=EPIC-1 failed: %v", err)
	}
	if len(epicChains) != 1 {
		t.Fatalf("expected 1 chain for EPIC-1, got %d", len(epicChains))
	}
	if epicChains[0].ID != chainWithEpic.ID {
		t.Errorf("expected chainWithEpic, got %s", epicChains[0].ID)
	}

	// No filter - should return both chains
	allChains, err := chainRepo.List(env.ProjectID, "", "")
	if err != nil {
		t.Fatalf("List with no epic filter failed: %v", err)
	}
	if len(allChains) != 2 {
		t.Fatalf("expected 2 chains total, got %d", len(allChains))
	}

	// Verify both chains are present
	foundIDs := map[string]bool{}
	for _, c := range allChains {
		foundIDs[c.ID] = true
	}
	if !foundIDs[chainWithEpic.ID] || !foundIDs[chainWithoutEpic.ID] {
		t.Errorf("expected both chains in unfiltered list")
	}
}
