package repo

import (
	"fmt"

	"be/internal/model"
)

// ListFinishedFilter specifies criteria for ListFinished.
type ListFinishedFilter struct {
	ProjectID string
}

// ListFinished returns paginated finished agent sessions joined with workflow_instances
// and agent_definitions. Excludes active statuses (running, continued, callback, user_interactive).
func (r *AgentSessionRepo) ListFinished(f ListFinishedFilter, page, perPage int) ([]*model.AgentSessionLogRow, int, error) {
	var total int
	if err := r.db.QueryRow(
		`SELECT COUNT(*) FROM agent_sessions s
		WHERE LOWER(s.project_id) = LOWER(?)
		AND s.status NOT IN ('running', 'continued', 'callback', 'user_interactive')
		AND s.kind = '`+model.AgentSessionKindWorkflowAgent+`'`,
		f.ProjectID,
	).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("count agent session logs: %w", err)
	}

	if total == 0 {
		return nil, 0, nil
	}

	offset := (page - 1) * perPage
	rows, err := r.db.Query(`
		SELECT s.id, s.project_id, s.agent_type, s.model_id, s.status,
		       s.started_at, s.ended_at, s.updated_at,
		       s.effective_mode,
		       wi.workflow_id, s.workflow_instance_id, wi.scheduled_task_id,
		       ad.execution_mode
		FROM agent_sessions s
		JOIN workflow_instances wi ON s.workflow_instance_id = wi.id
		LEFT JOIN agent_definitions ad
		       ON ad.project_id = s.project_id
		       AND ad.workflow_id = wi.workflow_id
		       AND ad.id = s.agent_type
		WHERE LOWER(s.project_id) = LOWER(?)
		AND s.status NOT IN ('running', 'continued', 'callback', 'user_interactive')
		ORDER BY COALESCE(s.ended_at, s.updated_at) DESC, s.id DESC
		LIMIT ? OFFSET ?`,
		f.ProjectID, perPage, offset,
	)
	if err != nil {
		return nil, 0, fmt.Errorf("list agent session logs: %w", err)
	}
	defer rows.Close()

	var result []*model.AgentSessionLogRow
	for rows.Next() {
		row := &model.AgentSessionLogRow{}
		err := rows.Scan(
			&row.SessionID, &row.ProjectID, &row.AgentType, &row.ModelID, &row.Status,
			&row.StartedAt, &row.EndedAt, &row.UpdatedAt,
			&row.EffectiveMode,
			&row.WorkflowID, &row.WorkflowInstanceID, &row.ScheduledTaskID,
			&row.ExecutionMode,
		)
		if err != nil {
			return nil, 0, fmt.Errorf("scan agent session log row: %w", err)
		}
		result = append(result, row)
	}
	return result, total, rows.Err()
}

// ListLiveByProject returns running/user_interactive sessions with pid > 0 for the given project,
// joined with workflow_instances and agent_definitions.
func (r *AgentSessionRepo) ListLiveByProject(projectID string) ([]*model.AgentSessionLogRow, error) {
	rows, err := r.db.Query(`
		SELECT s.id, s.project_id, s.agent_type, s.model_id, s.status,
		       s.started_at, s.ended_at, s.updated_at,
		       s.effective_mode, s.pid,
		       wi.workflow_id, s.workflow_instance_id, wi.scheduled_task_id,
		       ad.execution_mode, s.rate_limit_until_ts
		FROM agent_sessions s
		JOIN workflow_instances wi ON s.workflow_instance_id = wi.id
		LEFT JOIN agent_definitions ad
		       ON ad.project_id = s.project_id
		       AND ad.workflow_id = wi.workflow_id
		       AND ad.id = s.agent_type
		WHERE LOWER(s.project_id) = LOWER(?)
		AND s.status IN ('running', 'user_interactive')
		AND s.pid IS NOT NULL AND s.pid > 0
		ORDER BY s.started_at DESC`,
		projectID,
	)
	if err != nil {
		return nil, fmt.Errorf("list live agent sessions: %w", err)
	}
	defer rows.Close()

	var result []*model.AgentSessionLogRow
	for rows.Next() {
		row := &model.AgentSessionLogRow{}
		err := rows.Scan(
			&row.SessionID, &row.ProjectID, &row.AgentType, &row.ModelID, &row.Status,
			&row.StartedAt, &row.EndedAt, &row.UpdatedAt,
			&row.EffectiveMode, &row.PID,
			&row.WorkflowID, &row.WorkflowInstanceID, &row.ScheduledTaskID,
			&row.ExecutionMode, &row.RateLimitUntilTs,
		)
		if err != nil {
			return nil, fmt.Errorf("scan live agent session row: %w", err)
		}
		result = append(result, row)
	}
	return result, rows.Err()
}

// CleanupOrphanedMessages deletes agent_messages whose session no longer exists
// (e.g. removed by CASCADE when a workflow_instance was deleted).
func (r *AgentSessionRepo) CleanupOrphanedMessages() (int64, error) {
	result, err := r.db.Exec(`
		DELETE FROM agent_messages
		WHERE session_id NOT IN (SELECT id FROM agent_sessions)`)
	if err != nil {
		return 0, err
	}
	return result.RowsAffected()
}
