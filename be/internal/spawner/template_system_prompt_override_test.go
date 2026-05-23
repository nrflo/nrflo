package spawner

import (
	"strings"
	"testing"

	"github.com/google/uuid"
)

// TestLoadTemplate_SystemPromptOverride_ClaudeWithToggle verifies that the 3rd return
// value of loadTemplate is non-empty when the model has CLIType="claude" and
// OverrideSystemPrompt=true (seeded by migration 000126 injectable "system-prompt").
func TestLoadTemplate_SystemPromptOverride_ClaudeWithToggle(t *testing.T) {
	t.Parallel()
	env := newSpawnerTestEnv(t)
	ticketID := "SPO-" + uuid.New().String()[:6]
	env.initWorkflow(t, ticketID)
	createAgentDef(t, env, "analyzer", "Test body")

	sp := &Spawner{
		config: Config{
			DataPath: env.dbPath,
			Pool:     env.pool,
			ModelConfigs: map[string]ModelConfig{
				"sonnet": {CLIType: "claude", OverrideSystemPrompt: true},
			},
		},
	}

	body, suffix, override, err := sp.loadTemplate("analyzer", ticketID, env.project,
		"p", "c", "test", "claude:sonnet", "", "", nil, 0)
	if err != nil {
		t.Fatalf("loadTemplate failed: %v", err)
	}
	if body == "" {
		t.Error("body should not be empty")
	}
	if override == "" {
		t.Error("systemPromptOverride should be non-empty for CLIType=claude with OverrideSystemPrompt=true")
	}
	// Suffix must still be present and correct regardless of override setting
	if suffix == "" || !strings.Contains(suffix, "Completion Contract") {
		t.Errorf("suffix should contain 'Completion Contract', got: %q", suffix)
	}
}

// TestLoadTemplate_SystemPromptOverride_ClaudeToggleFalse verifies that the 3rd return
// value is empty when CLIType="claude" but OverrideSystemPrompt=false.
func TestLoadTemplate_SystemPromptOverride_ClaudeToggleFalse(t *testing.T) {
	t.Parallel()
	env := newSpawnerTestEnv(t)
	ticketID := "SPO-" + uuid.New().String()[:6]
	env.initWorkflow(t, ticketID)
	createAgentDef(t, env, "analyzer", "Test body")

	sp := &Spawner{
		config: Config{
			DataPath: env.dbPath,
			Pool:     env.pool,
			ModelConfigs: map[string]ModelConfig{
				"sonnet": {CLIType: "claude", OverrideSystemPrompt: false},
			},
		},
	}

	_, suffix, override, err := sp.loadTemplate("analyzer", ticketID, env.project,
		"p", "c", "test", "claude:sonnet", "", "", nil, 0)
	if err != nil {
		t.Fatalf("loadTemplate failed: %v", err)
	}
	if override != "" {
		t.Errorf("systemPromptOverride should be empty when OverrideSystemPrompt=false, got: %q", override)
	}
	if suffix == "" || !strings.Contains(suffix, "Completion Contract") {
		t.Errorf("suffix should still contain 'Completion Contract', got: %q", suffix)
	}
}

// TestLoadTemplate_SystemPromptOverride_NonClaudeCLIType verifies that the 3rd return
// value is empty when the model has a non-claude CLIType, even if OverrideSystemPrompt=true.
func TestLoadTemplate_SystemPromptOverride_NonClaudeCLIType(t *testing.T) {
	t.Parallel()
	env := newSpawnerTestEnv(t)
	ticketID := "SPO-" + uuid.New().String()[:6]
	env.initWorkflow(t, ticketID)
	createAgentDef(t, env, "analyzer", "Test body")

	tests := []struct {
		name    string
		cliType string
		model   string
		modelID string
	}{
		{"opencode", "opencode", "opencode_minimax_m25_free", "opencode:opencode_minimax_m25_free"},
		{"codex", "codex", "codex_gpt_high", "codex:codex_gpt_high"},
		{"gemini", "gemini", "gemini_pro", "gemini:gemini_pro"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			sp := &Spawner{
				config: Config{
					DataPath: env.dbPath,
					Pool:     env.pool,
					ModelConfigs: map[string]ModelConfig{
						tt.model: {CLIType: tt.cliType, OverrideSystemPrompt: true},
					},
				},
			}

			_, suffix, override, err := sp.loadTemplate("analyzer", ticketID, env.project,
				"p", "c", "test", tt.modelID, "", "", nil, 0)
			if err != nil {
				t.Fatalf("loadTemplate failed: %v", err)
			}
			if override != "" {
				t.Errorf("systemPromptOverride should be empty for CLIType=%q, got: %q", tt.cliType, override)
			}
			if suffix == "" || !strings.Contains(suffix, "Completion Contract") {
				t.Errorf("suffix should still be present, got: %q", suffix)
			}
		})
	}
}

