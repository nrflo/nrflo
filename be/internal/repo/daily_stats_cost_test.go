package repo

import (
	"testing"

	"be/internal/clock"
	"be/internal/model"
)

// TestDailyStatsUpsertAndGet_CostEstimate verifies cost_estimate round-trips
// through Upsert/GetByDate alongside the pre-existing token/time columns.
func TestDailyStatsUpsertAndGet_CostEstimate(t *testing.T) {
	t.Parallel()
	database := newTestDB(t)
	if _, err := database.Exec(`INSERT INTO projects (id, name, created_at, updated_at) VALUES (?, ?, datetime('now'), datetime('now'))`, "test-proj-cost", "Test Project"); err != nil {
		t.Fatalf("failed to create project: %v", err)
	}

	repo := NewDailyStatsRepo(database, clock.Real())
	stats := model.DailyStats{
		TicketsCreated: 1,
		TokensSpent:    1000,
		AgentTimeSec:   10,
		CostEstimate:   12.3456,
	}
	if err := repo.Upsert("test-proj-cost", "2025-01-15", stats); err != nil {
		t.Fatalf("Upsert: %v", err)
	}

	retrieved, err := repo.GetByDate("test-proj-cost", "2025-01-15")
	if err != nil {
		t.Fatalf("GetByDate: %v", err)
	}
	if retrieved.CostEstimate != stats.CostEstimate {
		t.Errorf("CostEstimate = %v, want %v", retrieved.CostEstimate, stats.CostEstimate)
	}
}

// TestDailyStatsUpsert_CostEstimateDefaultsToZero verifies a stats value with
// no CostEstimate set persists and reads back as 0, not NULL/error (the
// daily_stats.cost_estimate column is NOT NULL DEFAULT 0).
func TestDailyStatsUpsert_CostEstimateDefaultsToZero(t *testing.T) {
	t.Parallel()
	database := newTestDB(t)
	if _, err := database.Exec(`INSERT INTO projects (id, name, created_at, updated_at) VALUES (?, ?, datetime('now'), datetime('now'))`, "test-proj-cost-zero", "Test Project"); err != nil {
		t.Fatalf("failed to create project: %v", err)
	}

	repo := NewDailyStatsRepo(database, clock.Real())
	if err := repo.Upsert("test-proj-cost-zero", "2025-01-15", model.DailyStats{TicketsCreated: 1}); err != nil {
		t.Fatalf("Upsert: %v", err)
	}

	retrieved, err := repo.GetByDate("test-proj-cost-zero", "2025-01-15")
	if err != nil {
		t.Fatalf("GetByDate: %v", err)
	}
	if retrieved.CostEstimate != 0 {
		t.Errorf("CostEstimate = %v, want 0", retrieved.CostEstimate)
	}
}
