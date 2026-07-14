package repo

import (
	"database/sql"
	"time"

	"be/internal/model"
)

const sessionCols = `id, project_id, ticket_id, workflow_instance_id, phase, node_id, agent_type,
	model_id, status, result, result_reason, pid,
	context_left, ancestor_session_id, spawn_command, prompt, system_prompt,
	restart_count, nudge_count, config, started_at, ended_at, spawn_token, effective_mode, created_at, updated_at,
	rate_limit_retry_count, rate_limit_until_ts, last_retry_class, kind, observer_scope, observer_workflow_id, console_engine`

func scanSession(scanner interface{ Scan(...interface{}) error }) (*model.AgentSession, error) {
	s := &model.AgentSession{}
	var createdAt, updatedAt string
	var wfi sql.NullString
	err := scanner.Scan(
		&s.ID, &s.ProjectID, &s.TicketID, &wfi, &s.Phase, &s.NodeID, &s.AgentType,
		&s.ModelID, &s.Status, &s.Result, &s.ResultReason, &s.PID,
		&s.ContextLeft, &s.AncestorSessionID, &s.SpawnCommand, &s.Prompt, &s.SystemPrompt,
		&s.RestartCount, &s.NudgeCount, &s.Config, &s.StartedAt, &s.EndedAt, &s.SpawnToken, &s.EffectiveMode, &createdAt, &updatedAt,
		&s.RateLimitRetryCount, &s.RateLimitUntilTs, &s.LastRetryClass, &s.Kind, &s.ObserverScope, &s.ObserverWorkflowID, &s.ConsoleEngine,
	)
	if err != nil {
		return nil, err
	}
	s.WorkflowInstanceID = wfi.String
	s.CreatedAt, _ = time.Parse(time.RFC3339Nano, createdAt)
	s.UpdatedAt, _ = time.Parse(time.RFC3339Nano, updatedAt)
	return s, nil
}

// sessionColsWithWorkflow returns columns for JOINed queries that include workflow_id
const sessionColsJoined = `s.id, s.project_id, s.ticket_id, s.workflow_instance_id, s.phase, s.node_id, s.agent_type,
	s.model_id, s.status, s.result, s.result_reason, s.pid,
	s.context_left, s.ancestor_session_id, s.spawn_command, s.prompt, s.system_prompt,
	s.restart_count, s.nudge_count, s.config, s.started_at, s.ended_at, s.spawn_token, s.effective_mode, s.created_at, s.updated_at,
	s.rate_limit_retry_count, s.rate_limit_until_ts, s.last_retry_class, s.kind, s.observer_scope, s.observer_workflow_id, wi.workflow_id`

func scanSessionJoined(scanner interface{ Scan(...interface{}) error }) (*model.AgentSession, error) {
	s := &model.AgentSession{}
	var createdAt, updatedAt string
	var wfi sql.NullString
	err := scanner.Scan(
		&s.ID, &s.ProjectID, &s.TicketID, &wfi, &s.Phase, &s.NodeID, &s.AgentType,
		&s.ModelID, &s.Status, &s.Result, &s.ResultReason, &s.PID,
		&s.ContextLeft, &s.AncestorSessionID, &s.SpawnCommand, &s.Prompt, &s.SystemPrompt,
		&s.RestartCount, &s.NudgeCount, &s.Config, &s.StartedAt, &s.EndedAt, &s.SpawnToken, &s.EffectiveMode, &createdAt, &updatedAt,
		&s.RateLimitRetryCount, &s.RateLimitUntilTs, &s.LastRetryClass, &s.Kind, &s.ObserverScope, &s.ObserverWorkflowID, &s.Workflow,
	)
	if err != nil {
		return nil, err
	}
	s.WorkflowInstanceID = wfi.String
	s.CreatedAt, _ = time.Parse(time.RFC3339Nano, createdAt)
	s.UpdatedAt, _ = time.Parse(time.RFC3339Nano, updatedAt)
	return s, nil
}
