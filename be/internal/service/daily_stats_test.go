package service

import (
	"path/filepath"
	"strings"
	"testing"
	"time"

	"be/internal/clock"
	"be/internal/db"
)

func setupDailyStatsTestDB(t *testing.T) (*db.Pool, string) {
	t.Helper()
	dbPath := filepath.Join(t.TempDir(), "daily_stats_test.db")
	pool, err := db.NewPoolPath(dbPath, db.DefaultPoolConfig())
	if err != nil {
		t.Fatalf("failed to create pool: %v", err)
	}

	projectID := "test-project"
	now := time.Now().UTC().Format(time.RFC3339Nano)
	_, err = pool.Exec(`
		INSERT INTO projects (id, name, root_path, created_at, updated_at)
		VALUES (?, 'Test Project', '/tmp/test', ?, ?)`,
		strings.ToLower(projectID), now, now)
	if err != nil {
		t.Fatalf("failed to create test project: %v", err)
	}

	return pool, projectID
}

func TestComputeAndStore_WithKnownData(t *testing.T) {
	pool, projectID := setupDailyStatsTestDB(t)
	defer pool.Close()

	today := "2026-02-13"
	todayTime := time.Date(2026, 2, 13, 10, 0, 0, 0, time.UTC)

	// Insert tickets created today
	createdToday := todayTime.Format(time.RFC3339Nano)
	for i := 1; i <= 5; i++ {
		_, err := pool.Exec(`
			INSERT INTO tickets (id, project_id, title, status, issue_type, priority, created_at, updated_at, created_by)
			VALUES (?, ?, ?, 'open', 'feature', 2, ?, ?, 'test')`,
			strings.ToLower(projectID)+"-"+string(rune(i)), strings.ToLower(projectID),
			"Ticket "+string(rune(i)), createdToday, createdToday, "test")
		if err != nil {
			t.Fatalf("failed to insert ticket: %v", err)
		}
	}

	// Insert tickets closed today (some were created earlier, some created today)
	yesterdayTime := time.Date(2026, 2, 12, 10, 0, 0, 0, time.UTC)
	yesterday := yesterdayTime.Format(time.RFC3339Nano)
	closedToday := todayTime.Add(2 * time.Hour).Format(time.RFC3339Nano)

	// Tickets created yesterday, closed today (count towards closed)
	for i := 6; i <= 8; i++ {
		_, err := pool.Exec(`
			INSERT INTO tickets (id, project_id, title, status, issue_type, priority, created_at, updated_at, closed_at, created_by)
			VALUES (?, ?, ?, 'closed', 'feature', 2, ?, ?, ?, 'test')`,
			strings.ToLower(projectID)+"-"+string(rune(i)), strings.ToLower(projectID),
			"Ticket "+string(rune(i)), yesterday, closedToday, closedToday, "test")
		if err != nil {
			t.Fatalf("failed to insert closed ticket: %v", err)
		}
	}

	// Create workflow instance for agent sessions
	wfID := "test-workflow-instance"
	_, err := pool.Exec(`
		INSERT INTO workflows (id, project_id, description, phases, created_at, updated_at)
		VALUES ('feature', ?, 'Test workflow', '[]', ?, ?)`,
		strings.ToLower(projectID), createdToday, createdToday)
	if err != nil {
		t.Fatalf("failed to create workflow: %v", err)
	}

	_, err = pool.Exec(`
		INSERT INTO workflow_instances (id, project_id, ticket_id, workflow_id, status, phase_order, created_at, updated_at)
		VALUES (?, ?, 'TICKET-1', 'feature', 'active', '[]', ?, ?)`,
		wfID, strings.ToLower(projectID), createdToday, createdToday)
	if err != nil {
		t.Fatalf("failed to create workflow instance: %v", err)
	}

	// Insert agent sessions ending today with various context_left values
	// Session 1: completed, context_left=50 -> tokens = 200000 * (100-50) / 100 = 100000
	// Session 2: completed, context_left=75 -> tokens = 200000 * (100-75) / 100 = 50000
	// Session 3: completed, context_left=25 -> tokens = 200000 * (100-25) / 100 = 150000
	// Total expected tokens: 300000

	startedAt1 := todayTime.Format(time.RFC3339Nano)
	endedAt1 := todayTime.Add(1 * time.Hour).Format(time.RFC3339Nano) // 3600 seconds

	startedAt2 := todayTime.Add(2 * time.Hour).Format(time.RFC3339Nano)
	endedAt2 := todayTime.Add(2*time.Hour + 30*time.Minute).Format(time.RFC3339Nano) // 1800 seconds

	startedAt3 := todayTime.Add(3 * time.Hour).Format(time.RFC3339Nano)
	endedAt3 := todayTime.Add(3*time.Hour + 15*time.Minute).Format(time.RFC3339Nano) // 900 seconds

	// Total expected agent_time_sec: 3600 + 1800 + 900 = 6300

	sessions := []struct {
		id          string
		startedAt   string
		endedAt     string
		status      string
		contextLeft int
	}{
		{"session-1", startedAt1, endedAt1, "completed", 50},
		{"session-2", startedAt2, endedAt2, "completed", 75},
		{"session-3", startedAt3, endedAt3, "completed", 25},
	}

	for _, s := range sessions {
		_, err := pool.Exec(`
			INSERT INTO agent_sessions (id, project_id, ticket_id, workflow_instance_id, phase, agent_type, status, context_left, started_at, ended_at, created_at, updated_at)
			VALUES (?, ?, 'TICKET-1', ?, 'implementation', 'implementor', ?, ?, ?, ?, ?, ?)`,
			s.id, strings.ToLower(projectID), wfID, s.status, s.contextLeft, s.startedAt, s.endedAt, createdToday, createdToday)
		if err != nil {
			t.Fatalf("failed to insert agent session %s: %v", s.id, err)
		}
	}

	// Test ComputeAndStore
	svc := NewDailyStatsService(pool, clock.Real())
	stats, err := svc.ComputeAndStore(projectID, today)
	if err != nil {
		t.Fatalf("ComputeAndStore failed: %v", err)
	}

	// Verify results
	if stats.ProjectID != projectID {
		t.Errorf("expected project_id %q, got %q", projectID, stats.ProjectID)
	}
	if stats.Date != today {
		t.Errorf("expected date %q, got %q", today, stats.Date)
	}
	if stats.TicketsCreated != 5 {
		t.Errorf("expected tickets_created 5, got %d", stats.TicketsCreated)
	}
	if stats.TicketsClosed != 3 {
		t.Errorf("expected tickets_closed 3, got %d", stats.TicketsClosed)
	}
	if stats.TokensSpent != 300000 {
		t.Errorf("expected tokens_spent 300000, got %d", stats.TokensSpent)
	}
	// Use tolerance for float comparison
	const tolerance = 0.1
	if stats.AgentTimeSec < 6300.0-tolerance || stats.AgentTimeSec > 6300.0+tolerance {
		t.Errorf("expected agent_time_sec ~6300.0, got %.1f", stats.AgentTimeSec)
	}
}