// TestLoadTemplate_SystemPromptOverride_ModelAbsentFromConfigs verifies that the 3rd
// return value is empty when the model is not present in ModelConfigs at all.
func TestLoadTemplate_SystemPromptOverride_ModelAbsentFromConfigs(t *testing.T) {
	t.Parallel()
	env := newSpawnerTestEnv(t)
	ticketID := "SPO-" + uuid.New().String()[:6]
	env.initWorkflow(t, ticketID)
	createAgentDef(t, env, "analyzer", "Test body")

	sp := &Spawner{
		config: Config{
			DataPath: env.dbPath,
			Pool:     env.pool,
			ModelConfigs: map[string]ModelConfig{
				"other-model": {CLIType: "claude", OverrideSystemPrompt: true},
			},
		},
	}

	// "sonnet" is absent from ModelConfigs
	_, suffix, override, err := sp.loadTemplate("analyzer", ticketID, env.project,
		"p", "c", "test", "claude:sonnet", "", "", nil, 0)
	if err != nil {
		t.Fatalf("loadTemplate failed: %v", err)
	}
	if override != "" {
		t.Errorf("systemPromptOverride should be empty when model absent from ModelConfigs, got: %q", override)
	}
	if suffix == "" || !strings.Contains(suffix, "Completion Contract") {
		t.Errorf("suffix should still be present, got: %q", suffix)
	}
}

// TestLoadTemplate_SystemPromptOverride_NilModelConfigs verifies that the 3rd return
// value is empty when ModelConfigs is nil (no DB-sourced config).
func TestLoadTemplate_SystemPromptOverride_NilModelConfigs(t *testing.T) {
	t.Parallel()
	env := newSpawnerTestEnv(t)
	ticketID := "SPO-" + uuid.New().String()[:6]
	env.initWorkflow(t, ticketID)
	createAgentDef(t, env, "analyzer", "Test body")

	sp := env.newSpawner() // no ModelConfigs set

	_, suffix, override, err := sp.loadTemplate("analyzer", ticketID, env.project,
		"p", "c", "test", "claude:sonnet", "", "", nil, 0)
	if err != nil {
		t.Fatalf("loadTemplate failed: %v", err)
	}
	if override != "" {
		t.Errorf("systemPromptOverride should be empty with nil ModelConfigs, got: %q", override)
	}
	if suffix == "" || !strings.Contains(suffix, "Completion Contract") {
		t.Errorf("suffix should still be present, got: %q", suffix)
	}
}

// TestSystemPromptOverrideFor_ClaudeWithToggle tests the helper directly for gating logic.
func TestSystemPromptOverrideFor_ClaudeWithToggle(t *testing.T) {
	t.Parallel()
	env := newSpawnerTestEnv(t)

	sp := &Spawner{
		config: Config{
			DataPath: env.dbPath,
			Pool:     env.pool,
			ModelConfigs: map[string]ModelConfig{
				"opus_4_7": {CLIType: "claude", OverrideSystemPrompt: true},
			},
		},
	}

	got := sp.systemPromptOverrideFor("opus_4_7", nil)
	if got == "" {
		t.Error("systemPromptOverrideFor should return non-empty for claude+OverrideSystemPrompt=true")
	}
}

// TestSystemPromptOverrideFor_GatesOnCLITypeAndToggle table-tests all gating combinations.
func TestSystemPromptOverrideFor_GatesOnCLITypeAndToggle(t *testing.T) {
	t.Parallel()
	env := newSpawnerTestEnv(t)

	tests := []struct {
		name         string
		configs      map[string]ModelConfig
		model        string
		wantNonEmpty bool
	}{
		{
			name:         "claude+override=true → non-empty",
			configs:      map[string]ModelConfig{"m": {CLIType: "claude", OverrideSystemPrompt: true}},
			model:        "m",
			wantNonEmpty: true,
		},
		{
			name:         "claude+override=false → empty",
			configs:      map[string]ModelConfig{"m": {CLIType: "claude", OverrideSystemPrompt: false}},
			model:        "m",
			wantNonEmpty: false,
		},
		{
			name:         "opencode+override=true → empty",
			configs:      map[string]ModelConfig{"m": {CLIType: "opencode", OverrideSystemPrompt: true}},
			model:        "m",
			wantNonEmpty: false,
		},
		{
			name:         "model absent → empty",
			configs:      map[string]ModelConfig{"other": {CLIType: "claude", OverrideSystemPrompt: true}},
			model:        "m",
			wantNonEmpty: false,
		},
		{
			name:         "nil configs → empty",
			configs:      nil,
			model:        "m",
			wantNonEmpty: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			sp := &Spawner{
				config: Config{
					DataPath:     env.dbPath,
					Pool:         env.pool,
					ModelConfigs: tt.configs,
				},
			}
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
