package service

import (
	"encoding/json"
	"testing"

	"be/internal/model"
	"be/internal/types"
)

// TestImport_RoundTrip_PreservesStepwiseMode verifies a prompt_mode='stepwise'
// agent def survives an export -> import round-trip with its steps intact —
// the bundle carries prompt_mode/steps, and Import must map both onto the
// create request rather than silently defaulting the imported def to full mode
// (which would leave the prompt telling the agent to "work the steps" with no
// steps and no complete_step tool).
func TestImport_RoundTrip_PreservesStepwiseMode(t *testing.T) {
	t.Parallel()
	env := setupExportImportEnv(t)

	if _, err := env.workflowSvc.CreateWorkflowDef(env.projectID, &types.WorkflowDefCreateRequest{
		ID: "wf-sw", Description: "stepwise src",
	}); err != nil {
		t.Fatalf("CreateWorkflowDef: %v", err)
	}
	steps := []model.StepDefinition{
		{StepID: "explore", Title: "Explore", Instruction: "look around", RotationAllowed: true},
		{StepID: "finish", Title: "Finish", Instruction: "wrap up"},
	}
	if _, err := env.agentSvc.CreateAgentDef(env.projectID, "wf-sw", &types.AgentDefCreateRequest{
		ID: "ag-sw", Layer: 0, Prompt: "work the steps", PromptMode: "stepwise", Steps: &steps,
	}); err != nil {
		t.Fatalf("CreateAgentDef stepwise: %v", err)
	}
	// A plain full-mode agent alongside it must stay full mode after import.
	if _, err := env.agentSvc.CreateAgentDef(env.projectID, "wf-sw", &types.AgentDefCreateRequest{
		ID: "ag-full", Layer: 1, Prompt: "single shot",
	}); err != nil {
		t.Fatalf("CreateAgentDef full: %v", err)
	}

	bundle, err := env.exportSvc.Export(env.projectID, []string{"wf-sw"})
	if err != nil {
		t.Fatalf("Export: %v", err)
	}

	proj2 := env.seedProject2(t)
	if _, err := env.exportSvc.Import(proj2, &types.ImportRequest{Bundle: *bundle, Action: "overwrite"}); err != nil {
		t.Fatalf("Import: %v", err)
	}

	sw, err := env.agentSvc.GetAgentDef(proj2, "wf-sw", "ag-sw")
	if err != nil {
		t.Fatalf("GetAgentDef(ag-sw): %v", err)
	}
	if sw.PromptMode != PromptModeStepwise {
		t.Errorf("imported PromptMode = %q, want %q (import dropped the mode)", sw.PromptMode, PromptModeStepwise)
	}
	if sw.Steps == nil || *sw.Steps == "" {
		t.Fatalf("imported Steps = %v, want the exported step JSON (import dropped steps)", sw.Steps)
	}
	var gotSteps []model.StepDefinition
	if err := json.Unmarshal([]byte(*sw.Steps), &gotSteps); err != nil {
		t.Fatalf("unmarshal imported steps %q: %v", *sw.Steps, err)
	}
	if len(gotSteps) != 2 || gotSteps[0].StepID != "explore" || gotSteps[1].StepID != "finish" {
		t.Errorf("imported steps = %+v, want [explore finish]", gotSteps)
	}

	full, err := env.agentSvc.GetAgentDef(proj2, "wf-sw", "ag-full")
	if err != nil {
		t.Fatalf("GetAgentDef(ag-full): %v", err)
	}
	if full.PromptMode != PromptModeFull {
		t.Errorf("imported full agent PromptMode = %q, want %q", full.PromptMode, PromptModeFull)
	}
	if full.Steps != nil {
		t.Errorf("imported full agent Steps = %v, want nil", *full.Steps)
	}
}
