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
