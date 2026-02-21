package service

import (
	"strings"
	"testing"
	"time"

	"be/internal/clock"
	"be/internal/types"
)

// TestCreateChain_WithCustomOrder verifies that a valid custom order is used instead of auto-sort.
func TestCreateChain_WithCustomOrder(t *testing.T) {
	pool, projectID := setupChainTestDB(t)
	defer pool.Close()

	base := time.Now()
	createTestTickets(t, pool, projectID, map[string]time.Time{
		"A": base,
		"B": base.Add(time.Second),
		"C": base.Add(2 * time.Second),
	})
	// No inter-dependencies — any order is valid

	svc := NewChainService(pool, clock.Real())
	chain, err := svc.CreateChain(projectID, &types.ChainCreateRequest{
		Name:             "Custom Order Chain",
		WorkflowName:     "test",
		TicketIDs:        []string{"A", "B", "C"},
		OrderedTicketIDs: []string{"C", "A", "B"}, // custom order, all valid
	})
	if err != nil {
		t.Fatalf("CreateChain with custom order failed: %v", err)
	}

	if len(chain.Items) != 3 {
		t.Fatalf("expected 3 items, got %d", len(chain.Items))
	}
	if chain.Items[0].TicketID != "c" {
		t.Errorf("expected first item c, got %s", chain.Items[0].TicketID)
	}
	if chain.Items[1].TicketID != "a" {
		t.Errorf("expected second item a, got %s", chain.Items[1].TicketID)
	}
	if chain.Items[2].TicketID != "b" {
		t.Errorf("expected third item b, got %s", chain.Items[2].TicketID)
	}
	// Positions should match custom order
	for i, item := range chain.Items {
		if item.Position != i {
			t.Errorf("item %d: expected position %d, got %d", i, i, item.Position)
		}
	}
}

// TestCreateChain_WithCustomOrderRespectingDeps verifies custom order that respects deps is accepted.
func TestCreateChain_WithCustomOrderRespectingDeps(t *testing.T) {
	pool, projectID := setupChainTestDB(t)
	defer pool.Close()

	base := time.Now()
	createTestTickets(t, pool, projectID, map[string]time.Time{
		"A": base,
		"B": base.Add(time.Second),
		"C": base.Add(2 * time.Second),
	})
	createTestDependencies(t, pool, projectID, map[string][]string{
		"C": {"A"}, // C depends on A, but not B
	})

	svc := NewChainService(pool, clock.Real())
	// Custom order: A, C, B — valid because A (blocker of C) is before C
	chain, err := svc.CreateChain(projectID, &types.ChainCreateRequest{
		Name:             "Deps Respecting Order",
		WorkflowName:     "test",
		TicketIDs:        []string{"A", "B", "C"},
		OrderedTicketIDs: []string{"a", "c", "b"},
	})
	if err != nil {
		t.Fatalf("CreateChain with valid dep-respecting order failed: %v", err)
	}

	if chain.Items[0].TicketID != "a" || chain.Items[1].TicketID != "c" || chain.Items[2].TicketID != "b" {
		t.Errorf("expected [a, c, b], got [%s, %s, %s]",
			chain.Items[0].TicketID, chain.Items[1].TicketID, chain.Items[2].TicketID)
	}
}

// TestCreateChain_WithInvalidCustomOrder verifies that blocker-after-dependent is rejected.
func TestCreateChain_WithInvalidCustomOrder(t *testing.T) {
	pool, projectID := setupChainTestDB(t)
	defer pool.Close()

	base := time.Now()
	createTestTickets(t, pool, projectID, map[string]time.Time{
		"A": base,
		"B": base.Add(time.Second),
	})
	createTestDependencies(t, pool, projectID, map[string][]string{
		"B": {"A"}, // B depends on A
	})

	svc := NewChainService(pool, clock.Real())
	_, err := svc.CreateChain(projectID, &types.ChainCreateRequest{
		Name:             "Invalid Order Chain",
		WorkflowName:     "test",
		TicketIDs:        []string{"B"},
		OrderedTicketIDs: []string{"b", "a"}, // invalid: B before A (its blocker)
	})
	if err == nil {
		t.Fatal("expected error for invalid order (blocker after dependent), got nil")
	}
	if !strings.Contains(err.Error(), "invalid order") {
		t.Errorf("expected 'invalid order' error, got: %v", err)
	}
}

