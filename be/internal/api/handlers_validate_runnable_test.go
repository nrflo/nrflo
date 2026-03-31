package api

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"be/internal/db"
)

// seedProjectInPool inserts a project row into the test DB.
func seedProjectInPool(t *testing.T, pool *db.Pool, projectID string) {
	t.Helper()
	now := time.Now().UTC().Format(time.RFC3339Nano)
	_, err := pool.Exec(`
		INSERT INTO projects (id, name, created_at, updated_at)
		VALUES (?, ?, ?, ?)`,
		strings.ToLower(projectID), projectID, now, now)
	if err != nil {
		t.Fatalf("failed to seed project %s: %v", projectID, err)
	}
}

// seedTicketInPool inserts a ticket row with the given status into the test DB.
func seedTicketInPool(t *testing.T, pool *db.Pool, projectID, ticketID, status string) {
	t.Helper()
	now := time.Now().UTC().Format(time.RFC3339Nano)
	_, err := pool.Exec(`
		INSERT INTO tickets (id, project_id, title, status, issue_type, priority, created_at, updated_at, created_by)
		VALUES (?, ?, ?, ?, 'task', 2, ?, ?, 'tester')`,
		strings.ToLower(ticketID), strings.ToLower(projectID), ticketID, status, now, now)
	if err != nil {
		t.Fatalf("failed to seed ticket %s: %v", ticketID, err)
	}
}

// seedDepInPool inserts a dependency record: issueID is blocked by blockerID.
func seedDepInPool(t *testing.T, pool *db.Pool, projectID, issueID, blockerID string) {
	t.Helper()
	now := time.Now().UTC().Format(time.RFC3339Nano)
	_, err := pool.Exec(`
		INSERT INTO dependencies (project_id, issue_id, depends_on_id, type, created_at, created_by)
		VALUES (?, ?, ?, 'blocks', ?, 'tester')`,
		strings.ToLower(projectID), strings.ToLower(issueID), strings.ToLower(blockerID), now)
	if err != nil {
		t.Fatalf("failed to seed dependency %s -> %s: %v", issueID, blockerID, err)
	}
}

// ── handleRunWorkflow: ValidateRunnable guards ────────────────────────────────

// TestHandleRunWorkflow_ClosedTicket verifies 409 Conflict when the target ticket is closed.
func TestHandleRunWorkflow_ClosedTicket(t *testing.T) {
	s := newTakeControlServer(t)
	seedProjectInPool(t, s.pool, "proj")
	seedTicketInPool(t, s.pool, "proj", "TKT-1", "closed")

	body := `{"workflow":"feature"}`
	req := httptest.NewRequest(http.MethodPost,
		withProject("/api/v1/tickets/TKT-1/workflow/run", "proj"),
		strings.NewReader(body))
	req.SetPathValue("id", "TKT-1")
	rr := httptest.NewRecorder()
	s.handleRunWorkflow(rr, req)

	if rr.Code != http.StatusConflict {
		t.Errorf("status = %d, want 409 (closed ticket)", rr.Code)
	}
	assertErrorContains(t, rr, "closed ticket")
}

// TestHandleRunWorkflow_BlockedTicket verifies 409 Conflict when the ticket has an open blocker.
func TestHandleRunWorkflow_BlockedTicket(t *testing.T) {
	s := newTakeControlServer(t)
	seedProjectInPool(t, s.pool, "proj")
	seedTicketInPool(t, s.pool, "proj", "TKT-1", "open")
	seedTicketInPool(t, s.pool, "proj", "BLOCKER-1", "open")
	seedDepInPool(t, s.pool, "proj", "TKT-1", "BLOCKER-1")

	body := `{"workflow":"feature"}`
	req := httptest.NewRequest(http.MethodPost,
		withProject("/api/v1/tickets/TKT-1/workflow/run", "proj"),
		strings.NewReader(body))
	req.SetPathValue("id", "TKT-1")
	rr := httptest.NewRecorder()
	s.handleRunWorkflow(rr, req)

	if rr.Code != http.StatusConflict {
		t.Errorf("status = %d, want 409 (blocked ticket)", rr.Code)
	}
	assertErrorContains(t, rr, "blocked by")
}

// TestHandleRunWorkflow_TicketNotFound verifies 404 when the ticket does not exist.
func TestHandleRunWorkflow_TicketNotFound(t *testing.T) {
	s := newTakeControlServer(t)
	seedProjectInPool(t, s.pool, "proj")
	// No ticket seeded

	body := `{"workflow":"feature"}`
	req := httptest.NewRequest(http.MethodPost,
		withProject("/api/v1/tickets/TKT-GHOST/workflow/run", "proj"),
		strings.NewReader(body))
	req.SetPathValue("id", "TKT-GHOST")
	rr := httptest.NewRecorder()
	s.handleRunWorkflow(rr, req)

	if rr.Code != http.StatusNotFound {
		t.Errorf("status = %d, want 404 (ticket not found)", rr.Code)
	}
}

// TestHandleRunWorkflow_OpenUnblockedPassesValidation verifies that an open,
// unblocked ticket passes ValidateRunnable and does NOT produce 409 or 404.
func TestHandleRunWorkflow_OpenUnblockedPassesValidation(t *testing.T) {
	s := newTakeControlServer(t)
	seedProjectInPool(t, s.pool, "proj")
	seedTicketInPool(t, s.pool, "proj", "TKT-1", "open")

	body := `{"workflow":"feature"}`
	req := httptest.NewRequest(http.MethodPost,
		withProject("/api/v1/tickets/TKT-1/workflow/run", "proj"),
		strings.NewReader(body))
	req.SetPathValue("id", "TKT-1")
	rr := httptest.NewRecorder()
	s.handleRunWorkflow(rr, req)

	// Validation passes — the response will be an error from orchestrator.Start,
	// but NOT a 409 (closed/blocked) or 404 (ticket not found).
	if rr.Code == http.StatusConflict {
		t.Errorf("open unblocked ticket must not produce 409 Conflict")
	}
	if rr.Code == http.StatusNotFound {
		t.Errorf("open unblocked ticket must not produce 404 (ValidateRunnable regression)")
	}
}

