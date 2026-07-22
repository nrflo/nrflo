package service

import (
	"strings"
	"testing"

	"be/internal/types"
)

// TestSystemAgentDef_CreateEmptyModelRequiresTier verifies Create rejects an
// empty model when no tier is supplied (the cross-field invariant: effective
// model non-empty OR effective tier set).
func TestSystemAgentDef_CreateEmptyModelRequiresTier(t *testing.T) {
	t.Parallel()
	svc, cleanup := setupSysAgentDefTestEnv(t)
	defer cleanup()

	_, err := svc.Create(&types.SystemAgentDefCreateRequest{
		ID:     "no-model-no-tier",
		Prompt: "p",
	})
	if err == nil {
		t.Fatal("expected error for empty model + nil tier, got nil")
	}
	if !strings.Contains(err.Error(), "model or tier is required") {
		t.Errorf("err = %q, want mention of model or tier required", err.Error())
	}
}

// TestSystemAgentDef_CreateEmptyModelWithTierSucceeds verifies Create accepts
// an empty model when a tier is supplied, and persists the tier.
func TestSystemAgentDef_CreateEmptyModelWithTierSucceeds(t *testing.T) {
	t.Parallel()
	svc, cleanup := setupSysAgentDefTestEnv(t)
	defer cleanup()

	def, err := svc.Create(&types.SystemAgentDefCreateRequest{
		ID:     "tiered-agent",
		Prompt: "p",
		Tier:   intPtr(1),
	})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if def.Model != "" {
		t.Errorf("Model = %q, want empty", def.Model)
	}
	if def.Tier == nil || *def.Tier != 1 {
		t.Errorf("Tier = %v, want 1", def.Tier)
	}

	got, err := svc.Get("tiered-agent")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got.Tier == nil || *got.Tier != 1 {
		t.Errorf("Get Tier = %v, want 1", got.Tier)
	}
}

// TestSystemAgentDef_UpdateModelEmptyWithTierClearsOverride verifies Update
// allows Model="" to clear a model override when a tier is present (either
// already stored or supplied in the same request).
func TestSystemAgentDef_UpdateModelEmptyWithTierClearsOverride(t *testing.T) {
	t.Parallel()
	svc, cleanup := setupSysAgentDefTestEnv(t)
	defer cleanup()

	if _, err := svc.Create(&types.SystemAgentDefCreateRequest{
		ID: "clear-override", Prompt: "p", Model: "haiku-4-5",
	}); err != nil {
		t.Fatalf("Create: %v", err)
	}

	emptyModel := ""
	tier := 2
	if err := svc.Update("clear-override", &types.SystemAgentDefUpdateRequest{
		Model: &emptyModel,
		Tier:  &tier,
	}); err != nil {
		t.Fatalf("Update: %v", err)
	}

	got, err := svc.Get("clear-override")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got.Model != "" {
		t.Errorf("Model = %q, want empty after clearing override", got.Model)
	}
	if got.Tier == nil || *got.Tier != 2 {
		t.Errorf("Tier = %v, want 2", got.Tier)
	}
}

// TestSystemAgentDef_UpdateModelEmptyNoTierErrors verifies Update rejects
// Model="" when neither the stored row nor the request carries a tier.
func TestSystemAgentDef_UpdateModelEmptyNoTierErrors(t *testing.T) {
	t.Parallel()
	svc, cleanup := setupSysAgentDefTestEnv(t)
	defer cleanup()

	if _, err := svc.Create(&types.SystemAgentDefCreateRequest{
		ID: "no-tier-clear", Prompt: "p", Model: "haiku-4-5",
	}); err != nil {
		t.Fatalf("Create: %v", err)
	}

	emptyModel := ""
	err := svc.Update("no-tier-clear", &types.SystemAgentDefUpdateRequest{Model: &emptyModel})
	if err == nil {
		t.Fatal("expected error clearing model with no tier present, got nil")
	}
	if !strings.Contains(err.Error(), "model or tier is required") {
		t.Errorf("err = %q, want mention of model or tier required", err.Error())
	}
}
