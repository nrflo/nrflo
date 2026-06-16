package service

import (
	"database/sql"
	"time"
)

// RateLimitWindow is one Claude subscription rate-limit window (5h or 7d rolling)
// from the statusline rate_limits payload.
type RateLimitWindow struct {
	UsedPercentage *float64
	ResetsAt       string // RFC3339, e.g. "2026-06-16T05:00:00Z"
}

// nearExhaustedPct is the used-percentage at/above which a subscription window is
// treated as effectively exhausted, so its reset time drives the next retry wait.
const nearExhaustedPct = 95.0

// latestExhaustedReset returns the latest (max) reset timestamp string among the
// windows that are at/above nearExhaustedPct and have a parseable future reset,
// or "" when none qualify. The later reset binds because the agent stays blocked
// until every exhausted window has reset.
func latestExhaustedReset(now time.Time, windows ...RateLimitWindow) string {
	var best time.Time
	var bestStr string
	for _, w := range windows {
		if w.UsedPercentage == nil || *w.UsedPercentage < nearExhaustedPct {
			continue
		}
		t, err := time.Parse(time.RFC3339, w.ResetsAt)
		if err != nil || !t.After(now) {
			continue
		}
		if t.After(best) {
			best, bestStr = t, w.ResetsAt
		}
	}
	return bestStr
}

// UpdateRateLimits records the latest Claude subscription rate-limit windows for a
// session (from the statusline rate_limits payload). When a window is near-exhausted
// with a future reset, the latest-unblocking reset is persisted to
// rate_limit_reset_ts so a later rate-limited restart waits until the exact reset
// instead of guessing an exponential backoff. Returns project/ticket/workflow for
// broadcasting; empty strings (no error) when the session is unknown.
func (s *AgentService) UpdateRateLimits(sessionID string, fiveHour, sevenDay RateLimitWindow) (projectID, ticketID, workflowName string, err error) {
	resetTs := latestExhaustedReset(s.clock.Now().UTC(), fiveHour, sevenDay)
	now := s.clock.Now().UTC().Format(time.RFC3339Nano)
	_, err = s.pool.Exec(
		`UPDATE agent_sessions SET rate_limit_reset_ts = ?, updated_at = ? WHERE id = ?`,
		sql.NullString{String: resetTs, Valid: resetTs != ""}, now, sessionID)
	if err != nil {
		return "", "", "", err
	}

	err = s.pool.QueryRow(`
		SELECT s.project_id, s.ticket_id, wi.workflow_id
		FROM agent_sessions s
		JOIN workflow_instances wi ON s.workflow_instance_id = wi.id
		WHERE s.id = ?`, sessionID).Scan(&projectID, &ticketID, &workflowName)
	if err != nil {
		// Session not found or workflow instance deleted — skip broadcast.
		return "", "", "", nil
	}
	return projectID, ticketID, workflowName, nil
}
