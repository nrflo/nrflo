package service

import (
	"strings"
	"testing"
	"time"

	"be/internal/types"
)

// seedFinalizeScripts inserts an agent-kind and a tool-kind python_script into the test DB.
// Returns (agentScriptID, toolScriptID).
func seedFinalizeScripts(t *testing.T, svc *WorkflowService, projectID string) (string, string) {
	t.Helper()
	now := time.Now().UTC().Format(time.RFC3339Nano)
	agentID := "fin-ps-agent"
	toolID := "fin-ps-tool"

	if _, err := svc.pool.Exec(
		`INSERT INTO python_scripts (id, project_id, name, description, code, kind, created_at, updated_at)
		 VALUES (?, ?, ?, '', '', 'agent', ?, ?)`,
		agentID, projectID, "fin-agent-script", now, now,
	); err != nil {
		t.Fatalf("insert agent python_script: %v", err)
	}
	if _, err := svc.pool.Exec(
		`INSERT INTO python_scripts (id, project_id, name, description, code, kind, tool_description, created_at, updated_at)
		 VALUES (?, ?, ?, '', '', 'tool', 'does work', ?, ?)`,
		toolID, projectID, "fin-tool-script", now, now,
	); err != nil {
		t.Fatalf("insert tool python_script: %v", err)
	}
	return agentID, toolID
}

// TestCreateWorkflowDef_FinalizeSuccess_CommandOnly verifies a command-only success slot is accepted.
func TestCreateWorkflowDef_FinalizeSuccess_CommandOnly(t *testing.T) {
	t.Parallel()
	_, svc := setupWorkflowDefsTestEnv(t)

	wf, err := svc.CreateWorkflowDef("proj1", &types.WorkflowDefCreateRequest{
		ID:                     "wf-fin-cmd-ok",
		FinalizeSuccessCommand: "echo done",
	})
	if err != nil {
		t.Fatalf("CreateWorkflowDef command-only: %v", err)
	}
	if wf == nil {
		t.Fatal("expected non-nil workflow")
	}
}

// TestCreateWorkflowDef_FinalizeFailure_CommandOnly verifies a command-only failure slot is accepted.
func TestCreateWorkflowDef_FinalizeFailure_CommandOnly(t *testing.T) {
	t.Parallel()
	_, svc := setupWorkflowDefsTestEnv(t)

	wf, err := svc.CreateWorkflowDef("proj1", &types.WorkflowDefCreateRequest{
		ID:                     "wf-fin-fail-cmd-ok",
		FinalizeFailureCommand: "alert fail",
	})
	if err != nil {
		t.Fatalf("CreateWorkflowDef failure command-only: %v", err)
	}
	if wf == nil {
		t.Fatal("expected non-nil workflow")
	}
}

// TestCreateWorkflowDef_FinalizeSuccess_BothCommandAndScript verifies mutual exclusivity on success slot.
func TestCreateWorkflowDef_FinalizeSuccess_BothCommandAndScript(t *testing.T) {
	t.Parallel()
	_, svc := setupWorkflowDefsTestEnv(t)

	_, err := svc.CreateWorkflowDef("proj1", &types.WorkflowDefCreateRequest{
		ID:                      "wf-fin-both-succ",
		FinalizeSuccessCommand:  "echo ok",
		FinalizeSuccessScriptID: "some-script",
	})
	if err == nil {
		t.Fatal("expected error for both command+script_id on success slot, got nil")
	}
	if !strings.Contains(err.Error(), "mutually exclusive") {
		t.Errorf("error = %q, want to contain 'mutually exclusive'", err.Error())
	}
}

// TestCreateWorkflowDef_FinalizeFailure_BothCommandAndScript verifies mutual exclusivity on failure slot.
func TestCreateWorkflowDef_FinalizeFailure_BothCommandAndScript(t *testing.T) {
	t.Parallel()
	_, svc := setupWorkflowDefsTestEnv(t)

	_, err := svc.CreateWorkflowDef("proj1", &types.WorkflowDefCreateRequest{
		ID:                      "wf-fin-both-fail",
		FinalizeFailureCommand:  "alert fail",
		FinalizeFailureScriptID: "some-script",
	})
	if err == nil {
		t.Fatal("expected error for both command+script_id on failure slot, got nil")
	}
	if !strings.Contains(err.Error(), "mutually exclusive") {
		t.Errorf("error = %q, want to contain 'mutually exclusive'", err.Error())
	}
}

