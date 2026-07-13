package spawner

import (
	"database/sql"
	"encoding/json"

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
	var reasonStr sql.NullString
	err = pool.QueryRow(`
		SELECT id, result_reason FROM agent_sessions
		WHERE workflow_instance_id = ? AND agent_type = ? AND model_id = ? AND node_id = ? AND status = 'continued'
		ORDER BY ended_at DESC LIMIT 1`,
		wfiID, agentType, modelID, phase).Scan(&sessionID, &reasonStr)
	if err != nil {
		return "", ""
	}

	reason := ""
	if reasonStr.Valid {
		reason = reasonStr.String
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