// TestCreateChain_WithMismatchedTicketSet verifies that OrderedTicketIDs not matching expanded set is rejected.
func TestCreateChain_WithMismatchedTicketSet(t *testing.T) {
	pool, projectID := setupChainTestDB(t)
	defer pool.Close()

	base := time.Now()
	createTestTickets(t, pool, projectID, map[string]time.Time{
		"A": base,
		"B": base.Add(time.Second),
	})

	svc := NewChainService(pool, clock.Real())
	// OrderedTicketIDs contains a ticket (c) not in the expanded set
	_, err := svc.CreateChain(projectID, &types.ChainCreateRequest{
		Name:             "Mismatched Set Chain",
		WorkflowName:     "test",
		TicketIDs:        []string{"A", "B"},
		OrderedTicketIDs: []string{"a", "b", "c"}, // c not in expanded set
	})
	if err == nil {
		t.Fatal("expected error for mismatched ticket set, got nil")
	}
}

// TestCreateChain_WithMismatchedSizeLess verifies OrderedTicketIDs with fewer tickets is rejected.
func TestCreateChain_WithMismatchedSizeLess(t *testing.T) {
	pool, projectID := setupChainTestDB(t)
	defer pool.Close()

	base := time.Now()
	createTestTickets(t, pool, projectID, map[string]time.Time{
		"A": base,
		"B": base.Add(time.Second),
	})

	svc := NewChainService(pool, clock.Real())
	_, err := svc.CreateChain(projectID, &types.ChainCreateRequest{
		Name:             "Short Order Chain",
		WorkflowName:     "test",
		TicketIDs:        []string{"A", "B"},
		OrderedTicketIDs: []string{"a"}, // missing B
	})
	if err == nil {
		t.Fatal("expected error for OrderedTicketIDs missing a ticket, got nil")
	}
}

// TestCreateChain_WithoutCustomOrder_BackwardCompat verifies auto-sort still works when omitted.
func TestCreateChain_WithoutCustomOrder_BackwardCompat(t *testing.T) {
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

	svc := NewChainService(pool, clock.Real())
	chain, err := svc.CreateChain(projectID, &types.ChainCreateRequest{
		Name:         "Auto Sort Chain",
		WorkflowName: "test",
		TicketIDs:    []string{"B"},
		// No OrderedTicketIDs — uses auto-sort
	})
	if err != nil {
		t.Fatalf("CreateChain without custom order failed: %v", err)
	}

	if len(chain.Items) != 2 {
		t.Fatalf("expected 2 items, got %d", len(chain.Items))
	}
	// Auto-sort should put A before B (A is the blocker)
	if chain.Items[0].TicketID != "a" {
		t.Errorf("expected first item a (blocker), got %s", chain.Items[0].TicketID)
	}
	if chain.Items[1].TicketID != "b" {
		t.Errorf("expected second item b, got %s", chain.Items[1].TicketID)
	}
}

// TestCreateChain_DepsPopulatedInResponse verifies deps map is included in the create response.
func TestCreateChain_DepsPopulatedInResponse(t *testing.T) {
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

	svc := NewChainService(pool, clock.Real())
	chain, err := svc.CreateChain(projectID, &types.ChainCreateRequest{
		Name:         "Deps Test Chain",
		WorkflowName: "test",
		TicketIDs:    []string{"B"},
	})
	if err != nil {
		t.Fatalf("CreateChain failed: %v", err)
	}

	if chain.Deps == nil {
		t.Fatal("expected non-nil Deps in create response")
	}
	if len(chain.Deps["b"]) != 1 || chain.Deps["b"][0] != "a" {
		t.Errorf("expected Deps[b]=[a], got %v", chain.Deps)
	}
}

