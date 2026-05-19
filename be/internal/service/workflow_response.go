package service

import (
	"database/sql"
	"encoding/json"
	"strings"
	"time"

	"be/internal/clock"
	"be/internal/db"
	"be/internal/model"
	"be/internal/repo"
)

func (s *WorkflowService) buildActiveAgentsMap(wfiID string, detailsMap map[string][]RestartDetail) map[string]interface{} {
	agents := make(map[string]interface{})
	rows, err := s.pool.Query(`
		SELECT s.id, s.phase, s.agent_type, s.model_id, s.pid, s.result, s.started_at, s.context_left, s.restart_count,
		       ad.restart_threshold, s.ancestor_session_id, ad.tag, s.nudge_count, s.effective_mode, s.status, s.rate_limit_until_ts, s.rate_limit_retry_count
		FROM agent_sessions s
		LEFT JOIN workflow_instances wi ON wi.id = s.workflow_instance_id
		LEFT JOIN agent_definitions ad ON LOWER(ad.project_id) = LOWER(wi.project_id)
			AND LOWER(ad.workflow_id) = LOWER(wi.workflow_id)
			AND LOWER(ad.id) = LOWER(s.agent_type)
		WHERE s.workflow_instance_id = ? AND (s.status = 'running' OR (s.status = 'continued' AND s.rate_limit_until_ts IS NOT NULL)) AND s.agent_type NOT IN ('planner', 'context-saver', 'conflict-resolver')`, wfiID)
	if err != nil {
		return agents
	}
	defer rows.Close()

	for rows.Next() {
		var id, agentType, status string
		var phase, modelID, agentResult, startedAt, ancestorSessionID, tag, effectiveMode, rateLimitUntilTs sql.NullString
		var pid, contextLeft, restartThreshold sql.NullInt64
		var restartCount, nudgeCount, rateLimitRetryCount int
		rows.Scan(&id, &phase, &agentType, &modelID, &pid, &agentResult, &startedAt, &contextLeft, &restartCount, &restartThreshold, &ancestorSessionID, &tag, &nudgeCount, &effectiveMode, &status, &rateLimitUntilTs, &rateLimitRetryCount)

		key := agentType
		agent := map[string]interface{}{
			"agent_id":   id,
			"agent_type": agentType,
			"session_id": id,
			"result":     nil,
		}
		if phase.Valid {
			agent["phase"] = phase.String
		}
		if modelID.Valid && modelID.String != "" {
			key = agentType + ":" + modelID.String
			agent["model_id"] = modelID.String
			parts := strings.SplitN(modelID.String, ":", 2)
			if len(parts) == 2 {
				agent["cli"] = parts[0]
				agent["model"] = parts[1]
			}
		}
		if pid.Valid {
			agent["pid"] = pid.Int64
		}
		if agentResult.Valid {
			agent["result"] = agentResult.String
		}
		if startedAt.Valid {
			agent["started_at"] = startedAt.String
		}
		if contextLeft.Valid {
			agent["context_left"] = contextLeft.Int64
		}
		agent["restart_count"] = restartCount
		agent["nudge_count"] = nudgeCount
		if restartThreshold.Valid {
			agent["restart_threshold"] = restartThreshold.Int64
		}
		if tag.Valid && tag.String != "" {
			agent["tag"] = tag.String
		}
		if effectiveMode.Valid && effectiveMode.String != "" {
			agent["effective_mode"] = effectiveMode.String
		}
		if restartCount > 0 {
			chainRoot := id
			if ancestorSessionID.Valid {
				chainRoot = ancestorSessionID.String
			}
			if dets, ok := detailsMap[chainRoot]; ok {
				agent["restart_details"] = dets
			}
		}
		if status == "continued" {
			if !rateLimitUntilTs.Valid {
				continue
			}
			ts, parseErr := time.Parse(time.RFC3339Nano, rateLimitUntilTs.String)
			if parseErr != nil {
				ts, parseErr = time.Parse(time.RFC3339, rateLimitUntilTs.String)
			}
			if parseErr != nil || !ts.After(s.clock.Now()) {
				continue
			}
			agent["waiting_for_rate_limit"] = true
			agent["rate_limit_until_ts"] = rateLimitUntilTs.String
			agent["rate_limit_retry_count"] = rateLimitRetryCount
		}
		agents[key] = agent
	}
	return agents
}

