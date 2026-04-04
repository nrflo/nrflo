package orchestrator

import (
	"testing"

	"be/internal/service"
	"be/internal/spawner"
	"be/internal/types"
)

// ── cliNameFromModelConfigs ────────────────────────────────────────────────────

func TestCLINameFromModelConfigs_UsesDBCLIType(t *testing.T) {
	tests := []struct {
		name    string
		model   string
		configs map[string]spawner.ModelConfig
		want    string
	}{
		{
			name:    "claude model returns claude from DB",
			model:   "opus",
			configs: map[string]spawner.ModelConfig{"opus": {CLIType: "claude"}},
			want:    "claude",
		},
		{
			name:    "DB codex type overrides default for non-codex model",
			model:   "opus",
			configs: map[string]spawner.ModelConfig{"opus": {CLIType: "codex"}},
			want:    "codex",
		},
		{
			name:    "DB claude type overrides opencode-prefix default",
			model:   "opencode_minimax_m25_free",
			configs: map[string]spawner.ModelConfig{"opencode_minimax_m25_free": {CLIType: "claude"}},
			want:    "claude",
		},
		{
			name:    "DB opencode type for custom model",
			model:   "my-custom-model",
			configs: map[string]spawner.ModelConfig{"my-custom-model": {CLIType: "opencode"}},
			want:    "opencode",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := cliNameFromModelConfigs(tt.configs, tt.model)
			if got != tt.want {
				t.Errorf("cliNameFromModelConfigs(configs, %q) = %q, want %q", tt.model, got, tt.want)
			}
		})
	}
}

func TestCLINameFromModelConfigs_FallsBackToDefault(t *testing.T) {
	tests := []struct {
		name    string
		model   string
		configs map[string]spawner.ModelConfig
		want    string
	}{
		{
			name:    "nil configs falls back for claude model",
			model:   "opus",
			configs: nil,
			want:    "claude",
		},
		{
			name:    "empty configs falls back for opencode model",
			model:   "opencode_minimax_m25_free",
			configs: map[string]spawner.ModelConfig{},
			want:    "opencode",
		},
		{
			name:    "model not in configs map falls back",
			model:   "codex_gpt_high",
			configs: map[string]spawner.ModelConfig{"other": {CLIType: "opencode"}},
			want:    "codex",
		},
		{
			name:    "empty CLIType in DB entry falls back to default",
			model:   "opus",
			configs: map[string]spawner.ModelConfig{"opus": {CLIType: "", ContextLength: 200000}},
			want:    "claude",
		},
		{
			name:    "codex prefix model without DB entry uses hardcoded default",
			model:   "codex_gpt_normal",
			configs: nil,
			want:    "codex",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := cliNameFromModelConfigs(tt.configs, tt.model)
			if got != tt.want {
				t.Errorf("cliNameFromModelConfigs(configs, %q) = %q, want %q", tt.model, got, tt.want)
			}
		})
	}
}

// ── loadModelConfigs ───────────────────────────────────────────────────────────

func TestLoadModelConfigs_ContainsSeedModels(t *testing.T) {
	env := newTestEnv(t)

	configs, err := env.orch.loadModelConfigs(env.pool)
	if err != nil {
		t.Fatalf("loadModelConfigs() error: %v", err)
	}

	// All seeded models with their expected CLIType
	expected := map[string]string{
		"opus":               "claude",
		"opus_1m":            "claude",
		"sonnet":             "claude",
		"haiku":              "claude",
		"opencode_minimax_m25_free": "opencode",
		"opencode_qwen36_plus_free": "opencode",
		"opencode_gpt54":            "opencode",
		"codex_gpt_normal":    "codex",
		"codex_gpt_high":      "codex",
		"codex_gpt54_normal":  "codex",
		"codex_gpt54_high":    "codex",
	}

	for model, wantCLI := range expected {
		mc, ok := configs[model]
		if !ok {
			t.Errorf("loadModelConfigs() missing seeded model %q", model)
			continue
		}
		if mc.CLIType != wantCLI {
			t.Errorf("configs[%q].CLIType = %q, want %q", model, mc.CLIType, wantCLI)
		}
	}
}

func TestLoadModelConfigs_ModelConfigFields(t *testing.T) {
	env := newTestEnv(t)

	configs, err := env.orch.loadModelConfigs(env.pool)
	if err != nil {
		t.Fatalf("loadModelConfigs() error: %v", err)
	}

	// opus_1m should have 1M context and mapped model opus[1m]
	opus1m, ok := configs["opus_1m"]
	if !ok {
		t.Fatal("loadModelConfigs() missing 'opus_1m'")
	}
	if opus1m.MappedModel != "opus[1m]" {
		t.Errorf("opus_1m MappedModel = %q, want %q", opus1m.MappedModel, "opus[1m]")
	}
	if opus1m.ContextLength != 1000000 {
		t.Errorf("opus_1m ContextLength = %d, want 1000000", opus1m.ContextLength)
	}

	// codex_gpt54_normal should have reasoning effort "medium"
	codex54, ok := configs["codex_gpt54_normal"]
	if !ok {
		t.Fatal("loadModelConfigs() missing 'codex_gpt54_normal'")
	}
	if codex54.ReasoningEffort != "medium" {
		t.Errorf("codex_gpt54_normal ReasoningEffort = %q, want %q", codex54.ReasoningEffort, "medium")
	}
}

func TestLoadModelConfigs_CustomModelIncluded(t *testing.T) {
	env := newTestEnv(t)

	// Add a custom model via CLIModelService
	cliModelSvc := service.NewCLIModelService(env.pool, env.orch.clock)
	_, err := cliModelSvc.Create(types.CLIModelCreateRequest{
		ID:          "my-custom-gpt",
		CLIType:     "opencode",
		DisplayName: "My Custom GPT",
		MappedModel: "openai/custom-gpt",
	})
	if err != nil {
		t.Fatalf("CLIModelService.Create: %v", err)
	}

	configs, err := env.orch.loadModelConfigs(env.pool)
	if err != nil {
		t.Fatalf("loadModelConfigs() error: %v", err)
	}

	mc, ok := configs["my-custom-gpt"]
	if !ok {
		t.Fatal("loadModelConfigs() missing custom model 'my-custom-gpt'")
	}
	if mc.CLIType != "opencode" {
		t.Errorf("custom model CLIType = %q, want %q", mc.CLIType, "opencode")
	}
	if mc.MappedModel != "openai/custom-gpt" {
		t.Errorf("custom model MappedModel = %q, want %q", mc.MappedModel, "openai/custom-gpt")
	}
}
