package service

import (
	"testing"

	"be/internal/types"
)

// TestExportImport_PauseEventCommand_RoundTrip verifies pause_event_command is preserved through export/import.
func TestExportImport_PauseEventCommand_RoundTrip(t *testing.T) {
	t.Parallel()
	env := setupExportImportEnv(t)

	if _, err := env.workflowSvc.CreateWorkflowDef(env.projectID, &types.WorkflowDefCreateRequest{
		ID:                "wf-pause-cmd-rt",
		PauseEventCommand: "on-pause-cmd",
	}); err != nil {
		t.Fatalf("CreateWorkflowDef: %v", err)
	}
	if _, err := env.agentSvc.CreateAgentDef(env.projectID, "wf-pause-cmd-rt", &types.AgentDefCreateRequest{
		ID: "ag-a", Layer: 0, Prompt: "p",
	}); err != nil {
		t.Fatalf("CreateAgentDef: %v", err)
	}

	bundle, err := env.exportSvc.Export(env.projectID, []string{"wf-pause-cmd-rt"})
	if err != nil {
		t.Fatalf("Export: %v", err)
	}
	if len(bundle.Workflows) != 1 {
		t.Fatalf("bundle.Workflows len = %d, want 1", len(bundle.Workflows))
	}
	if got := bundle.Workflows[0].Workflow.PauseEventCommand; got != "on-pause-cmd" {
		t.Errorf("bundle PauseEventCommand = %q, want %q", got, "on-pause-cmd")
	}

	proj2 := env.seedProject2(t)
	result, err := env.exportSvc.Import(proj2, &types.ImportRequest{Bundle: *bundle, Action: "overwrite"})
	if err != nil {
		t.Fatalf("Import: %v", err)
	}
	if result.Skipped {
		t.Error("result.Skipped = true, want false")
	}

	def, err := env.workflowSvc.GetWorkflowDef(proj2, "wf-pause-cmd-rt")
	if err != nil {
		t.Fatalf("GetWorkflowDef(proj2): %v", err)
	}
	if def.PauseEventCommand != "on-pause-cmd" {
		t.Errorf("imported PauseEventCommand = %q, want %q", def.PauseEventCommand, "on-pause-cmd")
	}
	if def.PauseEventScriptID != "" {
		t.Errorf("imported PauseEventScriptID = %q, want empty", def.PauseEventScriptID)
	}
}

// TestExport_BundleContainsPauseEventScriptID verifies that export serializes pause_event_script_id
// in the bundle's workflow entry (not remapped — only agent-bound scripts are included in PythonScripts).
func TestExport_BundleContainsPauseEventScriptID(t *testing.T) {
	t.Parallel()
	env := setupExportImportEnv(t)

	script, err := env.pythonScriptSvc.Create(env.projectID, &types.PythonScriptCreateRequest{
		Name: "pause-script",
		Code: "print('pause')",
	})
	if err != nil {
		t.Fatalf("Create script: %v", err)
	}

	if _, err := env.workflowSvc.CreateWorkflowDef(env.projectID, &types.WorkflowDefCreateRequest{
		ID:                 "wf-pause-sid-exp",
		PauseEventScriptID: script.ID,
	}); err != nil {
		t.Fatalf("CreateWorkflowDef: %v", err)
	}
	if _, err := env.agentSvc.CreateAgentDef(env.projectID, "wf-pause-sid-exp", &types.AgentDefCreateRequest{
		ID: "ag-b", Layer: 0, Prompt: "p",
	}); err != nil {
		t.Fatalf("CreateAgentDef: %v", err)
	}

	bundle, err := env.exportSvc.Export(env.projectID, []string{"wf-pause-sid-exp"})
	if err != nil {
		t.Fatalf("Export: %v", err)
	}
	if got := bundle.Workflows[0].Workflow.PauseEventScriptID; got != script.ID {
		t.Errorf("bundle PauseEventScriptID = %q, want %q", got, script.ID)
	}
	if bundle.Workflows[0].Workflow.PauseEventCommand != "" {
		t.Errorf("bundle PauseEventCommand = %q, want empty", bundle.Workflows[0].Workflow.PauseEventCommand)
	}

	// Import within the same project (rename action) so the script already exists for validation.
	result, err := env.exportSvc.Import(env.projectID, &types.ImportRequest{Bundle: *bundle, Action: "rename"})
	if err != nil {
		t.Fatalf("Import (rename, same project): %v", err)
	}
	if result.Skipped {
		t.Error("result.Skipped = true, want false")
	}
	if len(result.WorkflowIDs) != 1 {
		t.Fatalf("result.WorkflowIDs len = %d, want 1", len(result.WorkflowIDs))
	}
	importedID := result.WorkflowIDs[0]

	def, err := env.workflowSvc.GetWorkflowDef(env.projectID, importedID)
	if err != nil {
		t.Fatalf("GetWorkflowDef(%q): %v", importedID, err)
	}
	if def.PauseEventScriptID != script.ID {
		t.Errorf("imported PauseEventScriptID = %q, want %q", def.PauseEventScriptID, script.ID)
	}
}

