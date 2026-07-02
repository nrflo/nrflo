package service

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"be/internal/types"
)

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
	if req.PurgeOnCompletion != nil || req.CallableAsSubworkflow != nil || req.ScopeType != nil {
		if err := s.validateCallableUpdate(projectID, workflowID, req.CallableAsSubworkflow, req.PurgeOnCompletion, req.ScopeType); err != nil {
			return err
		}
	}
	if req.PurgeOnCompletion != nil {
		updates = append(updates, "purge_on_completion = ?")
		args = append(args, *req.PurgeOnCompletion)
	}
	if req.CallableAsSubworkflow != nil {
		updates = append(updates, "callable_as_subworkflow = ?")
		args = append(args, *req.CallableAsSubworkflow)
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

	if req.FindingSchemas != nil {
		if err := ValidateFindingSchemas(*req.FindingSchemas); err != nil {
			return err
		}
		schemasJSON := []byte("[]")
		if len(*req.FindingSchemas) > 0 {
			schemasJSON, _ = json.Marshal(*req.FindingSchemas)
		}
		updates = append(updates, "finding_schemas = ?")
		args = append(args, string(schemasJSON))
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
