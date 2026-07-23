package service

import (
	"encoding/json"
	"strings"
	"testing"

	"be/internal/clock"
	"be/internal/model"
)

// agentDef builds an in-memory AgentDefinition for ResolveDefChain fixtures
// (never persisted — resolution only reads the struct's tier-spec fields).
func agentDef(model_, executionMode string, tier *int, effort *string) *model.AgentDefinition {
	return &model.AgentDefinition{
		ID:              "test-def",
		ExecutionMode:   executionMode,
		Model:           model_,
		Tier:            tier,
		ReasoningEffort: effort,
	}
}

// TestResolveDefChain_OverrideWinsAsSingleEntry verifies a non-empty
// def.Model short-circuits to a single-entry chain, mirroring
// resolveOverrideEntry's system-agent counterpart.
func TestResolveDefChain_OverrideWinsAsSingleEntry(t *testing.T) {
	t.Parallel()
	svc, cleanup := setupSysAgentDefTestEnv(t)
	t.Cleanup(cleanup)
	createTestModel(t, svc, "override-model", "medium")

	def := agentDef("override-model", "api", nil, nil)
	chain, err := ResolveDefChain(svc.pool, clock.Real(), svc.modelSvc, def)
	if err != nil {
		t.Fatalf("ResolveDefChain: %v", err)
	}
	if len(chain) != 1 || chain[0].ModelID != "override-model" || chain[0].ReasoningEffort != "medium" {
		t.Errorf("chain = %+v, want single entry override-model/medium", chain)
	}
}

// TestResolveDefChain_TierOrderedByPosition verifies a tier-only def resolves
// the tier_models chain ordered by position ASC.
func TestResolveDefChain_TierOrderedByPosition(t *testing.T) {
	t.Parallel()
	svc, cleanup := setupSysAgentDefTestEnv(t)
	t.Cleanup(cleanup)

	tier := 1
	def := agentDef("", "api", &tier, nil)
	chain, err := ResolveDefChain(svc.pool, clock.Real(), svc.modelSvc, def)
	if err != nil {
		t.Fatalf("ResolveDefChain: %v", err)
	}
	if len(chain) != 2 || chain[0].ModelID != "haiku-4-5" {
		t.Errorf("chain = %+v, want tier1 chain (haiku-4-5 primary)", chain)
	}
}

// TestResolveDefChain_InheritanceWalksDown verifies an empty tier walks down
// to the nearest lower populated tier (tier 5 is unseeded -> falls to tier 4).
func TestResolveDefChain_InheritanceWalksDown(t *testing.T) {
	t.Parallel()
	svc, cleanup := setupSysAgentDefTestEnv(t)
	t.Cleanup(cleanup)

	tier := 5
	def := agentDef("", "api", &tier, nil)
	chain, err := ResolveDefChain(svc.pool, clock.Real(), svc.modelSvc, def)
	if err != nil {
		t.Fatalf("ResolveDefChain: %v", err)
	}
	if len(chain) != 2 || chain[0].ModelID != "sonnet-5" {
		t.Errorf("chain = %+v, want inherited tier4 chain (sonnet-5 primary)", chain)
	}
}

// TestResolveDefChain_NilTierEmptyModel_Errors verifies a def with neither a
// model override nor a tier fails resolution.
func TestResolveDefChain_NilTierEmptyModel_Errors(t *testing.T) {
	t.Parallel()
	svc, cleanup := setupSysAgentDefTestEnv(t)
	t.Cleanup(cleanup)

	def := agentDef("", "api", nil, nil)
	_, err := ResolveDefChain(svc.pool, clock.Real(), svc.modelSvc, def)
	if err == nil {
		t.Fatal("ResolveDefChain: expected error for nil tier + no override, got nil")
	}
	if !strings.Contains(err.Error(), "no model override and no tier") {
		t.Errorf("error = %q, want mention of no override/no tier", err.Error())
	}
}

// TestResolveDefChain_InvalidModelInChain_Errors verifies an invalid/disabled
// model referenced by a tier_models row surfaces as an error.
func TestResolveDefChain_InvalidModelInChain_Errors(t *testing.T) {
	t.Parallel()
	svc, cleanup := setupSysAgentDefTestEnv(t)
	t.Cleanup(cleanup)

	if _, err := svc.pool.Exec(
		`INSERT INTO tier_models (tier, position, provider, execution_mode, model_id, reasoning_effort)
		 VALUES (3, 2, 'anthropic', 'api', 'does-not-exist', 'low')`,
	); err != nil {
		t.Fatalf("insert bogus tier_models row: %v", err)
	}

	tier := 3
	def := agentDef("", "api", &tier, nil)
	_, err := ResolveDefChain(svc.pool, clock.Real(), svc.modelSvc, def)
	if err == nil {
		t.Fatal("ResolveDefChain: expected error for invalid model in chain, got nil")
	}
}

// TestResolveDefChain_ExecutionModeInherit verifies a tier_models row with
// execution_mode==” resolves with the def's own ExecutionMode substituted in
// (so the def's own mode applies), validated against that mode's registry.
func TestResolveDefChain_ExecutionModeInherit(t *testing.T) {
	t.Parallel()
	svc, cleanup := setupSysAgentDefTestEnv(t)
	t.Cleanup(cleanup)

	// Tier 2 position 0 is seeded execution_mode='' by migration 000200.
	tier := 2
	def := agentDef("", "cli_interactive", &tier, nil)
	chain, err := ResolveDefChain(svc.pool, clock.Real(), svc.modelSvc, def)
	if err != nil {
		t.Fatalf("ResolveDefChain: %v", err)
	}
	if len(chain) == 0 {
		t.Fatal("chain is empty")
	}
	if chain[0].ExecutionMode != "cli_interactive" {
		t.Errorf("chain[0].ExecutionMode = %q, want cli_interactive (inherited from def)", chain[0].ExecutionMode)
	}
}

// TestResolveDefChain_NilDef_Errors verifies ResolveDefChain rejects a nil def.
func TestResolveDefChain_NilDef_Errors(t *testing.T) {
	t.Parallel()
	svc, cleanup := setupSysAgentDefTestEnv(t)
	t.Cleanup(cleanup)

	_, err := ResolveDefChain(svc.pool, clock.Real(), svc.modelSvc, nil)
	if err == nil {
		t.Fatal("ResolveDefChain(nil): expected error, got nil")
	}
}

// TestAgentChainEntry_JSONTagsLowerCase verifies AgentChainEntry marshals
// with lower-case snake_case keys — this is the wire contract
// tier_observability.go writes into agent_sessions.fallback_from and the
// system-agent-runs API passes through for the UI's fallback indicator.
func TestAgentChainEntry_JSONTagsLowerCase(t *testing.T) {
	t.Parallel()
	entry := AgentChainEntry{
		Provider:        "anthropic",
		ExecutionMode:   "api",
		ModelID:         "sonnet-5",
		ReasoningEffort: "low",
		Tier:            2,
	}
	b, err := json.Marshal(entry)
	if err != nil {
		t.Fatalf("json.Marshal: %v", err)
	}
	body := string(b)
	for _, key := range []string{`"provider":`, `"execution_mode":`, `"model_id":`, `"reasoning_effort":`, `"tier":`} {
		if !strings.Contains(body, key) {
			t.Errorf("marshaled body = %s, want key %s", body, key)
		}
	}
	for _, key := range []string{`"Provider":`, `"ExecutionMode":`, `"ModelID":`} {
		if strings.Contains(body, key) {
			t.Errorf("marshaled body = %s, want no capitalized Go field name %s", body, key)
		}
	}
}
