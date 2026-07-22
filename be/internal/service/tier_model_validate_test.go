package service

import (
	"strings"
	"testing"

	"be/internal/types"
)

// --- SetTierChain: validation ---

// TestTierModel_SetTierChain_TierOutOfRange verifies tier must be in [1,5].
func TestTierModel_SetTierChain_TierOutOfRange(t *testing.T) {
	t.Parallel()
	svc, _, cleanup := setupTierModelTestEnv(t)
	defer cleanup()

	for _, tier := range []int{0, 6, -1} {
		err := svc.SetTierChain(tier, []types.TierChainEntry{
			{ExecutionMode: "api", ModelID: "haiku-4-5"},
		})
		if err == nil {
			t.Errorf("tier=%d: expected error, got nil", tier)
			continue
		}
		if !strings.Contains(err.Error(), "between 1 and 5") {
			t.Errorf("tier=%d: err = %q, want mention of range", tier, err.Error())
		}
	}
}

// TestTierModel_SetTierChain_InvalidModelID verifies an unknown model_id is
// rejected rather than silently inserted.
func TestTierModel_SetTierChain_InvalidModelID(t *testing.T) {
	t.Parallel()
	svc, _, cleanup := setupTierModelTestEnv(t)
	defer cleanup()

	err := svc.SetTierChain(2, []types.TierChainEntry{
		{ExecutionMode: "api", ModelID: "does-not-exist"},
	})
	if err == nil {
		t.Fatal("expected error for invalid model_id, got nil")
	}
	if !strings.Contains(err.Error(), "invalid model") {
		t.Errorf("err = %q, want mention of invalid model", err.Error())
	}
}

// TestTierModel_SetTierChain_InvalidModelID_EmptyString verifies a
// zero-value model_id is rejected with a distinct message from an unknown id.
func TestTierModel_SetTierChain_InvalidModelID_EmptyString(t *testing.T) {
	t.Parallel()
	svc, _, cleanup := setupTierModelTestEnv(t)
	defer cleanup()

	err := svc.SetTierChain(2, []types.TierChainEntry{
		{ExecutionMode: "api", ModelID: ""},
	})
	if err == nil {
		t.Fatal("expected error for empty model_id, got nil")
	}
	if !strings.Contains(err.Error(), "model_id is required") {
		t.Errorf("err = %q, want mention of model_id required", err.Error())
	}
}

// TestTierModel_SetTierChain_ModelInvalidForMode verifies a model valid for
// one execution mode but not another is rejected (registryMode gate).
func TestTierModel_SetTierChain_ModelInvalidForMode(t *testing.T) {
	t.Parallel()
	svc, modelSvc, cleanup := setupTierModelTestEnv(t)
	defer cleanup()

	// Create an api-only model row (empty cli_model) to exercise the gate.
	if _, err := modelSvc.Create(types.ModelCreateRequest{
		ID:            "api-only-model",
		Provider:      "openai",
		DisplayName:   "API Only",
		APIModel:      "api-only-upstream",
		APIEfforts:    []string{"low", "medium", "high"},
		DefaultEffort: "medium",
	}); err != nil {
		t.Fatalf("create api-only model: %v", err)
	}

	err := svc.SetTierChain(2, []types.TierChainEntry{
		{ExecutionMode: "cli_interactive", ModelID: "api-only-model"},
	})
	if err == nil {
		t.Fatal("expected error for api-only model in a cli_interactive entry, got nil")
	}
}

// --- SetTierChain: provider derivation ---

// TestTierModel_SetTierChain_ProviderDerivedFromModelRow verifies provider is
// always derived from the model row server-side — SetTierChainRequest has no
// client-settable provider field, so this also guards against a future
// regression that starts trusting client input.
func TestTierModel_SetTierChain_ProviderDerivedFromModelRow(t *testing.T) {
	t.Parallel()
	svc, _, cleanup := setupTierModelTestEnv(t)
	defer cleanup()

	if err := svc.SetTierChain(2, []types.TierChainEntry{
		{ExecutionMode: "api", ModelID: "gpt-5.6-sol"},
	}); err != nil {
		t.Fatalf("SetTierChain: %v", err)
	}

	rows, err := svc.List()
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	found := false
	for _, r := range rows {
		if r.Tier == 2 && r.ModelID == "gpt-5.6-sol" {
			found = true
			if r.Provider != "openai" {
				t.Errorf("Provider = %q, want %q (derived from model row)", r.Provider, "openai")
			}
		}
	}
	if !found {
		t.Fatal("tier=2 gpt-5.6-sol row not found in List")
	}
}

// TestTierModel_SetTierChain_EffortDefaultsFromModelRow verifies an empty
// ReasoningEffort entry inherits the model row's DefaultEffort.
func TestTierModel_SetTierChain_EffortDefaultsFromModelRow(t *testing.T) {
	t.Parallel()
	svc, _, cleanup := setupTierModelTestEnv(t)
	defer cleanup()

	if err := svc.SetTierChain(2, []types.TierChainEntry{
		{ExecutionMode: "cli_interactive", ModelID: "gpt-5.6-sol", ReasoningEffort: ""},
	}); err != nil {
		t.Fatalf("SetTierChain: %v", err)
	}

	rows, err := svc.List()
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	found := false
	for _, r := range rows {
		if r.Tier == 2 {
			found = true
			if r.ReasoningEffort != "low" {
				t.Errorf("ReasoningEffort = %q, want %q (inherited model row default)", r.ReasoningEffort, "low")
			}
		}
	}
	if !found {
		t.Fatal("tier=2 row not found in List")
	}
}

// TestTierModel_SetTierChain_EffortOverrideWins verifies a non-empty
// ReasoningEffort entry is kept as-is rather than overwritten by the model
// row's default.
func TestTierModel_SetTierChain_EffortOverrideWins(t *testing.T) {
	t.Parallel()
	svc, _, cleanup := setupTierModelTestEnv(t)
	defer cleanup()

	if err := svc.SetTierChain(2, []types.TierChainEntry{
		{ExecutionMode: "cli_interactive", ModelID: "opus-4-7", ReasoningEffort: "high"},
	}); err != nil {
		t.Fatalf("SetTierChain: %v", err)
	}

	rows, err := svc.List()
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	found := false
	for _, r := range rows {
		if r.Tier == 2 {
			found = true
			if r.ReasoningEffort != "high" {
				t.Errorf("ReasoningEffort = %q, want %q (explicit override)", r.ReasoningEffort, "high")
			}
		}
	}
	if !found {
		t.Fatal("tier=2 row not found in List")
	}
}