func (s *WorkflowService) buildAgentHistory(wfiID string, detailsMap map[string][]RestartDetail) []interface{} {
	history := []interface{}{}
	rows, err := s.pool.Query(`
		SELECT s.id, s.phase, s.agent_type, s.model_id, s.status, s.result, s.result_reason, s.pid, s.started_at, s.ended_at, s.context_left, s.restart_count, s.ancestor_session_id, ad.tag, s.nudge_count, s.effective_mode
		FROM agent_sessions s
		LEFT JOIN workflow_instances wi ON wi.id = s.workflow_instance_id
		LEFT JOIN agent_definitions ad ON LOWER(ad.project_id) = LOWER(wi.project_id)
			AND LOWER(ad.workflow_id) = LOWER(wi.workflow_id)
			AND LOWER(ad.id) = LOWER(s.agent_type)
		WHERE s.workflow_instance_id = ? AND s.status NOT IN ('running', 'continued') AND s.agent_type NOT IN ('planner', 'context-saver', 'conflict-resolver')
		ORDER BY s.created_at`, wfiID)
	if err != nil {
		return history
	}
	defer rows.Close()

	for rows.Next() {
		var id, agentType string
		var phase, modelID, status, agentResult, resultReason, startedAt, endedAt, ancestorSessionID, tag, effectiveMode sql.NullString
		var pid, contextLeft sql.NullInt64
		var restartCount, nudgeCount int
		rows.Scan(&id, &phase, &agentType, &modelID, &status, &agentResult, &resultReason, &pid, &startedAt, &endedAt, &contextLeft, &restartCount, &ancestorSessionID, &tag, &nudgeCount, &effectiveMode)

		entry := map[string]interface{}{
			"agent_id":   id,
			"agent_type": agentType,
			"session_id": id,
		}
		if phase.Valid {
			entry["phase"] = phase.String
		}
		if modelID.Valid {
			entry["model_id"] = modelID.String
		}
		if status.Valid {
			entry["status"] = status.String
		}
		if agentResult.Valid {
			entry["result"] = agentResult.String
		}
		if resultReason.Valid {
			entry["result_reason"] = resultReason.String
		}
		if startedAt.Valid {
			entry["started_at"] = startedAt.String
		}
		if endedAt.Valid {
			entry["ended_at"] = endedAt.String
		}
		if startedAt.Valid && endedAt.Valid {
			if start, err := time.Parse(time.RFC3339Nano, startedAt.String); err == nil {
				if end, err := time.Parse(time.RFC3339Nano, endedAt.String); err == nil {
					dur := end.Sub(start).Seconds()
					if dur < 0 {
						dur = 0
					}
					entry["duration_sec"] = dur
				}
			}
		}
		if contextLeft.Valid {
			entry["context_left"] = contextLeft.Int64
		}
		if tag.Valid && tag.String != "" {
			entry["tag"] = tag.String
		}
		if effectiveMode.Valid && effectiveMode.String != "" {
			entry["effective_mode"] = effectiveMode.String
		}
		entry["restart_count"] = restartCount
		entry["nudge_count"] = nudgeCount
		if restartCount > 0 {
			chainRoot := id
			if ancestorSessionID.Valid {
				chainRoot = ancestorSessionID.String
			}
			if dets, ok := detailsMap[chainRoot]; ok {
				entry["restart_details"] = dets
			}
		}
		history = append(history, entry)
	}
	return history
}

// derivePhaseStatuses derives phase statuses from agent_sessions rows instead of the phases JSON column.
// This eliminates the race condition where parallel agents overwrite each other's status in the JSON blob.
func (s *WorkflowService) derivePhaseStatuses(wfiID string, phases []PhaseDef) map[string]model.PhaseStatus {
	result := make(map[string]model.PhaseStatus, len(phases))

	// Default all phases to pending
	for _, p := range phases {
		result[p.ID] = model.PhaseStatus{Status: "pending"}
	}

	seen := make(map[string]bool)
	maxLayer := -1

	// Pre-pass: continued sessions with a future rate_limit_until_ts become "rate_limited".
	// Use a separate dedup set so expired waiting rows don't block the main loop.
	waitingDeduped := make(map[string]bool)
	waitingRows, waitingErr := s.pool.Query(`
		SELECT agent_type, rate_limit_until_ts, rate_limit_retry_count FROM agent_sessions
		WHERE workflow_instance_id = ? AND status = 'continued' AND rate_limit_until_ts IS NOT NULL
		AND agent_type NOT IN ('planner', 'context-saver', 'conflict-resolver')
		ORDER BY created_at DESC`, wfiID)
	if waitingErr == nil {
		now := s.clock.Now()
		for waitingRows.Next() {
			var agentType string
			var rateLimitUntilTs sql.NullString
			var rateLimitRetryCount int
			waitingRows.Scan(&agentType, &rateLimitUntilTs, &rateLimitRetryCount)
			if waitingDeduped[agentType] {
				continue
			}
			waitingDeduped[agentType] = true
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
			result[agentType] = model.PhaseStatus{
				Status:              "rate_limited",
				RateLimitUntilTs:    rateLimitUntilTs.String,
				RateLimitRetryCount: rateLimitRetryCount,
			}
			seen[agentType] = true
			for _, p := range phases {
				if p.ID == agentType && p.Layer > maxLayer {
					maxLayer = p.Layer
				}
			}
		}
		waitingRows.Close()
	}

	// Query latest non-continued/callback session per agent_type
	rows, err := s.pool.Query(`
		SELECT agent_type, status, result FROM agent_sessions
		WHERE workflow_instance_id = ? AND status NOT IN ('continued', 'callback') AND agent_type NOT IN ('planner', 'context-saver', 'conflict-resolver')
		ORDER BY created_at DESC`, wfiID)
	if err != nil {
		return result
	}
	defer rows.Close()

	// Group by agent_type, take latest session per agent
	for rows.Next() {
		var agentType, status string
		var sessionResult sql.NullString
		rows.Scan(&agentType, &status, &sessionResult)

		if seen[agentType] {
			continue
		}
		seen[agentType] = true

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
		result[agentType] = ps

		// Track max layer that has a session
		for _, p := range phases {
			if p.ID == agentType && p.Layer > maxLayer {
				maxLayer = p.Layer
			}
		}
	}

	// Infer "skipped": if a phase has no session but a later layer does have sessions,
	// the phase's layer was already processed and the phase was skipped.
	// This works for both active and terminal workflows because the orchestrator
	// processes all phases in a layer before advancing to the next layer.
	for _, p := range phases {
		if !seen[p.ID] && p.Layer < maxLayer {
			result[p.ID] = model.PhaseStatus{Status: "skipped", Result: "skipped"}
		}
	}

	return result
}

