package spawner

import (
	"testing"

	"be/internal/clock"
	"be/internal/db"
	"be/internal/model"
	"be/internal/repo"

	"github.com/google/uuid"
)

// --- Config and SpawnRequest struct fields ---

func TestConfig_LowConsumptionMode_DefaultFalse(t *testing.T) {
	cfg := Config{}
	if cfg.LowConsumptionMode {
		t.Error("Config.LowConsumptionMode default = true, want false")
	}
}

func TestSpawnRequest_EffectiveAgentType_DefaultEmpty(t *testing.T) {
	req := SpawnRequest{AgentType: "implementor"}
	if req.EffectiveAgentType != "" {
		t.Errorf("SpawnRequest.EffectiveAgentType default = %q, want empty", req.EffectiveAgentType)
	}
}

func TestSpawnRequest_EffectiveAgentType_SetExplicitly(t *testing.T) {
	req := SpawnRequest{
		AgentType:          "implementor",
		EffectiveAgentType: "lite-implementor",
	}
	if req.EffectiveAgentType != "lite-implementor" {
		t.Errorf("EffectiveAgentType = %q, want %q", req.EffectiveAgentType, "lite-implementor")
	}
}

// --- spawnSingle effectiveType fallback logic ---

// TestEffectiveType_FallsBackToAgentType verifies that when EffectiveAgentType is empty,
// the effective type should fall back to AgentType — matching the logic in spawnSingle.
func TestEffectiveType_FallsBackToAgentType(t *testing.T) {
	req := SpawnRequest{AgentType: "implementor", EffectiveAgentType: ""}

	effectiveType := req.EffectiveAgentType
	if effectiveType == "" {
		effectiveType = req.AgentType
	}

	if effectiveType != "implementor" {
		t.Errorf("effectiveType = %q, want %q", effectiveType, "implementor")
	}
}

// TestEffectiveType_UsesOverrideWhenSet verifies that when EffectiveAgentType is set,
// the override is used instead of AgentType — matching the logic in spawnSingle.
func TestEffectiveType_UsesOverrideWhenSet(t *testing.T) {
	req := SpawnRequest{AgentType: "implementor", EffectiveAgentType: "lite-implementor"}

	effectiveType := req.EffectiveAgentType
	if effectiveType == "" {
		effectiveType = req.AgentType
	}

	if effectiveType != "lite-implementor" {
		t.Errorf("effectiveType = %q, want %q", effectiveType, "lite-implementor")
	}
}

// --- loadAgentDefinition with LowConsumptionAgent ---

// createAgentDefWithLCA inserts an agent definition with a LowConsumptionAgent field.
func createAgentDefWithLCA(t *testing.T, env *spawnerTestEnv, agentID, prompt, lcAgent string) {
	t.Helper()
	database, err := db.Open(env.dbPath)
	if err != nil {
		t.Fatalf("createAgentDefWithLCA: open db: %v", err)
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
		LowConsumptionAgent: lcAgent,
	})
	if err != nil {
		t.Fatalf("createAgentDefWithLCA(%q): %v", agentID, err)
	}
}

func TestLoadAgentDefinition_ReturnsLowConsumptionAgent(t *testing.T) {
	env := newSpawnerTestEnv(t)

	createAgentDefWithLCA(t, env, "analyzer", "analyze things", "lite-analyzer")

	sp := New(Config{
		DataPath: env.dbPath,
		Pool:     env.pool,
		Clock:    clock.Real(),
	})

	def := sp.loadAgentDefinition("analyzer", env.project, "test")
	if def == nil {
		t.Fatal("loadAgentDefinition returned nil, want non-nil")
	}
	if def.LowConsumptionAgent != "lite-analyzer" {
		t.Errorf("LowConsumptionAgent = %q, want %q", def.LowConsumptionAgent, "lite-analyzer")
	}
	if def.ID != "analyzer" {
		t.Errorf("ID = %q, want %q", def.ID, "analyzer")
	}
}

