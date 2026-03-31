package service

import (
	"path/filepath"
	"strings"
	"testing"
	"time"

	"be/internal/clock"
	"be/internal/db"
)

// setupValidateRunnableDB creates an isolated DB and seeds a project for ValidateRunnable tests.
func setupValidateRunnableDB(t *testing.T, projectID string) (*db.Pool, *TicketService) {
	t.Helper()
	dbPath := filepath.Join(t.TempDir(), "validate_runnable_test.db")
	if err := svcCopyTemplateDB(dbPath); err != nil {
		t.Fatalf("svcCopyTemplateDB: %v", err)
	}
	pool, err := db.OpenPoolExisting(dbPath, db.DefaultPoolConfig())
	if err != nil {
		t.Fatalf("failed to open pool: %v", err)
	}
	t.Cleanup(func() { pool.Close() })

	now := time.Now().UTC().Format(time.RFC3339Nano)
	_, err = pool.Exec(`
		INSERT INTO projects (id, name, root_path, created_at, updated_at)
		VALUES (?, ?, '/tmp/test', ?, ?)`,
		strings.ToLower(projectID), projectID, now, now)
	if err != nil {
		t.Fatalf("failed to seed project: %v", err)
	}

	svc := NewTicketService(pool, clock.Real())
	return pool, svc
}

// svcSeedTicket inserts a ticket with the specified status.
func svcSeedTicket(t *testing.T, pool *db.Pool, projectID, ticketID, status string) {
	t.Helper()
	now := time.Now().UTC().Format(time.RFC3339Nano)
	_, err := pool.Exec(`
		INSERT INTO tickets (id, project_id, title, status, issue_type, priority, created_at, updated_at, created_by)
		VALUES (?, ?, ?, ?, 'task', 2, ?, ?, 'tester')`,
		strings.ToLower(ticketID), strings.ToLower(projectID), ticketID, status, now, now)
	if err != nil {
		t.Fatalf("failed to insert ticket %s: %v", ticketID, err)
	}
}

// svcSeedDependency inserts a dependency record: issueID depends on (is blocked by) blockerID.
func svcSeedDependency(t *testing.T, pool *db.Pool, projectID, issueID, blockerID string) {
	t.Helper()
	now := time.Now().UTC().Format(time.RFC3339Nano)
	_, err := pool.Exec(`
		INSERT INTO dependencies (project_id, issue_id, depends_on_id, type, created_at, created_by)
		VALUES (?, ?, ?, 'blocks', ?, 'tester')`,
		strings.ToLower(projectID), strings.ToLower(issueID), strings.ToLower(blockerID), now)
	if err != nil {
		t.Fatalf("failed to insert dependency %s -> %s: %v", issueID, blockerID, err)
	}
}

// TestValidateRunnable_TicketNotFound verifies that a missing ticket returns a "not found" error.
func TestValidateRunnable_TicketNotFound(t *testing.T) {
	_, svc := setupValidateRunnableDB(t, "proj")

	err := svc.ValidateRunnable("proj", "nonexistent-ticket")
	if err == nil {
		t.Fatal("expected error for nonexistent ticket, got nil")
	}
	if !strings.Contains(err.Error(), "not found") {
		t.Errorf("error = %q, want to contain 'not found'", err.Error())
	}
}

// TestValidateRunnable_ClosedTicket verifies that a closed ticket returns an error
// containing "closed ticket".
func TestValidateRunnable_ClosedTicket(t *testing.T) {
	pool, svc := setupValidateRunnableDB(t, "proj")
	svcSeedTicket(t, pool, "proj", "TICK-1", "closed")

	err := svc.ValidateRunnable("proj", "TICK-1")
	if err == nil {
		t.Fatal("expected error for closed ticket, got nil")
	}
	if !strings.Contains(err.Error(), "closed ticket") {
		t.Errorf("error = %q, want to contain 'closed ticket'", err.Error())
	}
}

// TestValidateRunnable_BlockedTicket verifies that a ticket blocked by an open dependency
// returns an error containing "blocked by" and the blocker's ID.
func TestValidateRunnable_BlockedTicket(t *testing.T) {
	pool, svc := setupValidateRunnableDB(t, "proj")
	svcSeedTicket(t, pool, "proj", "TICK-1", "open")
	svcSeedTicket(t, pool, "proj", "BLOCKER-1", "open")
	svcSeedDependency(t, pool, "proj", "TICK-1", "BLOCKER-1")

	err := svc.ValidateRunnable("proj", "TICK-1")
	if err == nil {
		t.Fatal("expected error for blocked ticket, got nil")
	}
	if !strings.Contains(err.Error(), "blocked by") {
		t.Errorf("error = %q, want to contain 'blocked by'", err.Error())
	}
	if !strings.Contains(strings.ToLower(err.Error()), "blocker-1") {
		t.Errorf("error = %q, want to contain blocker ID 'blocker-1'", err.Error())
	}
}

