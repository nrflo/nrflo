package repo

import (
	"testing"

	"be/internal/clock"
	"be/internal/model"
)

// TestAgentDefinition_ContextBudgetTokens_RoundTripsCreateGetList verifies
// ContextBudgetTokens survives Create, Get, and List — the shared
// agentDefColumns/scanAgentDefRows path — with the NULL/0/value tri-state:
// unset stays nil (inherit config default), 0 stays non-nil 0 (disabled),
// and a positive value round-trips verbatim.
func TestAgentDefinition_ContextBudgetTokens_RoundTripsCreateGetList(t *testing.T) {
	t.Parallel()
	pool := newTestPool(t)
	seedProjectAndWorkflow(t, pool, "proj-ctxbudget", "wf-ctxbudget")

	r := NewAgentDefinitionRepo(pool, clock.Real())

	budget := 65000
	zero := 0
	if err := r.Create(&model.AgentDefinition{
		ID: "with-budget", ProjectID: "proj-ctxbudget", WorkflowID: "wf-ctxbudget",
		ExecutionMode: "api", Layer: 0, Model: "sonnet-5",
		ContextBudgetTokens: &budget,
	}); err != nil {
		t.Fatalf("create with-budget: %v", err)
	}
	if err := r.Create(&model.AgentDefinition{
		ID: "disabled-budget", ProjectID: "proj-ctxbudget", WorkflowID: "wf-ctxbudget",
		ExecutionMode: "api", Layer: 0, Model: "sonnet-5",
		ContextBudgetTokens: &zero,
	}); err != nil {
		t.Fatalf("create disabled-budget: %v", err)
	}
	if err := r.Create(&model.AgentDefinition{
		ID: "no-budget", ProjectID: "proj-ctxbudget", WorkflowID: "wf-ctxbudget",
		ExecutionMode: "api", Layer: 0, Model: "sonnet-5",
	}); err != nil {
		t.Fatalf("create no-budget: %v", err)
	}

	got, err := r.Get("proj-ctxbudget", "wf-ctxbudget", "with-budget")
	if err != nil {
		t.Fatalf("Get with-budget: %v", err)
	}
	if got.ContextBudgetTokens == nil || *got.ContextBudgetTokens != 65000 {
		t.Errorf("ContextBudgetTokens = %v, want 65000", got.ContextBudgetTokens)
	}

	gotZero, err := r.Get("proj-ctxbudget", "wf-ctxbudget", "disabled-budget")
	if err != nil {
		t.Fatalf("Get disabled-budget: %v", err)
	}
	if gotZero.ContextBudgetTokens == nil || *gotZero.ContextBudgetTokens != 0 {
		t.Errorf("ContextBudgetTokens = %v, want non-nil 0", gotZero.ContextBudgetTokens)
	}

	gotNil, err := r.Get("proj-ctxbudget", "wf-ctxbudget", "no-budget")
	if err != nil {
		t.Fatalf("Get no-budget: %v", err)
	}
	if gotNil.ContextBudgetTokens != nil {
		t.Errorf("ContextBudgetTokens = %v, want nil (default when unset on Create)", gotNil.ContextBudgetTokens)
	}

	all, err := r.List("proj-ctxbudget", "wf-ctxbudget")
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(all) != 3 {
		t.Fatalf("List count = %d, want 3", len(all))
	}
	for _, d := range all {
		switch d.ID {
		case "with-budget":
			if d.ContextBudgetTokens == nil || *d.ContextBudgetTokens != 65000 {
				t.Errorf("List: with-budget ContextBudgetTokens = %v, want 65000", d.ContextBudgetTokens)
			}
		case "disabled-budget":
			if d.ContextBudgetTokens == nil || *d.ContextBudgetTokens != 0 {
				t.Errorf("List: disabled-budget ContextBudgetTokens = %v, want non-nil 0", d.ContextBudgetTokens)
			}
		case "no-budget":
			if d.ContextBudgetTokens != nil {
				t.Errorf("List: no-budget ContextBudgetTokens = %v, want nil", d.ContextBudgetTokens)
			}
		}
	}
}

// TestAgentDefinition_ContextBudgetTokens_RoundTripsUpdate verifies
// AgentDefUpdateFields.ContextBudgetTokens sets the value when non-nil and is
// a no-op (like the stall-timeout fields, not tri-state like ReasoningEffort)
// when left nil on an unrelated update.
func TestAgentDefinition_ContextBudgetTokens_RoundTripsUpdate(t *testing.T) {
	t.Parallel()
	pool := newTestPool(t)
	seedProjectAndWorkflow(t, pool, "proj-ctxbudget2", "wf-ctxbudget2")

	r := NewAgentDefinitionRepo(pool, clock.Real())
	if err := r.Create(&model.AgentDefinition{
		ID: "agent-a", ProjectID: "proj-ctxbudget2", WorkflowID: "wf-ctxbudget2",
		ExecutionMode: "api", Layer: 0, Model: "sonnet-5",
	}); err != nil {
		t.Fatalf("create: %v", err)
	}

	// nil ContextBudgetTokens field: no-op.
	newTag := "some-tag"
	if err := r.Update("proj-ctxbudget2", "wf-ctxbudget2", "agent-a", &AgentDefUpdateFields{Tag: &newTag}); err != nil {
		t.Fatalf("Update (unrelated field): %v", err)
	}
	got, err := r.Get("proj-ctxbudget2", "wf-ctxbudget2", "agent-a")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got.ContextBudgetTokens != nil {
		t.Fatalf("ContextBudgetTokens after unrelated update = %v, want nil", got.ContextBudgetTokens)
	}

	budget := 42000
	if err := r.Update("proj-ctxbudget2", "wf-ctxbudget2", "agent-a", &AgentDefUpdateFields{
		ContextBudgetTokens: &budget,
	}); err != nil {
		t.Fatalf("Update ContextBudgetTokens=42000: %v", err)
	}
	got, err = r.Get("proj-ctxbudget2", "wf-ctxbudget2", "agent-a")
	if err != nil {
		t.Fatalf("Get after set: %v", err)
	}
	if got.ContextBudgetTokens == nil || *got.ContextBudgetTokens != 42000 {
		t.Errorf("ContextBudgetTokens after set = %v, want 42000", got.ContextBudgetTokens)
	}
}
