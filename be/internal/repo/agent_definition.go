package repo

import (
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
		INSERT INTO agent_definitions (`+agentDefColumns+`)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
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
		def.NativeTools,
		def.Sandbox,
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
	rows, err := r.db.Query(`
		SELECT `+agentDefColumns+`
		FROM agent_definitions
		WHERE LOWER(project_id) = LOWER(?) AND LOWER(workflow_id) = LOWER(?) AND LOWER(id) = LOWER(?)`,
		projectID, workflowID, id)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	defs, err := scanAgentDefRows(rows)
	if err != nil {
		return nil, err
	}
	if len(defs) == 0 {
		return nil, fmt.Errorf("agent definition not found: %s/%s/%s", projectID, workflowID, id)
	}
	return defs[0], nil
}

// List retrieves all agent definitions for a workflow
func (r *AgentDefinitionRepo) List(projectID, workflowID string) ([]*model.AgentDefinition, error) {
	rows, err := r.db.Query(`
		SELECT `+agentDefColumns+`
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
		SELECT `+agentDefColumns+`
		FROM agent_definitions
		WHERE LOWER(project_id) = LOWER(?) AND LOWER(workflow_id) = LOWER(?) AND consultant = 0 AND node_role = 'static'
		ORDER BY layer ASC, id ASC`, projectID, workflowID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	return scanAgentDefRows(rows)
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