// TestCreateWorkflowDef_FinalizeSuccess_MissingScript verifies that a non-existent script_id is rejected.
func TestCreateWorkflowDef_FinalizeSuccess_MissingScript(t *testing.T) {
	t.Parallel()
	_, svc := setupWorkflowDefsTestEnv(t)

	_, err := svc.CreateWorkflowDef("proj1", &types.WorkflowDefCreateRequest{
		ID:                      "wf-fin-missing-ps",
		FinalizeSuccessScriptID: "nonexistent-script",
	})
	if err == nil {
		t.Fatal("expected error for non-existent script_id, got nil")
	}
	if !strings.Contains(err.Error(), "python_script_not_found") {
		t.Errorf("error = %q, want to contain 'python_script_not_found'", err.Error())
	}
}

// TestCreateWorkflowDef_FinalizeFailure_MissingScript verifies missing script rejected on failure slot.
func TestCreateWorkflowDef_FinalizeFailure_MissingScript(t *testing.T) {
	t.Parallel()
	_, svc := setupWorkflowDefsTestEnv(t)

	_, err := svc.CreateWorkflowDef("proj1", &types.WorkflowDefCreateRequest{
		ID:                      "wf-fin-fail-missing-ps",
		FinalizeFailureScriptID: "ghost-script",
	})
	if err == nil {
		t.Fatal("expected error for non-existent failure script_id, got nil")
	}
	if !strings.Contains(err.Error(), "python_script_not_found") {
		t.Errorf("error = %q, want to contain 'python_script_not_found'", err.Error())
	}
}

// TestCreateWorkflowDef_FinalizeSuccess_ToolKindScript verifies tool-kind script rejected on success slot.
func TestCreateWorkflowDef_FinalizeSuccess_ToolKindScript(t *testing.T) {
	t.Parallel()
	_, svc := setupWorkflowDefsTestEnv(t)
	_, toolID := seedFinalizeScripts(t, svc, "proj1")

	_, err := svc.CreateWorkflowDef("proj1", &types.WorkflowDefCreateRequest{
		ID:                      "wf-fin-tool-succ",
		FinalizeSuccessScriptID: toolID,
	})
	if err == nil {
		t.Fatal("expected error for tool-kind script on success slot, got nil")
	}
	if !strings.Contains(err.Error(), "python_script_kind_mismatch") {
		t.Errorf("error = %q, want to contain 'python_script_kind_mismatch'", err.Error())
	}
}

// TestCreateWorkflowDef_FinalizeFailure_ToolKindScript verifies tool-kind script rejected on failure slot.
func TestCreateWorkflowDef_FinalizeFailure_ToolKindScript(t *testing.T) {
	t.Parallel()
	_, svc := setupWorkflowDefsTestEnv(t)
	_, toolID := seedFinalizeScripts(t, svc, "proj1")

	_, err := svc.CreateWorkflowDef("proj1", &types.WorkflowDefCreateRequest{
		ID:                      "wf-fin-tool-fail",
		FinalizeFailureScriptID: toolID,
	})
	if err == nil {
		t.Fatal("expected error for tool-kind script on failure slot, got nil")
	}
	if !strings.Contains(err.Error(), "python_script_kind_mismatch") {
		t.Errorf("error = %q, want to contain 'python_script_kind_mismatch'", err.Error())
	}
}

// TestCreateWorkflowDef_FinalizeSuccess_AgentKindScript verifies agent-kind script accepted on success slot.
func TestCreateWorkflowDef_FinalizeSuccess_AgentKindScript(t *testing.T) {
	t.Parallel()
	_, svc := setupWorkflowDefsTestEnv(t)
	agentID, _ := seedFinalizeScripts(t, svc, "proj1")

	wf, err := svc.CreateWorkflowDef("proj1", &types.WorkflowDefCreateRequest{
		ID:                      "wf-fin-agent-succ",
		FinalizeSuccessScriptID: agentID,
	})
	if err != nil {
		t.Fatalf("CreateWorkflowDef with agent-kind script on success slot: %v", err)
	}
	if wf == nil {
		t.Fatal("expected non-nil workflow")
	}

	def, err := svc.GetWorkflowDef("proj1", "wf-fin-agent-succ")
	if err != nil {
		t.Fatalf("GetWorkflowDef: %v", err)
	}
	if def.FinalizeSuccessScriptID != agentID {
		t.Errorf("GetWorkflowDef FinalizeSuccessScriptID = %q, want %q", def.FinalizeSuccessScriptID, agentID)
	}
}

