package service

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"be/internal/model"
	"be/internal/types"
)

// --- Workflow Definition CRUD ---

// CreateWorkflowDef creates a new workflow definition in the database
func (s *WorkflowService) CreateWorkflowDef(projectID string, req *types.WorkflowDefCreateRequest) (*model.Workflow, error) {
	if req.ID == "" {
		return nil, fmt.Errorf("workflow id is required")
	}

	// Validate scope_type
	scopeType := req.ScopeType
	if scopeType == "" {
		scopeType = "ticket"
	}
	if err := ValidateScopeType(scopeType); err != nil {
		return nil, err
	}

	// Validate groups
	if err := ValidateGroups(req.Groups); err != nil {
		return nil, err
	}
	groupsJSON, _ := json.Marshal(req.Groups)
	if req.Groups == nil {
		groupsJSON = []byte("[]")
	}

	closeTicketOnComplete := true
	if req.CloseTicketOnComplete != nil {
		closeTicketOnComplete = *req.CloseTicketOnComplete
	}

	now := s.clock.Now().UTC().Format(time.RFC3339Nano)
	wf := &model.Workflow{
		ID:                    strings.ToLower(req.ID),
		ProjectID:             strings.ToLower(projectID),
		Description:           req.Description,
		ScopeType:             scopeType,
		CloseTicketOnComplete: closeTicketOnComplete,
		Groups:                string(groupsJSON),
		CreatedAt:             s.clock.Now().UTC(),
		UpdatedAt:             s.clock.Now().UTC(),
	}

	_, err := s.pool.Exec(`
		INSERT INTO workflows (id, project_id, description, scope_type, groups, close_ticket_on_complete, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		wf.ID, wf.ProjectID, wf.Description, wf.ScopeType, wf.Groups, wf.CloseTicketOnComplete, now, now)
	if err != nil {
		if strings.Contains(err.Error(), "UNIQUE constraint") || strings.Contains(err.Error(), "PRIMARY KEY") {
			return nil, fmt.Errorf("workflow '%s' already exists", req.ID)
		}
		return nil, fmt.Errorf("failed to create workflow: %w", err)
	}

	return wf, nil
}

// GetWorkflowDef gets a single workflow definition from the database
func (s *WorkflowService) GetWorkflowDef(projectID, workflowID string) (*WorkflowDef, error) {
	var description, scopeType, groupsStr string
	var closeTicketOnComplete bool

	err := s.pool.QueryRow(`
		SELECT description, scope_type, groups, close_ticket_on_complete
		FROM workflows WHERE LOWER(project_id) = LOWER(?) AND LOWER(id) = LOWER(?)`,
		projectID, workflowID).Scan(&description, &scopeType, &groupsStr, &closeTicketOnComplete)
	if err == sql.ErrNoRows {
		return nil, fmt.Errorf("workflow not found: %s", workflowID)
	}
	if err != nil {
		return nil, err
	}

	// Derive phases from agent_definitions
	agentDefs, err := s.listAgentDefsForWorkflow(projectID, workflowID)
	if err != nil {
		return nil, err
	}

	wf := parseWorkflowDefFromDB(description, agentDefs)
	wf.ScopeType = scopeType
	wf.CloseTicketOnComplete = closeTicketOnComplete
	var groups []string
	if groupsStr != "" {
		json.Unmarshal([]byte(groupsStr), &groups)
	}
	if groups == nil {
		groups = []string{}
	}
	wf.Groups = groups
	return wf, nil
}

// ListWorkflowDefs loads all workflow definitions for a project from the database
func (s *WorkflowService) ListWorkflowDefs(projectID string) (map[string]WorkflowDef, error) {
	rows, err := s.pool.Query(`
		SELECT id, description, scope_type, groups, close_ticket_on_complete
		FROM workflows WHERE LOWER(project_id) = LOWER(?)
		ORDER BY id`, projectID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	// Collect workflow metadata
	type wfMeta struct {
		id, description, scopeType, groupsStr string
		closeTicketOnComplete                  bool
	}
	var metas []wfMeta
	for rows.Next() {
		var m wfMeta
		if err := rows.Scan(&m.id, &m.description, &m.scopeType, &m.groupsStr, &m.closeTicketOnComplete); err != nil {
			return nil, err
		}
		metas = append(metas, m)
	}

	// Load all agent definitions for the project at once
	allAgentDefs, err := s.listAgentDefsForProject(projectID)
	if err != nil {
		return nil, err
	}

	// Group agent definitions by workflow ID
	agentsByWorkflow := make(map[string][]*model.AgentDefinition)
	for _, ad := range allAgentDefs {
		agentsByWorkflow[ad.WorkflowID] = append(agentsByWorkflow[ad.WorkflowID], ad)
	}

	result := make(map[string]WorkflowDef)
	for _, m := range metas {
		wf := parseWorkflowDefFromDB(m.description, agentsByWorkflow[m.id])
		wf.ScopeType = m.scopeType
		wf.CloseTicketOnComplete = m.closeTicketOnComplete
		var groups []string
		if m.groupsStr != "" {
			json.Unmarshal([]byte(m.groupsStr), &groups)
		}
		if groups == nil {
			groups = []string{}
		}
		wf.Groups = groups
		result[m.id] = *wf
	}

	return result, nil
}

// UpdateWorkflowDef updates an existing workflow definition
func (s *WorkflowService) UpdateWorkflowDef(projectID, workflowID string, req *types.WorkflowDefUpdateRequest) error {
	updates := []string{}
	args := []interface{}{}

	if req.Description != nil {
		updates = append(updates, "description = ?")
		args = append(args, *req.Description)
	}
	if req.ScopeType != nil {
		if err := ValidateScopeType(*req.ScopeType); err != nil {
			return err
		}
		updates = append(updates, "scope_type = ?")
		args = append(args, *req.ScopeType)
	}
	if req.Groups != nil {
		if err := ValidateGroups(*req.Groups); err != nil {
			return err
		}
		groupsJSON, _ := json.Marshal(*req.Groups)
		updates = append(updates, "groups = ?")
		args = append(args, string(groupsJSON))
	}
	if req.CloseTicketOnComplete != nil {
		updates = append(updates, "close_ticket_on_complete = ?")
		args = append(args, *req.CloseTicketOnComplete)
	}

	if len(updates) == 0 {
		return nil
	}

	now := s.clock.Now().UTC().Format(time.RFC3339Nano)
	updates = append(updates, "updated_at = ?")
	args = append(args, now, projectID, workflowID)

	query := "UPDATE workflows SET " + strings.Join(updates, ", ") +
		" WHERE LOWER(project_id) = LOWER(?) AND LOWER(id) = LOWER(?)"

	result, err := s.pool.Exec(query, args...)
	if err != nil {
		return fmt.Errorf("failed to update workflow: %w", err)
	}

	rowsAffected, _ := result.RowsAffected()
	if rowsAffected == 0 {
		return fmt.Errorf("workflow not found: %s", workflowID)
	}
	return nil
}

// DeleteWorkflowDef deletes a workflow definition
func (s *WorkflowService) DeleteWorkflowDef(projectID, workflowID string) error {
	result, err := s.pool.Exec(
		"DELETE FROM workflows WHERE LOWER(project_id) = LOWER(?) AND LOWER(id) = LOWER(?)",
		projectID, workflowID)
	if err != nil {
		return fmt.Errorf("failed to delete workflow: %w", err)
	}

	rowsAffected, _ := result.RowsAffected()
	if rowsAffected == 0 {
		return fmt.Errorf("workflow not found: %s", workflowID)
	}
	return nil
}

// listAgentDefsForWorkflow queries agent_definitions for a specific workflow, ordered by layer ASC, id ASC
func (s *WorkflowService) listAgentDefsForWorkflow(projectID, workflowID string) ([]*model.AgentDefinition, error) {
	rows, err := s.pool.Query(`
		SELECT id, project_id, workflow_id, model, timeout, prompt, restart_threshold, max_fail_restarts,
			stall_start_timeout_sec, stall_running_timeout_sec, tag, low_consumption_model, layer, created_at, updated_at
		FROM agent_definitions
		WHERE LOWER(project_id) = LOWER(?) AND LOWER(workflow_id) = LOWER(?)
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
			stall_start_timeout_sec, stall_running_timeout_sec, tag, low_consumption_model, layer, created_at, updated_at
		FROM agent_definitions
		WHERE LOWER(project_id) = LOWER(?)
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
		var restartThreshold, maxFailRestarts, stallStartTimeout, stallRunningTimeout sql.NullInt64

		err := rows.Scan(
			&def.ID, &def.ProjectID, &def.WorkflowID,
			&def.Model, &def.Timeout, &def.Prompt,
			&restartThreshold, &maxFailRestarts, &stallStartTimeout, &stallRunningTimeout,
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
		defs = append(defs, def)
	}
	return defs, nil
}
