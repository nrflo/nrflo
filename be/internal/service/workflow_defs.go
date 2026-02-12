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

	// Validate and normalize phases
	normalizedPhases, err := normalizePhasesJSON(req.Phases)
	if err != nil {
		return nil, fmt.Errorf("invalid phases: %w", err)
	}

	// Serialize categories
	var categoriesStr sql.NullString
	if len(req.Categories) > 0 {
		catJSON, _ := json.Marshal(req.Categories)
		categoriesStr = sql.NullString{String: string(catJSON), Valid: true}
	}

	now := time.Now().UTC().Format(time.RFC3339)
	wf := &model.Workflow{
		ID:          strings.ToLower(req.ID),
		ProjectID:   strings.ToLower(projectID),
		Description: req.Description,
		Categories:  categoriesStr,
		Phases:      string(normalizedPhases),
		CreatedAt:   time.Now().UTC(),
		UpdatedAt:   time.Now().UTC(),
	}

	_, err = s.pool.Exec(`
		INSERT INTO workflows (id, project_id, description, categories, phases, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?)`,
		wf.ID, wf.ProjectID, wf.Description, wf.Categories, wf.Phases, now, now)
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
	var description string
	var categoriesStr sql.NullString
	var phasesStr string

	err := s.pool.QueryRow(`
		SELECT description, categories, phases
		FROM workflows WHERE LOWER(project_id) = LOWER(?) AND LOWER(id) = LOWER(?)`,
		projectID, workflowID).Scan(&description, &categoriesStr, &phasesStr)
	if err == sql.ErrNoRows {
		return nil, fmt.Errorf("workflow not found: %s", workflowID)
	}
	if err != nil {
		return nil, err
	}

	return parseWorkflowDefFromDB(description, categoriesStr, phasesStr)
}

// ListWorkflowDefs loads all workflow definitions for a project from the database
func (s *WorkflowService) ListWorkflowDefs(projectID string) (map[string]WorkflowDef, error) {
	rows, err := s.pool.Query(`
		SELECT id, description, categories, phases
		FROM workflows WHERE LOWER(project_id) = LOWER(?)
		ORDER BY id`, projectID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	result := make(map[string]WorkflowDef)
	for rows.Next() {
		var id, description, phasesStr string
		var categoriesStr sql.NullString

		if err := rows.Scan(&id, &description, &categoriesStr, &phasesStr); err != nil {
			return nil, err
		}

		wf, err := parseWorkflowDefFromDB(description, categoriesStr, phasesStr)
		if err != nil {
			return nil, fmt.Errorf("workflow '%s': %w", id, err)
		}
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
	if req.Categories != nil {
		catJSON, _ := json.Marshal(*req.Categories)
		updates = append(updates, "categories = ?")
		args = append(args, string(catJSON))
	}
	if req.Phases != nil {
		normalizedPhases, err := normalizePhasesJSON(*req.Phases)
		if err != nil {
			return fmt.Errorf("invalid phases: %w", err)
		}
		updates = append(updates, "phases = ?")
		args = append(args, string(normalizedPhases))
	}

	if len(updates) == 0 {
		return nil
	}

	now := time.Now().UTC().Format(time.RFC3339)
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
