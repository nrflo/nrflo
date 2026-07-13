package spawner

// Tests for api-mode context-window resolution: the DB api_models.context_length
// is authoritative over the provider's MaxContext fallback.

import (
	"context"
	"testing"

	"be/internal/clock"
	"be/internal/db"
	"be/internal/spawner/apirun/provider"
	"be/internal/spawner/apirun/provider/mock"
)

// TestPrepareSpawn_APIMode_ContextLengthDBWins verifies the DB context_length is
// authoritative: it is not overwritten by the provider's MaxContext fallback
// (the mock provider returns 200000). 777777 proves the DB value wins.
func TestPrepareSpawn_APIMode_ContextLengthDBWins(t *testing.T) {
	t.Parallel()
	env := setupContextSaveTestEnv(t)
	defer env.cleanup()

	insertAPIAgentDef(t, env, "impl", "sonnet")

	sp := New(Config{
		DataPath: env.dbPath,
		Pool:     db.WrapAsPool(env.database),
		Clock:    clock.Real(),
		APIMode:  true,
		BuildAPIProvider: func(_ context.Context, _, _ string) (provider.Provider, error) {
			return mock.New(), nil // mock.MaxContext returns 200000
		},
		APIModelConfigs: map[string]APIModelConfig{
			"sonnet": {Provider: "anthropic", MappedModel: "claude-sonnet-4-6", ContextLength: 777777},
		},
		AgentSvc: &noopAgentSvc{},
		Workflows: map[string]WorkflowDef{
			"feature": {Phases: []PhaseDef{{NodeID: "impl", Agent: "impl", Layer: 0}}},
		},
	})

	_, prep, err := sp.prepareSpawn(context.Background(), SpawnRequest{
		AgentType:          "impl",
		ProjectID:          env.projectID,
		WorkflowName:       "feature",
		WorkflowInstanceID: env.wfiID,
	}, "claude:sonnet", "impl", env.wfiID)
	if err != nil {
		t.Fatalf("prepareSpawn() error: %v", err)
	}

	if prep.apiMaxContext != 777777 {
		t.Errorf("prep.apiMaxContext = %d, want 777777 (DB context_length wins over provider fallback)", prep.apiMaxContext)
	}
}
