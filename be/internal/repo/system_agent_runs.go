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
	query := `SELECT s.id, s.workflow_instance_id, s.project_id, s.ticket_id, s.node_id, s.agent_type, s.model_id, s.status, s.result,
		 s.tier, s.resolved_provider, s.resolved_execution_mode, s.resolved_effort, s.chain_position, s.fallback_from,
		 s.tokens_json, s.cost_estimate, s.created_at,
		 dw.id, dw.caller_session_id, dw.tier, dw.fanout, dw.status, dw.branch_name
		 FROM agent_sessions s
		 LEFT JOIN (
		   SELECT je.value AS worker_session_id, d.id AS id, d.caller_session_id AS caller_session_id,
		          d.tier AS tier, d.fanout AS fanout, d.status AS status, d.branch_name AS branch_name
		   FROM delegations d, json_each(d.worker_session_ids) je
		   WHERE je.value <> ''
		   GROUP BY je.value
		 ) dw ON dw.worker_session_id = s.id
		 WHERE (s.tier IS NOT NULL OR s.agent_type IN (SELECT id FROM system_agent_definitions) OR s.node_id = '_consult')`
	args := []interface{}{}
	if !since.IsZero() {
		query += ` AND s.created_at >= ?`
		args = append(args, since.UTC().Format(time.RFC3339Nano))
	}
	query += ` ORDER BY s.created_at DESC LIMIT ?`
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
			workflowInstanceID, nodeID, modelID                           sql.NullString
			ticketID                                                      string
			tier                                                          sql.NullInt64
			resolvedProvider, resolvedExecutionMode, resolvedEffort       sql.NullString
			fallbackFrom, tokensJSON, result                              sql.NullString
			costEstimate                                                  sql.NullFloat64
			createdAt                                                     string
			delegationID, callerSessionID, delegateTier, delegationStatus sql.NullString
			fanout                                                        sql.NullInt64
			delegationBranch                                              sql.NullString
		)
		if err := rows.Scan(&run.SessionID, &workflowInstanceID, &run.ProjectID, &ticketID, &nodeID, &run.AgentType,
			&modelID, &run.Status, &result, &tier, &resolvedProvider, &resolvedExecutionMode,
			&resolvedEffort, &run.ChainPosition, &fallbackFrom, &tokensJSON, &costEstimate, &createdAt,
			&delegationID, &callerSessionID, &delegateTier, &fanout, &delegationStatus, &delegationBranch); err != nil {
			return nil, err
		}
		run.WorkflowInstanceID = workflowInstanceID.String
		run.TicketID = ticketID
		run.NodeID = nodeID.String
		run.ModelID = modelID.String
		run.Result = result.String
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
		run.DelegationID = delegationID.String
		run.CallerSessionID = callerSessionID.String
		run.DelegateTier = delegateTier.String
		run.DelegationStatus = delegationStatus.String
		run.DelegationBranch = delegationBranch.String
		if fanout.Valid {
			run.Fanout = int(fanout.Int64)
		}
		runs = append(runs, run)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return runs, nil
}
