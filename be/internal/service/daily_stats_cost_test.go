package service

import (
	"strings"
	"testing"
	"time"

	"be/internal/clock"
	"be/internal/repo"
)

// TestComputeAndStore_CostEstimateSumsCompletedSessions verifies
// ComputeAndStore sums agent_sessions.cost_estimate over the same
// ended-today/kind='workflow_agent' window as tokens_spent, excluding
// running/continued sessions and NULL cost_estimate rows (never-priced
// sessions contribute 0, not an error).
func TestComputeAndStore_CostEstimateSumsCompletedSessions(t *testing.T) {
	t.Parallel()
	pool, projectID := setupDailyStatsTestDB(t)
	defer pool.Close()

	today := "2026-02-13"
	todayTime := time.Date(2026, 2, 13, 10, 0, 0, 0, time.UTC)
	createdToday := todayTime.Format(time.RFC3339Nano)
	endedAt := todayTime.Add(1 * time.Hour).Format(time.RFC3339Nano)

	_, err := pool.Exec(`INSERT INTO workflows (id, project_id, description, created_at, updated_at) VALUES ('feature', ?, 'Test workflow', ?, ?)`,
		strings.ToLower(projectID), createdToday, createdToday)
	if err != nil {
		t.Fatalf("failed to create workflow: %v", err)
	}
	wfID := "test-workflow-instance"
	_, err = pool.Exec(`INSERT INTO workflow_instances (id, project_id, ticket_id, workflow_id, status, created_at, updated_at) VALUES (?, ?, 'TICKET-1', 'feature', 'active', ?, ?)`,
		wfID, strings.ToLower(projectID), createdToday, createdToday)
	if err != nil {
		t.Fatalf("failed to create workflow instance: %v", err)
	}

	sessions := []struct {
		id           string
		status       string
		costEstimate interface{}
	}{
		{"cost-session-completed-1", "completed", 1.25},
		{"cost-session-completed-2", "completed", 2.75},
		{"cost-session-running", "running", 100.0}, // excluded: still running
		{"cost-session-no-price", "completed", nil},
	}
	for _, s := range sessions {
		_, err := pool.Exec(`
			INSERT INTO agent_sessions (id, project_id, ticket_id, workflow_instance_id, phase, agent_type, status, context_left, cost_estimate, started_at, ended_at, created_at, updated_at)
			VALUES (?, ?, 'TICKET-1', ?, 'implementation', 'implementor', ?, 50, ?, ?, ?, ?, ?)`,
			s.id, strings.ToLower(projectID), wfID, s.status, s.costEstimate, createdToday, endedAt, createdToday, createdToday)
		if err != nil {
			t.Fatalf("failed to insert agent session %s: %v", s.id, err)
		}
	}

	svc := NewDailyStatsService(pool, clock.Real())
	stats, err := svc.ComputeAndStore(projectID, today)
	if err != nil {
		t.Fatalf("ComputeAndStore failed: %v", err)
	}

	const wantCost = 1.25 + 2.75
	const tolerance = 0.0001
	if stats.CostEstimate < wantCost-tolerance || stats.CostEstimate > wantCost+tolerance {
		t.Errorf("CostEstimate = %v, want %v (running session and NULL-cost session excluded)", stats.CostEstimate, wantCost)
	}

	// Persisted row must carry the same cost_estimate.
	persisted, err := repo.NewDailyStatsRepo(pool, clock.Real()).GetByDate(projectID, today)
	if err != nil {
		t.Fatalf("GetByDate: %v", err)
	}
	if persisted.CostEstimate < wantCost-tolerance || persisted.CostEstimate > wantCost+tolerance {
		t.Errorf("persisted CostEstimate = %v, want %v", persisted.CostEstimate, wantCost)
	}
}