func TestComputeAndStore_ExcludesRunningAndContinuedSessions(t *testing.T) {
	pool, projectID := setupDailyStatsTestDB(t)
	defer pool.Close()

	today := "2026-02-13"
	todayTime := time.Date(2026, 2, 13, 10, 0, 0, 0, time.UTC)
	createdToday := todayTime.Format(time.RFC3339Nano)

	// Create workflow instance
	wfID := "test-workflow-instance"
	_, err := pool.Exec(`
		INSERT INTO workflows (id, project_id, description, phases, created_at, updated_at)
		VALUES ('feature', ?, 'Test workflow', '[]', ?, ?)`,
		strings.ToLower(projectID), createdToday, createdToday)
	if err != nil {
		t.Fatalf("failed to create workflow: %v", err)
	}

	_, err = pool.Exec(`
		INSERT INTO workflow_instances (id, project_id, ticket_id, workflow_id, status, phase_order, created_at, updated_at)
		VALUES (?, ?, 'TICKET-1', 'feature', 'active', '[]', ?, ?)`,
		wfID, strings.ToLower(projectID), createdToday, createdToday)
	if err != nil {
		t.Fatalf("failed to create workflow instance: %v", err)
	}

	startedAt := todayTime.Format(time.RFC3339Nano)
	endedAt := todayTime.Add(1 * time.Hour).Format(time.RFC3339Nano)

	// Insert sessions with different statuses
	sessions := []struct {
		id          string
		status      string
		contextLeft int
	}{
		{"session-completed", "completed", 50}, // Should be counted
		{"session-running", "running", 60},     // Should NOT be counted
		{"session-continued", "continued", 70}, // Should NOT be counted
		{"session-failed", "failed", 80},       // Should be counted
		{"session-timeout", "timeout", 90},     // Should be counted
	}

	for _, s := range sessions {
		_, err := pool.Exec(`
			INSERT INTO agent_sessions (id, project_id, ticket_id, workflow_instance_id, phase, agent_type, status, context_left, started_at, ended_at, created_at, updated_at)
			VALUES (?, ?, 'TICKET-1', ?, 'implementation', 'implementor', ?, ?, ?, ?, ?, ?)`,
			s.id, strings.ToLower(projectID), wfID, s.status, s.contextLeft, startedAt, endedAt, createdToday, createdToday)
		if err != nil {
			t.Fatalf("failed to insert agent session %s: %v", s.id, err)
		}
	}

	// Test ComputeAndStore
	svc := NewDailyStatsService(pool, clock.Real())
	stats, err := svc.ComputeAndStore(projectID, today)
	if err != nil {
		t.Fatalf("ComputeAndStore failed: %v", err)
	}

	// Expected tokens: completed (100k) + failed (40k) + timeout (20k) = 160000
	expectedTokens := int64(100000 + 40000 + 20000)
	if stats.TokensSpent != expectedTokens {
		t.Errorf("expected tokens_spent %d (excluding running/continued), got %d", expectedTokens, stats.TokensSpent)
	}

	// Expected time: 3 sessions * 3600 sec each = 10800
	expectedTime := 10800.0
	const tolerance = 0.1
	if stats.AgentTimeSec < expectedTime-tolerance || stats.AgentTimeSec > expectedTime+tolerance {
		t.Errorf("expected agent_time_sec ~%.1f (excluding running/continued), got %.1f", expectedTime, stats.AgentTimeSec)
	}
}

