package service

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"be/internal/model"
	"be/internal/types"
)

// Reserved workflow/project name helpers live in workflow_reserved.go.

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

	if err := ValidateFindingSchemas(req.FindingSchemas); err != nil {
		return nil, err
	}
	findingSchemasJSON := []byte("[]")
	if len(req.FindingSchemas) > 0 {
		findingSchemasJSON, _ = json.Marshal(req.FindingSchemas)
	}

	closeTicketOnComplete := true
	if req.CloseTicketOnComplete != nil {
		closeTicketOnComplete = *req.CloseTicketOnComplete
	}

	purgeOnCompletion := false
	if req.PurgeOnCompletion != nil {
		purgeOnCompletion = *req.PurgeOnCompletion
	}

	callableAsSubworkflow := false
	if req.CallableAsSubworkflow != nil {
		callableAsSubworkflow = *req.CallableAsSubworkflow
	}
	if err := validateCallableSubworkflow(callableAsSubworkflow, purgeOnCompletion); err != nil {
		return nil, err
	}

	now := s.clock.Now().UTC().Format(time.RFC3339Nano)
	wf := &model.Workflow{
		ID:                      strings.ToLower(req.ID),
		ProjectID:               strings.ToLower(projectID),
		Description:             req.Description,
		ScopeType:               scopeType,
		CloseTicketOnComplete:   closeTicketOnComplete,
		PurgeOnCompletion:       purgeOnCompletion,
		CallableAsSubworkflow:   callableAsSubworkflow,
		NextWorkflowOnSuccess:   req.NextWorkflowOnSuccess,
		FinalizeSuccessCommand:  req.FinalizeSuccessCommand,
		FinalizeSuccessScriptID: req.FinalizeSuccessScriptID,
		FinalizeFailureCommand:  req.FinalizeFailureCommand,
		FinalizeFailureScriptID: req.FinalizeFailureScriptID,
		PauseEventCommand:       req.PauseEventCommand,
		PauseEventScriptID:      req.PauseEventScriptID,
		Groups:                  string(groupsJSON),
		FindingSchemas:          string(findingSchemasJSON),
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
		INSERT INTO workflows (id, project_id, description, scope_type, groups, close_ticket_on_complete, purge_on_completion, callable_as_subworkflow, next_workflow_on_success, finalize_success_command, finalize_success_script_id, finalize_failure_command, finalize_failure_script_id, pause_event_command, pause_event_script_id, observer_context, observer_provider, observer_model, finding_schemas, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		wf.ID, wf.ProjectID, wf.Description, wf.ScopeType, wf.Groups, wf.CloseTicketOnComplete, wf.PurgeOnCompletion, wf.CallableAsSubworkflow, wf.NextWorkflowOnSuccess,
		wf.FinalizeSuccessCommand, wf.FinalizeSuccessScriptID, wf.FinalizeFailureCommand, wf.FinalizeFailureScriptID,
		wf.PauseEventCommand, wf.PauseEventScriptID,
		req.ObserverContext, observerProvider, observerModel, wf.FindingSchemas, now, now)
	if err != nil {
		if strings.Contains(err.Error(), "UNIQUE constraint") || strings.Contains(err.Error(), "PRIMARY KEY") {
			return nil, fmt.Errorf("workflow '%s' already exists", req.ID)
		}
		return nil, fmt.Errorf("failed to create workflow: %w", err)
	}

	return wf, nil
}
