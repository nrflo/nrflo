package service

import (
	"strings"
	"testing"

	"be/internal/model"
	"be/internal/types"
)

// createTestModel is a small helper wrapping ModelService.Create for
// ResolveAgentChain fixtures.
func createTestModel(t *testing.T, svc *SystemAgentDefinitionService, id, defaultEffort string) {
	t.Helper()
	_, err := svc.modelSvc.Create(types.ModelCreateRequest{
		ID:            id,
		Provider:      "anthropic",
		DisplayName:   id,
		CLIModel:      id,
		APIModel:      id,
		CLIEfforts:    []string{"low", "medium", "high"},
		APIEfforts:    []string{"low", "medium", "high"},
		DefaultEffort: defaultEffort,
	})
	if err != nil {
		t.Fatalf("create test model %s: %v", id, err)
	}
}

// overrideDef builds an in-memory SystemAgentDefinition with a model
// override (never persisted — ResolveAgentChain only reads the struct).
func overrideDef(modelID, executionMode string, effort *string) *model.SystemAgentDefinition {
	return &model.SystemAgentDefinition{
		ID:              "test-agent",
		ExecutionMode:   executionMode,
		Model:           modelID,
		ReasoningEffort: effort,
	}
}

// tierDef builds an in-memory SystemAgentDefinition with no model override,
// selecting a tier fallback chain (nil tier = untiered).
func tierDef(tier *int) *model.SystemAgentDefinition {
	return &model.SystemAgentDefinition{
		ID:            "test-agent",
		ExecutionMode: "api",
		Tier:          tier,
	}
}

// TestResolveAgentChain_Override verifies a non-empty def.Model short-circuits
// to a single entry, with reasoning_effort taken from def when set, else
// inherited from the model row's default_effort.
func TestResolveAgentChain_Override(t *testing.T) {
	svc, cleanup := setupSysAgentDefTestEnv(t)
	t.Cleanup(cleanup)
	createTestModel(t, svc, "override-model", "medium")

	t.Run("effort override wins", func(t *testing.T) {
		def := overrideDef("override-model", "api", strPtr("high"))
		chain, err := svc.ResolveAgentChain(def)
		if err != nil {
			t.Fatalf("ResolveAgentChain: %v", err)
		}
		if len(chain) != 1 {
			t.Fatalf("chain length = %d, want 1", len(chain))
		}
		got := chain[0]
		if got.Provider != "anthropic" || got.ExecutionMode != "api" || got.ModelID != "override-model" || got.ReasoningEffort != "high" {
			t.Errorf("chain[0] = %+v, want provider=anthropic mode=api model=override-model effort=high", got)
		}
	})

	t.Run("effort inherits from model row default", func(t *testing.T) {
		def := overrideDef("override-model", "api", nil)
		chain, err := svc.ResolveAgentChain(def)
		if err != nil {
			t.Fatalf("ResolveAgentChain: %v", err)
		}
		if chain[0].ReasoningEffort != "medium" {
			t.Errorf("chain[0].ReasoningEffort = %q, want %q (model row default)", chain[0].ReasoningEffort, "medium")
		}
	})
}

// TestResolveAgentChain_TierPopulated verifies an empty override + populated
// tier resolves to the ordered tier_models chain, primary = position 0.
func TestResolveAgentChain_TierPopulated(t *testing.T) {
	svc, cleanup := setupSysAgentDefTestEnv(t)
	t.Cleanup(cleanup)

	def := tierDef(intPtr(1))
	chain, err := svc.ResolveAgentChain(def)
	if err != nil {
		t.Fatalf("ResolveAgentChain: %v", err)
	}
	if len(chain) != 2 {
		t.Fatalf("chain length = %d, want 2 (seeded tier1 chain)", len(chain))
	}
	if chain[0].ExecutionMode != "api" || chain[0].ModelID != "haiku-4-5" {
		t.Errorf("chain[0] = %+v, want api/haiku-4-5 (position 0)", chain[0])
	}
	if chain[1].ExecutionMode != "cli_interactive" || chain[1].ModelID != "haiku-4-5" {
		t.Errorf("chain[1] = %+v, want cli_interactive/haiku-4-5 (position 1)", chain[1])
	}
}

// TestResolveAgentChain_TierInheritance verifies an empty tier walks down to
// the nearest lower populated tier.
func TestResolveAgentChain_TierInheritance(t *testing.T) {
	svc, cleanup := setupSysAgentDefTestEnv(t)
	t.Cleanup(cleanup)

	// Tier 5 has no seeded rows (1-4 are all populated post-000200); should
	// fall through to tier 4's chain.
	def := tierDef(intPtr(5))
	chain, err := svc.ResolveAgentChain(def)
	if err != nil {
		t.Fatalf("ResolveAgentChain: %v", err)
	}
	if len(chain) != 2 || chain[0].ModelID != "sonnet-5" {
		t.Errorf("chain = %+v, want inherited tier4 chain (sonnet-5 x2)", chain)
	}
}

// TestResolveAgentChain_NoOverrideNoTier verifies a def with neither a model
// override nor a tier assignment fails resolution.
func TestResolveAgentChain_NoOverrideNoTier(t *testing.T) {
	svc, cleanup := setupSysAgentDefTestEnv(t)
	t.Cleanup(cleanup)

	def := tierDef(nil)
	_, err := svc.ResolveAgentChain(def)
	if err == nil {
		t.Fatal("ResolveAgentChain: expected error for nil tier + no override, got nil")
	}
	if !strings.Contains(err.Error(), "no model override and no tier") {
		t.Errorf("error = %q, want mention of no override/no tier", err.Error())
	}
}

// TestResolveAgentChain_InvalidModelInTierChain verifies an invalid/disabled
// model referenced by a tier_models row surfaces as an error.
func TestResolveAgentChain_InvalidModelInTierChain(t *testing.T) {
	svc, cleanup := setupSysAgentDefTestEnv(t)
	t.Cleanup(cleanup)

	// Tier 3's positions 0/1 are already seeded (000200); append a bogus
	// entry at position 2 so the whole chain (including the bad entry)
	// fails validation.
	if _, err := svc.pool.Exec(
		`INSERT INTO tier_models (tier, position, provider, execution_mode, model_id, reasoning_effort)
		 VALUES (3, 2, 'anthropic', 'api', 'does-not-exist', 'low')`,
	); err != nil {
		t.Fatalf("insert bogus tier_models row: %v", err)
	}

	def := tierDef(intPtr(3))
	_, err := svc.ResolveAgentChain(def)
	if err == nil {
		t.Fatal("ResolveAgentChain: expected error for invalid model in tier chain, got nil")
	}
}

// TestResolveAgentChain_InvalidOverrideModel verifies an override model that
// fails IsValidModelForMode surfaces as an error rather than a partial chain.
func TestResolveAgentChain_InvalidOverrideModel(t *testing.T) {
	svc, cleanup := setupSysAgentDefTestEnv(t)
	t.Cleanup(cleanup)

	def := overrideDef("does-not-exist", "api", nil)
	_, err := svc.ResolveAgentChain(def)
	if err == nil {
		t.Fatal("ResolveAgentChain: expected error for invalid override model, got nil")
	}
}
