package service

import (
	"strings"
	"testing"
	"time"

	"be/internal/clock"
	"be/internal/db"
)

// rangeTestNow is the fixed "today" used across all GetRange tests.
var rangeTestNow = time.Date(2026, 3, 15, 12, 0, 0, 0, time.UTC)

func insertRangeTicket(t *testing.T, pool *db.Pool, projectID, id, createdAt string) {
	t.Helper()
	_, err := pool.Exec(`INSERT INTO tickets (id, project_id, title, status, issue_type, priority, created_at, updated_at, created_by) VALUES (?, ?, 'T', 'open', 'feature', 2, ?, ?, 'test')`,
		strings.ToLower(projectID)+"-"+id, strings.ToLower(projectID), createdAt, createdAt)
	if err != nil {
		t.Fatalf("insertRangeTicket(%s): %v", id, err)
	}
}

func insertRangeClosedTicket(t *testing.T, pool *db.Pool, projectID, id, createdAt, closedAt string) {
	t.Helper()
	_, err := pool.Exec(`INSERT INTO tickets (id, project_id, title, status, issue_type, priority, created_at, updated_at, closed_at, created_by) VALUES (?, ?, 'T', 'closed', 'feature', 2, ?, ?, ?, 'test')`,
		strings.ToLower(projectID)+"-"+id, strings.ToLower(projectID), createdAt, closedAt, closedAt)
	if err != nil {
		t.Fatalf("insertRangeClosedTicket(%s): %v", id, err)
	}
}

func setupRangeWorkflow(t *testing.T, pool *db.Pool, projectID, wfInstanceID, ts string) {
	t.Helper()
	pid := strings.ToLower(projectID)
	_, _ = pool.Exec(`INSERT OR IGNORE INTO workflows (id, project_id, description, created_at, updated_at) VALUES ('range-wf', ?, 'Range test', ?, ?)`, pid, ts, ts)
	_, err := pool.Exec(`INSERT INTO workflow_instances (id, project_id, ticket_id, workflow_id, status, created_at, updated_at) VALUES (?, ?, 'TICKET-R', 'range-wf', 'active', ?, ?)`, wfInstanceID, pid, ts, ts)
	if err != nil {
		t.Fatalf("setupRangeWorkflow: %v", err)
	}
}

func insertRangeSession(t *testing.T, pool *db.Pool, projectID, wfInstanceID, id, startedAt, endedAt, status string, contextLeft int) {
	t.Helper()
	_, err := pool.Exec(`INSERT INTO agent_sessions (id, project_id, ticket_id, workflow_instance_id, phase, agent_type, status, context_left, started_at, ended_at, created_at, updated_at) VALUES (?, ?, 'TICKET-R', ?, 'impl', 'implementor', ?, ?, ?, ?, ?, ?)`,
		id, strings.ToLower(projectID), wfInstanceID, status, contextLeft, startedAt, endedAt, startedAt, startedAt)
	if err != nil {
		t.Fatalf("insertRangeSession(%s): %v", id, err)
	}
}

func TestValidRange(t *testing.T) {
	tests := []struct {
		rangeType string
		want      bool
	}{
		{"today", true},
		{"week", true},
		{"month", true},
		{"all", true},
		{"", false},
		{"day", false},
		{"yearly", false},
		{"TODAY", false},
		{"Week", false},
	}
	for _, tc := range tests {
		got := ValidRange(tc.rangeType)
		if got != tc.want {
			t.Errorf("ValidRange(%q) = %v, want %v", tc.rangeType, got, tc.want)
		}
	}
}

func TestGetRange_Today(t *testing.T) {
	pool, projectID := setupDailyStatsTestDB(t)
	defer pool.Close()

	clk := clock.NewTest(rangeTestNow)
	svc := NewDailyStatsService(pool, clk)
	ts := rangeTestNow.Format(time.RFC3339Nano)

	insertRangeTicket(t, pool, projectID, "t1", ts)

	stats, err := svc.GetRange(projectID, "today")
	if err != nil {
		t.Fatalf("GetRange today: %v", err)
	}
	if stats.TicketsCreated != 1 {
		t.Errorf("today TicketsCreated = %d, want 1", stats.TicketsCreated)
	}
	if stats.Date != "2026-03-15" {
		t.Errorf("today Date = %q, want 2026-03-15", stats.Date)
	}

	// Verify delegation to GetToday: result persists in daily_stats table.
	var count int
	_ = pool.QueryRow(`SELECT COUNT(*) FROM daily_stats WHERE project_id = ? AND date = '2026-03-15'`, strings.ToLower(projectID)).Scan(&count)
	if count != 1 {
		t.Errorf("daily_stats rows = %d, want 1 (today persists)", count)
	}
}

