package service

import (
	"testing"

	"be/internal/types"
)

// TestExportImport_ContextBudgetTokens_RoundTrip verifies context_budget_tokens
// survives Export -> Import for both a set value and the nil (inherit
// default) sentinel — Export serializes model.AgentDefinition directly via
// its json tag, so this exercises the full bundle round trip, not just the
// field mapping in workflow_export_import.go's Import.
func TestExportImport_ContextBudgetTokens_RoundTrip(t *testing.T) {
	t.Parallel()
	env := setupExportImportEnv(t)

	if _, err := env.workflowSvc.CreateWorkflowDef(env.projectID, &types.WorkflowDefCreateRequest{
		ID: "wf-ctxbudget-rt",
	}); err != nil {
		t.Fatalf("CreateWorkflowDef: %v", err)
	}
	if _, err := env.agentSvc.CreateAgentDef(env.projectID, "wf-ctxbudget-rt", &types.AgentDefCreateRequest{
		ID: "budget-agent", Layer: 0, Prompt: "do work", ContextBudgetTokens: intPtr(90000),
	}); err != nil {
		t.Fatalf("CreateAgentDef budget-agent: %v", err)
	}
	if _, err := env.agentSvc.CreateAgentDef(env.projectID, "wf-ctxbudget-rt", &types.AgentDefCreateRequest{
		ID: "nobudget-agent", Layer: 0, Prompt: "do work",
	}); err != nil {
		t.Fatalf("CreateAgentDef nobudget-agent: %v", err)
	}

	bundle, err := env.exportSvc.Export(env.projectID, []string{"wf-ctxbudget-rt"})
	if err != nil {
		t.Fatalf("Export: %v", err)
	}

	proj2 := env.seedProject2(t)
	if _, err := env.exportSvc.Import(proj2, &types.ImportRequest{Bundle: *bundle, Action: "overwrite"}); err != nil {
		t.Fatalf("Import: %v", err)
	}

	agents, err := env.agentSvc.ListAgentDefs(proj2, "wf-ctxbudget-rt")
	if err != nil {
		t.Fatalf("ListAgentDefs(proj2): %v", err)
	}
	if len(agents) != 2 {
		t.Fatalf("agents count = %d, want 2", len(agents))
	}

	for _, a := range agents {
		switch a.ID {
		case "budget-agent":
			assertIntPtr(t, "imported budget-agent.ContextBudgetTokens", a.ContextBudgetTokens, intPtr(90000))
		case "nobudget-agent":
			assertIntPtr(t, "imported nobudget-agent.ContextBudgetTokens", a.ContextBudgetTokens, nil)
		default:
			t.Errorf("unexpected imported agent ID %q", a.ID)
		}
	}
}
