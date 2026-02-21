package service

import (
	"strings"
	"testing"
	"time"

	"be/internal/clock"
	"be/internal/types"
)

// --- validateCustomOrder tests ---

// TestValidateCustomOrder_ValidOrder verifies that a valid order (blockers before dependents) passes.
func TestValidateCustomOrder_ValidOrder(t *testing.T) {
	// A -> B -> C
	deps := map[string][]string{
		"b": {"a"},
		"c": {"b"},
	}
	err := validateCustomOrder([]string{"a", "b", "c"}, deps)
	if err != nil {
		t.Errorf("expected valid order to pass, got error: %v", err)
	}
}

// TestValidateCustomOrder_InvalidOrder verifies that blocker-after-dependent returns an error.
func TestValidateCustomOrder_InvalidOrder(t *testing.T) {
	// B depends on A, but B is listed before A (wrong)
	deps := map[string][]string{
		"b": {"a"},
	}
	err := validateCustomOrder([]string{"b", "a"}, deps)
	if err == nil {
		t.Fatal("expected error for invalid order (blocker after dependent), got nil")
	}
	if !strings.Contains(err.Error(), "invalid order") {
		t.Errorf("expected 'invalid order' in error, got: %v", err)
	}
}

// TestValidateCustomOrder_EmptyDeps verifies that any order passes with no dependencies.
func TestValidateCustomOrder_EmptyDeps(t *testing.T) {
	err := validateCustomOrder([]string{"c", "a", "b"}, map[string][]string{})
	if err != nil {
		t.Errorf("expected no error for empty deps, got: %v", err)
	}
}

// TestValidateCustomOrder_DiamondValid verifies a diamond dependency with a valid order passes.
func TestValidateCustomOrder_DiamondValid(t *testing.T) {
	// D depends on B and C, B and C depend on A
	deps := map[string][]string{
		"b": {"a"},
		"c": {"a"},
		"d": {"b", "c"},
	}
	err := validateCustomOrder([]string{"a", "b", "c", "d"}, deps)
	if err != nil {
		t.Errorf("expected diamond valid order to pass, got: %v", err)
	}
}

// TestValidateCustomOrder_DiamondInvalid verifies that placing dependent before blocker in diamond fails.
func TestValidateCustomOrder_DiamondInvalid(t *testing.T) {
	deps := map[string][]string{
		"b": {"a"},
		"c": {"a"},
		"d": {"b", "c"},
	}
	// d before b — invalid because d depends on b
	err := validateCustomOrder([]string{"a", "c", "d", "b"}, deps)
	if err == nil {
		t.Fatal("expected error for invalid diamond order, got nil")
	}
}

// TestValidateCustomOrder_CaseInsensitive verifies that comparison is case-insensitive.
func TestValidateCustomOrder_CaseInsensitive(t *testing.T) {
	deps := map[string][]string{
		"b": {"a"},
	}
	err := validateCustomOrder([]string{"A", "B"}, deps)
	if err != nil {
		t.Errorf("expected case-insensitive comparison to pass, got: %v", err)
	}
}

// TestValidateCustomOrder_TableDriven covers multiple scenarios concisely.
func TestValidateCustomOrder_TableDriven(t *testing.T) {
	deps := map[string][]string{
		"b": {"a"},
		"c": {"b"},
	}
	cases := []struct {
		name    string
		order   []string
		wantErr bool
	}{
		{"linear valid", []string{"a", "b", "c"}, false},
		{"b before a invalid", []string{"b", "a", "c"}, true},
		{"c before b invalid", []string{"a", "c", "b"}, true},
		{"all reversed invalid", []string{"c", "b", "a"}, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := validateCustomOrder(tc.order, deps)
			if tc.wantErr && err == nil {
				t.Errorf("expected error, got nil")
			}
			if !tc.wantErr && err != nil {
				t.Errorf("expected no error, got: %v", err)
			}
		})
	}
}

// --- validateSameSet tests ---

// TestValidateSameSet_Match verifies that identical sets pass.
func TestValidateSameSet_Match(t *testing.T) {
	err := validateSameSet([]string{"a", "b", "c"}, []string{"c", "a", "b"})
	if err != nil {
		t.Errorf("expected matching sets to pass, got: %v", err)
	}
}

// TestValidateSameSet_DifferentSize verifies that different-sized sets fail.
func TestValidateSameSet_DifferentSize(t *testing.T) {
	err := validateSameSet([]string{"a", "b"}, []string{"a", "b", "c"})
	if err == nil {
		t.Fatal("expected error for different sizes, got nil")
	}
}

// TestValidateSameSet_MissingTicket verifies that a missing ticket causes failure.
func TestValidateSameSet_MissingTicket(t *testing.T) {
	err := validateSameSet([]string{"a", "b", "c"}, []string{"a", "b", "d"})
	if err == nil {
		t.Fatal("expected error for missing ticket, got nil")
	}
}

// TestValidateSameSet_CaseInsensitive verifies case-insensitive set comparison.
func TestValidateSameSet_CaseInsensitive(t *testing.T) {
	err := validateSameSet([]string{"A", "B"}, []string{"a", "b"})
	if err != nil {
		t.Errorf("expected case-insensitive match to pass, got: %v", err)
	}
}

// TestValidateSameSet_ExtraTicketInFirst verifies extra ticket in first set fails.
func TestValidateSameSet_ExtraTicketInFirst(t *testing.T) {
	err := validateSameSet([]string{"a", "b", "c"}, []string{"a", "b"})
	if err == nil {
		t.Fatal("expected error when first set has extra ticket, got nil")
	}
}

