package service

import (
	"strings"
	"testing"

	"be/internal/types"
)

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

// TestCreateWorkflowDef_PauseEvent_BothCommandAndScript verifies mutual exclusivity.
func TestCreateWorkflowDef_PauseEvent_BothCommandAndScript(t *testing.T) {
	t.Parallel()
	_, svc := setupWorkflowDefsTestEnv(t)

	_, err := svc.CreateWorkflowDef("proj1", &types.WorkflowDefCreateRequest{
		ID:                 "wf-pause-both",
		PauseEventCommand:  "echo paused",
		PauseEventScriptID: "some-script",
	})
	if err == nil {
		t.Fatal("expected error for both command+script_id on pause_event slot, got nil")
	}
	if !strings.Contains(err.Error(), "mutually exclusive") {
		t.Errorf("error = %q, want to contain 'mutually exclusive'", err.Error())
	}
}

// TestCreateWorkflowDef_PauseEvent_MissingScript verifies a non-existent script_id is rejected.
func TestCreateWorkflowDef_PauseEvent_MissingScript(t *testing.T) {
	t.Parallel()
	_, svc := setupWorkflowDefsTestEnv(t)

	_, err := svc.CreateWorkflowDef("proj1", &types.WorkflowDefCreateRequest{
		ID:                 "wf-pause-missing",
		PauseEventScriptID: "ghost-script",
	})
	if err == nil {
		t.Fatal("expected error for non-existent pause script_id, got nil")
	}
	if !strings.Contains(err.Error(), "python_script_not_found") {
		t.Errorf("error = %q, want to contain 'python_script_not_found'", err.Error())
	}
}

// TestCreateWorkflowDef_PauseEvent_ToolKindScript verifies tool-kind script rejected on pause_event slot.
func TestCreateWorkflowDef_PauseEvent_ToolKindScript(t *testing.T) {
	t.Parallel()
	_, svc := setupWorkflowDefsTestEnv(t)
	_, toolID := seedFinalizeScripts(t, svc, "proj1")

	_, err := svc.CreateWorkflowDef("proj1", &types.WorkflowDefCreateRequest{
		ID:                 "wf-pause-tool",
		PauseEventScriptID: toolID,
	})
	if err == nil {
		t.Fatal("expected error for tool-kind script on pause_event slot, got nil")
	}
	if !strings.Contains(err.Error(), "python_script_kind_mismatch") {
		t.Errorf("error = %q, want to contain 'python_script_kind_mismatch'", err.Error())
	}
}

// TestCreateWorkflowDef_PauseEvent_AgentKindScript verifies agent-kind script accepted on pause_event slot.
func TestCreateWorkflowDef_PauseEvent_AgentKindScript(t *testing.T) {
	t.Parallel()
	_, svc := setupWorkflowDefsTestEnv(t)
	agentID, _ := seedFinalizeScripts(t, svc, "proj1")

	wf, err := svc.CreateWorkflowDef("proj1", &types.WorkflowDefCreateRequest{
		ID:                 "wf-pause-agent",
		PauseEventScriptID: agentID,
	})
	if err != nil {
		t.Fatalf("CreateWorkflowDef with agent-kind script on pause_event slot: %v", err)
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

// TestUpdateWorkflowDef_PauseEvent_BothCommandAndScript verifies mutual exclusivity on update.
func TestUpdateWorkflowDef_PauseEvent_BothCommandAndScript(t *testing.T) {
	t.Parallel()
	_, svc := setupWorkflowDefsTestEnv(t)

	if _, err := svc.CreateWorkflowDef("proj1", &types.WorkflowDefCreateRequest{
		ID: "wf-pause-upd-both",
	}); err != nil {
		t.Fatalf("CreateWorkflowDef: %v", err)
	}

	cmd := "cmd"
	sid := "script"
	err := svc.UpdateWorkflowDef("proj1", "wf-pause-upd-both", &types.WorkflowDefUpdateRequest{
		PauseEventCommand:  &cmd,
		PauseEventScriptID: &sid,
	})
	if err == nil {
		t.Fatal("expected error for both command+script_id on update, got nil")
	}
	if !strings.Contains(err.Error(), "mutually exclusive") {
		t.Errorf("error = %q, want to contain 'mutually exclusive'", err.Error())
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
