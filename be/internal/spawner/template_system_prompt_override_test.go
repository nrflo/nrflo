package spawner

import (
	"strings"
	"testing"

	"github.com/google/uuid"
)

// The former per-model toggle was replaced by the global
// claude_system_prompt_override_enabled setting (seeded false by migration
// 000130, the override body re-seeded by migration 000129). systemPromptOverrideFor
// now gates on CLIType=="claude" AND a fresh spawn-time read of that global key.

// setSysPromptOverride flips the global claude_system_prompt_override_enabled key
// on the test pool. Passing on=false explicitly stores "false" (distinct from unset).
func setSysPromptOverride(t *testing.T, env *spawnerTestEnv, on bool) {
	t.Helper()
	val := "false"
	if on {
		val = "true"
	}
	if err := env.pool.SetConfig("claude_system_prompt_override_enabled", val); err != nil {
		t.Fatalf("SetConfig(claude_system_prompt_override_enabled): %v", err)
	}
}

// claudeOverrideSpawner builds a template-only Spawner backed by env.pool with the
// given ModelConfigs map.
func claudeOverrideSpawner(env *spawnerTestEnv, configs map[string]ModelConfig) *Spawner {
	return &Spawner{
		config: Config{
			DataPath:     env.dbPath,
			Pool:         env.pool,
			ModelConfigs: configs,
		},
	}
}

// TestLoadTemplate_SystemPromptOverride_ClaudeWithToggle verifies that the 3rd return
// value of loadTemplate is non-empty when the model has CLIType="claude" and the
// global claude_system_prompt_override_enabled setting is on.
func TestLoadTemplate_SystemPromptOverride_ClaudeWithToggle(t *testing.T) {
	t.Parallel()
	env := newSpawnerTestEnv(t)
	ticketID := "SPO-" + uuid.New().String()[:6]
	env.initWorkflow(t, ticketID)
	createAgentDef(t, env, "analyzer", "Test body")
	setSysPromptOverride(t, env, true)

	sp := claudeOverrideSpawner(env, map[string]ModelConfig{"sonnet": {CLIType: "claude"}})

	body, suffix, override, err := sp.loadTemplate("analyzer", ticketID, env.project,
		"p", "c", "test", "claude:sonnet", "", "", nil, 0)
	if err != nil {
		t.Fatalf("loadTemplate failed: %v", err)
	}
	if body == "" {
		t.Error("body should not be empty")
	}
	if override == "" {
		t.Error("systemPromptOverride should be non-empty for claude with the global toggle on")
	}
	// Suffix must still be present and correct regardless of override setting.
	if suffix == "" || !strings.Contains(suffix, "Completion Contract") {
		t.Errorf("suffix should contain 'Completion Contract', got: %q", suffix)
	}
}

// TestLoadTemplate_SystemPromptOverride_Gating table-tests every gating combination
// of the global key × CLIType × ModelConfigs presence at the loadTemplate boundary.
// The suffix (system-prompt-suffix injectable) must always be present.
func TestLoadTemplate_SystemPromptOverride_Gating(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name         string
		keyState     string // "true", "false", or "" (unset)
		configs      map[string]ModelConfig
		modelID      string
		wantNonEmpty bool
	}{
		{
			name:         "claude+key=true → non-empty",
			keyState:     "true",
			configs:      map[string]ModelConfig{"sonnet": {CLIType: "claude"}},
			modelID:      "claude:sonnet",
			wantNonEmpty: true,
		},
		{
			name:         "claude+key=false → empty",
			keyState:     "false",
			configs:      map[string]ModelConfig{"sonnet": {CLIType: "claude"}},
			modelID:      "claude:sonnet",
			wantNonEmpty: false,
		},
		{
			name:         "claude+key unset → empty",
			keyState:     "",
			configs:      map[string]ModelConfig{"sonnet": {CLIType: "claude"}},
			modelID:      "claude:sonnet",
			wantNonEmpty: false,
		},
		{
			name:         "codex+key=true → empty (non-claude)",
			keyState:     "true",
			configs:      map[string]ModelConfig{"codex_gpt_high": {CLIType: "codex"}},
			modelID:      "codex:codex_gpt_high",
			wantNonEmpty: false,
		},
		{
			name:         "model absent from configs → empty",
			keyState:     "true",
			configs:      map[string]ModelConfig{"other-model": {CLIType: "claude"}},
			modelID:      "claude:sonnet",
			wantNonEmpty: false,
		},
		{
			name:         "nil configs → empty",
			keyState:     "true",
			configs:      nil,
			modelID:      "claude:sonnet",
			wantNonEmpty: false,
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			env := newSpawnerTestEnv(t)
			ticketID := "SPO-" + uuid.New().String()[:6]
			env.initWorkflow(t, ticketID)
			createAgentDef(t, env, "analyzer", "Test body")
			if tt.keyState != "" {
				setSysPromptOverride(t, env, tt.keyState == "true")
			}

			sp := claudeOverrideSpawner(env, tt.configs)
			_, suffix, override, err := sp.loadTemplate("analyzer", ticketID, env.project,
				"p", "c", "test", tt.modelID, "", "", nil, 0)
			if err != nil {
				t.Fatalf("loadTemplate failed: %v", err)
			}
			if tt.wantNonEmpty && override == "" {
				t.Errorf("systemPromptOverride = empty, want non-empty")
			}
			if !tt.wantNonEmpty && override != "" {
				t.Errorf("systemPromptOverride = %q, want empty", override)
			}
			// The suffix is independent of the override toggle.
			if suffix == "" || !strings.Contains(suffix, "Completion Contract") {
				t.Errorf("suffix should contain 'Completion Contract', got: %q", suffix)
			}
		})
	}
}

