package service

import (
	"strings"
	"testing"
	"time"

	"be/internal/clock"
	"be/internal/db"
	"be/internal/types"
)

// closeTicket marks a ticket as closed via direct SQL (simulates ticket.Close()).
func closeTicket(t *testing.T, pool *db.Pool, projectID, ticketID string) {
	t.Helper()
	now := time.Now().UTC().Format(time.RFC3339Nano)
	_, err := pool.Exec(
		`UPDATE tickets SET status='closed', updated_at=? WHERE LOWER(project_id)=LOWER(?) AND LOWER(id)=LOWER(?)`,
		now, projectID, ticketID,
	)
	if err != nil {
		t.Fatalf("closeTicket(%s): %v", ticketID, err)
	}
}

// containsID reports whether a slice of lowercased IDs contains the given (case-insensitive) id.
func containsID(ids []string, id string) bool {
	lower := strings.ToLower(id)
	for _, v := range ids {
		if strings.ToLower(v) == lower {
			return true
		}
	}
	return false
}

// TestExpandWithBlockers_ClosedBlockerExcluded verifies that a single closed blocker
// is excluded from the expansion result and not added to the deps map.
//
// Setup: C→B→A, where A is closed.
// Expected: allIDs = {B, C}, deps["b"] is empty (A excluded).
func TestExpandWithBlockers_ClosedBlockerExcluded(t *testing.T) {
	pool, projectID := setupChainTestDB(t)
	defer pool.Close()

	base := time.Now()
	createTestTickets(t, pool, projectID, map[string]time.Time{
		"A": base,
		"B": base.Add(time.Second),
		"C": base.Add(2 * time.Second),
	})
	createTestDependencies(t, pool, projectID, map[string][]string{
		"B": {"A"}, // B is blocked by A
		"C": {"B"}, // C is blocked by B
	})

	// Close ticket A
	closeTicket(t, pool, projectID, "A")

	svc := NewChainService(pool, clock.Real())
	allIDs, deps, err := svc.expandWithBlockers(projectID, []string{"C"})
	if err != nil {
		t.Fatalf("expandWithBlockers failed: %v", err)
	}

	// A must not appear in allIDs
	if containsID(allIDs, "A") {
		t.Errorf("closed ticket A should be excluded from allIDs, got %v", allIDs)
	}

	// B and C must appear
	if !containsID(allIDs, "B") {
		t.Errorf("open ticket B should be in allIDs, got %v", allIDs)
	}
	if !containsID(allIDs, "C") {
		t.Errorf("ticket C should be in allIDs, got %v", allIDs)
	}

	// Total count: 2 (B and C)
	if len(allIDs) != 2 {
		t.Errorf("expected 2 tickets (B, C), got %d: %v", len(allIDs), allIDs)
	}

	// deps["b"] must be empty (A was the only blocker and it's closed)
	if len(deps["b"]) != 0 {
		t.Errorf("expected deps[b] to be empty (A is closed), got %v", deps["b"])
	}

	// deps["c"] points to B (open)
	if len(deps["c"]) != 1 || deps["c"][0] != "b" {
		t.Errorf("expected deps[c]=[b], got %v", deps["c"])
	}
}