// TestHandleRunWorkflow_ClosedBlockerDoesNotBlock verifies that a dependency on a
// closed ticket does not trigger the blocked-ticket guard (no 409).
func TestHandleRunWorkflow_ClosedBlockerDoesNotBlock(t *testing.T) {
	s := newTakeControlServer(t)
	seedProjectInPool(t, s.pool, "proj")
	seedTicketInPool(t, s.pool, "proj", "TKT-1", "open")
	seedTicketInPool(t, s.pool, "proj", "BLOCKER-DONE", "closed")
	seedDepInPool(t, s.pool, "proj", "TKT-1", "BLOCKER-DONE")

	body := `{"workflow":"feature"}`
	req := httptest.NewRequest(http.MethodPost,
		withProject("/api/v1/tickets/TKT-1/workflow/run", "proj"),
		strings.NewReader(body))
	req.SetPathValue("id", "TKT-1")
	rr := httptest.NewRecorder()
	s.handleRunWorkflow(rr, req)

	// A closed blocker must not trigger the 409 guard.
	if rr.Code == http.StatusConflict {
		t.Errorf("closed blocker must not produce 409 Conflict (not blocked)")
	}
}

// ── handleRetryFailedAgent: ValidateRunnable guards ───────────────────────────

// TestHandleRetryFailed_ClosedTicket verifies 409 Conflict when the target ticket is closed.
func TestHandleRetryFailed_ClosedTicket(t *testing.T) {
	s := newTakeControlServer(t)
	seedProjectInPool(t, s.pool, "proj")
	seedTicketInPool(t, s.pool, "proj", "TKT-2", "closed")

	body := `{"workflow":"feature","session_id":"sess-1"}`
	req := httptest.NewRequest(http.MethodPost,
		withProject("/api/v1/tickets/TKT-2/workflow/retry-failed", "proj"),
		strings.NewReader(body))
	req.SetPathValue("id", "TKT-2")
	rr := httptest.NewRecorder()
	s.handleRetryFailedAgent(rr, req)

	if rr.Code != http.StatusConflict {
		t.Errorf("status = %d, want 409 (closed ticket)", rr.Code)
	}
	assertErrorContains(t, rr, "closed ticket")
}

// TestHandleRetryFailed_BlockedTicket verifies 409 Conflict when the ticket has an open blocker.
func TestHandleRetryFailed_BlockedTicket(t *testing.T) {
	s := newTakeControlServer(t)
	seedProjectInPool(t, s.pool, "proj")
	seedTicketInPool(t, s.pool, "proj", "TKT-2", "open")
	seedTicketInPool(t, s.pool, "proj", "BLK-2", "open")
	seedDepInPool(t, s.pool, "proj", "TKT-2", "BLK-2")

	body := `{"workflow":"feature","session_id":"sess-1"}`
	req := httptest.NewRequest(http.MethodPost,
		withProject("/api/v1/tickets/TKT-2/workflow/retry-failed", "proj"),
		strings.NewReader(body))
	req.SetPathValue("id", "TKT-2")
	rr := httptest.NewRecorder()
	s.handleRetryFailedAgent(rr, req)

	if rr.Code != http.StatusConflict {
		t.Errorf("status = %d, want 409 (blocked ticket)", rr.Code)
	}
	assertErrorContains(t, rr, "blocked by")
}

// TestHandleRetryFailed_TicketNotFound verifies 404 when the ticket does not exist.
func TestHandleRetryFailed_TicketNotFound(t *testing.T) {
	s := newTakeControlServer(t)
	seedProjectInPool(t, s.pool, "proj")

	body := `{"workflow":"feature","session_id":"sess-1"}`
	req := httptest.NewRequest(http.MethodPost,
		withProject("/api/v1/tickets/TKT-GHOST/workflow/retry-failed", "proj"),
		strings.NewReader(body))
	req.SetPathValue("id", "TKT-GHOST")
	rr := httptest.NewRecorder()
	s.handleRetryFailedAgent(rr, req)

	if rr.Code != http.StatusNotFound {
		t.Errorf("status = %d, want 404 (ticket not found)", rr.Code)
	}
}

// TestHandleRetryFailed_OpenUnblockedPassesValidation verifies that an open,
// unblocked ticket passes ValidateRunnable (no 409 or 404 from validation).
func TestHandleRetryFailed_OpenUnblockedPassesValidation(t *testing.T) {
	s := newTakeControlServer(t)
	seedProjectInPool(t, s.pool, "proj")
	seedTicketInPool(t, s.pool, "proj", "TKT-2", "open")

	body := `{"workflow":"feature","session_id":"sess-1"}`
	req := httptest.NewRequest(http.MethodPost,
		withProject("/api/v1/tickets/TKT-2/workflow/retry-failed", "proj"),
		strings.NewReader(body))
	req.SetPathValue("id", "TKT-2")
	rr := httptest.NewRecorder()
	s.handleRetryFailedAgent(rr, req)

	// Validation passes — should NOT be 409 Conflict or 404 Not Found.
	// The orchestrator.RetryFailedAgent will fail with a different error.
	if rr.Code == http.StatusConflict {
		t.Errorf("open unblocked ticket must not produce 409 Conflict")
	}
	if rr.Code == http.StatusNotFound {
		t.Errorf("open unblocked ticket must not produce 404 (ValidateRunnable regression)")
	}
}
