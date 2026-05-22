package service

import (
	"testing"

	"be/internal/types"
)

// TestUpdateWorkflowDef_FinalizeSlots_TriState verifies tri-state semantics for finalize fields on update.
func TestUpdateWorkflowDef_FinalizeSlots_TriState(t *testing.T) {
	t.Parallel()
	_, svc := setupWorkflowDefsTestEnv(t)

	_, err := svc.CreateWorkflowDef("proj1", &types.WorkflowDefCreateRequest{
		ID:                     "wf-fin-upd",
		FinalizeSuccessCommand: "initial-cmd",
	})
	if err != nil {
		t.Fatalf("create: %v", err)
	}

	// Step 1: nil pointer — field must remain "initial-cmd".
	if err := svc.UpdateWorkflowDef("proj1", "wf-fin-upd", &types.WorkflowDefUpdateRequest{
		FinalizeSuccessCommand: nil,
	}); err != nil {
		t.Fatalf("update nil: %v", err)
	}
	def, err := svc.GetWorkflowDef("proj1", "wf-fin-upd")
	if err != nil {
		t.Fatalf("get after nil update: %v", err)
	}
	if def.FinalizeSuccessCommand != "initial-cmd" {
		t.Errorf("after nil update: FinalizeSuccessCommand = %q, want %q", def.FinalizeSuccessCommand, "initial-cmd")
	}

	// Step 2: set to "new-cmd".
	newCmd := "new-cmd"
	if err := svc.UpdateWorkflowDef("proj1", "wf-fin-upd", &types.WorkflowDefUpdateRequest{
		FinalizeSuccessCommand: &newCmd,
	}); err != nil {
		t.Fatalf("update set: %v", err)
	}
	def, err = svc.GetWorkflowDef("proj1", "wf-fin-upd")
	if err != nil {
		t.Fatalf("get after set: %v", err)
	}
	if def.FinalizeSuccessCommand != "new-cmd" {
		t.Errorf("after set: FinalizeSuccessCommand = %q, want %q", def.FinalizeSuccessCommand, "new-cmd")
	}

	// Step 3: clear via &"".
	empty := ""
	if err := svc.UpdateWorkflowDef("proj1", "wf-fin-upd", &types.WorkflowDefUpdateRequest{
		FinalizeSuccessCommand: &empty,
	}); err != nil {
		t.Fatalf("update clear: %v", err)
	}
	def, err = svc.GetWorkflowDef("proj1", "wf-fin-upd")
	if err != nil {
		t.Fatalf("get after clear: %v", err)
	}
	if def.FinalizeSuccessCommand != "" {
		t.Errorf("after clear: FinalizeSuccessCommand = %q, want empty", def.FinalizeSuccessCommand)
	}

	// Step 4: nil again — must not restore the value.
	if err := svc.UpdateWorkflowDef("proj1", "wf-fin-upd", &types.WorkflowDefUpdateRequest{
		FinalizeSuccessCommand: nil,
	}); err != nil {
		t.Fatalf("update nil after clear: %v", err)
	}
	def, err = svc.GetWorkflowDef("proj1", "wf-fin-upd")
	if err != nil {
		t.Fatalf("get after nil post-clear: %v", err)
	}
	if def.FinalizeSuccessCommand != "" {
		t.Errorf("after nil post-clear: FinalizeSuccessCommand = %q, want empty", def.FinalizeSuccessCommand)
	}
}

// TestUpdateWorkflowDef_FinalizeSlots_NoFinalizeFields_Skips verifies that an update with no finalize
// fields does not trigger validation at all.
func TestUpdateWorkflowDef_FinalizeSlots_NoFinalizeFields_Skips(t *testing.T) {
	t.Parallel()
	_, svc := setupWorkflowDefsTestEnv(t)

	_, err := svc.CreateWorkflowDef("proj1", &types.WorkflowDefCreateRequest{
		ID:                     "wf-fin-skip-validate",
		FinalizeSuccessCommand: "existing-cmd",
	})
	if err != nil {
		t.Fatalf("create: %v", err)
	}

	desc := "updated"
	if err := svc.UpdateWorkflowDef("proj1", "wf-fin-skip-validate", &types.WorkflowDefUpdateRequest{
		Description: &desc,
	}); err != nil {
		t.Fatalf("update description only: %v", err)
	}

	def, err := svc.GetWorkflowDef("proj1", "wf-fin-skip-validate")
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if def.Description != "updated" {
		t.Errorf("Description = %q, want %q", def.Description, "updated")
	}
	if def.FinalizeSuccessCommand != "existing-cmd" {
		t.Errorf("FinalizeSuccessCommand changed unexpectedly: %q", def.FinalizeSuccessCommand)
	}
}
