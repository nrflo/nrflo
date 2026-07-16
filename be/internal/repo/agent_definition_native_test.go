package repo

import (
	"testing"

	"be/internal/clock"
	"be/internal/model"
)

// TestAgentDefinition_NativeToolsSandbox_RoundTrip verifies native_tools and
// sandbox survive Create, Get, and List, and default to "" when unset.
func TestAgentDefinition_NativeToolsSandbox_RoundTrip(t *testing.T) {
	t.Parallel()
	pool := newTestPool(t)
	seedProjectAndWorkflow(t, pool, "proj-native", "wf-native")

	r := NewAgentDefinitionRepo(pool, clock.Real())

	if err := r.Create(&model.AgentDefinition{
		ID: "restricted", ProjectID: "proj-native", WorkflowID: "wf-native",
		ExecutionMode: "cli_interactive", Layer: 0, Model: "sonnet-5",
		NativeTools: "Read,Grep", Sandbox: model.SandboxReadOnly,
	}); err != nil {
		t.Fatalf("create restricted: %v", err)
	}
	if err := r.Create(&model.AgentDefinition{
		ID: "unrestricted", ProjectID: "proj-native", WorkflowID: "wf-native",
		ExecutionMode: "cli_interactive", Layer: 0, Model: "sonnet-5",
	}); err != nil {
		t.Fatalf("create unrestricted: %v", err)
	}

	got, err := r.Get("proj-native", "wf-native", "restricted")
	if err != nil {
		t.Fatalf("Get restricted: %v", err)
	}
	if got.NativeTools != "Read,Grep" || got.Sandbox != model.SandboxReadOnly {
		t.Errorf("Get: (native_tools, sandbox) = (%q, %q), want (Read,Grep, read-only)", got.NativeTools, got.Sandbox)
	}

	gotDefault, err := r.Get("proj-native", "wf-native", "unrestricted")
	if err != nil {
		t.Fatalf("Get unrestricted: %v", err)
	}
	if gotDefault.NativeTools != "" || gotDefault.Sandbox != "" {
		t.Errorf("Get default: (native_tools, sandbox) = (%q, %q), want empty", gotDefault.NativeTools, gotDefault.Sandbox)
	}

	all, err := r.List("proj-native", "wf-native")
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(all) != 2 {
		t.Fatalf("List count = %d, want 2", len(all))
	}
	for _, d := range all {
		if d.ID == "restricted" && (d.NativeTools != "Read,Grep" || d.Sandbox != model.SandboxReadOnly) {
			t.Errorf("List: restricted = (%q, %q), want (Read,Grep, read-only)", d.NativeTools, d.Sandbox)
		}
	}
}

// TestAgentDefinition_NativeToolsSandbox_Update verifies AgentDefUpdateFields
// pointer semantics: nil is a no-op, non-nil sets (including clear-to-"").
func TestAgentDefinition_NativeToolsSandbox_Update(t *testing.T) {
	t.Parallel()
	pool := newTestPool(t)
	seedProjectAndWorkflow(t, pool, "proj-native2", "wf-native2")

	r := NewAgentDefinitionRepo(pool, clock.Real())
	if err := r.Create(&model.AgentDefinition{
		ID: "agent-a", ProjectID: "proj-native2", WorkflowID: "wf-native2",
		ExecutionMode: "cli_interactive", Layer: 0, Model: "sonnet-5",
	}); err != nil {
		t.Fatalf("create: %v", err)
	}

	native := model.NativeToolsNone
	sandbox := model.SandboxWorkspaceWrite
	if err := r.Update("proj-native2", "wf-native2", "agent-a", &AgentDefUpdateFields{
		NativeTools: &native, Sandbox: &sandbox,
	}); err != nil {
		t.Fatalf("Update set: %v", err)
	}
	got, err := r.Get("proj-native2", "wf-native2", "agent-a")
	if err != nil {
		t.Fatalf("Get after set: %v", err)
	}
	if got.NativeTools != model.NativeToolsNone || got.Sandbox != model.SandboxWorkspaceWrite {
		t.Errorf("after set = (%q, %q), want (none, workspace-write)", got.NativeTools, got.Sandbox)
	}

	empty := ""
	if err := r.Update("proj-native2", "wf-native2", "agent-a", &AgentDefUpdateFields{
		NativeTools: &empty, Sandbox: &empty,
	}); err != nil {
		t.Fatalf("Update clear: %v", err)
	}
	got, err = r.Get("proj-native2", "wf-native2", "agent-a")
	if err != nil {
		t.Fatalf("Get after clear: %v", err)
	}
	if got.NativeTools != "" || got.Sandbox != "" {
		t.Errorf("after clear = (%q, %q), want empty", got.NativeTools, got.Sandbox)
	}
}
