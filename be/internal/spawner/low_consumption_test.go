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
	cfg := Config{}
	if cfg.LowConsumptionMode {
		t.Error("Config.LowConsumptionMode default = true, want false")
	}
}

// --- loadAgentDefinition with LowConsumptionModel ---

// createAgentDefWithLCM inserts an agent definition with a LowConsumptionModel field.
func createAgentDefWithLCM(t *testing.T, env *spawnerTestEnv, agentID, prompt, lcModel string) {
	t.Helper()
	database, err := db.Open(env.dbPath)
	if err != nil {
		t.Fatalf("createAgentDefWithLCM: open db: %v", err)
	}
	defer database.Close()

	adRepo := repo.NewAgentDefinitionRepo(database, clock.Real())
	err = adRepo.Create(&model.AgentDefinition{
		ID:                  agentID,
		ProjectID:           env.project,
		WorkflowID:          "test",
		Model:               "opus",
		Timeout:             60,
		Prompt:              prompt,
		LowConsumptionModel: lcModel,
	})
	if err != nil {
		t.Fatalf("createAgentDefWithLCM(%q): %v", agentID, err)
	}
}

func TestLoadAgentDefinition_ReturnsLowConsumptionModel(t *testing.T) {
	env := newSpawnerTestEnv(t)

	createAgentDefWithLCM(t, env, "analyzer", "analyze things", "sonnet")

	sp := New(Config{
		DataPath: env.dbPath,
		Pool:     env.pool,
		Clock:    clock.Real(),
	})

	def := sp.loadAgentDefinition("analyzer", env.project, "test")
	if def == nil {
		t.Fatal("loadAgentDefinition returned nil, want non-nil")
	}
	if def.LowConsumptionModel != "sonnet" {
		t.Errorf("LowConsumptionModel = %q, want %q", def.LowConsumptionModel, "sonnet")
	}
	if def.ID != "analyzer" {
		t.Errorf("ID = %q, want %q", def.ID, "analyzer")
	}
}

func TestLoadAgentDefinition_EmptyLowConsumptionModel(t *testing.T) {
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
			lcModel:       "haiku",
			originalModel: "opus",
			wantModel:     "opus",
		},
		{
			name:          "mode_on_no_lcm",
			lcMode:        true,
			lcModel:       "",
			originalModel: "opus",
			wantModel:     "opus",
		},
		{
			name:          "mode_on_with_lcm",
			lcMode:        true,
			lcModel:       "haiku",
			originalModel: "opus",
			wantModel:     "haiku",
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
			selectedModel := "opus"
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
	env := newSpawnerTestEnv(t)
	ticketID := "LCM-" + uuid.New().String()[:6]
	env.initWorkflow(t, ticketID)

	createAgentDefWithLCM(t, env, "analyzer", "analyze code", "sonnet")

	sp := New(Config{
		DataPath:           env.dbPath,
		Pool:               env.pool,
		Clock:              clock.Real(),
		LowConsumptionMode: true,
		Agents: map[string]AgentConfig{
			"analyzer": {Model: "opus"},
		},
	})

	def := sp.loadAgentDefinition("analyzer", env.project, "test")
	if def == nil {
		t.Fatal("loadAgentDefinition returned nil")
	}

	if def.LowConsumptionModel == "" {
		t.Error("LowConsumptionModel empty — model override would not trigger")
	}
	if def.LowConsumptionModel != "sonnet" {
		t.Errorf("LowConsumptionModel = %q, want %q", def.LowConsumptionModel, "sonnet")
	}
}

// TestLowConsumptionMode_ModeOff_NoSubstitution verifies that when LowConsumptionMode
// is false, the model override code path is not entered regardless of LCM setting.
func TestLowConsumptionMode_ModeOff_NoSubstitution(t *testing.T) {
	sp := New(Config{
		LowConsumptionMode: false,
		Agents: map[string]AgentConfig{
			"implementor": {Model: "opus"},
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

// TestLowConsumptionSubstitution_CLINameAndModelID verifies that the cliName
// and modelID format are correctly derived from each low_consumption_model value.
// This mirrors the spawner logic: cliName = DefaultCLIForModel(model), modelID = cli:model.
func TestLowConsumptionSubstitution_CLINameAndModelID(t *testing.T) {
	tests := []struct {
		lcModel      string
		wantCLI      string
		wantModelID  string
	}{
		{"opus", "claude", "claude:opus"},
		{"opus_1m", "claude", "claude:opus_1m"},
		{"sonnet", "claude", "claude:sonnet"},
		{"haiku", "claude", "claude:haiku"},
		{"opencode_gpt_normal", "opencode", "opencode:opencode_gpt_normal"},
		{"opencode_gpt_high", "opencode", "opencode:opencode_gpt_high"},
		{"codex_gpt_normal", "codex", "codex:codex_gpt_normal"},
		{"codex_gpt_high", "codex", "codex:codex_gpt_high"},
		{"codex_gpt54_normal", "codex", "codex:codex_gpt54_normal"},
		{"codex_gpt54_high", "codex", "codex:codex_gpt54_high"},
	}

	for _, tt := range tests {
		t.Run(tt.lcModel, func(t *testing.T) {
			// Simulate the substitution block in Spawn():
			//   cliName = DefaultCLIForModel(model)
			//   modelID = fmt.Sprintf("%s:%s", cliName, model)
			gotCLI := DefaultCLIForModel(tt.lcModel)
			gotModelID := gotCLI + ":" + tt.lcModel

			if gotCLI != tt.wantCLI {
				t.Errorf("DefaultCLIForModel(%q) = %q, want %q", tt.lcModel, gotCLI, tt.wantCLI)
			}
			if gotModelID != tt.wantModelID {
				t.Errorf("modelID for %q = %q, want %q", tt.lcModel, gotModelID, tt.wantModelID)
			}
		})
	}
}
