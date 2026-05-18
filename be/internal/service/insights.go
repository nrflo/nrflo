package service

import (
	"fmt"
	"time"

	"be/internal/clock"
	"be/internal/db"
	"be/internal/model"
	"be/internal/repo"
)

// InsightsService provides aggregation methods for dispatch dashboards.
type InsightsService struct {
	pool         *db.Pool
	clk          clock.Clock
	dispatchRepo *repo.DispatchRepo
}

// NewInsightsService creates a new InsightsService.
func NewInsightsService(pool *db.Pool, clk clock.Clock) *InsightsService {
	return &InsightsService{
		pool:         pool,
		clk:          clk,
		dispatchRepo: repo.NewDispatchRepo(pool, clk),
	}
}

// InsightsSummary combines dispatch stats.
type InsightsSummary struct {
	*model.DispatchSummary
}

// Summary returns aggregate dispatch stats for a project since the given time.
func (s *InsightsService) Summary(projectID string, since time.Time) (*InsightsSummary, error) {
	dispatch, err := s.dispatchRepo.ListSummary(projectID, since)
	if err != nil {
		return nil, fmt.Errorf("dispatch summary: %w", err)
	}
	return &InsightsSummary{DispatchSummary: dispatch}, nil
}

// EditRateResult is kept for API compatibility; review_items was dropped in migration 114.
type EditRateResult struct {
	ToolName    string  `json:"tool_name"`
	EditRatePct float64 `json:"edit_rate_pct"`
}

// EditRate returns an empty result set; review_items was dropped in migration 114.
func (s *InsightsService) EditRate(projectID string, since time.Time) ([]*EditRateResult, error) {
	return []*EditRateResult{}, nil
}

// Throughput returns bucketed dispatch counts over time.
func (s *InsightsService) Throughput(projectID string, since time.Time, bucket time.Duration) ([]*model.ThroughputPoint, error) {
	bucketSec := int(bucket.Seconds())
	return s.dispatchRepo.Throughput(projectID, since, bucketSec)
}

// ParseRange maps "7d" or "30d" to a since time.Time relative to clock.Now().
func ParseRange(r string, clk clock.Clock) (time.Time, error) {
	switch r {
	case "7d":
		return clk.Now().Add(-7 * 24 * time.Hour), nil
	case "30d":
		return clk.Now().Add(-30 * 24 * time.Hour), nil
	default:
		return time.Time{}, fmt.Errorf("unknown range %q: use 7d or 30d", r)
	}
}

// ParseBucket maps "1h", "6h", "1d" to a time.Duration.
func ParseBucket(b string) (time.Duration, error) {
	switch b {
	case "1h":
		return time.Hour, nil
	case "6h":
		return 6 * time.Hour, nil
	case "1d":
		return 24 * time.Hour, nil
	default:
		return 0, fmt.Errorf("unknown bucket %q: use 1h, 6h, or 1d", b)
	}
}
