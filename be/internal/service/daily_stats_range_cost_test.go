package service

import (
	"strings"
	"testing"
	"time"

	"be/internal/clock"
	"be/internal/db"
)

func insertRangeSessionWithCost(t *testing.T, pool *db.Pool, projectID, wfInstanceID, id, startedAt, endedAt, status string, costEstimate float64) {
	t.Helper()
	_, err := pool.Exec(`INSERT INTO agent_sessions (id, project_id, ticket_id, workflow_instance_id, phase, agent_type, status, context_left, cost_estimate, started_at, ended_at, created_at, updated_at) VALUES (?, ?, 'TICKET-R', ?, 'impl', 'implementor', ?, 50, ?, ?, ?, ?, ?)`,
		id, strings.ToLower(projectID), wfInstanceID, status, costEstimate, startedAt, endedAt, startedAt, startedAt)
	if err != nil {
		t.Fatalf("insertRangeSessionWithCost(%s): %v", id, err)
	}
}

// TestGetRange_CostEstimateSumsInRangeExcludesRunning verifies GetRange's
// cost_estimate sum follows the same in-range/status filter as tokens_spent.
func TestGetRange_CostEstimateSumsInRangeExcludesRunning(t *testing.T) {
	t.Parallel()
	pool, projectID := setupDailyStatsTestDB(t)
	defer pool.Close()

	clk := clock.NewTest(rangeTestNow)
	svc := NewDailyStatsService(pool, clk)
	ts := rangeTestNow.Format(time.RFC3339Nano)
	setupRangeWorkflow(t, pool, projectID, "wf-cost-range", ts)

	inStart := rangeTestNow.Add(-1 * time.Hour).Format(time.RFC3339Nano)
	inEnd := rangeTestNow.Format(time.RFC3339Nano)
	insertRangeSessionWithCost(t, pool, projectID, "wf-cost-range", "sess-cost-in-1", inStart, inEnd, "completed", 0.5)
	insertRangeSessionWithCost(t, pool, projectID, "wf-cost-range", "sess-cost-in-2", inStart, inEnd, "completed", 1.5)
	insertRangeSessionWithCost(t, pool, projectID, "wf-cost-range", "sess-cost-running", inStart, inEnd, "running", 100)

	ago8Start := rangeTestNow.AddDate(0, 0, -8).Add(-1 * time.Hour).Format(time.RFC3339Nano)
	ago8End := rangeTestNow.AddDate(0, 0, -8).Format(time.RFC3339Nano)
	insertRangeSessionWithCost(t, pool, projectID, "wf-cost-range", "sess-cost-out-of-range", ago8Start, ago8End, "completed", 100)

	stats, err := svc.GetRange(projectID, "week")
	if err != nil {
		t.Fatalf("GetRange week: %v", err)
	}
	const wantCost = 0.5 + 1.5
	const tolerance = 0.0001
	if stats.CostEstimate < wantCost-tolerance || stats.CostEstimate > wantCost+tolerance {
		t.Errorf("CostEstimate = %v, want %v (running + out-of-range excluded)", stats.CostEstimate, wantCost)
	}
}