func TestComputeAndStore_ExcludesNullTimestamps(t *testing.T) {
	pool, projectID := setupDailyStatsTestDB(t)
	defer pool.Close()

	today := "2026-02-13"
	todayTime := time.Date(2026, 2, 13, 10, 0, 0, 0, time.UTC)
	createdToday := todayTime.Format(time.RFC3339Nano)

	// Create workflow instance
	wfID := "test-workflow-instance"
	_, err := pool.Exec(`
		INSERT INTO workflows (id, project_id, description, phases, created_at, updated_at)
		VALUES ('feature', ?, 'Test workflow', '[]', ?, ?)`,
		strings.ToLower(projectID), createdToday, createdToday)
	if err != nil {
		t.Fatalf("failed to create workflow: %v", err)
	}

	_, err = pool.Exec(`
		INSERT INTO workflow_instances (id, project_id, ticket_id, workflow_id, status, phase_order, created_at, updated_at)
		VALUES (?, ?, 'TICKET-1', 'feature', 'active', '[]', ?, ?)`,
		wfID, strings.ToLower(projectID), createdToday, createdToday)
	if err != nil {
		t.Fatalf("failed to create workflow instance: %v", err)
	}

	startedAt := todayTime.Format(time.RFC3339Nano)
	endedAt := todayTime.Add(1 * time.Hour).Format(time.RFC3339Nano)

	// Insert sessions with NULL started_at or ended_at
	sessions := []struct {
		id          string
		startedAt   interface{}
		endedAt     interface{}
		contextLeft int
	}{
		{"session-valid", startedAt, endedAt, 50}, // Should be counted for time
		{"session-no-start", nil, endedAt, 60},    // Should NOT be counted for time (but tokens yes)
		{"session-no-end", startedAt, nil, 70},    // Should NOT be counted for time (but tokens yes)
		{"session-no-times", nil, nil, 80},        // Should NOT be counted for time (but tokens yes)
	}

	for _, s := range sessions {
		_, err := pool.Exec(`
			INSERT INTO agent_sessions (id, project_id, ticket_id, workflow_instance_id, phase, agent_type, status, context_left, started_at, ended_at, created_at, updated_at)
			VALUES (?, ?, 'TICKET-1', ?, 'implementation', 'implementor', 'completed', ?, ?, ?, ?, ?)`,
			s.id, strings.ToLower(projectID), wfID, s.contextLeft, s.startedAt, s.endedAt, createdToday, createdToday)
		if err != nil {
			t.Fatalf("failed to insert agent session %s: %v", s.id, err)
		}
	}

	// Test ComputeAndStore
	svc := NewDailyStatsService(pool, clock.Real())
	stats, err := svc.ComputeAndStore(projectID, today)
	if err != nil {
		t.Fatalf("ComputeAndStore failed: %v", err)
	}

	// Tokens: only sessions with ended_at today are counted (sessions 1 and 2 have ended_at, sessions 3 and 4 have NULL ended_at)
	// But the query filters by date(ended_at) = today, so NULL ended_at won't match
	// Session 1: context_left=50 -> 100000 tokens
	// Session 2: context_left=60 -> 80000 tokens
	expectedTokens := int64(100000 + 80000)
	if stats.TokensSpent != expectedTokens {
		t.Errorf("expected tokens_spent %d (sessions with ended_at today), got %d", expectedTokens, stats.TokensSpent)
	}

	// Time: only the first session with valid timestamps should be counted
	expectedTime := 3600.0
	const tolerance = 0.1
	if stats.AgentTimeSec < expectedTime-tolerance || stats.AgentTimeSec > expectedTime+tolerance {
		t.Errorf("expected agent_time_sec ~%.1f (only valid timestamps), got %.1f", expectedTime, stats.AgentTimeSec)
	}
}

