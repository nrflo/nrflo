package repo

import (
	"testing"

	"be/internal/clock"
	"be/internal/model"
)

// TestListExecutable_ExcludesNodeRole verifies that ListExecutable excludes
// planner and fanout_template defs (same as the consultant exclusion), while
// List still returns them.
func TestListExecutable_ExcludesNodeRole(t *testing.T) {
	t.Parallel()
	pool := newTestPool(t)
	seedProjectAndWorkflow(t, pool, "proj-nr", "wf-nr")

	r := NewAgentDefinitionRepo(pool, clock.Real())

	if err := r.Create(&model.AgentDefinition{
		ID: "real-agent", ProjectID: "proj-nr", WorkflowID: "wf-nr",
		ExecutionMode: "cli_interactive", Layer: 0, NodeRole: "static",
	}); err != nil {
		t.Fatalf("create static agent: %v", err)
	}
	if err := r.Create(&model.AgentDefinition{
		ID: "planner-agent", ProjectID: "proj-nr", WorkflowID: "wf-nr",
		ExecutionMode: "api", Layer: 0, NodeRole: "planner",
	}); err != nil {
		t.Fatalf("create planner agent: %v", err)
	}
	if err := r.Create(&model.AgentDefinition{
		ID: "fanout-agent", ProjectID: "proj-nr", WorkflowID: "wf-nr",
		ExecutionMode: "api", Layer: 0, NodeRole: "fanout_template",
	}); err != nil {
		t.Fatalf("create fanout_template agent: %v", err)
	}
	if err := r.Create(&model.AgentDefinition{
		ID: "consultant-agent", ProjectID: "proj-nr", WorkflowID: "wf-nr",
		ExecutionMode: "api", Layer: 0, Consultant: true, NodeRole: "static",
	}); err != nil {
		t.Fatalf("create consultant agent: %v", err)
	}

	all, err := r.List("proj-nr", "wf-nr")
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(all) != 4 {
		t.Errorf("List count = %d, want 4 (List must return every node_role)", len(all))
	}

	executable, err := r.ListExecutable("proj-nr", "wf-nr")
	if err != nil {
		t.Fatalf("ListExecutable: %v", err)
	}
	if len(executable) != 1 {
		t.Fatalf("ListExecutable count = %d, want 1", len(executable))
	}
	if executable[0].ID != "real-agent" {
		t.Errorf("ListExecutable[0].ID = %q, want real-agent", executable[0].ID)
	}
}

// TestListExecutable_EmptyWhenAllNonStatic verifies ListExecutable returns
// empty when every def in the workflow is planner/fanout_template/consultant.
func TestListExecutable_EmptyWhenAllNonStatic(t *testing.T) {
	t.Parallel()
	pool := newTestPool(t)
	seedProjectAndWorkflow(t, pool, "proj-nr2", "wf-nr2")

	r := NewAgentDefinitionRepo(pool, clock.Real())

	if err := r.Create(&model.AgentDefinition{
		ID: "planner-agent", ProjectID: "proj-nr2", WorkflowID: "wf-nr2",
		ExecutionMode: "api", Layer: 0, NodeRole: "planner",
	}); err != nil {
		t.Fatalf("create planner agent: %v", err)
	}
	if err := r.Create(&model.AgentDefinition{
		ID: "fanout-agent", ProjectID: "proj-nr2", WorkflowID: "wf-nr2",
		ExecutionMode: "api", Layer: 1, NodeRole: "fanout_template",
	}); err != nil {
		t.Fatalf("create fanout_template agent: %v", err)
	}

	executable, err := r.ListExecutable("proj-nr2", "wf-nr2")
	if err != nil {
		t.Fatalf("ListExecutable: %v", err)
	}
	if len(executable) != 0 {
		t.Errorf("ListExecutable count = %d, want 0", len(executable))
	}
}

// TestAgentDefinition_NodeRole_RoundTripsCreateGet verifies NodeRole survives
// Create then Get, and defaults to "static" when left unset.
func TestAgentDefinition_NodeRole_RoundTripsCreateGet(t *testing.T) {
	t.Parallel()
	pool := newTestPool(t)
	seedProjectAndWorkflow(t, pool, "proj-nr3", "wf-nr3")

	r := NewAgentDefinitionRepo(pool, clock.Real())

	if err := r.Create(&model.AgentDefinition{
		ID: "explicit-planner", ProjectID: "proj-nr3", WorkflowID: "wf-nr3",
		ExecutionMode: "api", Layer: 0, NodeRole: "planner",
	}); err != nil {
		t.Fatalf("create: %v", err)
	}
	if err := r.Create(&model.AgentDefinition{
		ID: "defaulted", ProjectID: "proj-nr3", WorkflowID: "wf-nr3",
		ExecutionMode: "cli_interactive", Layer: 0,
	}); err != nil {
		t.Fatalf("create (no node_role set): %v", err)
	}

	got, err := r.Get("proj-nr3", "wf-nr3", "explicit-planner")
	if err != nil {
		t.Fatalf("Get explicit-planner: %v", err)
	}
	if got.NodeRole != "planner" {
		t.Errorf("NodeRole = %q, want planner", got.NodeRole)
	}

	gotDefault, err := r.Get("proj-nr3", "wf-nr3", "defaulted")
	if err != nil {
		t.Fatalf("Get defaulted: %v", err)
	}
	if gotDefault.NodeRole != "static" {
		t.Errorf("NodeRole = %q, want static (default when unset on Create)", gotDefault.NodeRole)
	}
}

// TestAgentDefinition_NodeRole_RoundTripsUpdate verifies NodeRole survives an
// Update call and that the AgentDefUpdateFields.NodeRole=nil is a no-op.
func TestAgentDefinition_NodeRole_RoundTripsUpdate(t *testing.T) {
	t.Parallel()
	pool := newTestPool(t)
	seedProjectAndWorkflow(t, pool, "proj-nr4", "wf-nr4")

	r := NewAgentDefinitionRepo(pool, clock.Real())
	if err := r.Create(&model.AgentDefinition{
		ID: "agent-a", ProjectID: "proj-nr4", WorkflowID: "wf-nr4",
		ExecutionMode: "cli_interactive", Layer: 0, NodeRole: "static",
	}); err != nil {
		t.Fatalf("create: %v", err)
	}

	newRole := "fanout_template"
	if err := r.Update("proj-nr4", "wf-nr4", "agent-a", &AgentDefUpdateFields{NodeRole: &newRole}); err != nil {
		t.Fatalf("Update NodeRole: %v", err)
	}
	got, err := r.Get("proj-nr4", "wf-nr4", "agent-a")
	if err != nil {
		t.Fatalf("Get after update: %v", err)
	}
	if got.NodeRole != "fanout_template" {
		t.Errorf("NodeRole after update = %q, want fanout_template", got.NodeRole)
	}

	// Update omitting NodeRole must leave the stored value unchanged.
	newTag := "some-tag"
	if err := r.Update("proj-nr4", "wf-nr4", "agent-a", &AgentDefUpdateFields{Tag: &newTag}); err != nil {
		t.Fatalf("Update Tag only: %v", err)
	}
	gotAfter, err := r.Get("proj-nr4", "wf-nr4", "agent-a")
	if err != nil {
		t.Fatalf("Get after tag-only update: %v", err)
	}
	if gotAfter.NodeRole != "fanout_template" {
		t.Errorf("NodeRole after unrelated update = %q, want unchanged fanout_template", gotAfter.NodeRole)
	}
}
