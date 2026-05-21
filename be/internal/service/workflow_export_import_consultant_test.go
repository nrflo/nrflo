package service

import (
	"testing"

	"be/internal/clock"
	"be/internal/types"
)

// TestExportImport_Consultant_RoundTrip verifies that a consultant=true api agent
// round-trips through export and import with the flag preserved.
func TestExportImport_Consultant_RoundTrip(t *testing.T) {
	t.Parallel()
	env := setupExportImportEnv(t)

	// Enable api_mode so consultant=true agents can be created.
	settingsSvc := NewGlobalSettingsService(env.pool, clock.Real())
	if err := settingsSvc.Set("api_mode_enabled", "true"); err != nil {
		t.Fatalf("Set api_mode_enabled: %v", err)
	}

	if _, err := env.workflowSvc.CreateWorkflowDef(env.projectID, &types.WorkflowDefCreateRequest{
		ID: "wf-consultant-rt",
	}); err != nil {
		t.Fatalf("CreateWorkflowDef: %v", err)
	}
	if _, err := env.agentSvc.CreateAgentDef(env.projectID, "wf-consultant-rt", &types.AgentDefCreateRequest{
		ID:            "ag-consultant",
		Layer:         0,
		Prompt:        "advise on implementation",
		ExecutionMode: "api",
		Consultant:    true,
	}); err != nil {
		t.Fatalf("CreateAgentDef(consultant=true): %v", err)
	}

	bundle, err := env.exportSvc.Export(env.projectID, []string{"wf-consultant-rt"})
	if err != nil {
		t.Fatalf("Export: %v", err)
	}
	if len(bundle.Workflows) != 1 {
		t.Fatalf("bundle.Workflows len = %d, want 1", len(bundle.Workflows))
	}

	// Verify consultant flag is serialized in the bundle.
	bundleAgents := bundle.Workflows[0].Agents
	if len(bundleAgents) != 1 {
		t.Fatalf("bundle agents len = %d, want 1", len(bundleAgents))
	}
	if !bundleAgents[0].Consultant {
		t.Error("bundle agent Consultant = false, want true")
	}

	// Import into proj2 and verify the flag is preserved.
	proj2 := env.seedProject2(t)
	result, err := env.exportSvc.Import(proj2, &types.ImportRequest{Bundle: *bundle, Action: "overwrite"})
	if err != nil {
		t.Fatalf("Import: %v", err)
	}
	if result.Skipped {
		t.Error("result.Skipped = true, want false")
	}

	imported, err := env.agentSvc.GetAgentDef(proj2, "wf-consultant-rt", "ag-consultant")
	if err != nil {
		t.Fatalf("GetAgentDef after import: %v", err)
	}
	if !imported.Consultant {
		t.Error("imported agent Consultant = false, want true")
	}
	if imported.ExecutionMode != "api" {
		t.Errorf("imported agent ExecutionMode = %q, want api", imported.ExecutionMode)
	}
}

// TestExportImport_ConsultantFalse_RoundTrip verifies that a non-consultant agent
// (consultant=false) preserves the flag through export/import.
func TestExportImport_ConsultantFalse_RoundTrip(t *testing.T) {
	t.Parallel()
	env := setupExportImportEnv(t)
	env.createSimpleWorkflow(t, "wf-no-consultant")

	bundle, err := env.exportSvc.Export(env.projectID, []string{"wf-no-consultant"})
	if err != nil {
		t.Fatalf("Export: %v", err)
	}

	proj2 := env.seedProject2(t)
	if _, err := env.exportSvc.Import(proj2, &types.ImportRequest{Bundle: *bundle, Action: "overwrite"}); err != nil {
		t.Fatalf("Import: %v", err)
	}

	imported, err := env.agentSvc.GetAgentDef(proj2, "wf-no-consultant", "agent-a")
	if err != nil {
		t.Fatalf("GetAgentDef after import: %v", err)
	}
	if imported.Consultant {
		t.Error("imported non-consultant agent Consultant = true, want false")
	}
}
