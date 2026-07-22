package spawner

import (
	"context"
	"errors"
	"testing"

	"be/internal/clock"
	"be/internal/db"
	"be/internal/service"
	"be/internal/spawner/apirun/provider"
	"be/internal/spawner/apirun/provider/mock"
)

// buildFallbackChain returns a 2-entry api-mode chain: entry 0 ("badprov")
// always fails at BuildAPIProvider, entry 1 ("goodprov") always succeeds.
func buildFallbackChain() []service.AgentChainEntry {
	return []service.AgentChainEntry{
		{Provider: "badprov", ExecutionMode: "api", ModelID: "bad-model", ReasoningEffort: "low"},
		{Provider: "goodprov", ExecutionMode: "api", ModelID: "good-model", ReasoningEffort: "low"},
	}
}

func buildFallbackModelConfigs() map[string]ModelConfig {
	return map[string]ModelConfig{
		"bad-model":  {Provider: "badprov", APIModel: "bad-x", APIContext: 100000, APIEfforts: []string{"low"}, DefaultEffort: "low"},
		"good-model": {Provider: "goodprov", APIModel: "good-x", APIContext: 100000, APIEfforts: []string{"low"}, DefaultEffort: "low"},
	}
}

// TestSpawnEntryWithBuildFallback_AdvancesOnBuildError verifies that a
// build-time provider-construct failure on chain[0] (BuildAPIProvider
// returning an error for "badprov") advances to chain[1] ("goodprov"),
// landing the winning proc at chainPos==1 with the entry-1 model.
func TestSpawnEntryWithBuildFallback_AdvancesOnBuildError(t *testing.T) {
	t.Parallel()
	ensureTmpNrfloDir(t)
	env := setupContextSaveTestEnv(t)
	defer env.cleanup()
	insertAPIAgentDef(t, env, "impl", "bad-model")

	sp := New(Config{
		DataPath: env.dbPath,
		Pool:     db.WrapAsPool(env.database),
		Clock:    clock.Real(),
		APIMode:  true,
		BuildAPIProvider: func(_ context.Context, providerName, _ string) (provider.Provider, error) {
			if providerName == "badprov" {
				return nil, errors.New("missing credentials")
			}
			return mock.New(), nil
		},
		ModelConfigs: buildFallbackModelConfigs(),
		AgentSvc:     &noopAgentSvc{},
	})

	proc, chainPos, err := sp.spawnEntryWithBuildFallback(context.Background(), SpawnRequest{
		AgentType:          "impl",
		ProjectID:          env.projectID,
		WorkflowName:       "feature",
		WorkflowInstanceID: env.wfiID,
	}, "unused", "impl", env.wfiID, buildFallbackChain())
	if err != nil {
		t.Fatalf("spawnEntryWithBuildFallback() error: %v", err)
	}
	if chainPos != 1 {
		t.Errorf("chainPos = %d, want 1 (advanced past entry-0's build error)", chainPos)
	}
	if proc.modelID != "claude:good-model" {
		t.Errorf("proc.modelID = %q, want claude:good-model", proc.modelID)
	}
}

// TestSpawnEntryWithBuildFallback_StructuralErrorDoesNotAdvance verifies that
// a structural spawn error (unresolvable agent def / no template — surfaced
// here by an api-mode model with no ModelConfigs row at all) is returned
// immediately from chain[0] without trying chain[1], since it's never
// wrapped by wrapProviderBuildErr.
func TestSpawnEntryWithBuildFallback_StructuralErrorDoesNotAdvance(t *testing.T) {
	t.Parallel()
	ensureTmpNrfloDir(t)
	env := setupContextSaveTestEnv(t)
	defer env.cleanup()
	insertAPIAgentDef(t, env, "impl", "bad-model")

	var buildCalls int
	sp := New(Config{
		DataPath: env.dbPath,
		Pool:     db.WrapAsPool(env.database),
		Clock:    clock.Real(),
		APIMode:  true,
		BuildAPIProvider: func(_ context.Context, _ string, _ string) (provider.Provider, error) {
			buildCalls++
			return mock.New(), nil
		},
		// Only entry-1's model is registered; entry-0's "bad-model" is
		// missing from ModelConfigs entirely, which prepareAPIModeSpawn
		// reports as a plain (unwrapped) "model not found" error.
		ModelConfigs: map[string]ModelConfig{
			"good-model": {Provider: "goodprov", APIModel: "good-x", APIContext: 100000, APIEfforts: []string{"low"}, DefaultEffort: "low"},
		},
		AgentSvc: &noopAgentSvc{},
	})

	_, chainPos, err := sp.spawnEntryWithBuildFallback(context.Background(), SpawnRequest{
		AgentType:          "impl",
		ProjectID:          env.projectID,
		WorkflowName:       "feature",
		WorkflowInstanceID: env.wfiID,
	}, "unused", "impl", env.wfiID, buildFallbackChain())
	if err == nil {
		t.Fatal("spawnEntryWithBuildFallback() error = nil, want structural error")
	}
	if isProviderBuildError(err) {
		t.Errorf("error classified as provider-build error, want structural (never advances): %v", err)
	}
	if chainPos != 0 {
		t.Errorf("chainPos = %d, want 0 (must not advance on a structural error)", chainPos)
	}
	if buildCalls != 0 {
		t.Errorf("BuildAPIProvider called %d times, want 0 (model lookup fails before BuildAPIProvider)", buildCalls)
	}
}

// TestSpawnEntryWithBuildFallback_EmptyChainFallsBackToBareSpawnSingle
// verifies that main workflow-phase agents (empty chain) behave
// byte-identical to a bare spawnSingle call: chainPos is always 0 and no
// chain-advance logic runs.
func TestSpawnEntryWithBuildFallback_EmptyChainFallsBackToBareSpawnSingle(t *testing.T) {
	t.Parallel()
	ensureTmpNrfloDir(t)
	env := setupContextSaveTestEnv(t)
	defer env.cleanup()
	insertAPIAgentDef(t, env, "impl", "sonnet-5")

	sp := New(Config{
		DataPath: env.dbPath,
		Pool:     db.WrapAsPool(env.database),
		Clock:    clock.Real(),
		APIMode:  true,
		BuildAPIProvider: func(_ context.Context, _ string, _ string) (provider.Provider, error) {
			return mock.New(), nil
		},
		ModelConfigs: map[string]ModelConfig{
			"sonnet-5": {Provider: "anthropic", APIModel: "claude-sonnet-4-6", APIContext: 200000},
		},
		AgentSvc: &noopAgentSvc{},
	})

	proc, chainPos, err := sp.spawnEntryWithBuildFallback(context.Background(), SpawnRequest{
		AgentType:          "impl",
		ProjectID:          env.projectID,
		WorkflowName:       "feature",
		WorkflowInstanceID: env.wfiID,
	}, "claude:sonnet-5", "impl", env.wfiID, nil)
	if err != nil {
		t.Fatalf("spawnEntryWithBuildFallback() error: %v", err)
	}
	if chainPos != 0 {
		t.Errorf("chainPos = %d, want 0 for an empty chain", chainPos)
	}
	if proc.modelID != "claude:sonnet-5" {
		t.Errorf("proc.modelID = %q, want claude:sonnet-5", proc.modelID)
	}
}
