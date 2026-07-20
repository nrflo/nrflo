package service

import (
	"database/sql"
	"strings"
	"time"

	"be/internal/db"
)

// monthlyCostEstimate is a 30-day-trailing cost estimate for one
// (project, def) pair, repriced at a recommended model. It is an
// approximation: reprice assumes the recommended model's input/output token
// mix matches the current model's observed spend (blended-price ratio),
// which is not exact but is directionally useful for a dry-run report.
type monthlyCostEstimate struct {
	CurrentMonthly   float64
	ProjectedMonthly float64
	Delta            float64
	HasUsage         bool
}

// estimateMonthlyDelta sums agent_sessions.cost_estimate for workflow_agent
// sessions of defID in projectID over the trailing 30 days, then reprices
// that spend at recommendedModel using blended per-MTok pricing from the
// models table. Returns HasUsage=false (no other fields meaningful) when
// there is no cost data to reason about.
func estimateMonthlyDelta(pool *db.Pool, modelSvc *ModelService, projectID, defID, currentModel, recommendedModel string, now time.Time) (*monthlyCostEstimate, error) {
	since := now.AddDate(0, 0, -30).UTC().Format(time.RFC3339Nano)

	var sum sql.NullFloat64
	err := pool.QueryRow(`
		SELECT SUM(cost_estimate) FROM agent_sessions
		WHERE project_id = ? AND agent_type = ? AND kind = 'workflow_agent'
		  AND cost_estimate IS NOT NULL
		  AND COALESCE(ended_at, updated_at) >= ?`,
		projectID, defID, since,
	).Scan(&sum)
	if err != nil {
		return nil, err
	}
	if !sum.Valid || sum.Float64 == 0 {
		return &monthlyCostEstimate{}, nil
	}

	current := sum.Float64
	projected := current
	if !strings.EqualFold(currentModel, recommendedModel) {
		if curBlend, curOK := blendedPricePerMTok(modelSvc, currentModel); curOK && curBlend > 0 {
			if recBlend, recOK := blendedPricePerMTok(modelSvc, recommendedModel); recOK {
				projected = current * (recBlend / curBlend)
			}
		}
	}

	return &monthlyCostEstimate{
		CurrentMonthly:   current,
		ProjectedMonthly: projected,
		Delta:            projected - current,
		HasUsage:         true,
	}, nil
}

// blendedPricePerMTok averages a model's input/output per-MTok price as a
// single approximate cost figure. ok=false when the model row is missing or
// has no seeded pricing.
func blendedPricePerMTok(modelSvc *ModelService, modelID string) (float64, bool) {
	m, err := modelSvc.Get(modelID)
	if err != nil || m.PriceIn == nil || m.PriceOut == nil {
		return 0, false
	}
	return (*m.PriceIn + *m.PriceOut) / 2, true
}