// TestSystemPromptOverrideFor_GatesOnGlobalKeyAndCLIType exercises the helper directly
// for the same gating matrix (no template body / workflow needed).
func TestSystemPromptOverrideFor_GatesOnGlobalKeyAndCLIType(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name         string
		keyState     string // "true", "false", or "" (unset)
		configs      map[string]ModelConfig
		model        string
		wantNonEmpty bool
	}{
		{
			name:         "claude+key=true → non-empty",
			keyState:     "true",
			configs:      map[string]ModelConfig{"opus_4_7": {CLIType: "claude"}},
			model:        "opus_4_7",
			wantNonEmpty: true,
		},
		{
			name:         "claude+key=false → empty",
			keyState:     "false",
			configs:      map[string]ModelConfig{"opus_4_7": {CLIType: "claude"}},
			model:        "opus_4_7",
			wantNonEmpty: false,
		},
		{
			name:         "claude+key unset → empty",
			keyState:     "",
			configs:      map[string]ModelConfig{"opus_4_7": {CLIType: "claude"}},
			model:        "opus_4_7",
			wantNonEmpty: false,
		},
		{
			name:         "codex+key=true → empty",
			keyState:     "true",
			configs:      map[string]ModelConfig{"m": {CLIType: "codex"}},
			model:        "m",
			wantNonEmpty: false,
		},
		{
			name:         "model absent → empty",
			keyState:     "true",
			configs:      map[string]ModelConfig{"other": {CLIType: "claude"}},
			model:        "m",
			wantNonEmpty: false,
		},
		{
			name:         "nil configs → empty",
			keyState:     "true",
			configs:      nil,
			model:        "m",
			wantNonEmpty: false,
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			env := newSpawnerTestEnv(t)
			if tt.keyState != "" {
				setSysPromptOverride(t, env, tt.keyState == "true")
			}
			sp := claudeOverrideSpawner(env, tt.configs)
			got := sp.systemPromptOverrideFor(tt.model, nil)
			if tt.wantNonEmpty && got == "" {
				t.Errorf("systemPromptOverrideFor(%q) = empty, want non-empty", tt.model)
			}
			if !tt.wantNonEmpty && got != "" {
				t.Errorf("systemPromptOverrideFor(%q) = %q, want empty", tt.model, got)
			}
		})
	}
}

// TestSystemPromptOverrideFor_NilPool guards the nil-pool branch: a Spawner with a
// claude model config but no pool returns "" rather than panicking.
func TestSystemPromptOverrideFor_NilPool(t *testing.T) {
	t.Parallel()
	sp := &Spawner{
		config: Config{
			ModelConfigs: map[string]ModelConfig{"sonnet": {CLIType: "claude"}},
		},
	}
	if got := sp.systemPromptOverrideFor("sonnet", nil); got != "" {
		t.Errorf("systemPromptOverrideFor with nil pool = %q, want empty", got)
	}
}
