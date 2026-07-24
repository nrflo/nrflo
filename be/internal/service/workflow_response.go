package service

import (
	"database/sql"
	"encoding/json"
	"time"

	"be/internal/clock"
	"be/internal/db"
	"be/internal/model"
	"be/internal/repo"
)

// transientAgentTypeExclusion is the shared SQL WHERE fragment that hides system/internal
// agent types (named excludes), underscore-prefixed transient hook sessions (_finalize,
// _pause, _notification, …), and underscore-prefixed phases (_consult, …) from the v4 read model.
const transientAgentTypeExclusion = `agent_type NOT IN ('planner', 'context-saver', 'conflict-resolver') AND agent_type NOT LIKE '\_%' ESCAPE '\' AND phase NOT LIKE '\_%' ESCAPE '\'`

func (s *WorkflowService) buildActiveAgentsMap(wfiID string, detailsMap map[string][]RestartDetail) map[string]interface{} {
	agents := make(map[string]interface{})
	rows, err := s.pool.Query(`
		SELECT s.id, s.phase, s.node_id, s.agent_type, s.model_id, s.pid, s.result, s.started_at, s.context_left, s.restart_count,
		       ad.restart_threshold, s.ancestor_session_id, ad.tag, s.nudge_count, s.effective_mode, s.status, s.rate_limit_until_ts, s.rate_limit_retry_count
		FROM agent_sessions s
		LEFT JOIN workflow_instances wi ON wi.id = s.workflow_instance_id
		LEFT JOIN agent_definitions ad ON LOWER(ad.project_id) = LOWER(wi.project_id)
			AND LOWER(ad.workflow_id) = LOWER(wi.workflow_id)
			AND LOWER(ad.id) = LOWER(s.agent_type)
		WHERE s.workflow_instance_id = ? AND (s.status = 'running' OR (s.status = 'continued' AND s.rate_limit_until_ts IS NOT NULL)) AND `+transientAgentTypeExclusion, wfiID)
	if err != nil {
		return agents
	}
	defer rows.Close()

	for rows.Next() {
		var id, nodeID, agentType, status string
		var phase, modelID, agentResult, startedAt, ancestorSessionID, tag, effectiveMode, rateLimitUntilTs sql.NullString
		var pid, contextLeft, restartThreshold sql.NullInt64
		var restartCount, nudgeCount, rateLimitRetryCount int
		rows.Scan(&id, &phase, &nodeID, &agentType, &modelID, &pid, &agentResult, &startedAt, &contextLeft, &restartCount, &restartThreshold, &ancestorSessionID, &tag, &nudgeCount, &effectiveMode, &status, &rateLimitUntilTs, &rateLimitRetryCount)

		key := nodeID
		agent := map[string]interface{}{
			"node_id":    nodeID,
			"agent_type": agentType,
			"session_id": id,
		}
		if phase.Valid {
			agent["phase"] = phase.String
		}
		if modelID.Valid && modelID.String != "" {
			key = nodeID + ":" + modelID.String
			agent["model_id"] = modelID.String
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
		SELECT s.id, s.phase, s.node_id, s.agent_type, s.model_id, s.status, s.result, s.result_reason, s.pid, s.started_at, s.ended_at, s.context_left, s.restart_count, s.ancestor_session_id, ad.tag, s.nudge_count, s.effective_mode
		FROM agent_sessions s
		LEFT JOIN workflow_instances wi ON wi.id = s.workflow_instance_id
		LEFT JOIN agent_definitions ad ON LOWER(ad.project_id) = LOWER(wi.project_id)
			AND LOWER(ad.workflow_id) = LOWER(wi.workflow_id)
			AND LOWER(ad.id) = LOWER(s.agent_type)
		WHERE s.workflow_instance_id = ? AND s.status NOT IN ('running', 'continued') AND `+transientAgentTypeExclusion+`
		ORDER BY s.created_at`, wfiID)
	if err != nil {
		return history
	}
	defer rows.Close()

	for rows.Next() {
		var id, nodeID, agentType string
		var phase, modelID, status, agentResult, resultReason, startedAt, endedAt, ancestorSessionID, tag, effectiveMode sql.NullString
		var pid, contextLeft sql.NullInt64
		var restartCount, nudgeCount int
		rows.Scan(&id, &phase, &nodeID, &agentType, &modelID, &status, &agentResult, &resultReason, &pid, &startedAt, &endedAt, &contextLeft, &restartCount, &ancestorSessionID, &tag, &nudgeCount, &effectiveMode)

		entry := map[string]interface{}{
			"node_id":    nodeID,
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

// ExtractWorkflowFailureReason reads the workflow-instance-owned `_failure_reason`
// finding ({"reason": "..."}), or "" if not set.
func (s *WorkflowService) ExtractWorkflowFailureReason(wi *model.WorkflowInstance) string {
	fr := repo.NewFindingRepo(s.pool, s.clock)
	own, err := fr.GetOwn("workflow_instance", wi.ID)
	if err != nil {
		return ""
	}
	raw, ok := own["_failure_reason"]
	if !ok {
		return ""
	}
	var v struct {
		Reason string `json:"reason"`
	}
	if json.Unmarshal(raw, &v) != nil {
		return ""
	}
	return v.Reason
}
