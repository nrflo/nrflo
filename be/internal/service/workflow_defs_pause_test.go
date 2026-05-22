package service

import (
	"testing"

	"be/internal/types"
)

// Pause-event slot validation (mutual exclusivity, missing/tool-kind script) is covered
// by TestCreateWorkflowDef_SlotValidation in workflow_defs_finalize_test.go, since the
// pause_event slot delegates to the shared validateFinalizeSlot. These tests cover the
// pause-specific persistence/round-trip paths only.

// TestCreateWorkflowDef_PauseEvent_CommandOnly verifies a command-only pause_event slot is accepted.
func TestCreateWorkflowDef_PauseEvent_CommandOnly(t *testing.T) {
	t.Parallel()
	_, svc := setupWorkflowDefsTestEnv(t)

	wf, err := svc.CreateWorkflowDef("proj1", &types.WorkflowDefCreateRequest{
		ID:                "wf-pause-cmd-ok",
		PauseEventCommand: "echo paused",
	})
	if err != nil {
		t.Fatalf("CreateWorkflowDef command-only pause: %v", err)
	}
	if wf == nil {
		t.Fatal("expected non-nil workflow")
	}
}

// TestGetWorkflowDef_ReturnsPauseFields verifies GetWorkflowDef returns pause_event fields.
func TestGetWorkflowDef_ReturnsPauseFields(t *testing.T) {
	t.Parallel()
	_, svc := setupWorkflowDefsTestEnv(t)

	_, err := svc.CreateWorkflowDef("proj1", &types.WorkflowDefCreateRequest{
		ID:                "wf-pause-get",
		PauseEventCommand: "pause-cmd",
	})
	if err != nil {
		t.Fatalf("CreateWorkflowDef: %v", err)
	}

	def, err := svc.GetWorkflowDef("proj1", "wf-pause-get")
	if err != nil {
		t.Fatalf("GetWorkflowDef: %v", err)
	}
	if def.PauseEventCommand != "pause-cmd" {
		t.Errorf("PauseEventCommand = %q, want %q", def.PauseEventCommand, "pause-cmd")
	}
	if def.PauseEventScriptID != "" {
		t.Errorf("PauseEventScriptID = %q, want empty", def.PauseEventScriptID)
	}
}

// TestGetWorkflowDef_ReturnsPauseScriptID verifies GetWorkflowDef returns pause_event_script_id.
func TestGetWorkflowDef_ReturnsPauseScriptID(t *testing.T) {
	t.Parallel()
	_, svc := setupWorkflowDefsTestEnv(t)
	agentID, _ := seedFinalizeScripts(t, svc, "proj1")

	_, err := svc.CreateWorkflowDef("proj1", &types.WorkflowDefCreateRequest{
		ID:                 "wf-pause-get-sid",
		PauseEventScriptID: agentID,
	})
	if err != nil {
		t.Fatalf("CreateWorkflowDef: %v", err)
	}

	def, err := svc.GetWorkflowDef("proj1", "wf-pause-get-sid")
	if err != nil {
		t.Fatalf("GetWorkflowDef: %v", err)
	}
	if def.PauseEventScriptID != agentID {
		t.Errorf("PauseEventScriptID = %q, want %q", def.PauseEventScriptID, agentID)
	}
	if def.PauseEventCommand != "" {
		t.Errorf("PauseEventCommand = %q, want empty", def.PauseEventCommand)
	}
}

// TestUpdateWorkflowDef_PauseEvent_CommandOnly verifies updating pause_event_command succeeds.
func TestUpdateWorkflowDef_PauseEvent_CommandOnly(t *testing.T) {
	t.Parallel()
	_, svc := setupWorkflowDefsTestEnv(t)

	if _, err := svc.CreateWorkflowDef("proj1", &types.WorkflowDefCreateRequest{
		ID: "wf-pause-upd",
	}); err != nil {
		t.Fatalf("CreateWorkflowDef: %v", err)
	}

	cmd := "on-pause"
	if err := svc.UpdateWorkflowDef("proj1", "wf-pause-upd", &types.WorkflowDefUpdateRequest{
		PauseEventCommand: &cmd,
	}); err != nil {
		t.Fatalf("UpdateWorkflowDef: %v", err)
	}

	def, err := svc.GetWorkflowDef("proj1", "wf-pause-upd")
	if err != nil {
		t.Fatalf("GetWorkflowDef: %v", err)
	}
	if def.PauseEventCommand != "on-pause" {
		t.Errorf("PauseEventCommand = %q, want %q", def.PauseEventCommand, "on-pause")
	}
}

// TestListWorkflowDefs_ReturnsPauseFields verifies ListWorkflowDefs returns pause_event fields.
func TestListWorkflowDefs_ReturnsPauseFields(t *testing.T) {
	t.Parallel()
	_, svc := setupWorkflowDefsTestEnv(t)

	_, err := svc.CreateWorkflowDef("proj1", &types.WorkflowDefCreateRequest{
		ID:                "wf-pause-list",
		PauseEventCommand: "list-pause-cmd",
	})
	if err != nil {
		t.Fatalf("CreateWorkflowDef: %v", err)
	}

	defs, err := svc.ListWorkflowDefs("proj1")
	if err != nil {
		t.Fatalf("ListWorkflowDefs: %v", err)
	}
	def, ok := defs["wf-pause-list"]
	if !ok {
		t.Fatal("wf-pause-list not found in list result")
	}
	if def.PauseEventCommand != "list-pause-cmd" {
		t.Errorf("PauseEventCommand = %q, want %q", def.PauseEventCommand, "list-pause-cmd")
	}
	if def.PauseEventScriptID != "" {
		t.Errorf("PauseEventScriptID = %q, want empty", def.PauseEventScriptID)
	}
}