func TestComputeAndStore_NoData(t *testing.T) {
	pool, projectID := setupDailyStatsTestDB(t)
	defer pool.Close()

	today := "2026-02-13"

	// No tickets or agent sessions inserted
	svc := NewDailyStatsService(pool, clock.Real())
	stats, err := svc.ComputeAndStore(projectID, today)
	if err != nil {
		t.Fatalf("ComputeAndStore failed: %v", err)
	}

	// All fields should be zero
	if stats.TicketsCreated != 0 {
		t.Errorf("expected tickets_created 0, got %d", stats.TicketsCreated)
	}
	if stats.TicketsClosed != 0 {
		t.Errorf("expected tickets_closed 0, got %d", stats.TicketsClosed)
	}
	if stats.TokensSpent != 0 {
		t.Errorf("expected tokens_spent 0, got %d", stats.TokensSpent)
	}
	if stats.AgentTimeSec != 0.0 {
		t.Errorf("expected agent_time_sec 0.0, got %.1f", stats.AgentTimeSec)
	}
}

func TestComputeAndStore_UpsertUpdatesExisting(t *testing.T) {
	pool, projectID := setupDailyStatsTestDB(t)
	defer pool.Close()

	today := "2026-02-13"
	todayTime := time.Date(2026, 2, 13, 10, 0, 0, 0, time.UTC)
	createdToday := todayTime.Format(time.RFC3339Nano)

	// Insert initial ticket
	_, err := pool.Exec(`
		INSERT INTO tickets (id, project_id, title, status, issue_type, priority, created_at, updated_at, created_by)
		VALUES (?, ?, 'Ticket 1', 'open', 'feature', 2, ?, ?, 'test')`,
		strings.ToLower(projectID)+"-1", strings.ToLower(projectID), createdToday, createdToday, "test")
	if err != nil {
		t.Fatalf("failed to insert ticket: %v", err)
	}

	// First compute
	svc := NewDailyStatsService(pool, clock.Real())
	stats1, err := svc.ComputeAndStore(projectID, today)
	if err != nil {
		t.Fatalf("first ComputeAndStore failed: %v", err)
	}

	if stats1.TicketsCreated != 1 {
		t.Errorf("first compute: expected tickets_created 1, got %d", stats1.TicketsCreated)
	}

	// Insert more tickets
	for i := 2; i <= 3; i++ {
		_, err := pool.Exec(`
			INSERT INTO tickets (id, project_id, title, status, issue_type, priority, created_at, updated_at, created_by)
			VALUES (?, ?, ?, 'open', 'feature', 2, ?, ?, 'test')`,
			strings.ToLower(projectID)+"-"+string(rune(i)), strings.ToLower(projectID),
			"Ticket "+string(rune(i)), createdToday, createdToday, "test")
		if err != nil {
			t.Fatalf("failed to insert ticket: %v", err)
		}
	}

	// Second compute (should update existing row)
	stats2, err := svc.ComputeAndStore(projectID, today)
	if err != nil {
		t.Fatalf("second ComputeAndStore failed: %v", err)
	}

	if stats2.TicketsCreated != 3 {
		t.Errorf("second compute: expected tickets_created 3, got %d", stats2.TicketsCreated)
	}

	// Verify only one row exists in daily_stats
	var count int
	err = pool.QueryRow(`SELECT COUNT(*) FROM daily_stats WHERE project_id = ? AND date = ?`,
		projectID, today).Scan(&count)
	if err != nil {
		t.Fatalf("failed to count daily_stats rows: %v", err)
	}
	if count != 1 {
		t.Errorf("expected 1 row in daily_stats, got %d", count)
	}
}

