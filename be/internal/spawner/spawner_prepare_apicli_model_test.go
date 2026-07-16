package spawner

// Tests for the api-via-cli hybrid's CLI model-string selection. The Claude CLI
// picks its context window from the model string passed to --model (bare
// "claude-opus-4-8" opens 200k, the "[1m]" suffix opens 1M), not from
// proc.maxContext — so prepareAPIViaCLISpawn must encode the API window in the
// string when it exceeds the bare CLI window.

import (
	"context"
	"os"
	"testing"

	"be/internal/clock"
	"be/internal/db"
	"be/internal/spawner/apirun/provider"
	"be/internal/spawner/apirun/provider/mock"
)

// runAPIViaCLIModelSpawn spawns an api-via-cli hybrid for a single-row model
// config and returns the prep, so tests can assert the model string that reaches
// the CLI --model flag. MappedModel is what backend_interactive passes to --model.
func runAPIViaCLIModelSpawn(t *testing.T, cfg ModelConfig) *prepResult {
	t.Helper()
	ensureTmpNrfloDir(t)
	env := setupContextSaveTestEnv(t)
	t.Cleanup(env.cleanup)

	insertAPIAgentDefWithTools(t, env, "impl", "m", "agent_finished")

	sp := New(Config{
		DataPath:  env.dbPath,
		Pool:      db.WrapAsPool(env.database),
		Clock:     clock.Real(),
		APIMode:   true,
		APIViaCLI: true,
		BuildAPIProvider: func(_ context.Context, _ string, _ string) (provider.Provider, error) {
			return mock.New(), nil
		},
		ModelConfigs: map[string]ModelConfig{"m": cfg},
		AgentSvc:     &noopAgentSvc{},
		Workflows: map[string]WorkflowDef{
			"feature": {Phases: []PhaseDef{{NodeID: "impl", Agent: "impl", Layer: 0}}},
		},
	})

	_, prep, err := sp.prepareSpawn(context.Background(), SpawnRequest{
		AgentType:          "impl",
		ProjectID:          env.projectID,
		WorkflowName:       "feature",
		WorkflowInstanceID: env.wfiID,
	}, "claude:m", "impl", env.wfiID)
	if err != nil {
		t.Fatalf("prepareSpawn() error: %v", err)
	}
	if prep.systemPromptOverrideFile != "" {
		t.Cleanup(func() { os.Remove(prep.systemPromptOverrideFile) })
	}
	if prep.promptFile != "" {
		t.Cleanup(func() { os.Remove(prep.promptFile) })
	}
	return prep
}

// TestPrepareSpawn_APIViaCLI_ModelStringSelectsContext verifies that the model
// string reaching the CLI --model flag requests the 1M window (via the "[1m]"
// suffix) only when the API window exceeds the bare CLI window.
func TestPrepareSpawn_APIViaCLI_ModelStringSelectsContext(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name string
		cfg  ModelConfig
		want string
	}{
		{
			// opus-4-8-shaped: API 1M > CLI 200k, bare strings → suffix required.
			name: "opus_1m_over_200k_gets_suffix",
			cfg:  ModelConfig{Provider: "anthropic", APIModel: "claude-opus-4-8", APIContext: 1_000_000, CLIModel: "claude-opus-4-8", CLIContext: 200000},
			want: "claude-opus-4-8[1m]",
		},
		{
			// sonnet-5-shaped: API 1M, CLI already 1M → bare string opens 1M natively.
			name: "sonnet_1m_cli_1m_stays_bare",
			cfg:  ModelConfig{Provider: "anthropic", APIModel: "claude-sonnet-4-6", APIContext: 1_000_000, CLIModel: "claude-sonnet-4-6", CLIContext: 1_000_000},
			want: "claude-sonnet-4-6",
		},
		{
			// Already-suffixed APIModel (opus-*-1m rows) must not be double-suffixed.
			name: "already_suffixed_not_doubled",
			cfg:  ModelConfig{Provider: "anthropic", APIModel: "claude-opus-4-8[1m]", APIContext: 1_000_000, CLIModel: "claude-opus-4-8", CLIContext: 200000},
			want: "claude-opus-4-8[1m]",
		},
	}
	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			prep := runAPIViaCLIModelSpawn(t, tc.cfg)
			// MappedModel is the value backend_interactive feeds to --model.
			if prep.opts.MappedModel != tc.want {
				t.Errorf("opts.MappedModel = %q, want %q (reaches CLI --model)", prep.opts.MappedModel, tc.want)
			}
			if prep.opts.Model != tc.want {
				t.Errorf("opts.Model = %q, want %q", prep.opts.Model, tc.want)
			}
		})
	}
}
