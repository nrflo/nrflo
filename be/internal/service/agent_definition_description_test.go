package service

import (
	"testing"

	"be/internal/types"
)

// TestCreateAgentDef_Description_FanoutTemplateRequiresNonEmpty verifies a
// fanout_template def with an empty description is rejected — it is the
// planner's (and plan UI's) selection surface, so an undescribed template is
// effectively unusable.
func TestCreateAgentDef_Description_FanoutTemplateRequiresNonEmpty(t *testing.T) {
	t.Parallel()
	svc, _, wfID := setupAgentDefAPIModeEnv(t)

	_, err := svc.CreateAgentDef("proj1", wfID, &types.AgentDefCreateRequest{
		ID:       "tpl-no-desc",
		Prompt:   "do work",
		NodeRole: "fanout_template",
	})
	if err == nil {
		t.Fatal("CreateAgentDef(node_role=fanout_template, description=''): expected error, got nil")
	}
}

// TestCreateAgentDef_Description_StaticDefOptional verifies a static def
// (the default node_role) with an empty description is accepted — the
// requirement is scoped to fanout_template rows only.
func TestCreateAgentDef_Description_StaticDefOptional(t *testing.T) {
	t.Parallel()
	svc, _, wfID := setupAgentDefAPIModeEnv(t)

	def, err := svc.CreateAgentDef("proj1", wfID, &types.AgentDefCreateRequest{
		ID:     "static-no-desc",
		Prompt: "do work",
	})
	if err != nil {
		t.Fatalf("CreateAgentDef(static, description=''): %v", err)
	}
	if def.Description != "" {
		t.Errorf("Description = %q, want empty", def.Description)
	}
}

// TestCreateAgentDef_Description_RoundTripsThroughGetAndList verifies the
// description survives Create, Get, and List.
func TestCreateAgentDef_Description_RoundTripsThroughGetAndList(t *testing.T) {
	t.Parallel()
	svc, _, wfID := setupAgentDefAPIModeEnv(t)

	const desc = "Reviews a module and emits a pass/fail verdict."
	created, err := svc.CreateAgentDef("proj1", wfID, &types.AgentDefCreateRequest{
		ID:          "tpl-with-desc",
		Prompt:      "do work",
		NodeRole:    "fanout_template",
		Description: desc,
	})
	if err != nil {
		t.Fatalf("CreateAgentDef: %v", err)
	}
	if created.Description != desc {
		t.Errorf("Create Description = %q, want %q", created.Description, desc)
	}

	got, err := svc.GetAgentDef("proj1", wfID, "tpl-with-desc")
	if err != nil {
		t.Fatalf("GetAgentDef: %v", err)
	}
	if got.Description != desc {
		t.Errorf("Get Description = %q, want %q", got.Description, desc)
	}

	list, err := svc.ListAgentDefs("proj1", wfID)
	if err != nil {
		t.Fatalf("ListAgentDefs: %v", err)
	}
	found := false
	for _, d := range list {
		if d.ID == "tpl-with-desc" {
			found = true
			if d.Description != desc {
				t.Errorf("List Description = %q, want %q", d.Description, desc)
			}
		}
	}
	if !found {
		t.Fatal("tpl-with-desc missing from ListAgentDefs")
	}
}

// TestUpdateAgentDef_Description_PatchRoundTrips verifies a PATCH updating
// description alone persists.
func TestUpdateAgentDef_Description_PatchRoundTrips(t *testing.T) {
	t.Parallel()
	svc, _, wfID := setupAgentDefAPIModeEnv(t)

	if _, err := svc.CreateAgentDef("proj1", wfID, &types.AgentDefCreateRequest{
		ID:     "agent-patch-desc",
		Prompt: "do stuff",
	}); err != nil {
		t.Fatalf("create: %v", err)
	}

	newDesc := "Now carries a description."
	if err := svc.UpdateAgentDef("proj1", wfID, "agent-patch-desc", &types.AgentDefUpdateRequest{
		Description: &newDesc,
	}); err != nil {
		t.Fatalf("UpdateAgentDef(description): %v", err)
	}

	def, err := svc.GetAgentDef("proj1", wfID, "agent-patch-desc")
	if err != nil {
		t.Fatalf("GetAgentDef: %v", err)
	}
	if def.Description != newDesc {
		t.Errorf("Description after update = %q, want %q", def.Description, newDesc)
	}
}