func TestGetRange_Week(t *testing.T) {
	pool, projectID := setupDailyStatsTestDB(t)
	defer pool.Close()

	clk := clock.NewTest(rangeTestNow)
	svc := NewDailyStatsService(pool, clk)
	ts := rangeTestNow.Format(time.RFC3339Nano)
	setupRangeWorkflow(t, pool, projectID, "wf-week", ts)

	// In range (>= 2026-03-08): today and 3 days ago.
	insertRangeTicket(t, pool, projectID, "t-today", rangeTestNow.Format(time.RFC3339Nano))
	insertRangeTicket(t, pool, projectID, "t-3d", rangeTestNow.AddDate(0, 0, -3).Format(time.RFC3339Nano))
	// Out of range: 8 days ago (< 2026-03-08).
	insertRangeTicket(t, pool, projectID, "t-8d", rangeTestNow.AddDate(0, 0, -8).Format(time.RFC3339Nano))

	// Sessions: in range (today + 4 days ago), out of range (8 days ago).
	inStart := rangeTestNow.Add(-1 * time.Hour).Format(time.RFC3339Nano)
	inEnd := rangeTestNow.Format(time.RFC3339Nano)
	insertRangeSession(t, pool, projectID, "wf-week", "sess-today", inStart, inEnd, "completed", 50)
	ago4Start := rangeTestNow.AddDate(0, 0, -4).Add(-1 * time.Hour).Format(time.RFC3339Nano)
	ago4End := rangeTestNow.AddDate(0, 0, -4).Format(time.RFC3339Nano)
	insertRangeSession(t, pool, projectID, "wf-week", "sess-4d", ago4Start, ago4End, "completed", 50)
	ago8Start := rangeTestNow.AddDate(0, 0, -8).Add(-1 * time.Hour).Format(time.RFC3339Nano)
	ago8End := rangeTestNow.AddDate(0, 0, -8).Format(time.RFC3339Nano)
	insertRangeSession(t, pool, projectID, "wf-week", "sess-8d", ago8Start, ago8End, "completed", 50)

	stats, err := svc.GetRange(projectID, "week")
	if err != nil {
		t.Fatalf("GetRange week: %v", err)
	}
	if stats.TicketsCreated != 2 {
		t.Errorf("week TicketsCreated = %d, want 2", stats.TicketsCreated)
	}
	// 2 in-range sessions * 100000 tokens each = 200000.
	if stats.TokensSpent != 200000 {
		t.Errorf("week TokensSpent = %d, want 200000", stats.TokensSpent)
	}
}

func TestGetRange_Month(t *testing.T) {
	pool, projectID := setupDailyStatsTestDB(t)
	defer pool.Close()

	clk := clock.NewTest(rangeTestNow)
	svc := NewDailyStatsService(pool, clk)
	ts := rangeTestNow.Format(time.RFC3339Nano)
	setupRangeWorkflow(t, pool, projectID, "wf-month", ts)

	// In range (>= 2026-02-13): today and 20 days ago.
	insertRangeTicket(t, pool, projectID, "t-today", rangeTestNow.Format(time.RFC3339Nano))
	insertRangeTicket(t, pool, projectID, "t-20d", rangeTestNow.AddDate(0, 0, -20).Format(time.RFC3339Nano))
	// Out of range: 35 days ago (< 2026-02-13).
	insertRangeTicket(t, pool, projectID, "t-35d", rangeTestNow.AddDate(0, 0, -35).Format(time.RFC3339Nano))

	// Closed ticket: created 35 days ago (outside range), closed 20 days ago (inside range).
	createdOld := rangeTestNow.AddDate(0, 0, -35).Format(time.RFC3339Nano)
	closedAt20 := rangeTestNow.AddDate(0, 0, -20).Format(time.RFC3339Nano)
	insertRangeClosedTicket(t, pool, projectID, "tc-old", createdOld, closedAt20)

	stats, err := svc.GetRange(projectID, "month")
	if err != nil {
		t.Fatalf("GetRange month: %v", err)
	}
	if stats.TicketsCreated != 2 {
		t.Errorf("month TicketsCreated = %d, want 2", stats.TicketsCreated)
	}
	if stats.TicketsClosed != 1 {
		t.Errorf("month TicketsClosed = %d, want 1", stats.TicketsClosed)
	}
}

func TestGetRange_All(t *testing.T) {
	pool, projectID := setupDailyStatsTestDB(t)
	defer pool.Close()

	clk := clock.NewTest(rangeTestNow)
	svc := NewDailyStatsService(pool, clk)
	ts := rangeTestNow.Format(time.RFC3339Nano)
	setupRangeWorkflow(t, pool, projectID, "wf-all", ts)

	// Insert tickets at today, 100 days ago, and 500 days ago — all should count.
	insertRangeTicket(t, pool, projectID, "t-today", rangeTestNow.Format(time.RFC3339Nano))
	insertRangeTicket(t, pool, projectID, "t-100d", rangeTestNow.AddDate(0, 0, -100).Format(time.RFC3339Nano))
	insertRangeTicket(t, pool, projectID, "t-500d", rangeTestNow.AddDate(0, 0, -500).Format(time.RFC3339Nano))

	// Session 500 days ago.
	start500 := rangeTestNow.AddDate(0, 0, -500).Add(-30 * time.Minute).Format(time.RFC3339Nano)
	end500 := rangeTestNow.AddDate(0, 0, -500).Format(time.RFC3339Nano)
	insertRangeSession(t, pool, projectID, "wf-all", "sess-500d", start500, end500, "completed", 0)

	stats, err := svc.GetRange(projectID, "all")
	if err != nil {
		t.Fatalf("GetRange all: %v", err)
	}
	if stats.TicketsCreated != 3 {
		t.Errorf("all TicketsCreated = %d, want 3", stats.TicketsCreated)
	}
	// Session 500 days ago with context_left=0: 200000 tokens.
	if stats.TokensSpent != 200000 {
		t.Errorf("all TokensSpent = %d, want 200000", stats.TokensSpent)
	}
}

