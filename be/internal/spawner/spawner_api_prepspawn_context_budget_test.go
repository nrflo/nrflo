package spawner

// Tests for api-mode prepareSpawn's context_budget_tokens resolution:
// prep.apiContextBudget must reflect resolveContextBudget(agentDef, config
// context_budget_default), the same NULL->global->0-disabled shape as the
// stall-timeout fields.

import (
	"context"
	"testing"
	"time"

	"be/internal/clock"
	"be/internal/db"
	"be/internal/spawner/apirun/provider"
	"be/internal/spawner/apirun/provider/mock"
)

// insertAPIAgentDefWithBudget inserts an api-mode agent_definition row with an
// explicit context_budget_tokens value.
func insertAPIAgentDefWithBudget(t *testing.T, env *contextSaveTestEnv, agentID, model string, budget int) {
	t.Helper()
	now := time.Now().UTC().Format(time.RFC3339Nano)
	if _, err := env.database.Exec(
		`INSERT INTO agent_definitions
			(id, project_id, workflow_id, model, timeout, prompt, execution_mode, tools, context_budget_tokens, created_at, updated_at)
		VALUES (?, ?, 'feature', ?, 20, '# prompt', 'api', 'agent_finished', ?, ?, ?)`,
		agentID, env.projectID, model, budget, now, now,
	); err != nil {
		t.Fatalf("insert agent_definition %q: %v", agentID, err)
	}
}

func newAPIPrepSpawner(env *contextSaveTestEnv) *Spawner {
	return New(Config{
		DataPath: env.dbPath,
		Pool:     db.WrapAsPool(env.database),
		Clock:    clock.Real(),
		APIMode:  true,
		BuildAPIProvider: func(_ context.Context, _, _ string) (provider.Provider, error) {
			return mock.New(), nil
		},
		ModelConfigs: map[string]ModelConfig{
			"sonnet-5": {
				Provider:      "anthropic",
				APIModel:      "claude-sonnet-4-6",
				APIContext:    200000,
				DefaultEffort: "high",
				APIEfforts:    []string{"low", "medium", "high", "xhigh"},
			},
		},
		AgentSvc: &noopAgentSvc{},
		Workflows: map[string]WorkflowDef{
			"feature": {Phases: []PhaseDef{{NodeID: "impl", Agent: "impl", Layer: 0}}},
		},
	})
}

// TestPrepareSpawn_APIMode_ContextBudget_DefOverridePropagates verifies a
// positive per-def override lands in prep.apiContextBudget verbatim.
func TestPrepareSpawn_APIMode_ContextBudget_DefOverridePropagates(t *testing.T) {
	t.Parallel()
	env := setupContextSaveTestEnv(t)
	defer env.cleanup()
	insertAPIAgentDefWithBudget(t, env, "impl", "sonnet-5", 55000)

	sp := newAPIPrepSpawner(env)
	_, prep, err := sp.prepareSpawn(context.Background(), SpawnRequest{
		AgentType: "impl", ProjectID: env.projectID, WorkflowName: "feature", WorkflowInstanceID: env.wfiID,
	}, "claude:sonnet-5", "impl", env.wfiID)
	if err != nil {
		t.Fatalf("prepareSpawn() error: %v", err)
	}
	if prep.apiContextBudget != 55000 {
		t.Errorf("prep.apiContextBudget = %d, want 55000", prep.apiContextBudget)
	}
}

// TestPrepareSpawn_APIMode_ContextBudget_ZeroDisables verifies an explicit
// per-def 0 stays 0 (disabled) rather than falling through to the global
// default.
func TestPrepareSpawn_APIMode_ContextBudget_ZeroDisables(t *testing.T) {
	t.Parallel()
	env := setupContextSaveTestEnv(t)
	defer env.cleanup()
	insertAPIAgentDefWithBudget(t, env, "impl", "sonnet-5", 0)
	if err := env.database.SetConfig("context_budget_default", "40000"); err != nil {
		t.Fatalf("SetConfig: %v", err)
	}

	sp := newAPIPrepSpawner(env)
	_, prep, err := sp.prepareSpawn(context.Background(), SpawnRequest{
		AgentType: "impl", ProjectID: env.projectID, WorkflowName: "feature", WorkflowInstanceID: env.wfiID,
	}, "claude:sonnet-5", "impl", env.wfiID)
	if err != nil {
		t.Fatalf("prepareSpawn() error: %v", err)
	}
	if prep.apiContextBudget != 0 {
		t.Errorf("prep.apiContextBudget = %d, want 0 (explicit disable wins over global default)", prep.apiContextBudget)
	}
}

// TestPrepareSpawn_APIMode_ContextBudget_NilDefFallsThroughToGlobal verifies
// a def with no context_budget_tokens column value inherits the global
// context_budget_default config.
func TestPrepareSpawn_APIMode_ContextBudget_NilDefFallsThroughToGlobal(t *testing.T) {
	t.Parallel()
	env := setupContextSaveTestEnv(t)
	defer env.cleanup()
	insertAPIAgentDef(t, env, "impl", "sonnet-5") // no context_budget_tokens set: NULL
	if err := env.database.SetConfig("context_budget_default", "30000"); err != nil {
		t.Fatalf("SetConfig: %v", err)
	}

	sp := newAPIPrepSpawner(env)
	_, prep, err := sp.prepareSpawn(context.Background(), SpawnRequest{
		AgentType: "impl", ProjectID: env.projectID, WorkflowName: "feature", WorkflowInstanceID: env.wfiID,
	}, "claude:sonnet-5", "impl", env.wfiID)
	if err != nil {
		t.Fatalf("prepareSpawn() error: %v", err)
	}
	if prep.apiContextBudget != 30000 {
		t.Errorf("prep.apiContextBudget = %d, want 30000 (global default)", prep.apiContextBudget)
	}
}
