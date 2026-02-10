package service

import (
	"database/sql"
	"fmt"
	"strings"
	"time"

	"nrworkflow/internal/db"
	"nrworkflow/internal/model"
	"nrworkflow/internal/types"
)

// AgentDefinitionService handles agent definition business logic
type AgentDefinitionService struct {
	pool *db.Pool
}

// NewAgentDefinitionService creates a new agent definition service
func NewAgentDefinitionService(pool *db.Pool) *AgentDefinitionService {
	return &AgentDefinitionService{pool: pool}
}

// CreateAgentDef creates a new agent definition
func (s *AgentDefinitionService) CreateAgentDef(projectID, workflowID string, req *types.AgentDefCreateRequest) (*model.AgentDefinition, error) {
	if req.ID == "" {
		return nil, fmt.Errorf("agent id is required")
	}
	if req.Prompt == "" {
		return nil, fmt.Errorf("prompt is required")
	}

	// Verify workflow exists
	var count int
	err := s.pool.QueryRow(
		"SELECT COUNT(*) FROM workflows WHERE LOWER(project_id) = LOWER(?) AND LOWER(id) = LOWER(?)",
		projectID, workflowID).Scan(&count)
	if err != nil {
		return nil, err
	}
	if count == 0 {
		return nil, fmt.Errorf("workflow not found: %s", workflowID)
	}

	// Defaults
	modelName := req.Model
	if modelName == "" {
		modelName = "sonnet"
	}
	timeout := req.Timeout
	if timeout == 0 {
		timeout = 20
	}

	now := time.Now().UTC().Format(time.RFC3339)
	id := strings.ToLower(req.ID)
	pid := strings.ToLower(projectID)
	wid := strings.ToLower(workflowID)

	_, err = s.pool.Exec(`
		INSERT INTO agent_definitions (id, project_id, workflow_id, model, timeout, prompt, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		id, pid, wid, modelName, timeout, req.Prompt, now, now,
	)
	if err != nil {
		if strings.Contains(err.Error(), "UNIQUE constraint") || strings.Contains(err.Error(), "already exists") {
			return nil, fmt.Errorf("agent definition already exists: %s", req.ID)
		}
		return nil, err
	}

	ts, _ := time.Parse(time.RFC3339, now)
	return &model.AgentDefinition{
		ID:         id,
		ProjectID:  pid,
		WorkflowID: wid,
		Model:      modelName,
		Timeout:    timeout,
		Prompt:     req.Prompt,
		CreatedAt:  ts,
		UpdatedAt:  ts,
	}, nil
}

// GetAgentDef retrieves a single agent definition
func (s *AgentDefinitionService) GetAgentDef(projectID, workflowID, id string) (*model.AgentDefinition, error) {
	def := &model.AgentDefinition{}
	var createdAt, updatedAt string

	err := s.pool.QueryRow(`
		SELECT id, project_id, workflow_id, model, timeout, prompt, created_at, updated_at
		FROM agent_definitions
		WHERE LOWER(project_id) = LOWER(?) AND LOWER(workflow_id) = LOWER(?) AND LOWER(id) = LOWER(?)`,
		projectID, workflowID, id).Scan(
		&def.ID, &def.ProjectID, &def.WorkflowID,
		&def.Model, &def.Timeout, &def.Prompt,
		&createdAt, &updatedAt,
	)
	if err == sql.ErrNoRows {
		return nil, fmt.Errorf("agent definition not found: %s", id)
	}
	if err != nil {
		return nil, err
	}

	def.CreatedAt, _ = time.Parse(time.RFC3339, createdAt)
	def.UpdatedAt, _ = time.Parse(time.RFC3339, updatedAt)
	return def, nil
}

// ListAgentDefs retrieves all agent definitions for a workflow
func (s *AgentDefinitionService) ListAgentDefs(projectID, workflowID string) ([]*model.AgentDefinition, error) {
	rows, err := s.pool.Query(`
		SELECT id, project_id, workflow_id, model, timeout, prompt, created_at, updated_at
		FROM agent_definitions
		WHERE LOWER(project_id) = LOWER(?) AND LOWER(workflow_id) = LOWER(?)
		ORDER BY id`, projectID, workflowID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	defs := []*model.AgentDefinition{}
	for rows.Next() {
		def := &model.AgentDefinition{}
		var createdAt, updatedAt string

		err := rows.Scan(
			&def.ID, &def.ProjectID, &def.WorkflowID,
			&def.Model, &def.Timeout, &def.Prompt,
			&createdAt, &updatedAt,
		)
		if err != nil {
			return nil, err
		}

		def.CreatedAt, _ = time.Parse(time.RFC3339, createdAt)
		def.UpdatedAt, _ = time.Parse(time.RFC3339, updatedAt)
		defs = append(defs, def)
	}

	return defs, nil
}

// UpdateAgentDef updates an agent definition
func (s *AgentDefinitionService) UpdateAgentDef(projectID, workflowID, id string, req *types.AgentDefUpdateRequest) error {
	updates := []string{}
	args := []interface{}{}

	if req.Model != nil {
		updates = append(updates, "model = ?")
		args = append(args, *req.Model)
	}
	if req.Timeout != nil {
		updates = append(updates, "timeout = ?")
		args = append(args, *req.Timeout)
	}
	if req.Prompt != nil {
		updates = append(updates, "prompt = ?")
		args = append(args, *req.Prompt)
	}

	if len(updates) == 0 {
		return nil
	}

	now := time.Now().UTC().Format(time.RFC3339)
	updates = append(updates, "updated_at = ?")
	args = append(args, now)
	args = append(args, projectID, workflowID, id)

	query := "UPDATE agent_definitions SET " + strings.Join(updates, ", ") +
		" WHERE LOWER(project_id) = LOWER(?) AND LOWER(workflow_id) = LOWER(?) AND LOWER(id) = LOWER(?)"

	result, err := s.pool.Exec(query, args...)
	if err != nil {
		return err
	}

	rowsAffected, _ := result.RowsAffected()
	if rowsAffected == 0 {
		return fmt.Errorf("agent definition not found: %s", id)
	}
	return nil
}

// DeleteAgentDef deletes an agent definition
func (s *AgentDefinitionService) DeleteAgentDef(projectID, workflowID, id string) error {
	result, err := s.pool.Exec(
		"DELETE FROM agent_definitions WHERE LOWER(project_id) = LOWER(?) AND LOWER(workflow_id) = LOWER(?) AND LOWER(id) = LOWER(?)",
		projectID, workflowID, id)
	if err != nil {
		return err
	}

	rowsAffected, _ := result.RowsAffected()
	if rowsAffected == 0 {
		return fmt.Errorf("agent definition not found: %s", id)
	}
	return nil
}