// --- PreviewChain tests ---

// TestPreviewChain_ExpandsAndSorts verifies that PreviewChain returns sorted expanded tickets.
func TestPreviewChain_ExpandsAndSorts(t *testing.T) {
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
	resp, err := svc.PreviewChain(projectID, &types.ChainPreviewRequest{
		TicketIDs: []string{"C"},
	})
	if err != nil {
		t.Fatalf("PreviewChain failed: %v", err)
	}

	// Should expand to [a, b, c] in topological order
	if len(resp.TicketIDs) != 3 {
		t.Fatalf("expected 3 ticket IDs, got %d: %v", len(resp.TicketIDs), resp.TicketIDs)
	}
	if resp.TicketIDs[0] != "a" || resp.TicketIDs[1] != "b" || resp.TicketIDs[2] != "c" {
		t.Errorf("expected [a, b, c], got %v", resp.TicketIDs)
	}

	// Deps should include b->a and c->b
	if len(resp.Deps["b"]) != 1 || resp.Deps["b"][0] != "a" {
		t.Errorf("expected deps[b]=[a], got %v", resp.Deps["b"])
	}
	if len(resp.Deps["c"]) != 1 || resp.Deps["c"][0] != "b" {
		t.Errorf("expected deps[c]=[b], got %v", resp.Deps["c"])
	}
}

// TestPreviewChain_AutoAddedByDeps verifies added_by_deps contains transitive additions.
func TestPreviewChain_AutoAddedByDeps(t *testing.T) {
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
	resp, err := svc.PreviewChain(projectID, &types.ChainPreviewRequest{
		TicketIDs: []string{"B"}, // only B requested; A should be auto-added
	})
	if err != nil {
		t.Fatalf("PreviewChain failed: %v", err)
	}

	if len(resp.AddedByDeps) != 1 {
		t.Fatalf("expected 1 added_by_deps, got %d: %v", len(resp.AddedByDeps), resp.AddedByDeps)
	}
	if resp.AddedByDeps[0] != "a" {
		t.Errorf("expected added_by_deps=[a], got %v", resp.AddedByDeps)
	}
}

// TestPreviewChain_NoAutoAdded verifies empty added_by_deps when no transitive expansion occurs.
func TestPreviewChain_NoAutoAdded(t *testing.T) {
	pool, projectID := setupChainTestDB(t)
	defer pool.Close()

	base := time.Now()
	createTestTickets(t, pool, projectID, map[string]time.Time{
		"A": base,
		"B": base.Add(time.Second),
	})
	// No dependencies

	svc := NewChainService(pool, clock.Real())
	resp, err := svc.PreviewChain(projectID, &types.ChainPreviewRequest{
		TicketIDs: []string{"A", "B"},
	})
	if err != nil {
		t.Fatalf("PreviewChain failed: %v", err)
	}

	if len(resp.AddedByDeps) != 0 {
		t.Errorf("expected empty added_by_deps, got %v", resp.AddedByDeps)
	}
}

// TestPreviewChain_DetectsCycle verifies cycle detection returns an error.
func TestPreviewChain_DetectsCycle(t *testing.T) {
	pool, projectID := setupChainTestDB(t)
	defer pool.Close()

	base := time.Now()
	createTestTickets(t, pool, projectID, map[string]time.Time{
		"A": base,
		"B": base.Add(time.Second),
	})
	createTestDependencies(t, pool, projectID, map[string][]string{
		"A": {"B"},
		"B": {"A"},
	})

	svc := NewChainService(pool, clock.Real())
	_, err := svc.PreviewChain(projectID, &types.ChainPreviewRequest{
		TicketIDs: []string{"A"},
	})
	if err == nil {
		t.Fatal("expected cycle error, got nil")
	}
	if !strings.Contains(err.Error(), "cycle detected") {
		t.Errorf("expected cycle error, got: %v", err)
	}
}

// TestPreviewChain_EmptyTickets verifies error on empty ticket list.
func TestPreviewChain_EmptyTickets(t *testing.T) {
	pool, projectID := setupChainTestDB(t)
	defer pool.Close()

	svc := NewChainService(pool, clock.Real())
	_, err := svc.PreviewChain(projectID, &types.ChainPreviewRequest{
		TicketIDs: []string{},
	})
	if err == nil {
		t.Fatal("expected error for empty ticket list, got nil")
	}
	if !strings.Contains(err.Error(), "at least one ticket") {
		t.Errorf("expected 'at least one ticket' error, got: %v", err)
	}
}

// TestPreviewChain_NeverNilDepsOrAddedByDeps verifies that response fields are never nil.
func TestPreviewChain_NeverNilDepsOrAddedByDeps(t *testing.T) {
	pool, projectID := setupChainTestDB(t)
	defer pool.Close()

	createTestTickets(t, pool, projectID, map[string]time.Time{
		"A": time.Now(),
	})

	svc := NewChainService(pool, clock.Real())
	resp, err := svc.PreviewChain(projectID, &types.ChainPreviewRequest{
		TicketIDs: []string{"A"},
	})
	if err != nil {
		t.Fatalf("PreviewChain failed: %v", err)
	}

	if resp.Deps == nil {
		t.Error("expected non-nil Deps map")
	}
	if resp.AddedByDeps == nil {
		t.Error("expected non-nil AddedByDeps slice")
	}
}