func TestComputeAndStore_CaseInsensitiveProjectID(t *testing.T) {
	pool, projectID := setupDailyStatsTestDB(t)
	defer pool.Close()

	today := "2026-02-13"
	todayTime := time.Date(2026, 2, 13, 10, 0, 0, 0, time.UTC)
	createdToday := todayTime.Format(time.RFC3339Nano)

	// Insert tickets with lowercase project_id
	_, err := pool.Exec(`
		INSERT INTO tickets (id, project_id, title, status, issue_type, priority, created_at, updated_at, created_by)
		VALUES (?, ?, 'Ticket 1', 'open', 'feature', 2, ?, ?, 'test')`,
		strings.ToLower(projectID)+"-1", strings.ToLower(projectID), createdToday, createdToday, "test")
	if err != nil {
		t.Fatalf("failed to insert ticket: %v", err)
	}

	// Query with uppercase project_id (but use lowercase for upsert to avoid FK constraint)
	svc := NewDailyStatsService(pool, clock.Real())
	stats, err := svc.ComputeAndStore(projectID, today)
	if err != nil {
		t.Fatalf("ComputeAndStore failed: %v", err)
	}

	// Should still find the ticket (LOWER() in query handles case-insensitive matching)
	if stats.TicketsCreated != 1 {
		t.Errorf("expected tickets_created 1 (case-insensitive match), got %d", stats.TicketsCreated)
	}
}

func TestGetToday(t *testing.T) {
	pool, projectID := setupDailyStatsTestDB(t)
	defer pool.Close()

	// Insert a ticket created today (UTC)
	now := time.Now().UTC()
	createdToday := now.Format(time.RFC3339Nano)
	_, err := pool.Exec(`
		INSERT INTO tickets (id, project_id, title, status, issue_type, priority, created_at, updated_at, created_by)
		VALUES (?, ?, 'Ticket Today', 'open', 'feature', 2, ?, ?, 'test')`,
		strings.ToLower(projectID)+"-today", strings.ToLower(projectID), createdToday, createdToday, "test")
	if err != nil {
		t.Fatalf("failed to insert ticket: %v", err)
	}

	// Test GetToday
	svc := NewDailyStatsService(pool, clock.Real())
	stats, err := svc.GetToday(projectID)
	if err != nil {
		t.Fatalf("GetToday failed: %v", err)
	}

	// Should find the ticket created today
	if stats.TicketsCreated != 1 {
		t.Errorf("expected tickets_created 1, got %d", stats.TicketsCreated)
	}

	// Verify date is today
	expectedDate := now.Format("2006-01-02")
	if stats.Date != expectedDate {
		t.Errorf("expected date %q, got %q", expectedDate, stats.Date)
	}
}

func TestComputeAndStore_TokensCalculation(t *testing.T) {
	pool, projectID := setupDailyStatsTestDB(t)
	defer pool.Close()

	today := "2026-02-13"
	todayTime := time.Date(2026, 2, 13, 10, 0, 0, 0, time.UTC)
	createdToday := todayTime.Format(time.RFC3339Nano)

	// Create workflow instance
	wfID := "test-workflow-instance"
	_, err := pool.Exec(`
		INSERT INTO workflows (id, project_id, description, phases, created_at, updated_at)
		VALUES ('feature', ?, 'Test workflow', '[]', ?, ?)`,
		strings.ToLower(projectID), createdToday, createdToday)
	if err != nil {
		t.Fatalf("failed to create workflow: %v", err)
	}

	_, err = pool.Exec(`
		INSERT INTO workflow_instances (id, project_id, ticket_id, workflow_id, status, phase_order, created_at, updated_at)
		VALUES (?, ?, 'TICKET-1', 'feature', 'active', '[]', ?, ?)`,
		wfID, strings.ToLower(projectID), createdToday, createdToday)
	if err != nil {
		t.Fatalf("failed to create workflow instance: %v", err)
	}

	endedAt := todayTime.Add(1 * time.Hour).Format(time.RFC3339Nano)

	// Test edge cases for token calculation
	testCases := []struct {
		name        string
		contextLeft int
		expected    int64
	}{
		{"full context used", 0, 200000},
		{"75% context used", 25, 150000},
		{"50% context used", 50, 100000},
		{"25% context used", 75, 50000},
		{"no context used", 100, 0},
	}

	for i, tc := range testCases {
		sessionID := "session-" + string(rune(i))
		_, err := pool.Exec(`
			INSERT INTO agent_sessions (id, project_id, ticket_id, workflow_instance_id, phase, agent_type, status, context_left, started_at, ended_at, created_at, updated_at)
			VALUES (?, ?, 'TICKET-1', ?, 'implementation', 'implementor', 'completed', ?, ?, ?, ?, ?)`,
			sessionID, strings.ToLower(projectID), wfID, tc.contextLeft, createdToday, endedAt, createdToday, createdToday)
		if err != nil {
			t.Fatalf("failed to insert agent session for %s: %v", tc.name, err)
		}
	}

	// Test ComputeAndStore
	svc := NewDailyStatsService(pool, clock.Real())
	stats, err := svc.ComputeAndStore(projectID, today)
	if err != nil {
		t.Fatalf("ComputeAndStore failed: %v", err)
	}

	// Expected total: 200000 + 150000 + 100000 + 50000 + 0 = 500000
	expectedTotal := int64(500000)
	if stats.TokensSpent != expectedTotal {
		t.Errorf("expected tokens_spent %d, got %d", expectedTotal, stats.TokensSpent)
	}
}

