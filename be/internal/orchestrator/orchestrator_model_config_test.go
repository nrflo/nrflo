package orchestrator

import (
	"testing"

	"be/internal/clock"
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
			model:   "opus_4_7",
			configs: map[string]spawner.ModelConfig{"opus_4_7": {CLIType: "claude"}},
			want:    "claude",
		},
		{
			name:    "DB codex type overrides default for non-codex model",
			model:   "opus_4_7",
			configs: map[string]spawner.ModelConfig{"opus_4_7": {CLIType: "codex"}},
			want:    "codex",
		},
		{
			name:    "DB codex type for custom model",
			model:   "my-custom-model",
			configs: map[string]spawner.ModelConfig{"my-custom-model": {CLIType: "codex"}},
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

func TestCLINameFromModelConfigs_FallsBackToDefault(t *testing.T) {
	tests := []struct {
		name    string
		model   string
		configs map[string]spawner.ModelConfig
		want    string
	}{
		{
			name:    "nil configs falls back for claude model",
			model:   "opus_4_7",
			configs: nil,
			want:    "claude",
		},
		{
			name:    "model not in configs map falls back",
			model:   "codex_gpt_high",
			configs: map[string]spawner.ModelConfig{"other": {CLIType: "claude"}},
			want:    "codex",
		},
		{
			name:    "empty CLIType in DB entry falls back to default",
			model:   "opus_4_7",
			configs: map[string]spawner.ModelConfig{"opus_4_7": {CLIType: "", ContextLength: 200000}},
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
		"opus_4_6":           "claude",
		"opus_4_6_1m":        "claude",
		"opus_4_7":           "claude",
		"opus_4_7_1m":        "claude",
		"sonnet":             "claude",
		"haiku":              "claude",
		"codex_gpt_normal":   "codex",
		"codex_gpt_high":     "codex",
		"codex_gpt54_normal": "codex",
		"codex_gpt54_high":   "codex",
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

	// opus_4_7_1m should have 1M context and mapped model claude-opus-4-7[1m]
	opus1m, ok := configs["opus_4_7_1m"]
	if !ok {
		t.Fatal("loadModelConfigs() missing 'opus_4_7_1m'")
	}
	if opus1m.MappedModel != "claude-opus-4-7[1m]" {
		t.Errorf("opus_4_7_1m MappedModel = %q, want %q", opus1m.MappedModel, "claude-opus-4-7[1m]")
	}
	if opus1m.ContextLength != 1000000 {
		t.Errorf("opus_4_7_1m ContextLength = %d, want 1000000", opus1m.ContextLength)
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
		CLIType:     "codex",
		DisplayName: "My Custom GPT",
		MappedModel: "gpt-custom",
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
	if mc.CLIType != "codex" {
		t.Errorf("custom model CLIType = %q, want %q", mc.CLIType, "codex")
	}
	if mc.MappedModel != "gpt-custom" {
		t.Errorf("custom model MappedModel = %q, want %q", mc.MappedModel, "gpt-custom")
	}
}

// ── loadAPIModelConfigs ──────────────────────────────────────────────────────

// TestLoadAPIModelConfigs_ReturnsSeededModels verifies that the seed api_models rows
// (opus_4_7, sonnet, haiku, gpt53_codex_high, …) loaded from the migration are
// returned as spawner.APIModelConfig entries keyed by row id.
func TestLoadAPIModelConfigs_ReturnsSeededModels(t *testing.T) {
	t.Parallel()
	env := newTestEnv(t)

	configs, err := env.orch.loadAPIModelConfigs(env.pool)
	if err != nil {
		t.Fatalf("loadAPIModelConfigs() error: %v", err)
	}
	for _, id := range []string{"opus_4_7", "sonnet", "haiku", "gpt53_codex_high"} {
		mc, ok := configs[id]
		if !ok {
			t.Errorf("loadAPIModelConfigs() missing seed model %q", id)
			continue
		}
		if mc.MappedModel == "" {
			t.Errorf("model %q: MappedModel is empty", id)
		}
		if mc.Provider == "" {
			t.Errorf("model %q: Provider is empty", id)
		}
	}
}

// TestLoadAPIModelConfigs_CustomModelIncluded verifies that a newly created (enabled) row
// appears in the map returned by loadAPIModelConfigs.
func TestLoadAPIModelConfigs_CustomModelIncluded(t *testing.T) {
	t.Parallel()
	env := newTestEnv(t)

	svc := service.NewAPIModelService(env.pool, clock.Real())
	if _, err := svc.Create(types.APIModelCreateRequest{
		ID:          "my-api-model",
		Provider:    "anthropic",
		DisplayName: "My API Model",
		MappedModel: "claude-custom",
	}); err != nil {
		t.Fatalf("Create api model: %v", err)
	}

	configs, err := env.orch.loadAPIModelConfigs(env.pool)
	if err != nil {
		t.Fatalf("loadAPIModelConfigs() error: %v", err)
	}
	mc, ok := configs["my-api-model"]
	if !ok {
		t.Fatal("loadAPIModelConfigs() missing 'my-api-model' after creation")
	}
	if mc.Provider != "anthropic" {
		t.Errorf("Provider = %q, want %q", mc.Provider, "anthropic")
	}
	if mc.MappedModel != "claude-custom" {
		t.Errorf("MappedModel = %q, want %q", mc.MappedModel, "claude-custom")
	}
}

// TestLoadAPIModelConfigs_DisabledRowExcluded verifies that rows with enabled=false
// are not returned by loadAPIModelConfigs.
func TestLoadAPIModelConfigs_DisabledRowExcluded(t *testing.T) {
	t.Parallel()
	env := newTestEnv(t)

	svc := service.NewAPIModelService(env.pool, clock.Real())
	if _, err := svc.Create(types.APIModelCreateRequest{
		ID:          "disabled-model",
		Provider:    "anthropic",
		DisplayName: "Disabled Model",
		MappedModel: "claude-disabled",
	}); err != nil {
		t.Fatalf("Create api model: %v", err)
	}
	// Disable the row via Update.
	disabled := false
	if _, err := svc.Update("disabled-model", types.APIModelUpdateRequest{Enabled: &disabled}); err != nil {
		t.Fatalf("Update (disable) api model: %v", err)
	}

	configs, err := env.orch.loadAPIModelConfigs(env.pool)
	if err != nil {
		t.Fatalf("loadAPIModelConfigs() error: %v", err)
	}
	if _, ok := configs["disabled-model"]; ok {
		t.Error("loadAPIModelConfigs() included disabled model; want excluded")
	}
}
