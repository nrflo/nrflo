package service

import (
	"database/sql"
	"encoding/json"
	"strings"
	"time"

	"be/internal/model"
)

func (s *WorkflowService) buildActiveAgentsMap(wfiID string) map[string]interface{} {
	agents := make(map[string]interface{})
	rows, err := s.pool.Query(`
		SELECT s.id, s.phase, s.agent_type, s.model_id, s.pid, s.result, s.started_at, s.context_left, s.restart_count,
		       ad.restart_threshold
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
		var phase, modelID, agentResult, startedAt sql.NullString
		var pid, contextLeft, restartThreshold sql.NullInt64
		var restartCount int
		rows.Scan(&id, &phase, &agentType, &modelID, &pid, &agentResult, &startedAt, &contextLeft, &restartCount, &restartThreshold)

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
		agents[key] = agent
	}
	return agents
}

func (s *WorkflowService) buildAgentHistory(wfiID string) []interface{} {
	history := []interface{}{}
	rows, err := s.pool.Query(`
		SELECT id, phase, agent_type, model_id, status, result, result_reason, pid, started_at, ended_at, context_left, restart_count
		FROM agent_sessions
		WHERE workflow_instance_id = ? AND status NOT IN ('running', 'continued')
		ORDER BY created_at`, wfiID)
	if err != nil {
		return history
	}
	defer rows.Close()

	for rows.Next() {
		var id, agentType string
		var phase, modelID, status, agentResult, resultReason, startedAt, endedAt sql.NullString
		var pid, contextLeft sql.NullInt64
		var restartCount int
		rows.Scan(&id, &phase, &agentType, &modelID, &status, &agentResult, &resultReason, &pid, &startedAt, &endedAt, &contextLeft, &restartCount)

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
		entry["restart_count"] = restartCount
		history = append(history, entry)
	}
	return history
}

// BuildCombinedFindings merges workflow-level and per-session findings
func (s *WorkflowService) BuildCombinedFindings(wi *model.WorkflowInstance) map[string]interface{} {
	combined := wi.GetFindings()

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
