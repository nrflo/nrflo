package repo

import (
	"database/sql"
	"fmt"
	"strings"
	"time"

	"be/internal/clock"
	"be/internal/db"
	"be/internal/model"
)

// AgentDefinitionRepo handles agent definition CRUD operations
type AgentDefinitionRepo struct {
	clock clock.Clock
	db    db.Querier
}

// NewAgentDefinitionRepo creates a new agent definition repository
func NewAgentDefinitionRepo(database db.Querier, clk clock.Clock) *AgentDefinitionRepo {
	return &AgentDefinitionRepo{db: database, clock: clk}
}

// Create creates a new agent definition
func (r *AgentDefinitionRepo) Create(def *model.AgentDefinition) error {
	now := r.clock.Now().UTC().Format(time.RFC3339Nano)
	def.CreatedAt, _ = time.Parse(time.RFC3339Nano, now)
	def.UpdatedAt = def.CreatedAt

	executionMode := def.ExecutionMode
	if executionMode == "" {
		executionMode = "cli_interactive"
	}
	nodeRole := def.NodeRole
	if nodeRole == "" {
		nodeRole = "static"
	}
	_, err := r.db.Exec(`
		INSERT INTO agent_definitions (id, project_id, workflow_id, model, timeout, prompt, restart_threshold, max_fail_restarts, stall_start_timeout_sec, stall_running_timeout_sec, tag, low_consumption_model, layer, execution_mode, tools, api_max_iterations, api_max_tokens, python_script_id, validation_commands, consultant, node_role, description, reasoning_effort, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		strings.ToLower(def.ID),
		strings.ToLower(def.ProjectID),
		strings.ToLower(def.WorkflowID),
		def.Model,
		def.Timeout,
		def.Prompt,
		def.RestartThreshold,
		def.MaxFailRestarts,
		def.StallStartTimeoutSec,
		def.StallRunningTimeoutSec,
		def.Tag,
		def.LowConsumptionModel,
		def.Layer,
		executionMode,
		def.Tools,
		def.APIMaxIterations,
		def.APIMaxTokens,
		def.PythonScriptID,
		def.ValidationCommands,
		def.Consultant,
		nodeRole,
		def.Description,
		def.ReasoningEffort,
		now,
		now,
	)
	return err
}

// Get retrieves an agent definition by project, workflow, and ID
func (r *AgentDefinitionRepo) Get(projectID, workflowID, id string) (*model.AgentDefinition, error) {
	def := &model.AgentDefinition{}
	var createdAt, updatedAt string

	var restartThreshold, maxFailRestarts, stallStartTimeout, stallRunningTimeout, apiMaxIter, apiMaxTokens sql.NullInt64
	var pythonScriptID, reasoningEffort sql.NullString
	err := r.db.QueryRow(`
		SELECT id, project_id, workflow_id, model, timeout, prompt, restart_threshold, max_fail_restarts, stall_start_timeout_sec, stall_running_timeout_sec, tag, low_consumption_model, layer, execution_mode, tools, api_max_iterations, api_max_tokens, python_script_id, validation_commands, consultant, node_role, description, reasoning_effort, created_at, updated_at
		FROM agent_definitions
		WHERE LOWER(project_id) = LOWER(?) AND LOWER(workflow_id) = LOWER(?) AND LOWER(id) = LOWER(?)`,
		projectID, workflowID, id).Scan(
		&def.ID,
		&def.ProjectID,
		&def.WorkflowID,
		&def.Model,
		&def.Timeout,
		&def.Prompt,
		&restartThreshold,
		&maxFailRestarts,
		&stallStartTimeout,
		&stallRunningTimeout,
		&def.Tag,
		&def.LowConsumptionModel,
		&def.Layer,
		&def.ExecutionMode,
		&def.Tools,
		&apiMaxIter,
		&apiMaxTokens,
		&pythonScriptID,
		&def.ValidationCommands,
		&def.Consultant,
		&def.NodeRole,
		&def.Description,
		&reasoningEffort,
		&createdAt,
		&updatedAt,
	)
	if err == sql.ErrNoRows {
		return nil, fmt.Errorf("agent definition not found: %s/%s/%s", projectID, workflowID, id)
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

// List retrieves all agent definitions for a workflow
func (r *AgentDefinitionRepo) List(projectID, workflowID string) ([]*model.AgentDefinition, error) {
	rows, err := r.db.Query(`
		SELECT id, project_id, workflow_id, model, timeout, prompt, restart_threshold, max_fail_restarts, stall_start_timeout_sec, stall_running_timeout_sec, tag, low_consumption_model, layer, execution_mode, tools, api_max_iterations, api_max_tokens, python_script_id, validation_commands, consultant, node_role, description, reasoning_effort, created_at, updated_at
		FROM agent_definitions
		WHERE LOWER(project_id) = LOWER(?) AND LOWER(workflow_id) = LOWER(?)
		ORDER BY layer ASC, id ASC`, projectID, workflowID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	return scanAgentDefRows(rows)
}

// ListExecutable retrieves non-consultant agent definitions for a workflow.
// Use this for execution graph construction; consultants must never become workflow phases.
func (r *AgentDefinitionRepo) ListExecutable(projectID, workflowID string) ([]*model.AgentDefinition, error) {
	rows, err := r.db.Query(`
		SELECT id, project_id, workflow_id, model, timeout, prompt, restart_threshold, max_fail_restarts, stall_start_timeout_sec, stall_running_timeout_sec, tag, low_consumption_model, layer, execution_mode, tools, api_max_iterations, api_max_tokens, python_script_id, validation_commands, consultant, node_role, description, reasoning_effort, created_at, updated_at
		FROM agent_definitions
		WHERE LOWER(project_id) = LOWER(?) AND LOWER(workflow_id) = LOWER(?) AND consultant = 0 AND node_role = 'static'
		ORDER BY layer ASC, id ASC`, projectID, workflowID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	return scanAgentDefRows(rows)
}

func scanAgentDefRows(rows interface {
	Next() bool
	Scan(...interface{}) error
	Close() error
}) ([]*model.AgentDefinition, error) {

	var defs []*model.AgentDefinition
	for rows.Next() {
		def := &model.AgentDefinition{}
		var createdAt, updatedAt string
		var restartThreshold, maxFailRestarts, stallStartTimeout, stallRunningTimeout, apiMaxIter, apiMaxTokens sql.NullInt64
		var pythonScriptID, reasoningEffort sql.NullString

		err := rows.Scan(
			&def.ID,
			&def.ProjectID,
			&def.WorkflowID,
			&def.Model,
			&def.Timeout,
			&def.Prompt,
			&restartThreshold,
			&maxFailRestarts,
			&stallStartTimeout,
			&stallRunningTimeout,
			&def.Tag,
			&def.LowConsumptionModel,
			&def.Layer,
			&def.ExecutionMode,
			&def.Tools,
			&apiMaxIter,
			&apiMaxTokens,
			&pythonScriptID,
			&def.ValidationCommands,
			&def.Consultant,
			&def.NodeRole,
			&def.Description,
			&reasoningEffort,
			&createdAt,
			&updatedAt,
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

// Delete deletes an agent definition
func (r *AgentDefinitionRepo) Delete(projectID, workflowID, id string) error {
	result, err := r.db.Exec(
		"DELETE FROM agent_definitions WHERE LOWER(project_id) = LOWER(?) AND LOWER(workflow_id) = LOWER(?) AND LOWER(id) = LOWER(?)",
		projectID, workflowID, id)
	if err != nil {
		return err
	}

	rowsAffected, _ := result.RowsAffected()
	if rowsAffected == 0 {
		return fmt.Errorf("agent definition not found: %s/%s/%s", projectID, workflowID, id)
	}
	return nil
}

// Exists checks if an agent definition exists
func (r *AgentDefinitionRepo) Exists(projectID, workflowID, id string) (bool, error) {
	var count int
	err := r.db.QueryRow(
		"SELECT COUNT(*) FROM agent_definitions WHERE LOWER(project_id) = LOWER(?) AND LOWER(workflow_id) = LOWER(?) AND LOWER(id) = LOWER(?)",
		projectID, workflowID, id).Scan(&count)
	if err != nil {
		return false, err
	}
	return count > 0, nil
}
