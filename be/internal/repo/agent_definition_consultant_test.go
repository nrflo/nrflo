package repo

import (
	"testing"

	"be/internal/clock"
	"be/internal/model"
)

// TestFindConsultant_PrefersProjectOwnRow_OverGlobal verifies a
// project-local consultant def with the same id as a '__global__' one wins —
// the console consult tool's hidden-host path has no single caller-known
// workflow, so FindConsultant must rank the project's own row first.
func TestFindConsultant_PrefersProjectOwnRow_OverGlobal(t *testing.T) {
	t.Parallel()
	pool := newTestPool(t)
	seedProjectAndWorkflow(t, pool, "proj-fc1", "wf-fc1")
	seedProjectAndWorkflow(t, pool, "__global__", "wf-fc1-global")

	r := NewAgentDefinitionRepo(pool, clock.Real())
	if err := r.Create(&model.AgentDefinition{
		ID: "security-expert", ProjectID: "__global__", WorkflowID: "wf-fc1-global",
		ExecutionMode: "api", Layer: 0, Consultant: true, Description: "global row",
	}); err != nil {
		t.Fatalf("create global consultant: %v", err)
	}
	if err := r.Create(&model.AgentDefinition{
		ID: "security-expert", ProjectID: "proj-fc1", WorkflowID: "wf-fc1",
		ExecutionMode: "api", Layer: 0, Consultant: true, Description: "project row",
	}); err != nil {
		t.Fatalf("create project consultant: %v", err)
	}

	def, err := r.FindConsultant("proj-fc1", "security-expert")
	if err != nil {
		t.Fatalf("FindConsultant: %v", err)
	}
	if def.ProjectID != "proj-fc1" || def.Description != "project row" {
		t.Errorf("FindConsultant = %+v, want the project's own row", def)
	}
}

// TestFindConsultant_FallsBackToGlobal_WhenNoProjectRow verifies a project
// with no matching consultant def resolves the '__global__' one instead.
func TestFindConsultant_FallsBackToGlobal_WhenNoProjectRow(t *testing.T) {
	t.Parallel()
	pool := newTestPool(t)
	seedProjectAndWorkflow(t, pool, "proj-fc2", "wf-fc2")
	seedProjectAndWorkflow(t, pool, "__global__", "wf-fc2-global")

	r := NewAgentDefinitionRepo(pool, clock.Real())
	if err := r.Create(&model.AgentDefinition{
		ID: "docs-expert", ProjectID: "__global__", WorkflowID: "wf-fc2-global",
		ExecutionMode: "api", Layer: 0, Consultant: true,
	}); err != nil {
		t.Fatalf("create global consultant: %v", err)
	}

	def, err := r.FindConsultant("proj-fc2", "docs-expert")
	if err != nil {
		t.Fatalf("FindConsultant: %v", err)
	}
	if def.ProjectID != "__global__" {
		t.Errorf("FindConsultant.ProjectID = %q, want __global__", def.ProjectID)
	}
}

// TestFindConsultant_UnknownID_ReturnsError verifies no matching row (in
// either scope) is a hard error, not a nil/empty result.
func TestFindConsultant_UnknownID_ReturnsError(t *testing.T) {
	t.Parallel()
	pool := newTestPool(t)
	seedProjectAndWorkflow(t, pool, "proj-fc3", "wf-fc3")

	r := NewAgentDefinitionRepo(pool, clock.Real())
	if _, err := r.FindConsultant("proj-fc3", "no-such-consultant"); err == nil {
		t.Error("FindConsultant(unknown id) = nil error, want an error")
	}
}

// TestFindConsultant_ExcludesNonConsultantDef verifies a same-id def that
// isn't flagged consultant=1 is never returned, even when it's the only row.
func TestFindConsultant_ExcludesNonConsultantDef(t *testing.T) {
	t.Parallel()
	pool := newTestPool(t)
	seedProjectAndWorkflow(t, pool, "proj-fc4", "wf-fc4")

	r := NewAgentDefinitionRepo(pool, clock.Real())
	if err := r.Create(&model.AgentDefinition{
		ID: "implementor", ProjectID: "proj-fc4", WorkflowID: "wf-fc4",
		ExecutionMode: "cli_interactive", Layer: 0, Consultant: false,
	}); err != nil {
		t.Fatalf("create non-consultant def: %v", err)
	}

	if _, err := r.FindConsultant("proj-fc4", "implementor"); err == nil {
		t.Error("FindConsultant on a non-consultant def = nil error, want an error")
	}
}
