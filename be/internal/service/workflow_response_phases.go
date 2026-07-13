package service

import (
	"database/sql"
	"time"

	"be/internal/model"
)

// derivePhaseStatuses derives phase statuses from agent_sessions rows instead of the phases JSON column.
// This eliminates the race condition where parallel agents overwrite each other's status in the JSON blob.
// Keyed on node_id (execution identity), not agent_type (template identity) — for
// static workflows the two are identical, so the derived map is unchanged.
func (s *WorkflowService) derivePhaseStatuses(wfiID string, phases []PhaseDef) map[string]model.PhaseStatus {
	result := make(map[string]model.PhaseStatus, len(phases))

	// Default all phases to pending
	for _, p := range phases {
		result[p.NodeID] = model.PhaseStatus{Status: "pending"}
	}

	seen := make(map[string]bool)
	maxLayer := -1

	// Pre-pass: continued sessions with a future rate_limit_until_ts become "rate_limited".
	// Use a separate dedup set so expired waiting rows don't block the main loop.
	waitingDeduped := make(map[string]bool)
	waitingRows, waitingErr := s.pool.Query(`
		SELECT node_id, rate_limit_until_ts, rate_limit_retry_count FROM agent_sessions
		WHERE workflow_instance_id = ? AND status = 'continued' AND rate_limit_until_ts IS NOT NULL
		AND `+transientAgentTypeExclusion+`
		ORDER BY created_at DESC`, wfiID)
	if waitingErr == nil {
		now := s.clock.Now()
		for waitingRows.Next() {
			var nodeID string
			var rateLimitUntilTs sql.NullString
			var rateLimitRetryCount int
			waitingRows.Scan(&nodeID, &rateLimitUntilTs, &rateLimitRetryCount)
			if waitingDeduped[nodeID] {
				continue
			}
			waitingDeduped[nodeID] = true
			if !rateLimitUntilTs.Valid {
				continue
			}
			ts, parseErr := time.Parse(time.RFC3339Nano, rateLimitUntilTs.String)
			if parseErr != nil {
				ts, parseErr = time.Parse(time.RFC3339, rateLimitUntilTs.String)
			}
			if parseErr != nil || !ts.After(now) {
				continue
			}
			result[nodeID] = model.PhaseStatus{
				Status:              "rate_limited",
				RateLimitUntilTs:    rateLimitUntilTs.String,
				RateLimitRetryCount: rateLimitRetryCount,
			}
			seen[nodeID] = true
			for _, p := range phases {
				if p.NodeID == nodeID && p.Layer > maxLayer {
					maxLayer = p.Layer
				}
			}
		}
		waitingRows.Close()
	}

	// Query latest non-continued/callback session per node_id
	rows, err := s.pool.Query(`
		SELECT node_id, status, result FROM agent_sessions
		WHERE workflow_instance_id = ? AND status NOT IN ('continued', 'callback') AND `+transientAgentTypeExclusion+`
		ORDER BY created_at DESC`, wfiID)
	if err != nil {
		return result
	}
	defer rows.Close()

	// Group by node_id, take latest session per node
	for rows.Next() {
		var nodeID, status string
		var sessionResult sql.NullString
		rows.Scan(&nodeID, &status, &sessionResult)

		if seen[nodeID] {
			continue
		}
		seen[nodeID] = true

		var ps model.PhaseStatus
		switch status {
		case "running", "user_interactive":
			ps = model.PhaseStatus{Status: "in_progress"}
		case "completed", "project_completed", "interactive_completed":
			ps = model.PhaseStatus{Status: "completed", Result: "pass"}
			if sessionResult.Valid && sessionResult.String != "" {
				ps.Result = sessionResult.String
			}
		case "failed":
			ps = model.PhaseStatus{Status: "completed", Result: "fail"}
		case "timeout":
			ps = model.PhaseStatus{Status: "completed", Result: "timeout"}
		case "skipped":
			ps = model.PhaseStatus{Status: "skipped", Result: "skipped"}
		default:
			continue
		}
		result[nodeID] = ps

		// Track max layer that has a session
		for _, p := range phases {
			if p.NodeID == nodeID && p.Layer > maxLayer {
				maxLayer = p.Layer
			}
		}
	}

	// Infer "skipped": if a phase has no session but a later layer does have sessions,
	// the phase's layer was already processed and the phase was skipped.
	// This works for both active and terminal workflows because the orchestrator
	// processes all phases in a layer before advancing to the next layer.
	for _, p := range phases {
		if !seen[p.NodeID] && p.Layer < maxLayer {
			result[p.NodeID] = model.PhaseStatus{Status: "skipped", Result: "skipped"}
		}
	}

	return result
}

// deriveCurrentPhase returns the node_id of the latest active agent session (running,
// user_interactive, or rate-limited continued), or empty string if none.
func (s *WorkflowService) deriveCurrentPhase(wfiID string) string {
	var runningNode, runningCreatedAt sql.NullString
	s.pool.QueryRow(`
		SELECT node_id, created_at FROM agent_sessions
		WHERE workflow_instance_id = ? AND status IN ('running', 'user_interactive')
		ORDER BY created_at DESC LIMIT 1`, wfiID).Scan(&runningNode, &runningCreatedAt)

	nowStr := s.clock.Now().UTC().Format(time.RFC3339Nano)
	var waitingNode, waitingCreatedAt sql.NullString
	s.pool.QueryRow(`
		SELECT node_id, created_at FROM agent_sessions
		WHERE workflow_instance_id = ? AND status = 'continued' AND rate_limit_until_ts IS NOT NULL AND rate_limit_until_ts > ?
		ORDER BY created_at DESC LIMIT 1`, wfiID, nowStr).Scan(&waitingNode, &waitingCreatedAt)

	if !waitingNode.Valid {
		if runningNode.Valid {
			return runningNode.String
		}
		return ""
	}
	if !runningNode.Valid {
		return waitingNode.String
	}
	// Both candidates exist; return the one with the later created_at
	if waitingCreatedAt.String > runningCreatedAt.String {
		return waitingNode.String
	}
	return runningNode.String
}