// deriveCurrentPhase returns the phase of the latest active agent session (running,
// user_interactive, or rate-limited continued), or empty string if none.
func (s *WorkflowService) deriveCurrentPhase(wfiID string) string {
	var runningPhase, runningCreatedAt sql.NullString
	s.pool.QueryRow(`
		SELECT phase, created_at FROM agent_sessions
		WHERE workflow_instance_id = ? AND status IN ('running', 'user_interactive')
		ORDER BY created_at DESC LIMIT 1`, wfiID).Scan(&runningPhase, &runningCreatedAt)

	nowStr := s.clock.Now().UTC().Format(time.RFC3339Nano)
	var waitingPhase, waitingCreatedAt sql.NullString
	s.pool.QueryRow(`
		SELECT phase, created_at FROM agent_sessions
		WHERE workflow_instance_id = ? AND status = 'continued' AND rate_limit_until_ts IS NOT NULL AND rate_limit_until_ts > ?
		ORDER BY created_at DESC LIMIT 1`, wfiID, nowStr).Scan(&waitingPhase, &waitingCreatedAt)

	if !waitingPhase.Valid {
		if runningPhase.Valid {
			return runningPhase.String
		}
		return ""
	}
	if !runningPhase.Valid {
		return waitingPhase.String
	}
	// Both candidates exist; return the one with the later created_at
	if waitingCreatedAt.String > runningCreatedAt.String {
		return waitingPhase.String
	}
	return runningPhase.String
}

// DeriveWorkflowProgress computes workflow progress for a set of workflow instances.
// Returns a map of lowercased ticket ID -> WorkflowProgress.
func (s *WorkflowService) DeriveWorkflowProgress(instances map[string]*model.WorkflowInstance) map[string]*repo.WorkflowProgress {
	result := make(map[string]*repo.WorkflowProgress, len(instances))
	for ticketKey, wi := range instances {
		wf, err := s.GetWorkflowDef(wi.ProjectID, wi.WorkflowID)
		if err != nil {
			continue
		}
		phases := s.derivePhaseStatuses(wi.ID, wf.Phases)
		completed := 0
		for _, ps := range phases {
			if ps.Status == "completed" || ps.Status == "skipped" {
				completed++
			}
		}
		result[ticketKey] = &repo.WorkflowProgress{
			WorkflowName:    wi.WorkflowID,
			CurrentPhase:    s.deriveCurrentPhase(wi.ID),
			CompletedPhases: completed,
			TotalPhases:     len(wf.Phases),
			Status:          string(wi.Status),
		}
	}
	return result
}

// BuildCombinedFindings aggregates per-session findings into a single map keyed by agent_type[:model_id].
func (s *WorkflowService) BuildCombinedFindings(wi *model.WorkflowInstance) map[string]interface{} {
	combined := make(map[string]interface{})
	byAgent, err := s.findingRepo.ListByWorkflowInstance(wi.ID)
	if err != nil {
		return combined
	}
	for key, m := range byAgent {
		combined[key] = rawToInterface(m)
	}
	return combined
}

// ExtractWorkflowFinalResultByInstanceID returns the workflow_final_result finding value from
// any session finding for the workflow instance, or "" if not set.
func ExtractWorkflowFinalResultByInstanceID(pool *db.Pool, instanceID string, clk clock.Clock) string {
	fr := repo.NewFindingRepo(pool, clk)
	raw, found := fr.GetSessionFindingByKey(instanceID, "workflow_final_result")
	if !found {
		return ""
	}
	var s string
	if err := json.Unmarshal(raw, &s); err == nil {
		return s
	}
	return ""
}

// ExtractWorkflowFinalResult scans agent sessions for the workflow_final_result finding.
func (s *WorkflowService) ExtractWorkflowFinalResult(wi *model.WorkflowInstance) string {
	return ExtractWorkflowFinalResultByInstanceID(s.pool, wi.ID, s.clock)
}