// TestExportImport_LayerPauseAfter_RoundTrip verifies per-layer pause_after is preserved through export/import.
func TestExportImport_LayerPauseAfter_RoundTrip(t *testing.T) {
	t.Parallel()
	env := setupExportImportEnv(t)

	if _, err := env.workflowSvc.CreateWorkflowDef(env.projectID, &types.WorkflowDefCreateRequest{
		ID: "wf-lpa-rt",
	}); err != nil {
		t.Fatalf("CreateWorkflowDef: %v", err)
	}
	for _, id := range []string{"ag-l0", "ag-l1"} {
		layer := 0
		if id == "ag-l1" {
			layer = 1
		}
		if _, err := env.agentSvc.CreateAgentDef(env.projectID, "wf-lpa-rt", &types.AgentDefCreateRequest{
			ID: id, Layer: layer, Prompt: "p",
		}); err != nil {
			t.Fatalf("CreateAgentDef(%q): %v", id, err)
		}
	}
	if err := env.layerPolicySvc.SetLayerPauseAfter(env.projectID, "wf-lpa-rt", 0, true); err != nil {
		t.Fatalf("SetLayerPauseAfter(layer=0, true): %v", err)
	}
	if err := env.layerPolicySvc.SetLayerPauseAfter(env.projectID, "wf-lpa-rt", 1, false); err != nil {
		t.Fatalf("SetLayerPauseAfter(layer=1, false): %v", err)
	}

	bundle, err := env.exportSvc.Export(env.projectID, []string{"wf-lpa-rt"})
	if err != nil {
		t.Fatalf("Export: %v", err)
	}
	entry := bundle.Workflows[0]
	if !entry.LayerPauseAfter[0] {
		t.Errorf("bundle LayerPauseAfter[0] = false, want true")
	}
	if entry.LayerPauseAfter[1] {
		t.Errorf("bundle LayerPauseAfter[1] = true, want false")
	}

	proj2 := env.seedProject2(t)
	result, err := env.exportSvc.Import(proj2, &types.ImportRequest{Bundle: *bundle, Action: "overwrite"})
	if err != nil {
		t.Fatalf("Import: %v", err)
	}
	if result.Skipped {
		t.Error("result.Skipped = true, want false")
	}

	pauseMap, err := env.layerPolicySvc.GetLayerPauseAfter(proj2, "wf-lpa-rt")
	if err != nil {
		t.Fatalf("GetLayerPauseAfter(proj2): %v", err)
	}
	if !pauseMap[0] {
		t.Errorf("imported LayerPauseAfter[0] = false, want true")
	}
	if pauseMap[1] {
		t.Errorf("imported LayerPauseAfter[1] = true, want false")
	}
}

// TestExport_IncludesLayerPauseAfterInBundle verifies Export populates LayerPauseAfter in bundle entries.
func TestExport_IncludesLayerPauseAfterInBundle(t *testing.T) {
	t.Parallel()
	env := setupExportImportEnv(t)
	env.createSimpleWorkflow(t, "wf-lpa-exp")

	if err := env.layerPolicySvc.SetLayerPauseAfter(env.projectID, "wf-lpa-exp", 0, true); err != nil {
		t.Fatalf("SetLayerPauseAfter: %v", err)
	}

	bundle, err := env.exportSvc.Export(env.projectID, []string{"wf-lpa-exp"})
	if err != nil {
		t.Fatalf("Export: %v", err)
	}
	if len(bundle.Workflows) != 1 {
		t.Fatalf("bundle.Workflows len = %d, want 1", len(bundle.Workflows))
	}
	lpa := bundle.Workflows[0].LayerPauseAfter
	if lpa == nil {
		t.Fatal("LayerPauseAfter is nil in bundle, want non-nil")
	}
	if !lpa[0] {
		t.Errorf("LayerPauseAfter[0] = false, want true")
	}
}
