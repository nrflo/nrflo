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
	db db.Querier
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
	_, err := r.db.Exec(`
		INSERT INTO agent_definitions (id, project_id, workflow_id, model, timeout, prompt, restart_threshold, max_fail_restarts, stall_start_timeout_sec, stall_running_timeout_sec, tag, low_consumption_model, layer, execution_mode, tools, api_max_iterations, python_script_id, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
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
		def.PythonScriptID,
		now,
		now,
	)
	return err
}

// Get retrieves an agent definition by project, workflow, and ID
func (r *AgentDefinitionRepo) Get(projectID, workflowID, id string) (*model.AgentDefinition, error) {
	def := &model.AgentDefinition{}
	var createdAt, updatedAt string

	var restartThreshold, maxFailRestarts, stallStartTimeout, stallRunningTimeout, apiMaxIter sql.NullInt64
	var pythonScriptID sql.NullString
	err := r.db.QueryRow(`
		SELECT id, project_id, workflow_id, model, timeout, prompt, restart_threshold, max_fail_restarts, stall_start_timeout_sec, stall_running_timeout_sec, tag, low_consumption_model, layer, execution_mode, tools, api_max_iterations, python_script_id, created_at, updated_at
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
		&pythonScriptID,
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
	if pythonScriptID.Valid {
		s := pythonScriptID.String
		def.PythonScriptID = &s
	}

	return def, nil
}

// List retrieves all agent definitions for a workflow
func (r *AgentDefinitionRepo) List(projectID, workflowID string) ([]*model.AgentDefinition, error) {
	rows, err := r.db.Query(`
		SELECT id, project_id, workflow_id, model, timeout, prompt, restart_threshold, max_fail_restarts, stall_start_timeout_sec, stall_running_timeout_sec, tag, low_consumption_model, layer, execution_mode, tools, api_max_iterations, python_script_id, created_at, updated_at
		FROM agent_definitions
		WHERE LOWER(project_id) = LOWER(?) AND LOWER(workflow_id) = LOWER(?)
		ORDER BY layer ASC, id ASC`, projectID, workflowID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var defs []*model.AgentDefinition
	for rows.Next() {
		def := &model.AgentDefinition{}
		var createdAt, updatedAt string
		var restartThreshold, maxFailRestarts, stallStartTimeout, stallRunningTimeout, apiMaxIter sql.NullInt64
		var pythonScriptID sql.NullString

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
			&pythonScriptID,
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
		if pythonScriptID.Valid {
			s := pythonScriptID.String
			def.PythonScriptID = &s
		}

		defs = append(defs, def)
	}

	return defs, nil
}

// AgentDefUpdateFields contains fields that can be updated
type AgentDefUpdateFields struct {
	Model            *string
	Timeout          *int
	Prompt           *string
	Layer            *int
	RestartThreshold *int
	MaxFailRestarts        *int
	StallStartTimeoutSec   *int
	StallRunningTimeoutSec *int
	Tag                    *string
	LowConsumptionModel    *string
	ExecutionMode          *string
	Tools                  *string
	APIMaxIterations       *int
	PythonScriptID         *string
}

// Update updates an agent definition
func (r *AgentDefinitionRepo) Update(projectID, workflowID, id string, fields *AgentDefUpdateFields) error {
	updates := []string{}
	args := []interface{}{}

	if fields.Model != nil {
		updates = append(updates, "model = ?")
		args = append(args, *fields.Model)
	}
	if fields.Timeout != nil {
		updates = append(updates, "timeout = ?")
		args = append(args, *fields.Timeout)
	}
	if fields.Prompt != nil {
		updates = append(updates, "prompt = ?")
		args = append(args, *fields.Prompt)
	}
	if fields.Layer != nil {
		updates = append(updates, "layer = ?")
		args = append(args, *fields.Layer)
	}
	if fields.RestartThreshold != nil {
		updates = append(updates, "restart_threshold = ?")
		args = append(args, *fields.RestartThreshold)
	}
	if fields.MaxFailRestarts != nil {
		updates = append(updates, "max_fail_restarts = ?")
		args = append(args, *fields.MaxFailRestarts)
	}
	if fields.StallStartTimeoutSec != nil {
		updates = append(updates, "stall_start_timeout_sec = ?")
		args = append(args, *fields.StallStartTimeoutSec)
	}
	if fields.StallRunningTimeoutSec != nil {
		updates = append(updates, "stall_running_timeout_sec = ?")
		args = append(args, *fields.StallRunningTimeoutSec)
	}
	if fields.Tag != nil {
		updates = append(updates, "tag = ?")
		args = append(args, *fields.Tag)
	}
	if fields.LowConsumptionModel != nil {
		updates = append(updates, "low_consumption_model = ?")
		args = append(args, *fields.LowConsumptionModel)
	}
	if fields.ExecutionMode != nil {
		updates = append(updates, "execution_mode = ?")
		args = append(args, *fields.ExecutionMode)
	}
	if fields.Tools != nil {
		updates = append(updates, "tools = ?")
		args = append(args, *fields.Tools)
	}
	if fields.APIMaxIterations != nil {
		updates = append(updates, "api_max_iterations = ?")
		args = append(args, *fields.APIMaxIterations)
	}
	if fields.PythonScriptID != nil {
		updates = append(updates, "python_script_id = ?")
		args = append(args, *fields.PythonScriptID)
	}

	if len(updates) == 0 {
		return nil
	}

	now := r.clock.Now().UTC().Format(time.RFC3339Nano)
	updates = append(updates, "updated_at = ?")
	args = append(args, now)
	args = append(args, projectID, workflowID, id)

	query := "UPDATE agent_definitions SET " + strings.Join(updates, ", ") +
		" WHERE LOWER(project_id) = LOWER(?) AND LOWER(workflow_id) = LOWER(?) AND LOWER(id) = LOWER(?)"

	result, err := r.db.Exec(query, args...)
	if err != nil {
		return err
	}

	rowsAffected, _ := result.RowsAffected()
	if rowsAffected == 0 {
		return fmt.Errorf("agent definition not found: %s/%s/%s", projectID, workflowID, id)
	}
	return nil
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

// AllToolsCSVs returns the tools CSV string for every row in agent_definitions.
// system_agent_definitions.tools is intentionally excluded in v1.
func (r *AgentDefinitionRepo) AllToolsCSVs() ([]string, error) {
	rows, err := r.db.Query("SELECT tools FROM agent_definitions")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var tools string
		if err := rows.Scan(&tools); err != nil {
			return nil, err
		}
		out = append(out, tools)
	}
	return out, nil
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
