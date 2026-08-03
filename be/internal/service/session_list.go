package service

import (
	"be/internal/clock"
	"be/internal/db"
	"be/internal/repo"
	"be/internal/types"
)

// ListSessions returns projectID's Sessions-tab listing across every
// agent_sessions kind, newest first — read-time, write-nothing, the same
// shape as BuildTrace's role for the trace subsystem. limit<=0 defaults to
// repo.ListSessionSummaries's own default (100).
func ListSessions(pool *db.Pool, clk clock.Clock, projectID string, limit int) (types.SessionListResponse, error) {
	rows, err := repo.NewAgentSessionRepo(pool, clk).ListSessionSummaries(projectID, limit)
	if err != nil {
		return types.SessionListResponse{}, err
	}
	return types.SessionListResponse{Sessions: sessionSummariesToItems(rows)}, nil
}

// ListSessionsGlobal is ListSessions without a project filter — cross-project
// by design, admin/global Sessions tab only (mirrors handleGetActiveWorkflows).
func ListSessionsGlobal(pool *db.Pool, clk clock.Clock, limit int) (types.SessionListResponse, error) {
	rows, err := repo.NewAgentSessionRepo(pool, clk).ListSessionSummariesGlobal(limit)
	if err != nil {
		return types.SessionListResponse{}, err
	}
	return types.SessionListResponse{Sessions: sessionSummariesToItems(rows)}, nil
}

func sessionSummariesToItems(rows []repo.SessionSummaryRow) []types.SessionListItem {
	items := make([]types.SessionListItem, 0, len(rows))
	for _, row := range rows {
		item := types.SessionListItem{
			SessionID:          row.SessionID,
			ProjectID:          row.ProjectID,
			Kind:               row.Kind,
			AgentType:          row.AgentType,
			ModelID:            row.ModelID.String,
			Status:             row.Status,
			Result:             row.Result.String,
			WorkflowInstanceID: row.WorkflowInstanceID.String,
			Workflow:           row.WorkflowID.String,
			TicketID:           row.TicketID.String,
			StartedAt:          row.StartedAt.String,
			EndedAt:            row.EndedAt.String,
		}
		if row.CostEstimate.Valid {
			cost := row.CostEstimate.Float64
			item.CostEstimate = &cost
		}
		items = append(items, item)
	}
	return items
}
