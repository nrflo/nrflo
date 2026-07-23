package repo

import (
	"database/sql"
	"time"

	"be/internal/model"
)

// ListSystemAgentRuns returns recent agent_sessions rows that carry a
// resolved tier or belong to a system-agent definition, newest first. Cross-
// project by design — callers gate this admin-only.
func (r *AgentSessionRepo) ListSystemAgentRuns(limit int, since time.Time) ([]*model.SystemAgentRun, error) {
	query := `SELECT id, workflow_instance_id, project_id, node_id, agent_type, model_id, status, result,
		 tier, resolved_provider, resolved_execution_mode, resolved_effort, chain_position, fallback_from,
		 tokens_json, cost_estimate, created_at
		 FROM agent_sessions
		 WHERE (tier IS NOT NULL OR agent_type IN (SELECT id FROM system_agent_definitions))`
	args := []interface{}{}
	if !since.IsZero() {
		query += ` AND created_at >= ?`
		args = append(args, since.UTC().Format(time.RFC3339Nano))
	}
	query += ` ORDER BY created_at DESC LIMIT ?`
	args = append(args, limit)

	rows, err := r.db.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	runs := []*model.SystemAgentRun{}
	for rows.Next() {
		run := &model.SystemAgentRun{Kind: "agent_session"}
		var (
			workflowInstanceID, nodeID, modelID                     sql.NullString
			tier                                                    sql.NullInt64
			resolvedProvider, resolvedExecutionMode, resolvedEffort sql.NullString
			fallbackFrom, tokensJSON                                sql.NullString
			costEstimate                                            sql.NullFloat64
			createdAt                                               string
		)
		if err := rows.Scan(&run.SessionID, &workflowInstanceID, &run.ProjectID, &nodeID, &run.AgentType,
			&modelID, &run.Status, &run.Result, &tier, &resolvedProvider, &resolvedExecutionMode,
			&resolvedEffort, &run.ChainPosition, &fallbackFrom, &tokensJSON, &costEstimate, &createdAt); err != nil {
			return nil, err
		}
		run.WorkflowInstanceID = workflowInstanceID.String
		run.NodeID = nodeID.String
		run.ModelID = modelID.String
		run.ResolvedProvider = resolvedProvider.String
		run.ResolvedExecutionMode = resolvedExecutionMode.String
		run.ResolvedEffort = resolvedEffort.String
		if tier.Valid {
			t := int(tier.Int64)
			run.Tier = &t
		}
		if fallbackFrom.Valid {
			run.FallbackFrom = []byte(fallbackFrom.String)
		}
		if tokensJSON.Valid {
			run.TokensJSON = []byte(tokensJSON.String)
		}
		if costEstimate.Valid {
			run.CostEstimate = &costEstimate.Float64
		}
		run.CreatedAt, _ = time.Parse(time.RFC3339Nano, createdAt)
		runs = append(runs, run)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return runs, nil
}
