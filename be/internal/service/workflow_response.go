package service

import (
	"database/sql"
	"encoding/json"
	"strings"
	"time"

	"be/internal/model"
	"be/internal/repo"
)

func (s *WorkflowService) buildActiveAgentsMap(wfiID string, detailsMap map[string][]RestartDetail) map[string]interface{} {
	agents := make(map[string]interface{})
	rows, err := s.pool.Query(`
		SELECT s.id, s.phase, s.agent_type, s.model_id, s.pid, s.result, s.started_at, s.context_left, s.restart_count,
		       ad.restart_threshold, s.ancestor_session_id, ad.tag
		FROM agent_sessions s
		LEFT JOIN workflow_instances wi ON wi.id = s.workflow_instance_id
		LEFT JOIN agent_definitions ad ON LOWER(ad.project_id) = LOWER(wi.project_id)
			AND LOWER(ad.workflow_id) = LOWER(wi.workflow_id)
			AND LOWER(ad.id) = LOWER(s.agent_type)
		WHERE s.workflow_instance_id = ? AND s.status = 'running'`, wfiID)
	if err != nil {
		return agents
	}
	defer rows.Close()

	for rows.Next() {
		var id, agentType string
		var phase, modelID, agentResult, startedAt, ancestorSessionID, tag sql.NullString
		var pid, contextLeft, restartThreshold sql.NullInt64
		var restartCount int
		rows.Scan(&id, &phase, &agentType, &modelID, &pid, &agentResult, &startedAt, &contextLeft, &restartCount, &restartThreshold, &ancestorSessionID, &tag)

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
		if restartThreshold.Valid {
			agent["restart_threshold"] = restartThreshold.Int64
		}
		if tag.Valid && tag.String != "" {
			agent["tag"] = tag.String
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
		agents[key] = agent
	}
	return agents
}

func (s *WorkflowService) buildAgentHistory(wfiID string, detailsMap map[string][]RestartDetail) []interface{} {
	history := []interface{}{}
	rows, err := s.pool.Query(`
		SELECT s.id, s.phase, s.agent_type, s.model_id, s.status, s.result, s.result_reason, s.pid, s.started_at, s.ended_at, s.context_left, s.restart_count, s.ancestor_session_id, ad.tag
		FROM agent_sessions s
		LEFT JOIN workflow_instances wi ON wi.id = s.workflow_instance_id
		LEFT JOIN agent_definitions ad ON LOWER(ad.project_id) = LOWER(wi.project_id)
			AND LOWER(ad.workflow_id) = LOWER(wi.workflow_id)
			AND LOWER(ad.id) = LOWER(s.agent_type)
		WHERE s.workflow_instance_id = ? AND s.status NOT IN ('running', 'continued')
		ORDER BY s.created_at`, wfiID)
	if err != nil {
		return history
	}
	defer rows.Close()

	for rows.Next() {
		var id, agentType string
		var phase, modelID, status, agentResult, resultReason, startedAt, endedAt, ancestorSessionID, tag sql.NullString
		var pid, contextLeft sql.NullInt64
		var restartCount int
		rows.Scan(&id, &phase, &agentType, &modelID, &status, &agentResult, &resultReason, &pid, &startedAt, &endedAt, &contextLeft, &restartCount, &ancestorSessionID, &tag)

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
		entry["restart_count"] = restartCount
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

	// Query latest non-continued/callback session per agent_type
	rows, err := s.pool.Query(`
		SELECT agent_type, status, result FROM agent_sessions
		WHERE workflow_instance_id = ? AND status NOT IN ('continued', 'callback')
		ORDER BY created_at DESC`, wfiID)
	if err != nil {
		return result
	}
	defer rows.Close()

	// Group by agent_type, take latest session per agent
	seen := make(map[string]bool)
	maxLayer := -1 // track highest layer with a session
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

// deriveCurrentPhase returns the phase of the latest running agent session, or empty string if none.
func (s *WorkflowService) deriveCurrentPhase(wfiID string) string {
	var phase sql.NullString
	err := s.pool.QueryRow(`
		SELECT phase FROM agent_sessions
		WHERE workflow_instance_id = ? AND status IN ('running', 'user_interactive')
		ORDER BY created_at DESC LIMIT 1`, wfiID).Scan(&phase)
	if err != nil || !phase.Valid {
		return ""
	}
	return phase.String
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

// BuildCombinedFindings aggregates per-session findings into a single map.
// Workflow-level findings are served separately via the workflow_findings response field.
func (s *WorkflowService) BuildCombinedFindings(wi *model.WorkflowInstance) map[string]interface{} {
	combined := make(map[string]interface{})

	rows, err := s.pool.Query(`
		SELECT agent_type, model_id, findings
		FROM agent_sessions
		WHERE workflow_instance_id = ? AND findings IS NOT NULL AND findings != ''`, wi.ID)
	if err != nil {
		return combined
	}
	defer rows.Close()

	for rows.Next() {
		var agentType string
		var modelID, findingsStr sql.NullString
		rows.Scan(&agentType, &modelID, &findingsStr)

		if !findingsStr.Valid || findingsStr.String == "" {
			continue
		}
		var sessionFindings map[string]interface{}
		if json.Unmarshal([]byte(findingsStr.String), &sessionFindings) != nil {
			continue
		}

		key := agentType
		if modelID.Valid && modelID.String != "" {
			key = agentType + ":" + modelID.String
		}
		combined[key] = sessionFindings
	}
	return combined
}

// ExtractWorkflowFinalResult scans agent sessions for the workflow_final_result finding,
// returning the value from the session with the latest ended_at. Running sessions (NULL
// ended_at) are deprioritized — completed sessions always take precedence.
func (s *WorkflowService) ExtractWorkflowFinalResult(wi *model.WorkflowInstance) string {
	rows, err := s.pool.Query(`
		SELECT findings FROM agent_sessions
		WHERE workflow_instance_id = ? AND findings IS NOT NULL AND findings != ''
		ORDER BY ended_at IS NULL, ended_at DESC`, wi.ID)
	if err != nil {
		return ""
	}
	defer rows.Close()

	for rows.Next() {
		var findingsStr string
		rows.Scan(&findingsStr)

		var sessionFindings map[string]interface{}
		if json.Unmarshal([]byte(findingsStr), &sessionFindings) != nil {
			continue
		}
		if val, ok := sessionFindings["workflow_final_result"]; ok {
			if s, ok := val.(string); ok {
				return s
			}
			return ""
		}
	}
	return ""
}
