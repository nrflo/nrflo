package repo

import (
	"testing"

	"be/internal/clock"
	"be/internal/model"
)

// TestAgentDefinition_SystemTemplateID_RoundTripsCreateGetList verifies
// SystemTemplateID survives Create, Get, and List, and defaults to "" (empty
// string, not NULL — the migration's NOT NULL DEFAULT ”) when unset on Create.
func TestAgentDefinition_SystemTemplateID_RoundTripsCreateGetList(t *testing.T) {
	t.Parallel()
	pool := newTestPool(t)
	seedProjectAndWorkflow(t, pool, "proj-systempl", "wf-systempl")

	r := NewAgentDefinitionRepo(pool, clock.Real())

	if err := r.Create(&model.AgentDefinition{
		ID: "with-systempl", ProjectID: "proj-systempl", WorkflowID: "wf-systempl",
		ExecutionMode: "cli_interactive", Layer: 0, Model: "opus-4-8",
		SystemTemplateID: "tier-t2-extractor",
	}); err != nil {
		t.Fatalf("create with-systempl: %v", err)
	}
	if err := r.Create(&model.AgentDefinition{
		ID: "no-systempl", ProjectID: "proj-systempl", WorkflowID: "wf-systempl",
		ExecutionMode: "cli_interactive", Layer: 0, Model: "sonnet-5",
	}); err != nil {
		t.Fatalf("create no-systempl: %v", err)
	}

	got, err := r.Get("proj-systempl", "wf-systempl", "with-systempl")
	if err != nil {
		t.Fatalf("Get with-systempl: %v", err)
	}
	if got.SystemTemplateID != "tier-t2-extractor" {
		t.Errorf("SystemTemplateID = %q, want tier-t2-extractor", got.SystemTemplateID)
	}

	gotDefault, err := r.Get("proj-systempl", "wf-systempl", "no-systempl")
	if err != nil {
		t.Fatalf("Get no-systempl: %v", err)
	}
	if gotDefault.SystemTemplateID != "" {
		t.Errorf("SystemTemplateID = %q, want empty (default when unset on Create)", gotDefault.SystemTemplateID)
	}

	all, err := r.List("proj-systempl", "wf-systempl")
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(all) != 2 {
		t.Fatalf("List count = %d, want 2", len(all))
	}
	for _, d := range all {
		if d.ID == "with-systempl" && d.SystemTemplateID != "tier-t2-extractor" {
			t.Errorf("List: with-systempl SystemTemplateID = %q, want tier-t2-extractor", d.SystemTemplateID)
		}
		if d.ID == "no-systempl" && d.SystemTemplateID != "" {
			t.Errorf("List: no-systempl SystemTemplateID = %q, want empty", d.SystemTemplateID)
		}
	}
}

// TestAgentDefinition_SystemTemplateID_RoundTripsUpdate verifies
// AgentDefUpdateFields.SystemTemplateID: nil is a no-op, a non-nil pointer
// (including a pointer to "") sets the value.
func TestAgentDefinition_SystemTemplateID_RoundTripsUpdate(t *testing.T) {
	t.Parallel()
	pool := newTestPool(t)
	seedProjectAndWorkflow(t, pool, "proj-systempl2", "wf-systempl2")

	r := NewAgentDefinitionRepo(pool, clock.Real())
	if err := r.Create(&model.AgentDefinition{
		ID: "agent-b", ProjectID: "proj-systempl2", WorkflowID: "wf-systempl2",
		ExecutionMode: "cli_interactive", Layer: 0, Model: "opus-4-8",
	}); err != nil {
		t.Fatalf("create: %v", err)
	}

	// nil SystemTemplateID field: no-op.
	newTag := "some-tag"
	if err := r.Update("proj-systempl2", "wf-systempl2", "agent-b", &AgentDefUpdateFields{Tag: &newTag}); err != nil {
		t.Fatalf("Update (unrelated field): %v", err)
	}
	got, err := r.Get("proj-systempl2", "wf-systempl2", "agent-b")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got.SystemTemplateID != "" {
		t.Fatalf("SystemTemplateID after unrelated update = %q, want empty", got.SystemTemplateID)
	}

	// Non-nil pointer: sets the value.
	tierT0 := "tier-t0-decider"
	if err := r.Update("proj-systempl2", "wf-systempl2", "agent-b", &AgentDefUpdateFields{
		SystemTemplateID: &tierT0,
	}); err != nil {
		t.Fatalf("Update SystemTemplateID=tier-t0-decider: %v", err)
	}
	got, err = r.Get("proj-systempl2", "wf-systempl2", "agent-b")
	if err != nil {
		t.Fatalf("Get after set: %v", err)
	}
	if got.SystemTemplateID != "tier-t0-decider" {
		t.Errorf("SystemTemplateID after set = %q, want tier-t0-decider", got.SystemTemplateID)
	}

	// Pointer to empty string: explicitly reverts to "".
	empty := ""
	if err := r.Update("proj-systempl2", "wf-systempl2", "agent-b", &AgentDefUpdateFields{
		SystemTemplateID: &empty,
	}); err != nil {
		t.Fatalf("Update SystemTemplateID=\"\": %v", err)
	}
	got, err = r.Get("proj-systempl2", "wf-systempl2", "agent-b")
	if err != nil {
		t.Fatalf("Get after revert: %v", err)
	}
	if got.SystemTemplateID != "" {
		t.Errorf("SystemTemplateID after explicit revert = %q, want empty", got.SystemTemplateID)
	}
}