func TestComputeAndStore_AgentTimeCalculation(t *testing.T) {
	pool, projectID := setupDailyStatsTestDB(t)
	defer pool.Close()

	today := "2026-02-13"
	baseTime := time.Date(2026, 2, 13, 10, 0, 0, 0, time.UTC)
	createdToday := baseTime.Format(time.RFC3339Nano)

	// Create workflow instance
	wfID := "test-workflow-instance"
	_, err := pool.Exec(`
		INSERT INTO workflows (id, project_id, description, phases, created_at, updated_at)
		VALUES ('feature', ?, 'Test workflow', '[]', ?, ?)`,
		strings.ToLower(projectID), createdToday, createdToday)
	if err != nil {
		t.Fatalf("failed to create workflow: %v", err)
	}

	_, err = pool.Exec(`
		INSERT INTO workflow_instances (id, project_id, ticket_id, workflow_id, status, phase_order, created_at, updated_at)
		VALUES (?, ?, 'TICKET-1', 'feature', 'active', '[]', ?, ?)`,
		wfID, strings.ToLower(projectID), createdToday, createdToday)
	if err != nil {
		t.Fatalf("failed to create workflow instance: %v", err)
	}

	// Test various durations
	testCases := []struct {
		name        string
		durationSec float64
	}{
		{"1 hour", 3600},
		{"30 minutes", 1800},
		{"1 minute", 60},
		{"1 second", 1},
		{"2.5 hours", 9000},
	}

	for i, tc := range testCases {
		sessionID := "session-" + string(rune(i))
		startedAt := baseTime.Add(time.Duration(i) * time.Hour).Format(time.RFC3339Nano)
		endedAt := baseTime.Add(time.Duration(i)*time.Hour + time.Duration(tc.durationSec)*time.Second).Format(time.RFC3339Nano)

		_, err := pool.Exec(`
			INSERT INTO agent_sessions (id, project_id, ticket_id, workflow_instance_id, phase, agent_type, status, context_left, started_at, ended_at, created_at, updated_at)
			VALUES (?, ?, 'TICKET-1', ?, 'implementation', 'implementor', 'completed', 50, ?, ?, ?, ?)`,
			sessionID, strings.ToLower(projectID), wfID, startedAt, endedAt, createdToday, createdToday)
		if err != nil {
			t.Fatalf("failed to insert agent session for %s: %v", tc.name, err)
		}
	}

	// Test ComputeAndStore
	svc := NewDailyStatsService(pool, clock.Real())
	stats, err := svc.ComputeAndStore(projectID, today)
	if err != nil {
		t.Fatalf("ComputeAndStore failed: %v", err)
	}

	// Expected total: 3600 + 1800 + 60 + 1 + 9000 = 14461
	expectedTotal := 14461.0
	const tolerance = 0.1
	if stats.AgentTimeSec < expectedTotal-tolerance || stats.AgentTimeSec > expectedTotal+tolerance {
		t.Errorf("expected agent_time_sec ~%.1f, got %.1f", expectedTotal, stats.AgentTimeSec)
	}
}
