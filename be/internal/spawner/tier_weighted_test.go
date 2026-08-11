package spawner

import (
	"context"
	"testing"

	"be/internal/clock"
	"be/internal/db"
	"be/internal/service"
	"be/internal/spawner/apirun/provider"
	"be/internal/spawner/apirun/provider/mock"
)

// TestSpawnEntryWithBuildFallback_WeightedRotation verifies a weighted chain
// starts at the highest-deficit entry (with no landed history: the highest
// weight), landing the non-primary model at slice index 0 while the DB
// chain_position records the entry's canonical tier_models position.
func TestSpawnEntryWithBuildFallback_WeightedRotation(t *testing.T) {
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
		BuildAPIProvider: func(_ context.Context, _, _ string) (provider.Provider, error) {
			return mock.New(), nil
		},
		HasAPICredentials: func(_ context.Context, _, _ string) bool { return true },
		ModelConfigs:      buildFallbackModelConfigs(),
		AgentSvc:          &noopAgentSvc{},
	})

	chain := []service.AgentChainEntry{
		{Provider: "badprov", ExecutionMode: "api", ModelID: "bad-model", ReasoningEffort: "low", Position: 0, Weight: 10},
		{Provider: "goodprov", ExecutionMode: "api", ModelID: "good-model", ReasoningEffort: "low", Position: 1, Weight: 90},
	}
	proc, chainPos, err := sp.spawnEntryWithBuildFallback(context.Background(), SpawnRequest{
		AgentType:          "impl",
		ProjectID:          env.projectID,
		WorkflowName:       "feature",
		WorkflowInstanceID: env.wfiID,
	}, "unused", "impl", env.wfiID, chain)
	if err != nil {
		t.Fatalf("spawnEntryWithBuildFallback() error: %v", err)
	}
	if chainPos != 0 {
		t.Errorf("chainPos = %d, want 0 (rotated entry leads the reordered chain)", chainPos)
	}
	if proc.modelID != "claude:good-model" {
		t.Errorf("proc.modelID = %q, want claude:good-model (weight 90 entry starts)", proc.modelID)
	}
	if len(proc.chain) != 2 || proc.chain[0].Position != 1 {
		t.Errorf("proc.chain[0].Position = %v, want the rotated chain with position 1 first", proc.chain)
	}

	_, _, _, _, gotPos, fallbackFrom := querySessionResolution(t, env.database, proc.sessionID)
	if gotPos != 1 {
		t.Errorf("chain_position = %d, want canonical position 1", gotPos)
	}
	if fallbackFrom.Valid {
		t.Errorf("fallback_from = %+v, want NULL (nothing failed before the rotated start)", fallbackFrom)
	}
}

// TestApplyWeightedRotation_NoWeightsNoop verifies the default all-zero
// weights leave the chain strictly ordinal.
func TestApplyWeightedRotation_NoWeightsNoop(t *testing.T) {
	t.Parallel()
	sp := New(Config{APIMode: true, AgentSvc: &noopAgentSvc{}, Clock: clock.Real()})
	chain := buildFallbackChain()
	got := sp.applyWeightedRotation(context.Background(), chain, "p1")
	if got[0].Position != 0 || got[1].Position != 1 {
		t.Errorf("rotation applied to a weightless chain: %+v", got)
	}
}