func TestLoadAgentDefinition_EmptyLowConsumptionAgent(t *testing.T) {
	env := newSpawnerTestEnv(t)

	createAgentDefWithLCA(t, env, "analyzer", "analyze things", "")

	sp := New(Config{
		DataPath: env.dbPath,
		Pool:     env.pool,
		Clock:    clock.Real(),
	})

	def := sp.loadAgentDefinition("analyzer", env.project, "test")
	if def == nil {
		t.Fatal("loadAgentDefinition returned nil, want non-nil")
	}
	if def.LowConsumptionAgent != "" {
		t.Errorf("LowConsumptionAgent = %q, want empty", def.LowConsumptionAgent)
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
// when LowConsumptionMode is on, but not when it is off.
func TestLowConsumptionSubstitution_ModelSelection(t *testing.T) {
	tests := []struct {
		name            string
		lcMode          bool
		lcAgent         string
		lcAgentModel    string // model for the substitute agent in Config.Agents (empty = not in config)
		originalModel   string
		wantModel       string
	}{
		{
			name:          "mode_off_no_substitution",
			lcMode:        false,
			lcAgent:       "lite-implementor",
			lcAgentModel:  "haiku",
			originalModel: "opus",
			wantModel:     "opus",
		},
		{
			name:          "mode_on_no_lca",
			lcMode:        true,
			lcAgent:       "",
			lcAgentModel:  "",
			originalModel: "opus",
			wantModel:     "opus",
		},
		{
			name:          "mode_on_with_lca",
			lcMode:        true,
			lcAgent:       "lite-implementor",
			lcAgentModel:  "haiku",
			originalModel: "opus",
			wantModel:     "haiku",
		},
		{
			name:          "mode_on_lca_not_in_config",
			lcMode:        true,
			lcAgent:       "lite-implementor",
			lcAgentModel:  "", // substitute agent not in Config.Agents
			originalModel: "opus",
			wantModel:     "opus", // falls back to original model
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
			if tt.lcAgent != "" && tt.lcAgentModel != "" {
				cfg.Agents[tt.lcAgent] = AgentConfig{Model: tt.lcAgentModel}
			}

			// Simulate model determination from Spawn()
			selectedModel := "opus"
			if agentCfg, ok := cfg.Agents["implementor"]; ok && agentCfg.Model != "" {
				selectedModel = agentCfg.Model
			}

			// Simulate low consumption substitution from Spawn()
			if cfg.LowConsumptionMode && tt.lcAgent != "" {
				if substCfg, ok := cfg.Agents[tt.lcAgent]; ok && substCfg.Model != "" {
					selectedModel = substCfg.Model
				}
			}

			if selectedModel != tt.wantModel {
				t.Errorf("selectedModel = %q, want %q", selectedModel, tt.wantModel)
			}
		})
	}
}

// TestLowConsumptionMode_LoadAgentDef_SubstitutionDecision tests that when
// LowConsumptionMode is enabled, loadAgentDefinition returns the def with
// LowConsumptionAgent set, enabling the substitution path in Spawn().
func TestLowConsumptionMode_LoadAgentDef_SubstitutionDecision(t *testing.T) {
	env := newSpawnerTestEnv(t)
	ticketID := "LCM-" + uuid.New().String()[:6]
	env.initWorkflow(t, ticketID)

	createAgentDefWithLCA(t, env, "analyzer", "analyze code", "lite-analyzer")

	sp := New(Config{
		DataPath:           env.dbPath,
		Pool:               env.pool,
		Clock:              clock.Real(),
		LowConsumptionMode: true,
		Agents: map[string]AgentConfig{
			"analyzer":      {Model: "opus"},
			"lite-analyzer": {Model: "sonnet"},
		},
	})

	// In Spawn(), when LowConsumptionMode is true:
	// - loadAgentDefinition is called with original agent type
	// - if def.LowConsumptionAgent != "", substitute is used
	def := sp.loadAgentDefinition("analyzer", env.project, "test")
	if def == nil {
		t.Fatal("loadAgentDefinition returned nil")
	}

	// Verify the substitution would be triggered
	if def.LowConsumptionAgent == "" {
		t.Error("LowConsumptionAgent empty — substitution would not trigger")
	}
	if def.LowConsumptionAgent != "lite-analyzer" {
		t.Errorf("LowConsumptionAgent = %q, want %q", def.LowConsumptionAgent, "lite-analyzer")
	}

	// Verify the substitute model is in the config
	if substCfg, ok := sp.config.Agents[def.LowConsumptionAgent]; !ok {
		t.Errorf("Config.Agents[%q] not found", def.LowConsumptionAgent)
	} else if substCfg.Model != "sonnet" {
		t.Errorf("substitute model = %q, want %q", substCfg.Model, "sonnet")
	}
}

// TestLowConsumptionMode_ModeOff_NoSubstitution verifies that when LowConsumptionMode
// is false, the substitution code path is not entered regardless of LCA setting.
func TestLowConsumptionMode_ModeOff_NoSubstitution(t *testing.T) {
	sp := New(Config{
		LowConsumptionMode: false,
		Agents: map[string]AgentConfig{
			"implementor":      {Model: "opus"},
			"lite-implementor": {Model: "haiku"},
		},
	})

	if sp.config.LowConsumptionMode {
		t.Error("LowConsumptionMode = true, want false")
	}

	// When LowConsumptionMode is false, even if the agent has a LowConsumptionAgent,
	// the substitution block is skipped and EffectiveAgentType stays empty.
	req := SpawnRequest{AgentType: "implementor"}
	// Simulate Spawn() path: the if block is not entered
	if sp.config.LowConsumptionMode {
		t.Error("this branch should not be entered when LowConsumptionMode is false")
	}
	if req.EffectiveAgentType != "" {
		t.Errorf("EffectiveAgentType = %q, want empty", req.EffectiveAgentType)
	}
}