// TestValidateRunnable_MultipleBlockers verifies that all blocker IDs are listed in the error.
func TestValidateRunnable_MultipleBlockers(t *testing.T) {
	pool, svc := setupValidateRunnableDB(t, "proj")
	svcSeedTicket(t, pool, "proj", "TICK-1", "open")
	svcSeedTicket(t, pool, "proj", "BLOCKER-A", "open")
	svcSeedTicket(t, pool, "proj", "BLOCKER-B", "open")
	svcSeedDependency(t, pool, "proj", "TICK-1", "BLOCKER-A")
	svcSeedDependency(t, pool, "proj", "TICK-1", "BLOCKER-B")

	err := svc.ValidateRunnable("proj", "TICK-1")
	if err == nil {
		t.Fatal("expected error for blocked ticket, got nil")
	}
	if !strings.Contains(err.Error(), "blocked by") {
		t.Errorf("error = %q, want to contain 'blocked by'", err.Error())
	}
	errLower := strings.ToLower(err.Error())
	if !strings.Contains(errLower, "blocker-a") || !strings.Contains(errLower, "blocker-b") {
		t.Errorf("error = %q, want to contain both 'blocker-a' and 'blocker-b'", err.Error())
	}
}

// TestValidateRunnable_OpenUnblocked verifies that an open ticket with no blockers returns nil.
func TestValidateRunnable_OpenUnblocked(t *testing.T) {
	pool, svc := setupValidateRunnableDB(t, "proj")
	svcSeedTicket(t, pool, "proj", "TICK-1", "open")

	err := svc.ValidateRunnable("proj", "TICK-1")
	if err != nil {
		t.Errorf("expected nil for open, unblocked ticket, got %v", err)
	}
}

// TestValidateRunnable_InProgressUnblocked verifies that an in_progress ticket with no
// blockers also returns nil (not blocked by status check).
func TestValidateRunnable_InProgressUnblocked(t *testing.T) {
	pool, svc := setupValidateRunnableDB(t, "proj")
	svcSeedTicket(t, pool, "proj", "TICK-1", "in_progress")

	err := svc.ValidateRunnable("proj", "TICK-1")
	if err != nil {
		t.Errorf("expected nil for in_progress unblocked ticket, got %v", err)
	}
}

// TestValidateRunnable_ClosedBlockerDoesNotBlock verifies that a dependency on a
// *closed* ticket is not treated as a blocker.
func TestValidateRunnable_ClosedBlockerDoesNotBlock(t *testing.T) {
	pool, svc := setupValidateRunnableDB(t, "proj")
	svcSeedTicket(t, pool, "proj", "TICK-1", "open")
	svcSeedTicket(t, pool, "proj", "BLOCKER-DONE", "closed")
	svcSeedDependency(t, pool, "proj", "TICK-1", "BLOCKER-DONE")

	err := svc.ValidateRunnable("proj", "TICK-1")
	if err != nil {
		t.Errorf("closed blocker should not block; got error: %v", err)
	}
}

// TestValidateRunnable_MixedBlockers verifies that only open blockers trigger the error.
// When one blocker is closed and one is open, the error should only list the open blocker.
func TestValidateRunnable_MixedBlockers(t *testing.T) {
	pool, svc := setupValidateRunnableDB(t, "proj")
	svcSeedTicket(t, pool, "proj", "TICK-1", "open")
	svcSeedTicket(t, pool, "proj", "BLK-OPEN", "open")
	svcSeedTicket(t, pool, "proj", "BLK-CLOSED", "closed")
	svcSeedDependency(t, pool, "proj", "TICK-1", "BLK-OPEN")
	svcSeedDependency(t, pool, "proj", "TICK-1", "BLK-CLOSED")

	err := svc.ValidateRunnable("proj", "TICK-1")
	if err == nil {
		t.Fatal("expected error: one open blocker present, got nil")
	}
	if !strings.Contains(strings.ToLower(err.Error()), "blk-open") {
		t.Errorf("error = %q, want to contain open blocker 'blk-open'", err.Error())
	}
	if strings.Contains(strings.ToLower(err.Error()), "blk-closed") {
		t.Errorf("error = %q, must NOT contain closed blocker 'blk-closed'", err.Error())
	}
}
