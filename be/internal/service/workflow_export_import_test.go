package service

import (
	"context"
	"testing"

	"be/internal/types"
)

func TestImport_RoundTrip_Overwrite(t *testing.T) {
	t.Parallel()
	env := setupExportImportEnv(t)

	// Build source workflow: layer 0 normal agent, layer 1 script agent, quorum:1 policy, 1 channel.
	script, err := env.pythonScriptSvc.Create(env.projectID, &types.PythonScriptCreateRequest{
		Name: "rt-script",
		Code: "print('rt')",
	})
	if err != nil {
		t.Fatalf("Create script: %v", err)
	}
	if _, err := env.workflowSvc.CreateWorkflowDef(env.projectID, &types.WorkflowDefCreateRequest{
		ID:          "wf-rt",
		Description: "round-trip",
	}); err != nil {
		t.Fatalf("CreateWorkflowDef: %v", err)
	}
	if _, err := env.agentSvc.CreateAgentDef(env.projectID, "wf-rt", &types.AgentDefCreateRequest{
		ID: "ag-l0", Layer: 0, Prompt: "layer 0",
	}); err != nil {
		t.Fatalf("CreateAgentDef ag-l0: %v", err)
	}
	if _, err := env.agentSvc.CreateAgentDef(env.projectID, "wf-rt", &types.AgentDefCreateRequest{
		ID:             "ag-l1",
		Layer:          1,
		ExecutionMode:  "script",
		PythonScriptID: &script.ID,
	}); err != nil {
		t.Fatalf("CreateAgentDef ag-l1: %v", err)
	}
	if err := env.layerPolicySvc.SetLayerPolicy(env.projectID, "wf-rt", 1, "quorum:1"); err != nil {
		t.Fatalf("SetLayerPolicy: %v", err)
	}
	enabled := true
	if _, err := env.notifySvc.Create(context.Background(), env.projectID, "wf-rt", &types.NotificationChannelCreateRequest{
		Name:    "ch-rt",
		Kind:    "slack",
		Enabled: &enabled,
	}); err != nil {
		t.Fatalf("Create notification: %v", err)
	}

	bundle, err := env.exportSvc.Export(env.projectID, []string{"wf-rt"})
	if err != nil {
		t.Fatalf("Export: %v", err)
	}

	proj2 := env.seedProject2(t)
	result, err := env.exportSvc.Import(proj2, &types.ImportRequest{Bundle: *bundle, Action: "overwrite"})
	if err != nil {
		t.Fatalf("Import: %v", err)
	}
	if result.Skipped {
		t.Error("result.Skipped = true, want false")
	}
	if len(result.WorkflowIDs) != 1 || result.WorkflowIDs[0] != "wf-rt" {
		t.Errorf("result.WorkflowIDs = %v, want [wf-rt]", result.WorkflowIDs)
	}
	if len(result.PythonScriptIDs) != 1 {
		t.Fatalf("result.PythonScriptIDs len = %d, want 1", len(result.PythonScriptIDs))
	}

	wfDef, err := env.workflowSvc.GetWorkflowDef(proj2, "wf-rt")
	if err != nil {
		t.Fatalf("GetWorkflowDef(proj2): %v", err)
	}
	if wfDef.Description != "round-trip" {
		t.Errorf("description = %q, want \"round-trip\"", wfDef.Description)
	}

	agents, err := env.agentSvc.ListAgentDefs(proj2, "wf-rt")
	if err != nil {
		t.Fatalf("ListAgentDefs(proj2): %v", err)
	}
	if len(agents) != 2 {
		t.Fatalf("agents count = %d, want 2", len(agents))
	}
	for _, a := range agents {
		if a.ExecutionMode == "script" {
			if a.PythonScriptID == nil {
				t.Fatal("script agent PythonScriptID nil after import")
			}
			if *a.PythonScriptID == script.ID {
				t.Error("script agent PythonScriptID unchanged (old ID), want remapped new ID")
			}
			if *a.PythonScriptID != result.PythonScriptIDs[0] {
				t.Errorf("PythonScriptID = %q, want %q", *a.PythonScriptID, result.PythonScriptIDs[0])
			}
		}
	}

	policies, err := env.layerPolicySvc.GetLayerPolicies(proj2, "wf-rt")
	if err != nil {
		t.Fatalf("GetLayerPolicies(proj2): %v", err)
	}
	if got := policies[1]; got != "quorum:1" {
		t.Errorf("layer 1 policy = %q, want \"quorum:1\"", got)
	}

	channels, err := env.notifySvc.List(proj2, "wf-rt")
	if err != nil {
		t.Fatalf("List notifications(proj2): %v", err)
	}
	if len(channels) != 1 {
		t.Fatalf("channels count = %d, want 1", len(channels))
	}
	if channels[0].Name != "ch-rt" {
		t.Errorf("channel name = %q, want \"ch-rt\"", channels[0].Name)
	}
	if channels[0].WorkflowID != "wf-rt" {
		t.Errorf("channel WorkflowID = %q, want \"wf-rt\"", channels[0].WorkflowID)
	}
}

