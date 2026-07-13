package repo

import (
	"testing"

	"be/internal/clock"
	"be/internal/model"
)

// TestAgentDefinition_Description_RoundTripsCreateGetList verifies
// Description survives Create, Get, and List, and defaults to "" when unset.
func TestAgentDefinition_Description_RoundTripsCreateGetList(t *testing.T) {
	t.Parallel()
	pool := newTestPool(t)
	seedProjectAndWorkflow(t, pool, "proj-desc", "wf-desc")

	r := NewAgentDefinitionRepo(pool, clock.Real())

	if err := r.Create(&model.AgentDefinition{
		ID: "with-desc", ProjectID: "proj-desc", WorkflowID: "wf-desc",
		ExecutionMode: "cli_interactive", Layer: 0, NodeRole: "fanout_template",
		Description: "Explores the codebase and reports findings.",
	}); err != nil {
		t.Fatalf("create with-desc: %v", err)
	}
	if err := r.Create(&model.AgentDefinition{
		ID: "no-desc", ProjectID: "proj-desc", WorkflowID: "wf-desc",
		ExecutionMode: "cli_interactive", Layer: 0,
	}); err != nil {
		t.Fatalf("create no-desc: %v", err)
	}

	got, err := r.Get("proj-desc", "wf-desc", "with-desc")
	if err != nil {
		t.Fatalf("Get with-desc: %v", err)
	}
	if got.Description != "Explores the codebase and reports findings." {
		t.Errorf("Description = %q, want the seeded description", got.Description)
	}

	gotDefault, err := r.Get("proj-desc", "wf-desc", "no-desc")
	if err != nil {
		t.Fatalf("Get no-desc: %v", err)
	}
	if gotDefault.Description != "" {
		t.Errorf("Description = %q, want empty (default when unset on Create)", gotDefault.Description)
	}

	all, err := r.List("proj-desc", "wf-desc")
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(all) != 2 {
		t.Fatalf("List count = %d, want 2", len(all))
	}
	for _, d := range all {
		if d.ID == "with-desc" && d.Description == "" {
			t.Error("List: with-desc lost its description")
		}
	}
}

// TestAgentDefinition_Description_RoundTripsUpdate verifies Description
// survives an Update call and that AgentDefUpdateFields.Description=nil is a
// no-op.
func TestAgentDefinition_Description_RoundTripsUpdate(t *testing.T) {
	t.Parallel()
	pool := newTestPool(t)
	seedProjectAndWorkflow(t, pool, "proj-desc2", "wf-desc2")

	r := NewAgentDefinitionRepo(pool, clock.Real())
	if err := r.Create(&model.AgentDefinition{
		ID: "agent-a", ProjectID: "proj-desc2", WorkflowID: "wf-desc2",
		ExecutionMode: "cli_interactive", Layer: 0,
	}); err != nil {
		t.Fatalf("create: %v", err)
	}

	newDesc := "updated description"
	if err := r.Update("proj-desc2", "wf-desc2", "agent-a", &AgentDefUpdateFields{Description: &newDesc}); err != nil {
		t.Fatalf("Update Description: %v", err)
	}
	got, err := r.Get("proj-desc2", "wf-desc2", "agent-a")
	if err != nil {
		t.Fatalf("Get after update: %v", err)
	}
	if got.Description != "updated description" {
		t.Errorf("Description after update = %q, want %q", got.Description, "updated description")
	}

	newTag := "some-tag"
	if err := r.Update("proj-desc2", "wf-desc2", "agent-a", &AgentDefUpdateFields{Tag: &newTag}); err != nil {
		t.Fatalf("Update Tag only: %v", err)
	}
	gotAfter, err := r.Get("proj-desc2", "wf-desc2", "agent-a")
	if err != nil {
		t.Fatalf("Get after tag-only update: %v", err)
	}
	if gotAfter.Description != "updated description" {
		t.Errorf("Description after unrelated update = %q, want unchanged", gotAfter.Description)
	}
}
