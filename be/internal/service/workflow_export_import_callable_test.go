package service

import (
	"testing"

	"be/internal/types"
)

// TestExportImport_CallableAsSubworkflow_RoundTrip verifies callable_as_subworkflow
// survives export → import (both sides previously dropped it silently).
func TestExportImport_CallableAsSubworkflow_RoundTrip(t *testing.T) {
	t.Parallel()
	env := setupExportImportEnv(t)

	callable := true
	if _, err := env.workflowSvc.CreateWorkflowDef(env.projectID, &types.WorkflowDefCreateRequest{
		ID:                    "wf-callable-rt",
		ScopeType:             "project",
		CallableAsSubworkflow: &callable,
	}); err != nil {
		t.Fatalf("CreateWorkflowDef: %v", err)
	}
	if _, err := env.agentSvc.CreateAgentDef(env.projectID, "wf-callable-rt", &types.AgentDefCreateRequest{
		ID: "ag-a", Layer: 0, Prompt: "p",
	}); err != nil {
		t.Fatalf("CreateAgentDef: %v", err)
	}

	bundle, err := env.exportSvc.Export(env.projectID, []string{"wf-callable-rt"})
	if err != nil {
		t.Fatalf("Export: %v", err)
	}
	if !bundle.Workflows[0].Workflow.CallableAsSubworkflow {
		t.Error("bundle CallableAsSubworkflow = false, want true")
	}

	proj2 := env.seedProject2(t)
	if _, err := env.exportSvc.Import(proj2, &types.ImportRequest{Bundle: *bundle, Action: "overwrite"}); err != nil {
		t.Fatalf("Import: %v", err)
	}
	def, err := env.workflowSvc.GetWorkflowDef(proj2, "wf-callable-rt")
	if err != nil {
		t.Fatalf("GetWorkflowDef(proj2): %v", err)
	}
	if !def.CallableAsSubworkflow {
		t.Error("imported CallableAsSubworkflow = false, want true")
	}
}
