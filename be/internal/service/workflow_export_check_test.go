package service

import (
	"testing"

	"be/internal/types"
)

func TestCheckImport_EmptyProject_NoConflicts(t *testing.T) {
	t.Parallel()
	env := setupExportImportEnv(t)
	env.createSimpleWorkflow(t, "wf-noconflict")

	bundle, err := env.exportSvc.Export(env.projectID, []string{"wf-noconflict"})
	if err != nil {
		t.Fatalf("Export: %v", err)
	}
	proj2 := env.seedProject2(t)
	conflicts, err := env.exportSvc.CheckImport(proj2, bundle)
	if err != nil {
		t.Fatalf("CheckImport: %v", err)
	}
	if len(conflicts.WorkflowIDs) != 0 {
		t.Errorf("WorkflowIDs = %v, want empty", conflicts.WorkflowIDs)
	}
	if len(conflicts.PythonScriptIDs) != 0 {
		t.Errorf("PythonScriptIDs = %v, want empty", conflicts.PythonScriptIDs)
	}
}

func TestCheckImport_DetectsWorkflowAndScriptConflicts(t *testing.T) {
	t.Parallel()
	env := setupExportImportEnv(t)

	script, err := env.pythonScriptSvc.Create(env.projectID, &types.PythonScriptCreateRequest{
		Name: "sc-conflict",
		Code: "print(1)",
	})
	if err != nil {
		t.Fatalf("Create script: %v", err)
	}
	if _, err := env.workflowSvc.CreateWorkflowDef(env.projectID, &types.WorkflowDefCreateRequest{
		ID: "wf-detc",
	}); err != nil {
		t.Fatalf("CreateWorkflowDef: %v", err)
	}
	if _, err := env.agentSvc.CreateAgentDef(env.projectID, "wf-detc", &types.AgentDefCreateRequest{
		ID:             "sc-agent",
		Layer:          0,
		ExecutionMode:  "script",
		PythonScriptID: &script.ID,
	}); err != nil {
		t.Fatalf("CreateAgentDef: %v", err)
	}

	bundle, err := env.exportSvc.Export(env.projectID, []string{"wf-detc"})
	if err != nil {
		t.Fatalf("Export: %v", err)
	}
	conflicts, err := env.exportSvc.CheckImport(env.projectID, bundle)
	if err != nil {
		t.Fatalf("CheckImport: %v", err)
	}
	if len(conflicts.WorkflowIDs) != 1 || conflicts.WorkflowIDs[0] != "wf-detc" {
		t.Errorf("WorkflowIDs = %v, want [wf-detc]", conflicts.WorkflowIDs)
	}
	if len(conflicts.PythonScriptIDs) != 1 || conflicts.PythonScriptIDs[0] != script.ID {
		t.Errorf("PythonScriptIDs = %v, want [%s]", conflicts.PythonScriptIDs, script.ID)
	}
}

func TestImport_InvalidAction_Error(t *testing.T) {
	t.Parallel()
	env := setupExportImportEnv(t)

	_, err := env.exportSvc.Import(env.projectID, &types.ImportRequest{
		Bundle: types.WorkflowBundle{Version: "1.0"},
		Action: "bad-action",
	})
	if err == nil {
		t.Fatal("Import with invalid action: expected error, got nil")
	}
}

func TestImport_Cancel_Skipped_NoWrites(t *testing.T) {
	t.Parallel()
	env := setupExportImportEnv(t)
	env.createSimpleWorkflow(t, "wf-cancel-src")

	bundle, err := env.exportSvc.Export(env.projectID, []string{"wf-cancel-src"})
	if err != nil {
		t.Fatalf("Export: %v", err)
	}
	before, _ := env.workflowSvc.ListWorkflowDefs(env.projectID)

	result, err := env.exportSvc.Import(env.projectID, &types.ImportRequest{
		Bundle: *bundle,
		Action: "cancel",
	})
	if err != nil {
		t.Fatalf("Import cancel: %v", err)
	}
	if !result.Skipped {
		t.Error("result.Skipped = false, want true")
	}
	after, _ := env.workflowSvc.ListWorkflowDefs(env.projectID)
	if len(before) != len(after) {
		t.Errorf("workflow count: before=%d after=%d, expected no writes on cancel", len(before), len(after))
	}
}
