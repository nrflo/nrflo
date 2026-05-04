package service

import (
	"fmt"
	"time"

	"be/internal/clock"
	"be/internal/db"
	"be/internal/model"
	"be/internal/repo"
)

// NrvappInsightsService provides aggregation methods for nrvapp dispatch/review dashboards.
type NrvappInsightsService struct {
	pool         *db.Pool
	clk          clock.Clock
	dispatchRepo *repo.NrvappDispatchRepo
	reviewRepo   *repo.NrvappReviewRepo
}

// NewNrvappInsightsService creates a new NrvappInsightsService.
func NewNrvappInsightsService(pool *db.Pool, clk clock.Clock) *NrvappInsightsService {
	return &NrvappInsightsService{
		pool:         pool,
		clk:          clk,
		dispatchRepo: repo.NewNrvappDispatchRepo(pool, clk),
		reviewRepo:   repo.NewNrvappReviewRepo(pool, clk),
	}
}

// InsightsSummary combines dispatch stats and review counts.
type InsightsSummary struct {
	*model.DispatchSummary
	ReviewPending  int     `json:"review_pending"`
	ReviewApproved int     `json:"review_approved"`
	ReviewRejected int     `json:"review_rejected"`
	ApprovalRate   float64 `json:"approval_rate"`
}

// Summary returns aggregate dispatch stats and review counts for a project since the given time.
func (s *NrvappInsightsService) Summary(projectID string, since time.Time) (*InsightsSummary, error) {
	dispatch, err := s.dispatchRepo.ListSummary(projectID, since)
	if err != nil {
		return nil, fmt.Errorf("dispatch summary: %w", err)
	}

	pending, err := s.countReviews(projectID, model.ReviewStatusPending)
	if err != nil {
		return nil, err
	}
	approved, err := s.countReviews(projectID, model.ReviewStatusApproved)
	if err != nil {
		return nil, err
	}
	rejected, err := s.countReviews(projectID, model.ReviewStatusRejected)
	if err != nil {
		return nil, err
	}

	total := approved + rejected
	var approvalRate float64
	if total > 0 {
		approvalRate = float64(approved) / float64(total)
	}

	return &InsightsSummary{
		DispatchSummary: dispatch,
		ReviewPending:   pending,
		ReviewApproved:  approved,
		ReviewRejected:  rejected,
		ApprovalRate:    approvalRate,
	}, nil
}

func (s *NrvappInsightsService) countReviews(projectID, status string) (int, error) {
	var n int
	err := s.pool.QueryRow(
		`SELECT COUNT(*) FROM nrvapp_review_items WHERE project_id = ? AND status = ?`,
		projectID, status,
	).Scan(&n)
	return n, err
}

// EditRateResult extends the repo row with a computed edit-rate percentage.
type EditRateResult struct {
	*model.EditRateRow
	EditRatePct float64 `json:"edit_rate_pct"`
}

// EditRate returns per-tool review outcome ratios with computed edit-rate percentage.
func (s *NrvappInsightsService) EditRate(projectID string, since time.Time) ([]*EditRateResult, error) {
	rows, err := s.dispatchRepo.EditRateByTool(projectID, since)
	if err != nil {
		return nil, err
	}
	result := make([]*EditRateResult, len(rows))
	for i, r := range rows {
		total := r.ApproveNoEdits + r.ApproveWithEdits + r.Rejected
		var pct float64
		if total > 0 {
			pct = float64(r.ApproveWithEdits) / float64(total) * 100
		}
		result[i] = &EditRateResult{EditRateRow: r, EditRatePct: pct}
	}
	return result, nil
}

// Throughput returns bucketed dispatch counts over time.
func (s *NrvappInsightsService) Throughput(projectID string, since time.Time, bucket time.Duration) ([]*model.ThroughputPoint, error) {
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