// TestExpandWithBlockers_TransitiveClosedBlockers verifies that a chain of closed
// blockers are all excluded and expansion stops at the first closed ticket.
//
// Setup: D→C→B→A, where A and B are closed.
// Expected: allIDs = {C, D}.
func TestExpandWithBlockers_TransitiveClosedBlockers(t *testing.T) {
	pool, projectID := setupChainTestDB(t)
	defer pool.Close()

	base := time.Now()
	createTestTickets(t, pool, projectID, map[string]time.Time{
		"A": base,
		"B": base.Add(time.Second),
		"C": base.Add(2 * time.Second),
		"D": base.Add(3 * time.Second),
	})
	createTestDependencies(t, pool, projectID, map[string][]string{
		"B": {"A"}, // B blocked by A (closed)
		"C": {"B"}, // C blocked by B (closed)
		"D": {"C"}, // D blocked by C (open)
	})

	// Close tickets A and B
	closeTicket(t, pool, projectID, "A")
	closeTicket(t, pool, projectID, "B")

	svc := NewChainService(pool, clock.Real())
	allIDs, deps, err := svc.expandWithBlockers(projectID, []string{"D"})
	if err != nil {
		t.Fatalf("expandWithBlockers failed: %v", err)
	}

	// A and B must not appear
	if containsID(allIDs, "A") {
		t.Errorf("closed ticket A should be excluded, got %v", allIDs)
	}
	if containsID(allIDs, "B") {
		t.Errorf("closed ticket B should be excluded, got %v", allIDs)
	}

	// C and D must appear
	if !containsID(allIDs, "C") {
		t.Errorf("open ticket C should be in allIDs, got %v", allIDs)
	}
	if !containsID(allIDs, "D") {
		t.Errorf("ticket D should be in allIDs, got %v", allIDs)
	}

	if len(allIDs) != 2 {
		t.Errorf("expected 2 tickets (C, D), got %d: %v", len(allIDs), allIDs)
	}

	// deps["c"] must be empty (B was the only blocker and it's closed)
	if len(deps["c"]) != 0 {
		t.Errorf("expected deps[c] to be empty (B is closed), got %v", deps["c"])
	}
}

// TestExpandWithBlockers_ExplicitTicketClosedStillIncluded verifies that when the
// caller explicitly requests a closed ticket it is NOT filtered out — only discovered
// blockers are excluded.
func TestExpandWithBlockers_ExplicitTicketClosedStillIncluded(t *testing.T) {
	pool, projectID := setupChainTestDB(t)
	defer pool.Close()

	base := time.Now()
	createTestTickets(t, pool, projectID, map[string]time.Time{
		"A": base,
		"B": base.Add(time.Second),
	})
	createTestDependencies(t, pool, projectID, map[string][]string{
		"B": {"A"}, // B blocked by A
	})

	// Close ticket B (the explicitly requested one)
	closeTicket(t, pool, projectID, "B")

	svc := NewChainService(pool, clock.Real())
	allIDs, _, err := svc.expandWithBlockers(projectID, []string{"B"})
	if err != nil {
		t.Fatalf("expandWithBlockers failed: %v", err)
	}

	// B (requested explicitly) must still be present even though it is closed
	if !containsID(allIDs, "B") {
		t.Errorf("explicitly requested closed ticket B should still be in allIDs, got %v", allIDs)
	}
}

// TestCreateChain_ClosedBlockerNotInItems verifies that CreateChain excludes closed
// blockers from chain items.
//
// Setup: B→A, A is closed.
// Expected: chain has 1 item (B only).
func TestCreateChain_ClosedBlockerNotInItems(t *testing.T) {
	pool, projectID := setupChainTestDB(t)
	defer pool.Close()

	base := time.Now()
	createTestTickets(t, pool, projectID, map[string]time.Time{
		"A": base,
		"B": base.Add(time.Second),
	})
	createTestDependencies(t, pool, projectID, map[string][]string{
		"B": {"A"}, // B blocked by A
	})

	// Close ticket A
	closeTicket(t, pool, projectID, "A")

	svc := NewChainService(pool, clock.Real())
	chain, err := svc.CreateChain(projectID, &types.ChainCreateRequest{
		Name:         "Closed Blocker Chain",
		WorkflowName: "test",
		TicketIDs:    []string{"B"},
	})
	if err != nil {
		t.Fatalf("CreateChain failed: %v", err)
	}

	if len(chain.Items) != 1 {
		t.Fatalf("expected 1 item (B), got %d: %v", len(chain.Items), chain.Items)
	}
	if chain.Items[0].TicketID != "b" {
		t.Errorf("expected item B, got %s", chain.Items[0].TicketID)
	}
}

