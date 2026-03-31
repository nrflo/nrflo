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
	if len(req.Phases) == 0 {
		return nil, fmt.Errorf("phases are required")
	}

	// Validate scope_type
	scopeType := req.ScopeType
	if scopeType == "" {
		scopeType = "ticket"
	}
	if err := ValidateScopeType(scopeType); err != nil {
		return nil, err
	}

	// Validate and normalize phases
	normalizedPhases, err := normalizePhasesJSON(req.Phases)
	if err != nil {
		return nil, fmt.Errorf("invalid phases: %w", err)
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
		Phases:                string(normalizedPhases),
		Groups:                string(groupsJSON),
		CreatedAt:             s.clock.Now().UTC(),
		UpdatedAt:             s.clock.Now().UTC(),
	}

	_, err = s.pool.Exec(`
		INSERT INTO workflows (id, project_id, description, scope_type, phases, groups, close_ticket_on_complete, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		wf.ID, wf.ProjectID, wf.Description, wf.ScopeType, wf.Phases, wf.Groups, wf.CloseTicketOnComplete, now, now)
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
	var phasesStr string
	var closeTicketOnComplete bool

	err := s.pool.QueryRow(`
		SELECT description, scope_type, phases, groups, close_ticket_on_complete
		FROM workflows WHERE LOWER(project_id) = LOWER(?) AND LOWER(id) = LOWER(?)`,
		projectID, workflowID).Scan(&description, &scopeType, &phasesStr, &groupsStr, &closeTicketOnComplete)
	if err == sql.ErrNoRows {
		return nil, fmt.Errorf("workflow not found: %s", workflowID)
	}
	if err != nil {
		return nil, err
	}

	wf, err := parseWorkflowDefFromDB(description, phasesStr)
	if err != nil {
		return nil, err
	}
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
		SELECT id, description, scope_type, phases, groups, close_ticket_on_complete
		FROM workflows WHERE LOWER(project_id) = LOWER(?)
		ORDER BY id`, projectID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	result := make(map[string]WorkflowDef)
	for rows.Next() {
		var id, description, scopeType, phasesStr, groupsStr string
		var closeTicketOnComplete bool

		if err := rows.Scan(&id, &description, &scopeType, &phasesStr, &groupsStr, &closeTicketOnComplete); err != nil {
			return nil, err
		}

		wf, err := parseWorkflowDefFromDB(description, phasesStr)
		if err != nil {
			return nil, fmt.Errorf("workflow '%s': %w", id, err)
		}
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
		result[id] = *wf
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
	if req.Phases != nil {
		normalizedPhases, err := normalizePhasesJSON(*req.Phases)
		if err != nil {
			return fmt.Errorf("invalid phases: %w", err)
		}
		updates = append(updates, "phases = ?")
		args = append(args, string(normalizedPhases))
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
