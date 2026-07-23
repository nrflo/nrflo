package repo

import (
	"encoding/json"
	"sort"
	"time"

	"be/internal/model"
)

// ListRotations returns up to limit stepwise rotation events (CompletedStep
// entries with Rotated=true), newest first, completed_at >= since when since
// is non-zero. Mirrors RefineryRunRepo.ListRecent's shape for the
// system-agent-runs Activity-view merge.
func (r *AgentStepCursorRepo) ListRotations(limit int, since time.Time) ([]*model.SystemAgentRun, error) {
	rows, err := r.db.Query(`
		SELECT c.workflow_instance_id, c.node_id, c.completed, wi.project_id, wi.ticket_id
		FROM agent_step_cursors c
		JOIN workflow_instances wi ON wi.id = c.workflow_instance_id
		ORDER BY c.updated_at DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var runs []*model.SystemAgentRun
	for rows.Next() {
		var instanceID, nodeID, completedJSON, projectID, ticketID string
		if err := rows.Scan(&instanceID, &nodeID, &completedJSON, &projectID, &ticketID); err != nil {
			return nil, err
		}
		var completed []model.CompletedStep
		if completedJSON == "" {
			continue
		}
		if err := json.Unmarshal([]byte(completedJSON), &completed); err != nil {
			continue
		}
		for _, cs := range completed {
			if !cs.Rotated {
				continue
			}
			completedAt, err := time.Parse(time.RFC3339Nano, cs.CompletedAt)
			if err != nil {
				continue
			}
			if !since.IsZero() && completedAt.Before(since) {
				continue
			}
			runs = append(runs, &model.SystemAgentRun{
				Kind:               "step_rotation",
				SessionID:          cs.SessionID,
				WorkflowInstanceID: instanceID,
				NodeID:             nodeID,
				StepID:             cs.StepID,
				ProjectID:          projectID,
				TicketID:           ticketID,
				Status:             "rotated",
				CreatedAt:          completedAt,
			})
		}
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	sort.Slice(runs, func(i, j int) bool { return runs[i].CreatedAt.After(runs[j].CreatedAt) })
	if len(runs) > limit {
		runs = runs[:limit]
	}
	return runs, nil
}