// TestPreviewChain_ClosedBlockerNotInAddedByDeps verifies that PreviewChain does not
// include closed blockers in AddedByDeps or TicketIDs.
//
// Setup: B→A, A is closed.
// Expected: AddedByDeps=[], TicketIDs=[b].
func TestPreviewChain_ClosedBlockerNotInAddedByDeps(t *testing.T) {
	pool, projectID := setupChainTestDB(t)
	defer pool.Close()

	base := time.Now()
	createTestTickets(t, pool, projectID, map[string]time.Time{
		"A": base,
		"B": base.Add(time.Second),
	})
	createTestDependencies(t, pool, projectID, map[string][]string{
		"B": {"A"}, // B blocked by A
	})

	// Close ticket A
	closeTicket(t, pool, projectID, "A")

	svc := NewChainService(pool, clock.Real())
	resp, err := svc.PreviewChain(projectID, &types.ChainPreviewRequest{
		TicketIDs: []string{"B"},
	})
	if err != nil {
		t.Fatalf("PreviewChain failed: %v", err)
	}

	// AddedByDeps should be empty since A (the blocker) is closed
	if len(resp.AddedByDeps) != 0 {
		t.Errorf("expected empty AddedByDeps (A is closed), got %v", resp.AddedByDeps)
	}

	// TicketIDs should only contain B
	if len(resp.TicketIDs) != 1 {
		t.Errorf("expected 1 ticket ID (B), got %d: %v", len(resp.TicketIDs), resp.TicketIDs)
	}
	if resp.TicketIDs[0] != "b" {
		t.Errorf("expected ticket ID 'b', got %s", resp.TicketIDs[0])
	}

	// Deps map must not include A
	for _, blockers := range resp.Deps {
		for _, blocker := range blockers {
			if strings.ToLower(blocker) == "a" {
				t.Errorf("closed ticket A should not appear in deps, got deps=%v", resp.Deps)
			}
		}
	}
}

// TestExpandWithBlockers_MixedOpenAndClosedBlockers verifies that when a ticket has
// multiple blockers, only the open ones are included.
//
// Setup: C blocked by A (closed) and B (open).
// Expected: allIDs = {B, C}, deps["c"] = [b].
func TestExpandWithBlockers_MixedOpenAndClosedBlockers(t *testing.T) {
	pool, projectID := setupChainTestDB(t)
	defer pool.Close()

	base := time.Now()
	createTestTickets(t, pool, projectID, map[string]time.Time{
		"A": base,
		"B": base.Add(time.Second),
		"C": base.Add(2 * time.Second),
	})
	createTestDependencies(t, pool, projectID, map[string][]string{
		"C": {"A", "B"}, // C blocked by A (closed) and B (open)
	})

	// Close ticket A only
	closeTicket(t, pool, projectID, "A")

	svc := NewChainService(pool, clock.Real())
	allIDs, deps, err := svc.expandWithBlockers(projectID, []string{"C"})
	if err != nil {
		t.Fatalf("expandWithBlockers failed: %v", err)
	}

	// A must not appear
	if containsID(allIDs, "A") {
		t.Errorf("closed ticket A should be excluded, got %v", allIDs)
	}

	// B and C must appear
	if !containsID(allIDs, "B") {
		t.Errorf("open ticket B should be in allIDs, got %v", allIDs)
	}
	if !containsID(allIDs, "C") {
		t.Errorf("ticket C should be in allIDs, got %v", allIDs)
	}

	if len(allIDs) != 2 {
		t.Errorf("expected 2 tickets (B, C), got %d: %v", len(allIDs), allIDs)
	}

	// deps["c"] should only contain B (not A)
	if len(deps["c"]) != 1 || deps["c"][0] != "b" {
		t.Errorf("expected deps[c]=[b], got %v", deps["c"])
	}
}
