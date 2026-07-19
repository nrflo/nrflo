package service

// Tests for AgentDefinitionService's system_template_id validation: '' is
// always accepted (mode default / global override gate); a non-empty value
// must resolve to an existing type='injectable' default_templates row.

import (
	"testing"

	"be/internal/types"
)

func TestCreateAgentDef_SystemTemplateID_ValidationMatrix(t *testing.T) {
	t.Parallel()
	_, svc, wfID := setupAgentDefTestEnv(t, nil)

	cases := []struct {
		name    string
		id      string
		wantErr bool
	}{
		{"empty accepted", "", false},
		{"existing injectable accepted", "tier-t2-extractor", false},
		{"another existing injectable accepted", "tier-t0-decider", false},
		{"unknown id rejected", "does-not-exist", true},
		{"agent-type row rejected (not injectable)", "implementor", true},
	}

	for i, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			id := "systempl-agent-" + string(rune('a'+i))
			def, err := svc.CreateAgentDef("proj1", wfID, &types.AgentDefCreateRequest{
				ID:               id,
				Prompt:           "do work",
				SystemTemplateID: tc.id,
			})
			if tc.wantErr {
				if err == nil {
					t.Fatalf("CreateAgentDef(system_template_id=%q): expected error, got nil", tc.id)
				}
				return
			}
			if err != nil {
				t.Fatalf("CreateAgentDef(system_template_id=%q): %v", tc.id, err)
			}
			if def.SystemTemplateID != tc.id {
				t.Errorf("SystemTemplateID = %q, want %q", def.SystemTemplateID, tc.id)
			}
		})
	}
}

func TestUpdateAgentDef_SystemTemplateID_DirectPatchValidated(t *testing.T) {
	t.Parallel()
	_, svc, wfID := setupAgentDefTestEnv(t, nil)

	if _, err := svc.CreateAgentDef("proj1", wfID, &types.AgentDefCreateRequest{
		ID:     "patch-systempl",
		Prompt: "do work",
	}); err != nil {
		t.Fatalf("create: %v", err)
	}

	bad := "does-not-exist"
	if err := svc.UpdateAgentDef("proj1", wfID, "patch-systempl", &types.AgentDefUpdateRequest{
		SystemTemplateID: &bad,
	}); err == nil {
		t.Fatal("UpdateAgentDef(system_template_id=does-not-exist): expected error, got nil")
	}

	good := "tier-t1-executor"
	if err := svc.UpdateAgentDef("proj1", wfID, "patch-systempl", &types.AgentDefUpdateRequest{
		SystemTemplateID: &good,
	}); err != nil {
		t.Fatalf("UpdateAgentDef(system_template_id=tier-t1-executor): %v", err)
	}
	def, err := svc.GetAgentDef("proj1", wfID, "patch-systempl")
	if err != nil {
		t.Fatalf("GetAgentDef: %v", err)
	}
	if def.SystemTemplateID != "tier-t1-executor" {
		t.Errorf("SystemTemplateID = %q, want tier-t1-executor", def.SystemTemplateID)
	}

	// Revert to empty (mode default / global override gate).
	empty := ""
	if err := svc.UpdateAgentDef("proj1", wfID, "patch-systempl", &types.AgentDefUpdateRequest{
		SystemTemplateID: &empty,
	}); err != nil {
		t.Fatalf("UpdateAgentDef(system_template_id=\"\"): %v", err)
	}
	def, err = svc.GetAgentDef("proj1", wfID, "patch-systempl")
	if err != nil {
		t.Fatalf("GetAgentDef after revert: %v", err)
	}
	if def.SystemTemplateID != "" {
		t.Errorf("SystemTemplateID after revert = %q, want empty", def.SystemTemplateID)
	}
}

func TestUpdateAgentDef_SystemTemplateID_NilField_NoOp(t *testing.T) {
	t.Parallel()
	_, svc, wfID := setupAgentDefTestEnv(t, nil)

	if _, err := svc.CreateAgentDef("proj1", wfID, &types.AgentDefCreateRequest{
		ID:               "patch-systempl-noop",
		Prompt:           "do work",
		SystemTemplateID: "tier-t0-decider",
	}); err != nil {
		t.Fatalf("create: %v", err)
	}

	newDesc := "unrelated description update"
	if err := svc.UpdateAgentDef("proj1", wfID, "patch-systempl-noop", &types.AgentDefUpdateRequest{
		Description: &newDesc,
	}); err != nil {
		t.Fatalf("UpdateAgentDef (unrelated field): %v", err)
	}
	def, err := svc.GetAgentDef("proj1", wfID, "patch-systempl-noop")
	if err != nil {
		t.Fatalf("GetAgentDef: %v", err)
	}
	if def.SystemTemplateID != "tier-t0-decider" {
		t.Errorf("SystemTemplateID after unrelated update = %q, want unchanged tier-t0-decider", def.SystemTemplateID)
	}
}
