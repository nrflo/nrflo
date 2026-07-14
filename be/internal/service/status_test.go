package service

import (
	"path/filepath"
	"testing"
	"time"

	"be/internal/clock"
	"be/internal/db"
	"be/internal/model"
	"be/internal/repo"
)

const statusTestProjectID = "status-proj"

func setupStatusTestEnv(t *testing.T) *db.Pool {
	t.Helper()
	dbPath := filepath.Join(t.TempDir(), "status_test.db")
	if err := svcCopyTemplateDB(dbPath); err != nil {
		t.Fatalf("copy template DB: %v", err)
	}
	pool, err := db.OpenPoolExisting(dbPath, db.DefaultPoolConfig())
	if err != nil {
		t.Fatalf("open pool: %v", err)
	}
	t.Cleanup(func() { pool.Close() })

	now := time.Now().UTC().Format(time.RFC3339Nano)
	mustExec(t, pool, `INSERT INTO projects (id, name, root_path, created_at, updated_at) VALUES (?, 'Status Test', '/tmp', ?, ?)`,
		statusTestProjectID, now, now)
	return pool
}

func seedStatusTicket(t *testing.T, pool *db.Pool, id, status string) {
	t.Helper()
	now := time.Now().UTC().Format(time.RFC3339Nano)
	mustExec(t, pool, `INSERT INTO tickets (id, project_id, title, status, priority, created_at, updated_at, created_by)
		VALUES (?, ?, ?, ?, 1, ?, ?, 'test')`, id, statusTestProjectID, "T "+id, status, now, now)
}

func seedStatusDependency(t *testing.T, pool *db.Pool, issueID, dependsOnID string) {
	t.Helper()
	now := time.Now().UTC().Format(time.RFC3339Nano)
	mustExec(t, pool, `INSERT INTO dependencies (project_id, issue_id, depends_on_id, type, created_at, created_by)
		VALUES (?, ?, ?, 'blocks', ?, 'test')`, statusTestProjectID, issueID, dependsOnID, now)
}

func TestStatusService_ProjectStatus_CountsAndBlockedReady(t *testing.T) {
	pool := setupStatusTestEnv(t)
	seedStatusTicket(t, pool, "T-open-blocked", "open")
	seedStatusTicket(t, pool, "T-open-blocker", "open")
	seedStatusTicket(t, pool, "T-ready", "in_progress")
	seedStatusTicket(t, pool, "T-closed", "closed")
	seedStatusDependency(t, pool, "T-open-blocked", "T-open-blocker")

	status, err := NewStatusService(pool, clock.Real()).ProjectStatus(statusTestProjectID, 10)
	if err != nil {
		t.Fatalf("ProjectStatus: %v", err)
	}

	counts, ok := status["counts"].(map[string]int)
	if !ok {
		t.Fatalf("counts = %v (%T), want map[string]int", status["counts"], status["counts"])
	}
	if counts["total"] != 4 {
		t.Errorf("counts.total = %d, want 4", counts["total"])
	}
	if counts["closed"] != 1 {
		t.Errorf("counts.closed = %d, want 1", counts["closed"])
	}
	if counts["blocked"] != 1 {
		t.Errorf("counts.blocked = %d, want 1 (T-open-blocked)", counts["blocked"])
	}
	if got := status["ready_count"]; got != 2 {
		t.Errorf("ready_count = %v, want 2 (T-open-blocker, T-ready)", got)
	}
}

func TestStatusService_ProjectStatus_LimitTrimsPending(t *testing.T) {
	pool := setupStatusTestEnv(t)
	for i := 0; i < 5; i++ {
		seedStatusTicket(t, pool, "T-limit-"+string(rune('a'+i)), "open")
	}

	status, err := NewStatusService(pool, clock.Real()).ProjectStatus(statusTestProjectID, 2)
	if err != nil {
		t.Fatalf("ProjectStatus: %v", err)
	}
	pending, ok := status["pending_tickets"].([]*repo.PendingTicket)
	if !ok {
		t.Fatalf("pending_tickets = %v (%T), want []*repo.PendingTicket", status["pending_tickets"], status["pending_tickets"])
	}
	if len(pending) != 2 {
		t.Errorf("len(pending_tickets) = %d, want 2 (limit applied)", len(pending))
	}
}

func TestStatusService_ProjectStatus_LimitZero_DefaultsTo10(t *testing.T) {
	pool := setupStatusTestEnv(t)
	for i := 0; i < 12; i++ {
		seedStatusTicket(t, pool, "T-def-"+string(rune('a'+i)), "open")
	}

	status, err := NewStatusService(pool, clock.Real()).ProjectStatus(statusTestProjectID, 0)
	if err != nil {
		t.Fatalf("ProjectStatus: %v", err)
	}
	pending, ok := status["pending_tickets"].([]*repo.PendingTicket)
	if !ok {
		t.Fatalf("pending_tickets = %v (%T), want []*repo.PendingTicket", status["pending_tickets"], status["pending_tickets"])
	}
	if len(pending) != 10 {
		t.Errorf("len(pending_tickets) = %d, want 10 (default limit)", len(pending))
	}
}

func TestStatusService_ProjectStatus_EmptyProject_NoTickets(t *testing.T) {
	pool := setupStatusTestEnv(t)

	status, err := NewStatusService(pool, clock.Real()).ProjectStatus(statusTestProjectID, 10)
	if err != nil {
		t.Fatalf("ProjectStatus: %v", err)
	}
	pending, ok := status["pending_tickets"].([]*repo.PendingTicket)
	if !ok || len(pending) != 0 {
		t.Errorf("pending_tickets = %v, want empty []*repo.PendingTicket", status["pending_tickets"])
	}
	closed, ok := status["recent_closed"].([]*model.Ticket)
	if !ok || len(closed) != 0 {
		t.Errorf("recent_closed = %v, want empty []*model.Ticket", status["recent_closed"])
	}
}
