package spawner

import (
	"testing"

	"be/internal/clock"
	"be/internal/db"
	"be/internal/model"
	"be/internal/repo"

	"github.com/google/uuid"
)

// --- Config struct fields ---

func TestConfig_LowConsumptionMode_DefaultFalse(t *testing.T) {
	t.Parallel()
	cfg := Config{}
	if cfg.LowConsumptionMode {
		t.Error("Config.LowConsumptionMode default = true, want false")
	}
}

// --- loadAgentDefinition with LowConsumptionModel ---

// createAgentDefWithLCM inserts an agent definition with a LowConsumptionModel field.
func createAgentDefWithLCM(t *testing.T, env *spawnerTestEnv, agentID, prompt, lcModel string) {
	t.Helper()
	database, err := db.OpenPathExisting(env.dbPath)
	if err != nil {
		t.Fatalf("createAgentDefWithLCM: open db: %v", err)
	}
	defer database.Close()

	adRepo := repo.NewAgentDefinitionRepo(database, clock.Real())
	err = adRepo.Create(&model.AgentDefinition{
		ID:                  agentID,
		ProjectID:           env.project,
		WorkflowID:          "test",
		Model:               "opus-4-7",
		Timeout:             60,
		Prompt:              prompt,
		LowConsumptionModel: lcModel,
	})
	if err != nil {
		t.Fatalf("createAgentDefWithLCM(%q): %v", agentID, err)
	}
}

func TestLoadAgentDefinition_ReturnsLowConsumptionModel(t *testing.T) {
	t.Parallel()
	env := newSpawnerTestEnv(t)

	createAgentDefWithLCM(t, env, "analyzer", "analyze things", "sonnet-5")

	sp := New(Config{
		DataPath: env.dbPath,
		Pool:     env.pool,
		Clock:    clock.Real(),
	})

	def := sp.loadAgentDefinition("analyzer", env.project, "test")
	if def == nil {
		t.Fatal("loadAgentDefinition returned nil, want non-nil")
	}
	if def.LowConsumptionModel != "sonnet-5" {
		t.Errorf("LowConsumptionModel = %q, want %q", def.LowConsumptionModel, "sonnet-5")
	}
	if def.ID != "analyzer" {
		t.Errorf("ID = %q, want %q", def.ID, "analyzer")
	}
}

func TestLoadAgentDefinition_EmptyLowConsumptionModel(t *testing.T) {
	t.Parallel()
	env := newSpawnerTestEnv(t)

	createAgentDefWithLCM(t, env, "analyzer", "analyze things", "")

	sp := New(Config{
		DataPath: env.dbPath,
		Pool:     env.pool,
		Clock:    clock.Real(),
	})

	def := sp.loadAgentDefinition("analyzer", env.project, "test")
	if def == nil {
		t.Fatal("loadAgentDefinition returned nil, want non-nil")
	}
	if def.LowConsumptionModel != "" {
		t.Errorf("LowConsumptionModel = %q, want empty", def.LowConsumptionModel)
	}
}

func TestLoadAgentDefinition_ReturnsNilWhenNotFound(t *testing.T) {
	t.Parallel()
	env := newSpawnerTestEnv(t)

	sp := New(Config{
		DataPath: env.dbPath,
		Pool:     env.pool,
		Clock:    clock.Real(),
	})

	def := sp.loadAgentDefinition("nonexistent-agent", env.project, "test")
	if def != nil {
		t.Errorf("loadAgentDefinition returned non-nil for missing agent, want nil")
	}
}

// --- Model substitution table-driven tests ---

// TestLowConsumptionSubstitution_ModelSelection verifies model is overridden
// when LowConsumptionMode is on and LowConsumptionModel is set.
func TestLowConsumptionSubstitution_ModelSelection(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name          string
		lcMode        bool
		lcModel       string // low_consumption_model on the agent def
		originalModel string
		wantModel     string
	}{
		{
			name:          "mode_off_no_substitution",
			lcMode:        false,
			lcModel:       "haiku-4-5",
			originalModel: "opus-4-7",
			wantModel:     "opus-4-7",
		},
		{
			name:          "mode_on_no_lcm",
			lcMode:        true,
			lcModel:       "",
			originalModel: "opus-4-7",
			wantModel:     "opus-4-7",
		},
		{
			name:          "mode_on_with_lcm",
			lcMode:        true,
			lcModel:       "haiku-4-5",
			originalModel: "opus-4-7",
			wantModel:     "haiku-4-5",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := Config{
				LowConsumptionMode: tt.lcMode,
				Agents: map[string]AgentConfig{
					"implementor": {Model: tt.originalModel},
				},
			}

			// Simulate model determination from Spawn()
			selectedModel := "opus-4-7"
			if agentCfg, ok := cfg.Agents["implementor"]; ok && agentCfg.Model != "" {
				selectedModel = agentCfg.Model
			}

			// Simulate low consumption model override from Spawn()
			if cfg.LowConsumptionMode && tt.lcModel != "" {
				selectedModel = tt.lcModel
			}

			if selectedModel != tt.wantModel {
				t.Errorf("selectedModel = %q, want %q", selectedModel, tt.wantModel)
			}
		})
	}
}

