package service

import (
	"database/sql"
	"time"

	"be/internal/model"
)

// listAgentDefsForWorkflow queries agent_definitions for a specific workflow, ordered by layer ASC, id ASC
func (s *WorkflowService) listAgentDefsForWorkflow(projectID, workflowID string) ([]*model.AgentDefinition, error) {
	rows, err := s.pool.Query(`
		SELECT id, project_id, workflow_id, model, timeout, prompt, restart_threshold, max_fail_restarts,
			stall_start_timeout_sec, stall_running_timeout_sec, context_budget_tokens, tag, low_consumption_model, layer, created_at, updated_at
		FROM agent_definitions
		WHERE LOWER(project_id) = LOWER(?) AND LOWER(workflow_id) = LOWER(?) AND consultant = 0 AND node_role = 'static'
		ORDER BY layer ASC, id ASC`, projectID, workflowID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanAgentDefs(rows)
}

// listAgentDefsForProject queries all agent_definitions for a project, ordered by layer ASC, id ASC
func (s *WorkflowService) listAgentDefsForProject(projectID string) ([]*model.AgentDefinition, error) {
	rows, err := s.pool.Query(`
		SELECT id, project_id, workflow_id, model, timeout, prompt, restart_threshold, max_fail_restarts,
			stall_start_timeout_sec, stall_running_timeout_sec, context_budget_tokens, tag, low_consumption_model, layer, created_at, updated_at
		FROM agent_definitions
		WHERE LOWER(project_id) = LOWER(?) AND consultant = 0 AND node_role = 'static'
		ORDER BY layer ASC, id ASC`, projectID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanAgentDefs(rows)
}

// scanAgentDefs scans agent definition rows into model objects
func scanAgentDefs(rows interface {
	Next() bool
	Scan(...interface{}) error
}) ([]*model.AgentDefinition, error) {
	var defs []*model.AgentDefinition
	for rows.Next() {
		def := &model.AgentDefinition{}
		var createdAt, updatedAt string
		var restartThreshold, maxFailRestarts, stallStartTimeout, stallRunningTimeout, contextBudgetTokens sql.NullInt64

		err := rows.Scan(
			&def.ID, &def.ProjectID, &def.WorkflowID,
			&def.Model, &def.Timeout, &def.Prompt,
			&restartThreshold, &maxFailRestarts, &stallStartTimeout, &stallRunningTimeout, &contextBudgetTokens,
			&def.Tag, &def.LowConsumptionModel, &def.Layer,
			&createdAt, &updatedAt,
		)
		if err != nil {
			return nil, err
		}

		def.CreatedAt, _ = time.Parse(time.RFC3339Nano, createdAt)
		def.UpdatedAt, _ = time.Parse(time.RFC3339Nano, updatedAt)
		if restartThreshold.Valid {
			v := int(restartThreshold.Int64)
			def.RestartThreshold = &v
		}
		if maxFailRestarts.Valid {
			v := int(maxFailRestarts.Int64)
			def.MaxFailRestarts = &v
		}
		if stallStartTimeout.Valid {
			v := int(stallStartTimeout.Int64)
			def.StallStartTimeoutSec = &v
		}
		if stallRunningTimeout.Valid {
			v := int(stallRunningTimeout.Int64)
			def.StallRunningTimeoutSec = &v
		}
		if contextBudgetTokens.Valid {
			v := int(contextBudgetTokens.Int64)
			def.ContextBudgetTokens = &v
		}
		defs = append(defs, def)
	}
	return defs, nil
}