func TestGetRange_NotPersisted(t *testing.T) {
	pool, projectID := setupDailyStatsTestDB(t)
	defer pool.Close()

	clk := clock.NewTest(rangeTestNow)
	svc := NewDailyStatsService(pool, clk)

	for _, rangeType := range []string{"week", "month", "all"} {
		_, err := svc.GetRange(projectID, rangeType)
		if err != nil {
			t.Fatalf("GetRange %s: %v", rangeType, err)
		}
	}

	var count int
	_ = pool.QueryRow(`SELECT COUNT(*) FROM daily_stats WHERE project_id = ?`, strings.ToLower(projectID)).Scan(&count)
	if count != 0 {
		t.Errorf("daily_stats rows = %d, want 0 (week/month/all must not persist)", count)
	}
}

func TestGetRange_BoundaryDate(t *testing.T) {
	pool, projectID := setupDailyStatsTestDB(t)
	defer pool.Close()

	clk := clock.NewTest(rangeTestNow)
	svc := NewDailyStatsService(pool, clk)
	ts := rangeTestNow.Format(time.RFC3339Nano)
	setupRangeWorkflow(t, pool, projectID, "wf-bound", ts)

	// Week fromDate = 2026-03-08. Session ending exactly on boundary must be included.
	boundaryEnd := time.Date(2026, 3, 8, 23, 59, 59, 0, time.UTC).Format(time.RFC3339Nano)
	boundaryStart := time.Date(2026, 3, 8, 22, 0, 0, 0, time.UTC).Format(time.RFC3339Nano)
	insertRangeSession(t, pool, projectID, "wf-bound", "sess-boundary", boundaryStart, boundaryEnd, "completed", 50)

	// Session one day before boundary (2026-03-07) must be excluded.
	beforeEnd := time.Date(2026, 3, 7, 23, 59, 59, 0, time.UTC).Format(time.RFC3339Nano)
	beforeStart := time.Date(2026, 3, 7, 22, 0, 0, 0, time.UTC).Format(time.RFC3339Nano)
	insertRangeSession(t, pool, projectID, "wf-bound", "sess-before", beforeStart, beforeEnd, "completed", 50)

	stats, err := svc.GetRange(projectID, "week")
	if err != nil {
		t.Fatalf("GetRange week boundary: %v", err)
	}
	// Only the boundary session: 100000 tokens.
	if stats.TokensSpent != 100000 {
		t.Errorf("boundary TokensSpent = %d, want 100000 (only boundary session, not before)", stats.TokensSpent)
	}
}

func TestGetRange_ExcludesRunningContinued(t *testing.T) {
	pool, projectID := setupDailyStatsTestDB(t)
	defer pool.Close()

	clk := clock.NewTest(rangeTestNow)
	svc := NewDailyStatsService(pool, clk)
	ts := rangeTestNow.Format(time.RFC3339Nano)
	setupRangeWorkflow(t, pool, projectID, "wf-excl", ts)

	startedAt := rangeTestNow.Add(-1 * time.Hour).Format(time.RFC3339Nano)
	endedAt := rangeTestNow.Format(time.RFC3339Nano)
	insertRangeSession(t, pool, projectID, "wf-excl", "sess-completed", startedAt, endedAt, "completed", 50)
	insertRangeSession(t, pool, projectID, "wf-excl", "sess-running", startedAt, endedAt, "running", 50)
	insertRangeSession(t, pool, projectID, "wf-excl", "sess-continued", startedAt, endedAt, "continued", 50)
	insertRangeSession(t, pool, projectID, "wf-excl", "sess-failed", startedAt, endedAt, "failed", 50)

	stats, err := svc.GetRange(projectID, "week")
	if err != nil {
		t.Fatalf("GetRange week exclude: %v", err)
	}
	// Only completed + failed: 2 * 100000 = 200000. running + continued excluded.
	if stats.TokensSpent != 200000 {
		t.Errorf("TokensSpent = %d, want 200000 (running/continued excluded)", stats.TokensSpent)
	}
	const tol = 0.1
	wantTime := 3600.0 * 2
	if stats.AgentTimeSec < wantTime-tol || stats.AgentTimeSec > wantTime+tol {
		t.Errorf("AgentTimeSec = %.1f, want ~%.1f (running/continued excluded)", stats.AgentTimeSec, wantTime)
	}
}
