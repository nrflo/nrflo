package spawner

// Tests for the api-mode system prompt going through the api-system-prompt
// injectable (spawner_prepare.go's api branch): fresh-DB byte-identity to the
// defaultAPISystemPrompt constant, suffix appended, template-edit reflected
// on the next spawn (no rebuild needed), and fallback when the row is
// missing/empty.

import (
	"context"
	"strings"
	"testing"

	"be/internal/clock"
	"be/internal/db"
	"be/internal/spawner/apirun/provider"
	"be/internal/spawner/apirun/provider/mock"
)

// newAPIModeSpawner builds a Spawner wired for api mode against env's pool,
// with a single "sonnet-5" anthropic model row and a mock provider.
func newAPIModeSpawner(env *contextSaveTestEnv) *Spawner {
	return New(Config{
		DataPath: env.dbPath,
		Pool:     db.WrapAsPool(env.database),
		Clock:    clock.Real(),
		APIMode:  true,
		BuildAPIProvider: func(_ context.Context, _, _ string) (provider.Provider, error) {
			return mock.New(), nil
		},
		ModelConfigs: map[string]ModelConfig{
			"sonnet-5": {Provider: "anthropic", APIModel: "claude-sonnet-4-6", APIContext: 200000},
		},
		AgentSvc: &noopAgentSvc{},
		Workflows: map[string]WorkflowDef{
			"feature": {Phases: []PhaseDef{{NodeID: "impl", Agent: "impl", Layer: 0}}},
		},
	})
}

func prepareAPISpawn(t *testing.T, sp *Spawner, env *contextSaveTestEnv) *prepResult {
	t.Helper()
	_, prep, err := sp.prepareSpawn(context.Background(), SpawnRequest{
		AgentType:          "impl",
		ProjectID:          env.projectID,
		WorkflowName:       "feature",
		WorkflowInstanceID: env.wfiID,
	}, "claude:sonnet-5", "impl", env.wfiID)
	if err != nil {
		t.Fatalf("prepareSpawn() error: %v", err)
	}
	return prep
}

// TestRenderInjectable_APISystemPrompt_FreshDBByteIdentity verifies that on a
// freshly migrated DB, the api-system-prompt injectable renders byte-identical
// to the defaultAPISystemPrompt constant (nil vars — the seeded text has no
// placeholders).
func TestRenderInjectable_APISystemPrompt_FreshDBByteIdentity(t *testing.T) {
	t.Parallel()
	env := setupContextSaveTestEnv(t)
	defer env.cleanup()

	pool := db.WrapAsPool(env.database)
	rendered := renderInjectable(context.Background(), pool, "api-system-prompt", nil)
	if rendered != defaultAPISystemPrompt {
		t.Fatalf("renderInjectable(api-system-prompt) = %q, want %q", rendered, defaultAPISystemPrompt)
	}
}

// TestPrepareSpawn_APIMode_SystemPromptSeedByteIdentity verifies prep.apiSystem
// from a real prepareSpawn call is prefixed with the exact seeded/constant
// text on a fresh DB.
func TestPrepareSpawn_APIMode_SystemPromptSeedByteIdentity(t *testing.T) {
	t.Parallel()
	env := setupContextSaveTestEnv(t)
	defer env.cleanup()
	insertAPIAgentDef(t, env, "impl", "sonnet-5")

	prep := prepareAPISpawn(t, newAPIModeSpawner(env), env)

	if !strings.HasPrefix(prep.apiSystem, defaultAPISystemPrompt) {
		t.Errorf("prep.apiSystem = %q, want prefix %q", prep.apiSystem, defaultAPISystemPrompt)
	}
}

// TestPrepareSpawn_APIMode_SystemPromptSuffixAppended verifies the rendered
// system-prompt-suffix injectable (migration 000132's completion contract) is
// appended after the api-system-prompt body, matching CLI-mode behavior.
func TestPrepareSpawn_APIMode_SystemPromptSuffixAppended(t *testing.T) {
	t.Parallel()
	env := setupContextSaveTestEnv(t)
	defer env.cleanup()
	insertAPIAgentDef(t, env, "impl", "sonnet-5")

	prep := prepareAPISpawn(t, newAPIModeSpawner(env), env)

	if !strings.HasPrefix(prep.apiSystem, defaultAPISystemPrompt) {
		t.Fatalf("prep.apiSystem = %q, want prefix %q", prep.apiSystem, defaultAPISystemPrompt)
	}
	if !strings.Contains(prep.apiSystem, "## Completion Contract") {
		t.Errorf("prep.apiSystem missing rendered system-prompt-suffix; got %q", prep.apiSystem)
	}
}

// TestPrepareSpawn_APIMode_SystemPromptTemplateEditReflectsOnNextSpawn proves
// an UPDATE to the api-system-prompt row is picked up by the next prepareSpawn
// call with no rebuild/restart needed.
func TestPrepareSpawn_APIMode_SystemPromptTemplateEditReflectsOnNextSpawn(t *testing.T) {
	t.Parallel()
	env := setupContextSaveTestEnv(t)
	defer env.cleanup()
	insertAPIAgentDef(t, env, "impl", "sonnet-5")

	sp := newAPIModeSpawner(env)

	before := prepareAPISpawn(t, sp, env)
	if !strings.HasPrefix(before.apiSystem, defaultAPISystemPrompt) {
		t.Fatalf("baseline prep.apiSystem = %q, want prefix %q", before.apiSystem, defaultAPISystemPrompt)
	}

	pool := db.WrapAsPool(env.database)
	if _, err := pool.Exec(`UPDATE default_templates SET template = 'CUSTOM-XYZ' WHERE id = 'api-system-prompt'`); err != nil {
		t.Fatalf("update api-system-prompt template: %v", err)
	}

	after := prepareAPISpawn(t, sp, env)
	if !strings.HasPrefix(after.apiSystem, "CUSTOM-XYZ") {
		t.Errorf("prep.apiSystem = %q, want prefix %q after template edit", after.apiSystem, "CUSTOM-XYZ")
	}
	if strings.HasPrefix(after.apiSystem, defaultAPISystemPrompt) {
		t.Errorf("prep.apiSystem = %q, still starts with stale constant after template edit", after.apiSystem)
	}
}

// TestPrepareSpawn_APIMode_SystemPromptFallsBackWhenRowMissingOrEmpty verifies
// prepareSpawn falls back to the defaultAPISystemPrompt constant when the
// api-system-prompt row is deleted or blanked.
func TestPrepareSpawn_APIMode_SystemPromptFallsBackWhenRowMissingOrEmpty(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name string
		sql  string
	}{
		{name: "row deleted", sql: `DELETE FROM default_templates WHERE id = 'api-system-prompt'`},
		{name: "row emptied", sql: `UPDATE default_templates SET template = '' WHERE id = 'api-system-prompt'`},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			env := setupContextSaveTestEnv(t)
			defer env.cleanup()
			insertAPIAgentDef(t, env, "impl", "sonnet-5")

			pool := db.WrapAsPool(env.database)
			if _, err := pool.Exec(tc.sql); err != nil {
				t.Fatalf("mutate api-system-prompt row: %v", err)
			}

			prep := prepareAPISpawn(t, newAPIModeSpawner(env), env)

			if !strings.HasPrefix(prep.apiSystem, defaultAPISystemPrompt) {
				t.Errorf("prep.apiSystem = %q, want fallback prefix %q", prep.apiSystem, defaultAPISystemPrompt)
			}
		})
	}
}