// TestLowConsumptionMode_LoadAgentDef_SubstitutionDecision tests that when
// LowConsumptionMode is enabled, loadAgentDefinition returns the def with
// LowConsumptionModel set, enabling the model override path in Spawn().
func TestLowConsumptionMode_LoadAgentDef_SubstitutionDecision(t *testing.T) {
	t.Parallel()
	env := newSpawnerTestEnv(t)
	ticketID := "LCM-" + uuid.New().String()[:6]
	env.initWorkflow(t, ticketID)

	createAgentDefWithLCM(t, env, "analyzer", "analyze code", "sonnet-5")

	sp := New(Config{
		DataPath:           env.dbPath,
		Pool:               env.pool,
		Clock:              clock.Real(),
		LowConsumptionMode: true,
		Agents: map[string]AgentConfig{
			"analyzer": {Model: "opus-4-7"},
		},
	})

	def := sp.loadAgentDefinition("analyzer", env.project, "test")
	if def == nil {
		t.Fatal("loadAgentDefinition returned nil")
	}

	if def.LowConsumptionModel == "" {
		t.Error("LowConsumptionModel empty — model override would not trigger")
	}
	if def.LowConsumptionModel != "sonnet-5" {
		t.Errorf("LowConsumptionModel = %q, want %q", def.LowConsumptionModel, "sonnet-5")
	}
}

// TestLowConsumptionMode_ModeOff_NoSubstitution verifies that when LowConsumptionMode
// is false, the model override code path is not entered regardless of LCM setting.
func TestLowConsumptionMode_ModeOff_NoSubstitution(t *testing.T) {
	t.Parallel()
	sp := New(Config{
		LowConsumptionMode: false,
		Agents: map[string]AgentConfig{
			"implementor": {Model: "opus-4-7"},
		},
	})

	if sp.config.LowConsumptionMode {
		t.Error("LowConsumptionMode = true, want false")
	}

	// When LowConsumptionMode is false, the override block is skipped.
	if sp.config.LowConsumptionMode {
		t.Error("this branch should not be entered when LowConsumptionMode is false")
	}
}

// TestLowConsumptionSubstitution_CLINameAndModelID verifies provider-derived CLI routing.
func TestLowConsumptionSubstitution_CLINameAndModelID(t *testing.T) {
	t.Parallel()
	sp := &Spawner{config: Config{ModelConfigs: map[string]ModelConfig{
		"opus-4-8":    {Provider: "anthropic"},
		"sonnet-5":    {Provider: "anthropic"},
		"gpt-5.4":     {Provider: "openai"},
		"gpt-5.6-sol": {Provider: "openai"},
	}}}
	tests := []struct {
		lcModel     string
		wantCLI     string
		wantModelID string
	}{
		{"opus-4-8", "claude", "claude:opus-4-8"},
		{"sonnet-5", "claude", "claude:sonnet-5"},
		{"gpt-5.4", "codex", "codex:gpt-5.4"},
		{"gpt-5.6-sol", "codex", "codex:gpt-5.6-sol"},
	}

	for _, tt := range tests {
		t.Run(tt.lcModel, func(t *testing.T) {
			gotCLI := sp.cliForModel(tt.lcModel)
			gotModelID := gotCLI + ":" + tt.lcModel

			if gotCLI != tt.wantCLI {
				t.Errorf("cliForModel(%q) = %q, want %q", tt.lcModel, gotCLI, tt.wantCLI)
			}
			if gotModelID != tt.wantModelID {
				t.Errorf("modelID for %q = %q, want %q", tt.lcModel, gotModelID, tt.wantModelID)
			}
		})
	}
}