// TestUpdateChain_WithCustomOrder verifies that a valid custom order is used on update.
func TestUpdateChain_WithCustomOrder(t *testing.T) {
	pool, projectID := setupChainTestDB(t)
	defer pool.Close()

	base := time.Now()
	createTestTickets(t, pool, projectID, map[string]time.Time{
		"A": base,
		"B": base.Add(time.Second),
		"C": base.Add(2 * time.Second),
	})
	// No dependencies among A, B, C

	svc := NewChainService(pool, clock.Real())
	chain, err := svc.CreateChain(projectID, &types.ChainCreateRequest{
		Name:         "Test Chain",
		WorkflowName: "test",
		TicketIDs:    []string{"A"},
	})
	if err != nil {
		t.Fatalf("CreateChain failed: %v", err)
	}

	// Update with custom order — no deps so any order is valid
	updated, err := svc.UpdateChain(chain.ID, &types.ChainUpdateRequest{
		TicketIDs:        []string{"A", "B", "C"},
		OrderedTicketIDs: []string{"c", "b", "a"},
	})
	if err != nil {
		t.Fatalf("UpdateChain with custom order failed: %v", err)
	}

	if len(updated.Items) != 3 {
		t.Fatalf("expected 3 items, got %d", len(updated.Items))
	}
	if updated.Items[0].TicketID != "c" {
		t.Errorf("expected first item c, got %s", updated.Items[0].TicketID)
	}
	if updated.Items[1].TicketID != "b" {
		t.Errorf("expected second item b, got %s", updated.Items[1].TicketID)
	}
	if updated.Items[2].TicketID != "a" {
		t.Errorf("expected third item a, got %s", updated.Items[2].TicketID)
	}
}

// TestUpdateChain_WithInvalidCustomOrder verifies invalid order is rejected on update.
func TestUpdateChain_WithInvalidCustomOrder(t *testing.T) {
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

	svc := NewChainService(pool, clock.Real())
	chain, err := svc.CreateChain(projectID, &types.ChainCreateRequest{
		Name:         "Test Chain",
		WorkflowName: "test",
		TicketIDs:    []string{"A"},
	})
	if err != nil {
		t.Fatalf("CreateChain failed: %v", err)
	}

	// Update with invalid order (B before A but B depends on A)
	_, err = svc.UpdateChain(chain.ID, &types.ChainUpdateRequest{
		TicketIDs:        []string{"B"},
		OrderedTicketIDs: []string{"b", "a"}, // invalid: B before A
	})
	if err == nil {
		t.Fatal("expected error for invalid order in update, got nil")
	}
	if !strings.Contains(err.Error(), "invalid order") {
		t.Errorf("expected 'invalid order' error, got: %v", err)
	}
}

// TestGetChainWithItems_IncludesDeps verifies deps map is included in GetChainWithItems response.
func TestGetChainWithItems_IncludesDeps(t *testing.T) {
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
		"C": {"B"},
	})

	svc := NewChainService(pool, clock.Real())
	chain, err := svc.CreateChain(projectID, &types.ChainCreateRequest{
		Name:         "Deps Chain",
		WorkflowName: "test",
		TicketIDs:    []string{"C"},
	})
	if err != nil {
		t.Fatalf("CreateChain failed: %v", err)
	}

	retrieved, err := svc.GetChainWithItems(chain.ID)
	if err != nil {
		t.Fatalf("GetChainWithItems failed: %v", err)
	}

	if retrieved.Deps == nil {
		t.Fatal("expected non-nil Deps in GetChainWithItems response")
	}
	if len(retrieved.Deps["b"]) == 0 {
		t.Errorf("expected deps for ticket b, got empty: %v", retrieved.Deps)
	}
	if len(retrieved.Deps["c"]) == 0 {
		t.Errorf("expected deps for ticket c, got empty: %v", retrieved.Deps)
	}
	if retrieved.Deps["b"][0] != "a" {
		t.Errorf("expected Deps[b]=[a], got %v", retrieved.Deps["b"])
	}
	if retrieved.Deps["c"][0] != "b" {
		t.Errorf("expected Deps[c]=[b], got %v", retrieved.Deps["c"])
	}
}

// TestUpdateChain_DepsPopulatedInResponse verifies deps map is included in update response.
func TestUpdateChain_DepsPopulatedInResponse(t *testing.T) {
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

	svc := NewChainService(pool, clock.Real())
	chain, err := svc.CreateChain(projectID, &types.ChainCreateRequest{
		Name:         "Deps Update Chain",
		WorkflowName: "test",
		TicketIDs:    []string{"A"},
	})
	if err != nil {
		t.Fatalf("CreateChain failed: %v", err)
	}

	updated, err := svc.UpdateChain(chain.ID, &types.ChainUpdateRequest{
		TicketIDs: []string{"B"},
	})
	if err != nil {
		t.Fatalf("UpdateChain failed: %v", err)
	}

	if updated.Deps == nil {
		t.Fatal("expected non-nil Deps in update response")
	}
	if len(updated.Deps["b"]) != 1 || updated.Deps["b"][0] != "a" {
		t.Errorf("expected Deps[b]=[a], got %v", updated.Deps)
	}
}
