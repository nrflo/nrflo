package service

import (
	"fmt"
	"time"

	"be/internal/clock"
	"be/internal/db"
	"be/internal/model"
	"be/internal/repo"
)

// DailyStatsService computes and stores daily statistics from source tables.
type DailyStatsService struct {
	clock clock.Clock
	pool  *db.Pool
}

// NewDailyStatsService creates a new daily stats service.
func NewDailyStatsService(pool *db.Pool, clk clock.Clock) *DailyStatsService {
	return &DailyStatsService{pool: pool, clock: clk}
}

// ComputeAndStore queries tickets and agent_sessions for the given project+date,
// upserts the result into daily_stats, and returns it.
func (s *DailyStatsService) ComputeAndStore(projectID, date string) (model.DailyStats, error) {
	var stats model.DailyStats
	stats.ProjectID = projectID
	stats.Date = date

	// Tickets created today
	err := s.pool.QueryRow(`
		SELECT COUNT(*) FROM tickets
		WHERE LOWER(project_id) = LOWER(?) AND date(created_at) = ?`,
		projectID, date,
	).Scan(&stats.TicketsCreated)
	if err != nil {
		return stats, fmt.Errorf("count tickets created: %w", err)
	}

	// Tickets closed today
	err = s.pool.QueryRow(`
		SELECT COUNT(*) FROM tickets
		WHERE LOWER(project_id) = LOWER(?) AND date(closed_at) = ?`,
		projectID, date,
	).Scan(&stats.TicketsClosed)
	if err != nil {
		return stats, fmt.Errorf("count tickets closed: %w", err)
	}

	// Tokens spent from completed agent sessions ending today
	err = s.pool.QueryRow(`
		SELECT COALESCE(SUM(200000 * (100 - context_left) / 100), 0)
		FROM agent_sessions
		WHERE LOWER(project_id) = LOWER(?) AND date(ended_at) = ?
		AND status NOT IN ('running', 'continued')`,
		projectID, date,
	).Scan(&stats.TokensSpent)
	if err != nil {
		return stats, fmt.Errorf("sum tokens spent: %w", err)
	}

	// Agent wall-clock time from completed sessions ending today
	err = s.pool.QueryRow(`
		SELECT COALESCE(SUM(CAST((julianday(ended_at) - julianday(started_at)) * 86400 AS REAL)), 0)
		FROM agent_sessions
		WHERE LOWER(project_id) = LOWER(?) AND date(ended_at) = ?
		AND status NOT IN ('running', 'continued')
		AND started_at IS NOT NULL AND ended_at IS NOT NULL`,
		projectID, date,
	).Scan(&stats.AgentTimeSec)
	if err != nil {
		return stats, fmt.Errorf("sum agent time: %w", err)
	}

	// Only persist if the project exists (avoids FK violation on fresh installs)
	var projectExists int
	_ = s.pool.QueryRow(`SELECT 1 FROM projects WHERE id = ?`, projectID).Scan(&projectExists)
	if projectExists == 1 {
		dailyRepo := repo.NewDailyStatsRepo(s.pool, s.clock)
		if err := dailyRepo.Upsert(projectID, date, stats); err != nil {
			return stats, fmt.Errorf("upsert daily stats: %w", err)
		}
	}

	return stats, nil
}

// GetToday computes and stores stats for today (UTC).
func (s *DailyStatsService) GetToday(projectID string) (model.DailyStats, error) {
	today := s.clock.Now().UTC().Format("2006-01-02")
	return s.ComputeAndStore(projectID, today)
}

// ValidRange returns true if rangeType is a supported range value.
func ValidRange(rangeType string) bool {
	switch rangeType {
	case "today", "week", "month", "all":
		return true
	}
	return false
}

// rangeToFromDate maps a rangeType to a from-date string.
func (s *DailyStatsService) rangeToFromDate(rangeType string) string {
	now := s.clock.Now().UTC()
	switch rangeType {
	case "week":
		return now.Add(-7 * 24 * time.Hour).Format("2006-01-02")
	case "month":
		return now.Add(-30 * 24 * time.Hour).Format("2006-01-02")
	case "all":
		return "2000-01-01"
	default:
		return now.Format("2006-01-02")
	}
}

// GetRange returns aggregated stats for the given date range.
// For "today", delegates to GetToday (persists). Other ranges are ephemeral.
func (s *DailyStatsService) GetRange(projectID, rangeType string) (model.DailyStats, error) {
	if rangeType == "today" {
		return s.GetToday(projectID)
	}

	fromDate := s.rangeToFromDate(rangeType)
	todayDate := s.clock.Now().UTC().Format("2006-01-02")

	var stats model.DailyStats
	stats.ProjectID = projectID
	stats.Date = fromDate

	// Tickets created in range
	err := s.pool.QueryRow(`
		SELECT COUNT(*) FROM tickets
		WHERE LOWER(project_id) = LOWER(?) AND date(created_at) >= ? AND date(created_at) <= ?`,
		projectID, fromDate, todayDate,
	).Scan(&stats.TicketsCreated)
	if err != nil {
		return stats, fmt.Errorf("count tickets created: %w", err)
	}

	// Tickets closed in range
	err = s.pool.QueryRow(`
		SELECT COUNT(*) FROM tickets
		WHERE LOWER(project_id) = LOWER(?) AND date(closed_at) >= ? AND date(closed_at) <= ?`,
		projectID, fromDate, todayDate,
	).Scan(&stats.TicketsClosed)
	if err != nil {
		return stats, fmt.Errorf("count tickets closed: %w", err)
	}

	// Tokens spent in range
	err = s.pool.QueryRow(`
		SELECT COALESCE(SUM(200000 * (100 - context_left) / 100), 0)
		FROM agent_sessions
		WHERE LOWER(project_id) = LOWER(?) AND date(ended_at) >= ? AND date(ended_at) <= ?
		AND status NOT IN ('running', 'continued')`,
		projectID, fromDate, todayDate,
	).Scan(&stats.TokensSpent)
	if err != nil {
		return stats, fmt.Errorf("sum tokens spent: %w", err)
	}

	// Agent wall-clock time in range
	err = s.pool.QueryRow(`
		SELECT COALESCE(SUM(CAST((julianday(ended_at) - julianday(started_at)) * 86400 AS REAL)), 0)
		FROM agent_sessions
		WHERE LOWER(project_id) = LOWER(?) AND date(ended_at) >= ? AND date(ended_at) <= ?
		AND status NOT IN ('running', 'continued')
		AND started_at IS NOT NULL AND ended_at IS NOT NULL`,
		projectID, fromDate, todayDate,
	).Scan(&stats.AgentTimeSec)
	if err != nil {
		return stats, fmt.Errorf("sum agent time: %w", err)
	}

	return stats, nil
}
