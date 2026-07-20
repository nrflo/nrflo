package spawner

import (
	"database/sql"
	"encoding/json"
	"time"

	"be/internal/repo"
)

// fetchPreviousDataAndReason retrieves to_resume data and result_reason from the most
// recent continued session for the same agent type, model, and phase.
// instanceID is optional — when set, used directly instead of DB lookup.
func (s *Spawner) fetchPreviousDataAndReason(projectID, ticketID, workflowName, agentType, modelID, phase, instanceID string) (data string, resultReason string) {
	if phase == "" {
		return "", ""
	}

	pool := s.pool()
	if pool == nil {
		return "", ""
	}

	wfiID := instanceID
	var err error
	if wfiID == "" {
		if ticketID == "" {
			err = pool.QueryRow(`
				SELECT id FROM workflow_instances
				WHERE LOWER(project_id) = LOWER(?) AND LOWER(workflow_id) = LOWER(?) AND scope_type = 'project' AND status = 'active'
				ORDER BY created_at DESC LIMIT 1`,
				projectID, workflowName).Scan(&wfiID)
		} else {
			err = pool.QueryRow(`
				SELECT id FROM workflow_instances
				WHERE LOWER(project_id) = LOWER(?) AND LOWER(ticket_id) = LOWER(?) AND LOWER(workflow_id) = LOWER(?)`,
				projectID, ticketID, workflowName).Scan(&wfiID)
		}
		if err != nil {
			return "", ""
		}
	}

	var sessionID string
	var reasonStr, startedAtStr sql.NullString
	err = pool.QueryRow(`
		SELECT id, result_reason, started_at FROM agent_sessions
		WHERE workflow_instance_id = ? AND agent_type = ? AND model_id = ? AND node_id = ? AND status = 'continued'
		ORDER BY ended_at DESC LIMIT 1`,
		wfiID, agentType, modelID, phase).Scan(&sessionID, &reasonStr, &startedAtStr)
	if err != nil {
		return "", ""
	}

	reason := ""
	if reasonStr.Valid {
		reason = reasonStr.String
	}

	// Fresh autonomous refinery slot digest takes priority over the
	// to_resume finding — one canonical source for the low-context
	// injectable's data (see digest_freshness.go).
	if startedAtStr.Valid {
		if prevStarted, perr := time.Parse(time.RFC3339Nano, startedAtStr.String); perr == nil {
			if content, ok := freshSlotDigest(pool, s.config.Clock, wfiID, phase, prevStarted); ok {
				return content, reason
			}
		}
	}

	findingRepo := repo.NewFindingRepo(pool, s.config.Clock)
	rawFindings, err := findingRepo.GetOwn("session", sessionID)
	if err != nil || len(rawFindings) == 0 {
		return "", reason
	}

	rawVal, ok := rawFindings["to_resume"]
	if !ok {
		return "", reason
	}
	var str string
	if json.Unmarshal(rawVal, &str) != nil || str == "" {
		return "", reason
	}
	return str, reason
}
