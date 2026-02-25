package service

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"be/internal/clock"
	"be/internal/db"
	"be/internal/model"
	"be/internal/types"
)

// AgentDefinitionService handles agent definition business logic
type AgentDefinitionService struct {
	clock clock.Clock
	pool  *db.Pool
}

// NewAgentDefinitionService creates a new agent definition service
func NewAgentDefinitionService(pool *db.Pool, clk clock.Clock) *AgentDefinitionService {
	return &AgentDefinitionService{pool: pool, clock: clk}
}

// CreateAgentDef creates a new agent definition
func (s *AgentDefinitionService) CreateAgentDef(projectID, workflowID string, req *types.AgentDefCreateRequest) (*model.AgentDefinition, error) {
	if req.ID == "" {
		return nil, fmt.Errorf("agent id is required")
	}
	if req.Prompt == "" {
		return nil, fmt.Errorf("prompt is required")
	}

	// Verify workflow exists and get groups for tag validation
	var groupsStr string
	err := s.pool.QueryRow(
		"SELECT groups FROM workflows WHERE LOWER(project_id) = LOWER(?) AND LOWER(id) = LOWER(?)",
		projectID, workflowID).Scan(&groupsStr)
	if err == sql.ErrNoRows {
		return nil, fmt.Errorf("workflow not found: %s", workflowID)
	}
	if err != nil {
		return nil, err
	}

	// Validate tag against workflow groups
	if req.Tag != "" {
		if err := validateTagInGroups(req.Tag, groupsStr); err != nil {
			return nil, err
		}
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

	now := s.clock.Now().UTC().Format(time.RFC3339Nano)
	id := strings.ToLower(req.ID)
	pid := strings.ToLower(projectID)
	wid := strings.ToLower(workflowID)

	_, err = s.pool.Exec(`
		INSERT INTO agent_definitions (id, project_id, workflow_id, model, timeout, prompt, restart_threshold, max_fail_restarts, tag, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		id, pid, wid, modelName, timeout, req.Prompt, req.RestartThreshold, req.MaxFailRestarts, req.Tag, now, now,
	)
	if err != nil {
		if strings.Contains(err.Error(), "UNIQUE constraint") || strings.Contains(err.Error(), "already exists") {
			return nil, fmt.Errorf("agent definition already exists: %s", req.ID)
		}
		return nil, err
	}

	ts, _ := time.Parse(time.RFC3339Nano, now)
	return &model.AgentDefinition{
		ID:               id,
		ProjectID:        pid,
		WorkflowID:       wid,
		Model:            modelName,
		Timeout:          timeout,
		Prompt:           req.Prompt,
		RestartThreshold: req.RestartThreshold,
		MaxFailRestarts:  req.MaxFailRestarts,
		Tag:              req.Tag,
		CreatedAt:        ts,
		UpdatedAt:        ts,
	}, nil
}

// GetAgentDef retrieves a single agent definition
func (s *AgentDefinitionService) GetAgentDef(projectID, workflowID, id string) (*model.AgentDefinition, error) {
	def := &model.AgentDefinition{}
	var createdAt, updatedAt string
	var restartThreshold, maxFailRestarts sql.NullInt64

	err := s.pool.QueryRow(`
		SELECT id, project_id, workflow_id, model, timeout, prompt, restart_threshold, max_fail_restarts, tag, created_at, updated_at
		FROM agent_definitions
		WHERE LOWER(project_id) = LOWER(?) AND LOWER(workflow_id) = LOWER(?) AND LOWER(id) = LOWER(?)`,
		projectID, workflowID, id).Scan(
		&def.ID, &def.ProjectID, &def.WorkflowID,
		&def.Model, &def.Timeout, &def.Prompt,
		&restartThreshold, &maxFailRestarts, &def.Tag,
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
	return def, nil
}

// ListAgentDefs retrieves all agent definitions for a workflow
func (s *AgentDefinitionService) ListAgentDefs(projectID, workflowID string) ([]*model.AgentDefinition, error) {
	rows, err := s.pool.Query(`
		SELECT id, project_id, workflow_id, model, timeout, prompt, restart_threshold, max_fail_restarts, tag, created_at, updated_at
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
		var restartThreshold, maxFailRestarts sql.NullInt64

		err := rows.Scan(
			&def.ID, &def.ProjectID, &def.WorkflowID,
			&def.Model, &def.Timeout, &def.Prompt,
			&restartThreshold, &maxFailRestarts, &def.Tag,
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
	if req.RestartThreshold != nil {
		updates = append(updates, "restart_threshold = ?")
		args = append(args, *req.RestartThreshold)
	}
	if req.MaxFailRestarts != nil {
		updates = append(updates, "max_fail_restarts = ?")
		args = append(args, *req.MaxFailRestarts)
	}
	if req.Tag != nil {
		if *req.Tag != "" {
			// Validate tag against workflow groups
			var groupsStr string
			err := s.pool.QueryRow(
				"SELECT groups FROM workflows WHERE LOWER(project_id) = LOWER(?) AND LOWER(id) = LOWER(?)",
				projectID, workflowID).Scan(&groupsStr)
			if err != nil {
				return fmt.Errorf("failed to load workflow for tag validation: %w", err)
			}
			if err := validateTagInGroups(*req.Tag, groupsStr); err != nil {
				return err
			}
		}
		updates = append(updates, "tag = ?")
		args = append(args, *req.Tag)
	}

	if len(updates) == 0 {
		return nil
	}

	now := s.clock.Now().UTC().Format(time.RFC3339Nano)
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

// validateTagInGroups checks that tag is present in the workflow's groups JSON string
func validateTagInGroups(tag, groupsStr string) error {
	var groups []string
	if groupsStr != "" {
		json.Unmarshal([]byte(groupsStr), &groups)
	}
	for _, g := range groups {
		if g == tag {
			return nil
		}
	}
	return fmt.Errorf("tag '%s' is not in workflow groups %v", tag, groups)
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
