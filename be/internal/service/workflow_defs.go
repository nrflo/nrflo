package service

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"be/internal/model"
	"be/internal/types"
)

// IsReservedWorkflowName returns true for internal system workflow names like __spec_import__.
// Reserved workflows are excluded from the workflow definition listing.
func IsReservedWorkflowName(name string) bool {
	return strings.HasPrefix(name, "__") && strings.HasSuffix(name, "__")
}

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

	if req.NextWorkflowOnSuccess != "" {
		if err := s.validateNextWorkflowOnSuccess(projectID, req.ID, req.NextWorkflowOnSuccess); err != nil {
			return nil, err
		}
	}

	if err := s.validateFinalizeSlots(projectID,
		req.FinalizeSuccessCommand, req.FinalizeSuccessScriptID,
		req.FinalizeFailureCommand, req.FinalizeFailureScriptID,
	); err != nil {
		return nil, err
	}

	if err := s.validatePauseSlot(projectID, req.PauseEventCommand, req.PauseEventScriptID); err != nil {
		return nil, err
	}

	closeTicketOnComplete := true
	if req.CloseTicketOnComplete != nil {
		closeTicketOnComplete = *req.CloseTicketOnComplete
	}

	now := s.clock.Now().UTC().Format(time.RFC3339Nano)
	wf := &model.Workflow{
		ID:                      strings.ToLower(req.ID),
		ProjectID:               strings.ToLower(projectID),
		Description:             req.Description,
		ScopeType:               scopeType,
		CloseTicketOnComplete:   closeTicketOnComplete,
		NextWorkflowOnSuccess:   req.NextWorkflowOnSuccess,
		FinalizeSuccessCommand:  req.FinalizeSuccessCommand,
		FinalizeSuccessScriptID: req.FinalizeSuccessScriptID,
		FinalizeFailureCommand:  req.FinalizeFailureCommand,
		FinalizeFailureScriptID: req.FinalizeFailureScriptID,
		PauseEventCommand:       req.PauseEventCommand,
		PauseEventScriptID:      req.PauseEventScriptID,
		Groups:                  string(groupsJSON),
		CreatedAt:               s.clock.Now().UTC(),
		UpdatedAt:               s.clock.Now().UTC(),
	}

	var observerProvider, observerModel interface{}
	if req.ObserverProvider != nil {
		observerProvider = *req.ObserverProvider
	}
	if req.ObserverModel != nil {
		observerModel = *req.ObserverModel
	}

	_, err := s.pool.Exec(`
		INSERT INTO workflows (id, project_id, description, scope_type, groups, close_ticket_on_complete, next_workflow_on_success, finalize_success_command, finalize_success_script_id, finalize_failure_command, finalize_failure_script_id, pause_event_command, pause_event_script_id, observer_context, observer_provider, observer_model, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		wf.ID, wf.ProjectID, wf.Description, wf.ScopeType, wf.Groups, wf.CloseTicketOnComplete, wf.NextWorkflowOnSuccess,
		wf.FinalizeSuccessCommand, wf.FinalizeSuccessScriptID, wf.FinalizeFailureCommand, wf.FinalizeFailureScriptID,
		wf.PauseEventCommand, wf.PauseEventScriptID,
		req.ObserverContext, observerProvider, observerModel, now, now)
	if err != nil {
		if strings.Contains(err.Error(), "UNIQUE constraint") || strings.Contains(err.Error(), "PRIMARY KEY") {
			return nil, fmt.Errorf("workflow '%s' already exists", req.ID)
		}
		return nil, fmt.Errorf("failed to create workflow: %w", err)
	}

	return wf, nil
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
	if req.NextWorkflowOnSuccess != nil {
		if *req.NextWorkflowOnSuccess != "" {
			if err := s.validateNextWorkflowOnSuccess(projectID, workflowID, *req.NextWorkflowOnSuccess); err != nil {
				return err
			}
		}
		updates = append(updates, "next_workflow_on_success = ?")
		args = append(args, *req.NextWorkflowOnSuccess)
	}

	// Validate finalize slots that are being updated
	successCmd := ""
	successScript := ""
	failureCmd := ""
	failureScript := ""
	if req.FinalizeSuccessCommand != nil {
		successCmd = *req.FinalizeSuccessCommand
	}
	if req.FinalizeSuccessScriptID != nil {
		successScript = *req.FinalizeSuccessScriptID
	}
	if req.FinalizeFailureCommand != nil {
		failureCmd = *req.FinalizeFailureCommand
	}
	if req.FinalizeFailureScriptID != nil {
		failureScript = *req.FinalizeFailureScriptID
	}
	if req.FinalizeSuccessCommand != nil || req.FinalizeSuccessScriptID != nil ||
		req.FinalizeFailureCommand != nil || req.FinalizeFailureScriptID != nil {
		if err := s.validateFinalizeSlots(projectID, successCmd, successScript, failureCmd, failureScript); err != nil {
			return err
		}
	}

	if req.FinalizeSuccessCommand != nil {
		updates = append(updates, "finalize_success_command = ?")
		args = append(args, *req.FinalizeSuccessCommand)
	}
	if req.FinalizeSuccessScriptID != nil {
		updates = append(updates, "finalize_success_script_id = ?")
		args = append(args, *req.FinalizeSuccessScriptID)
	}
	if req.FinalizeFailureCommand != nil {
		updates = append(updates, "finalize_failure_command = ?")
		args = append(args, *req.FinalizeFailureCommand)
	}
	if req.FinalizeFailureScriptID != nil {
		updates = append(updates, "finalize_failure_script_id = ?")
		args = append(args, *req.FinalizeFailureScriptID)
	}

	if req.PauseEventCommand != nil || req.PauseEventScriptID != nil {
		pauseCmd := ""
		pauseScript := ""
		if req.PauseEventCommand != nil {
			pauseCmd = *req.PauseEventCommand
		}
		if req.PauseEventScriptID != nil {
			pauseScript = *req.PauseEventScriptID
		}
		if err := s.validatePauseSlot(projectID, pauseCmd, pauseScript); err != nil {
			return err
		}
	}
	if req.PauseEventCommand != nil {
		updates = append(updates, "pause_event_command = ?")
		args = append(args, *req.PauseEventCommand)
	}
	if req.PauseEventScriptID != nil {
		updates = append(updates, "pause_event_script_id = ?")
		args = append(args, *req.PauseEventScriptID)
	}

	if req.ObserverContext != nil {
		updates = append(updates, "observer_context = ?")
		args = append(args, *req.ObserverContext)
	}
	if req.ObserverProvider != nil {
		updates = append(updates, "observer_provider = ?")
		args = append(args, *req.ObserverProvider)
	}
	if req.ObserverModel != nil {
		updates = append(updates, "observer_model = ?")
		args = append(args, *req.ObserverModel)
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
