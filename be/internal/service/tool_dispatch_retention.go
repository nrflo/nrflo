package service

import (
	"time"

	"be/internal/clock"
	"be/internal/db"
	"be/internal/repo"
)

// ToolCallRetentionDaysKey is the global config-KV key bounding how long
// tool_dispatches rows are kept.
const ToolCallRetentionDaysKey = "tool_call_retention_days"

// DefaultToolCallRetentionDays is used when ToolCallRetentionDaysKey is unset.
const DefaultToolCallRetentionDays = 30

// SweepToolDispatches purges tool_dispatches rows older than the configured
// retention window, mirroring ConsoleService.SweepIdle/PlanService.SweepExpiredDrafts
// — called from the server's periodic retention-cleanup loop.
func SweepToolDispatches(pool *db.Pool, clk clock.Clock, now time.Time) (int64, error) {
	days := SubworkflowCap(pool, "", ToolCallRetentionDaysKey, DefaultToolCallRetentionDays)
	cutoff := now.Add(-time.Duration(days) * 24 * time.Hour).UTC().Format(time.RFC3339Nano)
	return repo.NewDispatchRepo(pool, clk).DeleteBefore(cutoff)
}
