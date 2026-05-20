package service

import (
	"strings"
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

// TestUpdateWorkflowDef_FinalizeSlots_BothInOneCall rejects command+script_id in same update call.
func TestUpdateWorkflowDef_FinalizeSlots_BothInOneCall(t *testing.T) {
	t.Parallel()
	_, svc := setupWorkflowDefsTestEnv(t)

	_, err := svc.CreateWorkflowDef("proj1", &types.WorkflowDefCreateRequest{ID: "wf-fin-upd-both"})
	if err != nil {
		t.Fatalf("create: %v", err)
	}

	cmd := "do-thing"
	script := "some-script-id"
	err = svc.UpdateWorkflowDef("proj1", "wf-fin-upd-both", &types.WorkflowDefUpdateRequest{
		FinalizeSuccessCommand:  &cmd,
		FinalizeSuccessScriptID: &script,
	})
	if err == nil {
		t.Fatal("expected error for both command+script_id in update, got nil")
	}
	if !strings.Contains(err.Error(), "mutually exclusive") {
		t.Errorf("error = %q, want to contain 'mutually exclusive'", err.Error())
	}
}

// TestUpdateWorkflowDef_FinalizeSlots_MissingScript rejects a non-existent script_id on update.
func TestUpdateWorkflowDef_FinalizeSlots_MissingScript(t *testing.T) {
	t.Parallel()
	_, svc := setupWorkflowDefsTestEnv(t)

	_, err := svc.CreateWorkflowDef("proj1", &types.WorkflowDefCreateRequest{ID: "wf-fin-upd-missing"})
	if err != nil {
		t.Fatalf("create: %v", err)
	}

	ghost := "ghost-id"
	err = svc.UpdateWorkflowDef("proj1", "wf-fin-upd-missing", &types.WorkflowDefUpdateRequest{
		FinalizeFailureScriptID: &ghost,
	})
	if err == nil {
		t.Fatal("expected error for non-existent script_id in update, got nil")
	}
	if !strings.Contains(err.Error(), "python_script_not_found") {
		t.Errorf("error = %q, want to contain 'python_script_not_found'", err.Error())
	}
}

// TestUpdateWorkflowDef_FinalizeSlots_ToolKindRejected rejects tool-kind script on update.
func TestUpdateWorkflowDef_FinalizeSlots_ToolKindRejected(t *testing.T) {
	t.Parallel()
	_, svc := setupWorkflowDefsTestEnv(t)
	_, toolID := seedFinalizeScripts(t, svc, "proj1")

	_, err := svc.CreateWorkflowDef("proj1", &types.WorkflowDefCreateRequest{ID: "wf-fin-upd-tool"})
	if err != nil {
		t.Fatalf("create: %v", err)
	}

	err = svc.UpdateWorkflowDef("proj1", "wf-fin-upd-tool", &types.WorkflowDefUpdateRequest{
		FinalizeSuccessScriptID: &toolID,
	})
	if err == nil {
		t.Fatal("expected error for tool-kind script in update, got nil")
	}
	if !strings.Contains(err.Error(), "python_script_kind_mismatch") {
		t.Errorf("error = %q, want to contain 'python_script_kind_mismatch'", err.Error())
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