func TestImport_Rename_WorkflowSuffix(t *testing.T) {
	t.Parallel()
	env := setupExportImportEnv(t)
	env.createSimpleWorkflow(t, "wf-ren")

	bundle, err := env.exportSvc.Export(env.projectID, []string{"wf-ren"})
	if err != nil {
		t.Fatalf("Export: %v", err)
	}
	result, err := env.exportSvc.Import(env.projectID, &types.ImportRequest{Bundle: *bundle, Action: "rename"})
	if err != nil {
		t.Fatalf("Import rename: %v", err)
	}
	if len(result.WorkflowIDs) != 1 || result.WorkflowIDs[0] != "wf-ren-1" {
		t.Errorf("result.WorkflowIDs = %v, want [wf-ren-1]", result.WorkflowIDs)
	}
	if _, err := env.workflowSvc.GetWorkflowDef(env.projectID, "wf-ren"); err != nil {
		t.Errorf("original wf-ren missing after rename: %v", err)
	}
	if _, err := env.workflowSvc.GetWorkflowDef(env.projectID, "wf-ren-1"); err != nil {
		t.Errorf("renamed wf-ren-1 not found: %v", err)
	}
}

func TestImport_Overwrite_ReplacesWorkflow(t *testing.T) {
	t.Parallel()
	env := setupExportImportEnv(t)

	if _, err := env.workflowSvc.CreateWorkflowDef(env.projectID, &types.WorkflowDefCreateRequest{
		ID: "wf-ow",
	}); err != nil {
		t.Fatalf("CreateWorkflowDef: %v", err)
	}
	if _, err := env.agentSvc.CreateAgentDef(env.projectID, "wf-ow", &types.AgentDefCreateRequest{
		ID: "old-agent", Layer: 0, Prompt: "old",
	}); err != nil {
		t.Fatalf("CreateAgentDef old-agent: %v", err)
	}

	env2 := setupExportImportEnv(t)
	if _, err := env2.workflowSvc.CreateWorkflowDef(env2.projectID, &types.WorkflowDefCreateRequest{
		ID: "wf-ow",
	}); err != nil {
		t.Fatalf("env2 CreateWorkflowDef: %v", err)
	}
	if _, err := env2.agentSvc.CreateAgentDef(env2.projectID, "wf-ow", &types.AgentDefCreateRequest{
		ID: "new-agent", Layer: 0, Prompt: "new",
	}); err != nil {
		t.Fatalf("env2 CreateAgentDef: %v", err)
	}
	bundle, err := env2.exportSvc.Export(env2.projectID, []string{"wf-ow"})
	if err != nil {
		t.Fatalf("Export from env2: %v", err)
	}

	if _, err := env.exportSvc.Import(env.projectID, &types.ImportRequest{
		Bundle: *bundle, Action: "overwrite",
	}); err != nil {
		t.Fatalf("Import overwrite: %v", err)
	}

	agents, err := env.agentSvc.ListAgentDefs(env.projectID, "wf-ow")
	if err != nil {
		t.Fatalf("ListAgentDefs after overwrite: %v", err)
	}
	if len(agents) != 1 {
		t.Fatalf("agents count = %d, want 1 (old agent replaced)", len(agents))
	}
	if agents[0].ID != "new-agent" {
		t.Errorf("agent.ID = %q, want \"new-agent\"", agents[0].ID)
	}
}
