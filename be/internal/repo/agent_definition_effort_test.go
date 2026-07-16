package repo

import (
	"database/sql"
	"testing"

	"be/internal/clock"
	"be/internal/model"
)

// TestAgentDefinition_ReasoningEffort_RoundTripsCreateGetList verifies
// ReasoningEffort survives Create, Get, and List, and defaults to nil
// (NULL, i.e. inherit from the model row) when unset on Create.
func TestAgentDefinition_ReasoningEffort_RoundTripsCreateGetList(t *testing.T) {
	t.Parallel()
	pool := newTestPool(t)
	seedProjectAndWorkflow(t, pool, "proj-effort", "wf-effort")

	r := NewAgentDefinitionRepo(pool, clock.Real())

	effort := "xhigh"
	if err := r.Create(&model.AgentDefinition{
		ID: "with-effort", ProjectID: "proj-effort", WorkflowID: "wf-effort",
		ExecutionMode: "cli_interactive", Layer: 0, Model: "opus-4-8",
		ReasoningEffort: &effort,
	}); err != nil {
		t.Fatalf("create with-effort: %v", err)
	}
	if err := r.Create(&model.AgentDefinition{
		ID: "no-effort", ProjectID: "proj-effort", WorkflowID: "wf-effort",
		ExecutionMode: "cli_interactive", Layer: 0, Model: "sonnet-5",
	}); err != nil {
		t.Fatalf("create no-effort: %v", err)
	}

	got, err := r.Get("proj-effort", "wf-effort", "with-effort")
	if err != nil {
		t.Fatalf("Get with-effort: %v", err)
	}
	if got.ReasoningEffort == nil || *got.ReasoningEffort != "xhigh" {
		t.Errorf("ReasoningEffort = %v, want xhigh", got.ReasoningEffort)
	}

	gotDefault, err := r.Get("proj-effort", "wf-effort", "no-effort")
	if err != nil {
		t.Fatalf("Get no-effort: %v", err)
	}
	if gotDefault.ReasoningEffort != nil {
		t.Errorf("ReasoningEffort = %v, want nil (default when unset on Create)", gotDefault.ReasoningEffort)
	}

	all, err := r.List("proj-effort", "wf-effort")
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(all) != 2 {
		t.Fatalf("List count = %d, want 2", len(all))
	}
	for _, d := range all {
		if d.ID == "with-effort" && (d.ReasoningEffort == nil || *d.ReasoningEffort != "xhigh") {
			t.Errorf("List: with-effort ReasoningEffort = %v, want xhigh", d.ReasoningEffort)
		}
		if d.ID == "no-effort" && d.ReasoningEffort != nil {
			t.Errorf("List: no-effort ReasoningEffort = %v, want nil", d.ReasoningEffort)
		}
	}
}

// TestAgentDefinition_ReasoningEffort_RoundTripsUpdate verifies
// AgentDefUpdateFields.ReasoningEffort's tri-state semantics: nil is a no-op,
// a Valid=true sql.NullString sets the value, and a Valid=false
// sql.NullString explicitly writes NULL (reverts to inherit).
func TestAgentDefinition_ReasoningEffort_RoundTripsUpdate(t *testing.T) {
	t.Parallel()
	pool := newTestPool(t)
	seedProjectAndWorkflow(t, pool, "proj-effort2", "wf-effort2")

	r := NewAgentDefinitionRepo(pool, clock.Real())
	if err := r.Create(&model.AgentDefinition{
		ID: "agent-a", ProjectID: "proj-effort2", WorkflowID: "wf-effort2",
		ExecutionMode: "cli_interactive", Layer: 0, Model: "opus-4-8",
	}); err != nil {
		t.Fatalf("create: %v", err)
	}

	// nil ReasoningEffort field: no-op.
	newTag := "some-tag"
	if err := r.Update("proj-effort2", "wf-effort2", "agent-a", &AgentDefUpdateFields{Tag: &newTag}); err != nil {
		t.Fatalf("Update (unrelated field): %v", err)
	}
	got, err := r.Get("proj-effort2", "wf-effort2", "agent-a")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got.ReasoningEffort != nil {
		t.Fatalf("ReasoningEffort after unrelated update = %v, want nil", got.ReasoningEffort)
	}

	// Valid=true sql.NullString: sets the value.
	if err := r.Update("proj-effort2", "wf-effort2", "agent-a", &AgentDefUpdateFields{
		ReasoningEffort: &sql.NullString{String: "high", Valid: true},
	}); err != nil {
		t.Fatalf("Update ReasoningEffort=high: %v", err)
	}
	got, err = r.Get("proj-effort2", "wf-effort2", "agent-a")
	if err != nil {
		t.Fatalf("Get after set: %v", err)
	}
	if got.ReasoningEffort == nil || *got.ReasoningEffort != "high" {
		t.Errorf("ReasoningEffort after set = %v, want high", got.ReasoningEffort)
	}

	// Valid=false sql.NullString: explicitly reverts to NULL (inherit).
	if err := r.Update("proj-effort2", "wf-effort2", "agent-a", &AgentDefUpdateFields{
		ReasoningEffort: &sql.NullString{Valid: false},
	}); err != nil {
		t.Fatalf("Update ReasoningEffort=NULL: %v", err)
	}
	got, err = r.Get("proj-effort2", "wf-effort2", "agent-a")
	if err != nil {
		t.Fatalf("Get after revert: %v", err)
	}
	if got.ReasoningEffort != nil {
		t.Errorf("ReasoningEffort after explicit revert-to-NULL = %v, want nil", got.ReasoningEffort)
	}
}
