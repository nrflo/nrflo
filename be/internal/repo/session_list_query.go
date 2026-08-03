package repo

import "database/sql"

// SessionSummaryRow is one session's Sessions-tab listing row: identity,
// status, and cost/token rollup, joined to its workflow_instance's
// workflow/ticket ids when bound. Mirrors consoleChatListItem's role for the
// console-chat-only listing, generalized to every agent_sessions kind.
type SessionSummaryRow struct {
	SessionID          string
	ProjectID          string
	Kind               string
	AgentType          string
	ModelID            sql.NullString
	Status             string
	Result             sql.NullString
	WorkflowInstanceID sql.NullString
	WorkflowID         sql.NullString
	TicketID           sql.NullString
	StartedAt          sql.NullString
	EndedAt            sql.NullString
	CostEstimate       sql.NullFloat64
	TokensJSON         sql.NullString
}

const sessionSummaryQuery = `
	SELECT s.id, s.project_id, s.kind, s.agent_type, s.model_id, s.status, s.result,
	       s.workflow_instance_id, wi.workflow_id, s.ticket_id,
	       s.started_at, s.ended_at, s.cost_estimate, s.tokens_json
	FROM agent_sessions s
	LEFT JOIN workflow_instances wi ON wi.id = s.workflow_instance_id`

func scanSessionSummaries(rows *sql.Rows) ([]SessionSummaryRow, error) {
	defer rows.Close()
	var out []SessionSummaryRow
	for rows.Next() {
		var row SessionSummaryRow
		if err := rows.Scan(
			&row.SessionID, &row.ProjectID, &row.Kind, &row.AgentType, &row.ModelID, &row.Status, &row.Result,
			&row.WorkflowInstanceID, &row.WorkflowID, &row.TicketID,
			&row.StartedAt, &row.EndedAt, &row.CostEstimate, &row.TokensJSON,
		); err != nil {
			return nil, err
		}
		out = append(out, row)
	}
	return out, rows.Err()
}

// ListSessionSummaries returns projectID's sessions across every kind, newest
// first. limit<=0 defaults to 100.
func (r *AgentSessionRepo) ListSessionSummaries(projectID string, limit int) ([]SessionSummaryRow, error) {
	if limit <= 0 {
		limit = 100
	}
	rows, err := r.db.Query(sessionSummaryQuery+`
		WHERE LOWER(s.project_id) = LOWER(?)
		ORDER BY s.started_at DESC LIMIT ?`, projectID, limit)
	if err != nil {
		return nil, err
	}
	return scanSessionSummaries(rows)
}

// ListSessionSummariesGlobal is ListSessionSummaries without a project filter
// — cross-project by design, admin/global Sessions tab only.
func (r *AgentSessionRepo) ListSessionSummariesGlobal(limit int) ([]SessionSummaryRow, error) {
	if limit <= 0 {
		limit = 100
	}
	rows, err := r.db.Query(sessionSummaryQuery+`
		ORDER BY s.started_at DESC LIMIT ?`, limit)
	if err != nil {
		return nil, err
	}
	return scanSessionSummaries(rows)
}
