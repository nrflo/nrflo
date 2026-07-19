package service

import (
	"testing"

	"be/internal/clock"
	"be/internal/types"
)

// TestListAgentDefsForWorkflow_PlumbsContextBudgetTokens verifies the
// workflow-defs read model's own SELECT+scan (listAgentDefsForWorkflow,
// distinct from repo.AgentDefinitionRepo's Get/List) surfaces
// context_budget_tokens for both a set and an unset def.
func TestListAgentDefsForWorkflow_PlumbsContextBudgetTokens(t *testing.T) {
	t.Parallel()
	pool, agentSvc, wfID := setupAgentDefTestEnv(t, nil)
	wfSvc := NewWorkflowService(pool, clock.Real())

	if _, err := agentSvc.CreateAgentDef("proj1", wfID, &types.AgentDefCreateRequest{
		ID: "wf-budget-agent", Layer: 0, Prompt: "do work", ContextBudgetTokens: intPtr(70000),
	}); err != nil {
		t.Fatalf("CreateAgentDef with budget: %v", err)
	}
	if _, err := agentSvc.CreateAgentDef("proj1", wfID, &types.AgentDefCreateRequest{
		ID: "wf-nobudget-agent", Layer: 0, Prompt: "do work",
	}); err != nil {
		t.Fatalf("CreateAgentDef without budget: %v", err)
	}

	defs, err := wfSvc.listAgentDefsForWorkflow("proj1", wfID)
	if err != nil {
		t.Fatalf("listAgentDefsForWorkflow: %v", err)
	}
	if len(defs) != 2 {
		t.Fatalf("defs count = %d, want 2", len(defs))
	}

	var withBudget, withoutBudget int
	for _, d := range defs {
		if d.ContextBudgetTokens != nil {
			withBudget++
			if *d.ContextBudgetTokens != 70000 {
				t.Errorf("ContextBudgetTokens = %d, want 70000", *d.ContextBudgetTokens)
			}
		} else {
			withoutBudget++
		}
	}
	if withBudget != 1 || withoutBudget != 1 {
		t.Errorf("withBudget=%d withoutBudget=%d, want 1 each", withBudget, withoutBudget)
	}
}

// TestListAgentDefsForProject_PlumbsContextBudgetTokens covers the sibling
// project-wide query (used by workflow listing's global-workflow union) with
// the same nullable-int assertion.
func TestListAgentDefsForProject_PlumbsContextBudgetTokens(t *testing.T) {
	t.Parallel()
	pool, agentSvc, wfID := setupAgentDefTestEnv(t, nil)
	wfSvc := NewWorkflowService(pool, clock.Real())

	if _, err := agentSvc.CreateAgentDef("proj1", wfID, &types.AgentDefCreateRequest{
		ID: "proj-budget-agent", Layer: 0, Prompt: "do work", ContextBudgetTokens: intPtr(0),
	}); err != nil {
		t.Fatalf("CreateAgentDef with disabled budget: %v", err)
	}

	defs, err := wfSvc.listAgentDefsForProject("proj1")
	if err != nil {
		t.Fatalf("listAgentDefsForProject: %v", err)
	}
	if len(defs) != 1 {
		t.Fatalf("defs count = %d, want 1", len(defs))
	}
	if defs[0].ContextBudgetTokens == nil || *defs[0].ContextBudgetTokens != 0 {
		t.Errorf("ContextBudgetTokens = %v, want non-nil 0 (explicit disable, not inherited nil)", defs[0].ContextBudgetTokens)
	}
}
