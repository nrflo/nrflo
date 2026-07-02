package service

import (
	"database/sql"
	"fmt"
	"strings"

	"be/internal/repo"
)

// validateNextWorkflowOnSuccess validates that the target workflow is valid for use as next_workflow_on_success.
// It rejects self-references, non-existent targets, and non-project-scoped targets.
func (s *WorkflowService) validateNextWorkflowOnSuccess(projectID, sourceWorkflowID, target string) error {
	if strings.EqualFold(target, sourceWorkflowID) {
		return fmt.Errorf("next_workflow_on_success cannot reference itself")
	}
	var scopeType string
	err := s.pool.QueryRow(`
		SELECT scope_type FROM workflows
		WHERE LOWER(project_id) = LOWER(?) AND LOWER(id) = LOWER(?)`,
		projectID, target).Scan(&scopeType)
	if err == sql.ErrNoRows {
		return fmt.Errorf("next_workflow_on_success target %q does not exist in project", target)
	}
	if err != nil {
		return err
	}
	if scopeType != "project" {
		return fmt.Errorf("next_workflow_on_success target %q is not project-scoped", target)
	}
	return nil
}

// validateCallableSubworkflow rejects the callable_as_subworkflow + purge_on_completion
// combination: purge deletes the child's session findings before the sub-workflow
// caller can read its result back.
func validateCallableSubworkflow(callable, purge bool) error {
	if callable && purge {
		return fmt.Errorf("callable_as_subworkflow requires purge_on_completion=false: purge deletes the result finding before the caller can read it")
	}
	return nil
}

// validateCallableUpdate validates the effective callable/purge pair for an update,
// resolving unspecified sides against the current row.
func (s *WorkflowService) validateCallableUpdate(projectID, workflowID string, callable, purge *bool) error {
	var curCallable, curPurge bool
	err := s.pool.QueryRow(`
		SELECT callable_as_subworkflow, purge_on_completion FROM workflows
		WHERE LOWER(project_id) = LOWER(?) AND LOWER(id) = LOWER(?)`,
		projectID, workflowID).Scan(&curCallable, &curPurge)
	if err == sql.ErrNoRows {
		return fmt.Errorf("workflow not found: %s", workflowID)
	}
	if err != nil {
		return err
	}
	if callable != nil {
		curCallable = *callable
	}
	if purge != nil {
		curPurge = *purge
	}
	return validateCallableSubworkflow(curCallable, curPurge)
}

// validateFinalizeSlots checks mutual exclusivity of command vs script_id per slot,
// and that any script_id resolves to a python_scripts row of kind=agent.
func (s *WorkflowService) validateFinalizeSlots(
	projectID string,
	successCmd, successScriptID string,
	failureCmd, failureScriptID string,
) error {
	if err := validateFinalizeSlot(s, projectID, "finalize_success", successCmd, successScriptID); err != nil {
		return err
	}
	return validateFinalizeSlot(s, projectID, "finalize_failure", failureCmd, failureScriptID)
}

// validatePauseSlot checks mutual exclusivity of command vs script_id for the pause_event slot.
func (s *WorkflowService) validatePauseSlot(projectID, cmd, scriptID string) error {
	return validateFinalizeSlot(s, projectID, "pause_event", cmd, scriptID)
}

func validateFinalizeSlot(s *WorkflowService, projectID, slot, cmd, scriptID string) error {
	if cmd != "" && scriptID != "" {
		return fmt.Errorf("%s: command and script_id are mutually exclusive", slot)
	}
	if scriptID == "" {
		return nil
	}
	r := repo.NewPythonScriptRepo(s.pool, s.clock)
	script, err := r.Get(projectID, scriptID)
	if err != nil {
		return fmt.Errorf("python_script_not_found: %s", scriptID)
	}
	if script.Kind == "tool" {
		return fmt.Errorf("python_script_kind_mismatch")
	}
	return nil
}
