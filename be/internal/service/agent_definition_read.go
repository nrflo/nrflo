package service

import (
	"database/sql"
	"fmt"
	"time"

	"be/internal/model"
)

// GetAgentDef retrieves a single agent definition
func (s *AgentDefinitionService) GetAgentDef(projectID, workflowID, id string) (*model.AgentDefinition, error) {
	def := &model.AgentDefinition{}
	var createdAt, updatedAt string
	var restartThreshold, maxFailRestarts, stallStartTimeout, stallRunningTimeout, apiMaxIter, apiMaxTokens sql.NullInt64
	var pythonScriptID, reasoningEffort sql.NullString

	err := s.pool.QueryRow(`
		SELECT id, project_id, workflow_id, model, timeout, prompt, restart_threshold, max_fail_restarts, stall_start_timeout_sec, stall_running_timeout_sec, tag, low_consumption_model, layer, execution_mode, tools, api_max_iterations, api_max_tokens, python_script_id, validation_commands, consultant, node_role, description, reasoning_effort, created_at, updated_at
		FROM agent_definitions
		WHERE LOWER(project_id) = LOWER(?) AND LOWER(workflow_id) = LOWER(?) AND LOWER(id) = LOWER(?)`,
		projectID, workflowID, id).Scan(
		&def.ID, &def.ProjectID, &def.WorkflowID,
		&def.Model, &def.Timeout, &def.Prompt,
		&restartThreshold, &maxFailRestarts, &stallStartTimeout, &stallRunningTimeout, &def.Tag,
		&def.LowConsumptionModel, &def.Layer,
		&def.ExecutionMode, &def.Tools, &apiMaxIter, &apiMaxTokens, &pythonScriptID,
		&def.ValidationCommands, &def.Consultant, &def.NodeRole, &def.Description, &reasoningEffort,
		&createdAt, &updatedAt,
	)
	if err == sql.ErrNoRows {
		return nil, fmt.Errorf("agent definition not found: %s", id)
	}
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
	if apiMaxIter.Valid {
		v := int(apiMaxIter.Int64)
		def.APIMaxIterations = &v
	}
	if apiMaxTokens.Valid {
		v := int(apiMaxTokens.Int64)
		def.APIMaxTokens = &v
	}
	if pythonScriptID.Valid {
		s := pythonScriptID.String
		def.PythonScriptID = &s
	}
	if reasoningEffort.Valid {
		v := reasoningEffort.String
		def.ReasoningEffort = &v
	}
	return def, nil
}

// ListAgentDefs retrieves all agent definitions for a workflow
func (s *AgentDefinitionService) ListAgentDefs(projectID, workflowID string) ([]*model.AgentDefinition, error) {
	rows, err := s.pool.Query(`
		SELECT id, project_id, workflow_id, model, timeout, prompt, restart_threshold, max_fail_restarts, stall_start_timeout_sec, stall_running_timeout_sec, tag, low_consumption_model, layer, execution_mode, tools, api_max_iterations, api_max_tokens, python_script_id, validation_commands, consultant, node_role, description, reasoning_effort, created_at, updated_at
		FROM agent_definitions
		WHERE LOWER(project_id) = LOWER(?) AND LOWER(workflow_id) = LOWER(?)
		ORDER BY layer ASC, id ASC`, projectID, workflowID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	defs := []*model.AgentDefinition{}
	for rows.Next() {
		def := &model.AgentDefinition{}
		var createdAt, updatedAt string
		var restartThreshold, maxFailRestarts, stallStartTimeout, stallRunningTimeout, apiMaxIter, apiMaxTokens sql.NullInt64
		var pythonScriptID, reasoningEffort sql.NullString

		err := rows.Scan(
			&def.ID, &def.ProjectID, &def.WorkflowID,
			&def.Model, &def.Timeout, &def.Prompt,
			&restartThreshold, &maxFailRestarts, &stallStartTimeout, &stallRunningTimeout, &def.Tag,
			&def.LowConsumptionModel, &def.Layer,
			&def.ExecutionMode, &def.Tools, &apiMaxIter, &apiMaxTokens, &pythonScriptID,
			&def.ValidationCommands, &def.Consultant, &def.NodeRole, &def.Description, &reasoningEffort,
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
		if apiMaxIter.Valid {
			v := int(apiMaxIter.Int64)
			def.APIMaxIterations = &v
		}
		if apiMaxTokens.Valid {
			v := int(apiMaxTokens.Int64)
			def.APIMaxTokens = &v
		}
		if pythonScriptID.Valid {
			s := pythonScriptID.String
			def.PythonScriptID = &s
		}
		if reasoningEffort.Valid {
			v := reasoningEffort.String
			def.ReasoningEffort = &v
		}
		defs = append(defs, def)
	}

	return defs, nil
}