// TestUpdateAgentDef_Description_FlipToFanoutTemplateRequiresDescription
// verifies a PATCH that flips node_role to fanout_template on a def with no
// (or blank) description is rejected, even though the request itself may not
// touch the description field — the effective-value re-validation mirrors
// the consultant+node_role pattern.
func TestUpdateAgentDef_Description_FlipToFanoutTemplateRequiresDescription(t *testing.T) {
	t.Parallel()
	svc, _, wfID := setupAgentDefAPIModeEnv(t)

	if _, err := svc.CreateAgentDef("proj1", wfID, &types.AgentDefCreateRequest{
		ID:     "static-to-fanout",
		Prompt: "do stuff",
	}); err != nil {
		t.Fatalf("create: %v", err)
	}

	newRole := "fanout_template"
	err := svc.UpdateAgentDef("proj1", wfID, "static-to-fanout", &types.AgentDefUpdateRequest{
		NodeRole: &newRole,
	})
	if err == nil {
		t.Fatal("UpdateAgentDef(node_role=fanout_template, no description): expected error, got nil")
	}
}

// TestUpdateAgentDef_Description_BlankingExistingFanoutTemplateRejected
// verifies a PATCH that blanks the description on an existing
// fanout_template row is rejected.
func TestUpdateAgentDef_Description_BlankingExistingFanoutTemplateRejected(t *testing.T) {
	t.Parallel()
	svc, _, wfID := setupAgentDefAPIModeEnv(t)

	if _, err := svc.CreateAgentDef("proj1", wfID, &types.AgentDefCreateRequest{
		ID:          "fanout-blank-desc",
		Prompt:      "do stuff",
		NodeRole:    "fanout_template",
		Description: "has a description",
	}); err != nil {
		t.Fatalf("create: %v", err)
	}

	blank := ""
	err := svc.UpdateAgentDef("proj1", wfID, "fanout-blank-desc", &types.AgentDefUpdateRequest{
		Description: &blank,
	})
	if err == nil {
		t.Fatal("UpdateAgentDef(description='') on existing fanout_template: expected error, got nil")
	}
}

// TestUpdateAgentDef_Description_OmittedFieldIsNoOp verifies a PATCH that
// omits description leaves the stored value unchanged.
func TestUpdateAgentDef_Description_OmittedFieldIsNoOp(t *testing.T) {
	t.Parallel()
	svc, _, wfID := setupAgentDefAPIModeEnv(t)

	if _, err := svc.CreateAgentDef("proj1", wfID, &types.AgentDefCreateRequest{
		ID:          "fanout-omit-desc",
		Prompt:      "do stuff",
		NodeRole:    "fanout_template",
		Description: "original description",
	}); err != nil {
		t.Fatalf("create: %v", err)
	}

	newTimeout := 45
	if err := svc.UpdateAgentDef("proj1", wfID, "fanout-omit-desc", &types.AgentDefUpdateRequest{
		Timeout: &newTimeout,
	}); err != nil {
		t.Fatalf("UpdateAgentDef(timeout only): %v", err)
	}

	def, err := svc.GetAgentDef("proj1", wfID, "fanout-omit-desc")
	if err != nil {
		t.Fatalf("GetAgentDef: %v", err)
	}
	if def.Description != "original description" {
		t.Errorf("Description after unrelated update = %q, want unchanged", def.Description)
	}
}
