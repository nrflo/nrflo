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

// TestSpawnEntryWithBuildFallback_SkipsAPIEntryWithoutCredentials verifies
// the static pre-check: an api-mode entry whose provider credentials do not
// resolve is skipped without ever calling BuildAPIProvider, landing chain[1].
func TestSpawnEntryWithBuildFallback_SkipsAPIEntryWithoutCredentials(t *testing.T) {
	t.Parallel()
	ensureTmpNrfloDir(t)
	env := setupContextSaveTestEnv(t)
	defer env.cleanup()
	insertAPIAgentDef(t, env, "impl", "bad-model")

	built := map[string]int{}
	sp := New(Config{
		DataPath: env.dbPath,
		Pool:     db.WrapAsPool(env.database),
		Clock:    clock.Real(),
		APIMode:  true,
		BuildAPIProvider: func(_ context.Context, providerName, _ string) (provider.Provider, error) {
			built[providerName]++
			return mock.New(), nil
		},
		HasAPICredentials: func(_ context.Context, providerName, _ string) bool {
			return providerName != "badprov"
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
		t.Errorf("chainPos = %d, want 1 (skipped credential-less entry 0)", chainPos)
	}
	if proc.modelID != "claude:good-model" {
		t.Errorf("proc.modelID = %q, want claude:good-model", proc.modelID)
	}
	if built["badprov"] != 0 {
		t.Errorf("BuildAPIProvider(badprov) called %d times, want 0 (skipped before build)", built["badprov"])
	}
}

// TestSpawnEntryWithBuildFallback_LastEntryNeverSkipped verifies the skip
// only applies while a later entry exists: a single-entry chain with missing
// credentials is still attempted, surfacing the real build error.
func TestSpawnEntryWithBuildFallback_LastEntryNeverSkipped(t *testing.T) {
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
		BuildAPIProvider: func(_ context.Context, _, _ string) (provider.Provider, error) {
			buildCalls++
			return nil, errors.New("missing credentials")
		},
		HasAPICredentials: func(_ context.Context, _, _ string) bool { return false },
		ModelConfigs:      buildFallbackModelConfigs(),
		AgentSvc:          &noopAgentSvc{},
	})

	_, _, err := sp.spawnEntryWithBuildFallback(context.Background(), SpawnRequest{
		AgentType:          "impl",
		ProjectID:          env.projectID,
		WorkflowName:       "feature",
		WorkflowInstanceID: env.wfiID,
	}, "unused", "impl", env.wfiID, []service.AgentChainEntry{
		{Provider: "badprov", ExecutionMode: "api", ModelID: "bad-model", ReasoningEffort: "low"},
	})
	if err == nil {
		t.Fatal("spawnEntryWithBuildFallback() error = nil, want build failure from the attempted last entry")
	}
	if buildCalls != 1 {
		t.Errorf("BuildAPIProvider called %d times, want 1 (last entry must be attempted)", buildCalls)
	}
}