// TestCreateWorkflowDef_FinalizeFailure_AgentKindScript verifies agent-kind script accepted on failure slot.
func TestCreateWorkflowDef_FinalizeFailure_AgentKindScript(t *testing.T) {
	t.Parallel()
	_, svc := setupWorkflowDefsTestEnv(t)
	agentID, _ := seedFinalizeScripts(t, svc, "proj1")

	wf, err := svc.CreateWorkflowDef("proj1", &types.WorkflowDefCreateRequest{
		ID:                      "wf-fin-agent-fail",
		FinalizeFailureScriptID: agentID,
	})
	if err != nil {
		t.Fatalf("CreateWorkflowDef with agent-kind script on failure slot: %v", err)
	}
	if wf == nil {
		t.Fatal("expected non-nil workflow")
	}
}

// TestGetWorkflowDef_ReturnsFinalizeFields verifies Get returns all 4 finalize fields.
func TestGetWorkflowDef_ReturnsFinalizeFields(t *testing.T) {
	t.Parallel()
	_, svc := setupWorkflowDefsTestEnv(t)

	_, err := svc.CreateWorkflowDef("proj1", &types.WorkflowDefCreateRequest{
		ID:                     "wf-fin-get",
		FinalizeSuccessCommand: "success-cmd",
		FinalizeFailureCommand: "failure-cmd",
	})
	if err != nil {
		t.Fatalf("CreateWorkflowDef: %v", err)
	}

	def, err := svc.GetWorkflowDef("proj1", "wf-fin-get")
	if err != nil {
		t.Fatalf("GetWorkflowDef: %v", err)
	}
	if def.FinalizeSuccessCommand != "success-cmd" {
		t.Errorf("FinalizeSuccessCommand = %q, want %q", def.FinalizeSuccessCommand, "success-cmd")
	}
	if def.FinalizeSuccessScriptID != "" {
		t.Errorf("FinalizeSuccessScriptID = %q, want empty", def.FinalizeSuccessScriptID)
	}
	if def.FinalizeFailureCommand != "failure-cmd" {
		t.Errorf("FinalizeFailureCommand = %q, want %q", def.FinalizeFailureCommand, "failure-cmd")
	}
	if def.FinalizeFailureScriptID != "" {
		t.Errorf("FinalizeFailureScriptID = %q, want empty", def.FinalizeFailureScriptID)
	}
}

// TestListWorkflowDefs_ReturnsFinalizeFields verifies List returns all 4 finalize fields.
func TestListWorkflowDefs_ReturnsFinalizeFields(t *testing.T) {
	t.Parallel()
	_, svc := setupWorkflowDefsTestEnv(t)

	agentID, _ := seedFinalizeScripts(t, svc, "proj1")
	_, err := svc.CreateWorkflowDef("proj1", &types.WorkflowDefCreateRequest{
		ID:                      "wf-fin-list",
		FinalizeSuccessScriptID: agentID,
		FinalizeFailureCommand:  "on-fail",
	})
	if err != nil {
		t.Fatalf("CreateWorkflowDef: %v", err)
	}

	defs, err := svc.ListWorkflowDefs("proj1")
	if err != nil {
		t.Fatalf("ListWorkflowDefs: %v", err)
	}
	def, ok := defs["wf-fin-list"]
	if !ok {
		t.Fatal("wf-fin-list not in list result")
	}
	if def.FinalizeSuccessScriptID != agentID {
		t.Errorf("List FinalizeSuccessScriptID = %q, want %q", def.FinalizeSuccessScriptID, agentID)
	}
	if def.FinalizeSuccessCommand != "" {
		t.Errorf("List FinalizeSuccessCommand = %q, want empty", def.FinalizeSuccessCommand)
	}
	if def.FinalizeFailureCommand != "on-fail" {
		t.Errorf("List FinalizeFailureCommand = %q, want %q", def.FinalizeFailureCommand, "on-fail")
	}
	if def.FinalizeFailureScriptID != "" {
		t.Errorf("List FinalizeFailureScriptID = %q, want empty", def.FinalizeFailureScriptID)
	}
}
