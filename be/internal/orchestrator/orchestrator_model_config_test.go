package orchestrator

import (
	"testing"

	"be/internal/service"
	"be/internal/spawner"
	"be/internal/types"
)

func TestCLINameFromModelConfigs_DerivesProvider(t *testing.T) {
	t.Parallel()
	tests := []struct {
		model    string
		configs  map[string]spawner.ModelConfig
		expected string
	}{
		{"opus-4-8", map[string]spawner.ModelConfig{"opus-4-8": {Provider: "anthropic"}}, "claude"},
		{"gpt-5.6-sol", map[string]spawner.ModelConfig{"gpt-5.6-sol": {Provider: "openai"}}, "codex"},
		{"raw-model", nil, "claude"},
	}
	for _, tt := range tests {
		if got := cliNameFromModelConfigs(tt.configs, tt.model); got != tt.expected {
			t.Errorf("cliNameFromModelConfigs(%q) = %q, want %q", tt.model, got, tt.expected)
		}
	}
}

func TestLoadModelConfigs_ContainsUnifiedSeeds(t *testing.T) {
	env := newTestEnv(t)
	configs, err := env.orch.loadModelConfigs(env.pool)
	if err != nil {
		t.Fatalf("loadModelConfigs() error: %v", err)
	}
	for _, id := range []string{"sonnet-5", "opus-4-8-1m", "gpt-5.4", "gpt-5.6-sol"} {
		if _, ok := configs[id]; !ok {
			t.Errorf("loadModelConfigs() missing %q", id)
		}
	}
	opus := configs["opus-4-8-1m"]
	if opus.Provider != "anthropic" || opus.CLIContext != 1000000 || opus.APIContext != 1000000 {
		t.Errorf("opus config = %+v", opus)
	}
	gpt := configs["gpt-5.6-sol"]
	if gpt.Provider != "openai" || gpt.CLIModel != "gpt-5.6-sol" || gpt.APIModel != "gpt-5.6-sol" {
		t.Errorf("gpt config = %+v", gpt)
	}
}

func TestLoadModelConfigs_CustomAndDisabled(t *testing.T) {
	env := newTestEnv(t)
	svc := service.NewModelService(env.pool, env.orch.clock)
	_, err := svc.Create(types.ModelCreateRequest{
		ID: "custom-model", Provider: "openai", DisplayName: "Custom",
		CLIModel: "gpt-custom", APIModel: "gpt-custom-api",
		CLIEfforts: []string{"medium"}, APIEfforts: []string{"medium"}, DefaultEffort: "medium",
	})
	if err != nil {
		t.Fatalf("create model: %v", err)
	}
	if _, err := env.pool.Exec(`UPDATE models SET enabled = 0 WHERE id = 'gpt-5.4-mini'`); err != nil {
		t.Fatalf("disable model: %v", err)
	}
	configs, err := env.orch.loadModelConfigs(env.pool)
	if err != nil {
		t.Fatalf("loadModelConfigs() error: %v", err)
	}
	if got := configs["custom-model"]; got.CLIModel != "gpt-custom" || got.APIModel != "gpt-custom-api" {
		t.Errorf("custom config = %+v", got)
	}
	if _, ok := configs["gpt-5.4-mini"]; ok {
		t.Error("disabled model was loaded")
	}
}
