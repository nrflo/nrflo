package integration

// End-to-end coverage for agent_definitions.system_template_id through the
// full server stack (real DB, real service layer, real project/workflow
// seeded by NewTestEnv): create, get, and update all persist and return the
// field, and an unknown id is rejected before it ever reaches the spawner.

import (
	"testing"

	"be/internal/types"
)

func TestAgentDef_SystemTemplateID_FullStackRoundTrip(t *testing.T) {
	env := NewTestEnv(t)
	svc := env.getAgentDefService(t)

	// Create with a system_template_id set to a seeded injectable template.
	def, err := svc.CreateAgentDef(env.ProjectID, "test", &types.AgentDefCreateRequest{
		ID:               "e2e-systempl-agent",
		Prompt:           "do work",
		SystemTemplateID: "tier-t2-extractor",
	})
	if err != nil {
		t.Fatalf("CreateAgentDef: %v", err)
	}
	if def.SystemTemplateID != "tier-t2-extractor" {
		t.Errorf("CreateAgentDef: SystemTemplateID = %q, want tier-t2-extractor", def.SystemTemplateID)
	}

	// Get reflects the persisted value.
	got, err := svc.GetAgentDef(env.ProjectID, "test", "e2e-systempl-agent")
	if err != nil {
		t.Fatalf("GetAgentDef: %v", err)
	}
	if got.SystemTemplateID != "tier-t2-extractor" {
		t.Errorf("GetAgentDef: SystemTemplateID = %q, want tier-t2-extractor", got.SystemTemplateID)
	}

	// Update to a different injectable template persists.
	newID := "tier-t0-decider"
	if err := svc.UpdateAgentDef(env.ProjectID, "test", "e2e-systempl-agent", &types.AgentDefUpdateRequest{
		SystemTemplateID: &newID,
	}); err != nil {
		t.Fatalf("UpdateAgentDef: %v", err)
	}
	afterUpdate, err := svc.GetAgentDef(env.ProjectID, "test", "e2e-systempl-agent")
	if err != nil {
		t.Fatalf("GetAgentDef after update: %v", err)
	}
	if afterUpdate.SystemTemplateID != "tier-t0-decider" {
		t.Errorf("GetAgentDef after update: SystemTemplateID = %q, want tier-t0-decider", afterUpdate.SystemTemplateID)
	}

	// An unrelated agent def created without system_template_id defaults to
	// empty — byte-identical to pre-feature behavior.
	plain, err := svc.CreateAgentDef(env.ProjectID, "test", &types.AgentDefCreateRequest{
		ID:     "e2e-nosystempl-agent",
		Prompt: "do other work",
	})
	if err != nil {
		t.Fatalf("CreateAgentDef (no system_template_id): %v", err)
	}
	if plain.SystemTemplateID != "" {
		t.Errorf("CreateAgentDef (no system_template_id): SystemTemplateID = %q, want empty", plain.SystemTemplateID)
	}

	// An unknown system_template_id is rejected at the service boundary,
	// before it could ever reach the spawner.
	bogus := "does-not-exist"
	if err := svc.UpdateAgentDef(env.ProjectID, "test", "e2e-systempl-agent", &types.AgentDefUpdateRequest{
		SystemTemplateID: &bogus,
	}); err == nil {
		t.Error("UpdateAgentDef with unknown system_template_id: want error, got nil")
	}
}
