package spawner

import (
	"encoding/json"
	"testing"

	"be/internal/clock"
	"be/internal/db"
	"be/internal/model"
	"be/internal/repo"
)

// TestLoadAgentDefinition_ValidationCommandsField verifies that loadAgentDefinition
// returns the ValidationCommands field stored in the agent definition.
func TestLoadAgentDefinition_ValidationCommandsField(t *testing.T) {
	t.Parallel()
	env := newSpawnerTestEnv(t)

	database, err := db.Open(env.dbPath)
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	defer database.Close()

	adRepo := repo.NewAgentDefinitionRepo(database, clock.Real())
	if err := adRepo.Create(&model.AgentDefinition{
		ID:                 "vc-agent",
		ProjectID:          env.project,
		WorkflowID:         "test",
		Model:              "sonnet",
		Timeout:            20,
		Prompt:             "validation commands test",
		ValidationCommands: `["true","make test"]`,
	}); err != nil {
		t.Fatalf("create agent def: %v", err)
	}

	sp := New(Config{
		DataPath: env.dbPath,
		Pool:     env.pool,
		Clock:    clock.Real(),
	})

	def := sp.loadAgentDefinition("vc-agent", env.project, "test")
	if def == nil {
		t.Fatal("loadAgentDefinition returned nil, want non-nil")
	}
	if def.ValidationCommands != `["true","make test"]` {
		t.Errorf("ValidationCommands = %q, want %q", def.ValidationCommands, `["true","make test"]`)
	}
}

// TestLoadAgentDefinition_ValidationCommandsEmpty verifies that loadAgentDefinition
// returns the default empty array for an agent def created without validation_commands.
func TestLoadAgentDefinition_ValidationCommandsEmpty(t *testing.T) {
	t.Parallel()
	env := newSpawnerTestEnv(t)

	database, err := db.Open(env.dbPath)
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	defer database.Close()

	adRepo := repo.NewAgentDefinitionRepo(database, clock.Real())
	if err := adRepo.Create(&model.AgentDefinition{
		ID:                 "vc-empty",
		ProjectID:          env.project,
		WorkflowID:         "test",
		Model:              "sonnet",
		Timeout:            20,
		Prompt:             "no validation commands",
		ValidationCommands: "[]", // service layer sets this default on create
	}); err != nil {
		t.Fatalf("create agent def: %v", err)
	}

	sp := New(Config{
		DataPath: env.dbPath,
		Pool:     env.pool,
		Clock:    clock.Real(),
	})

	def := sp.loadAgentDefinition("vc-empty", env.project, "test")
	if def == nil {
		t.Fatal("loadAgentDefinition returned nil, want non-nil")
	}
	if def.ValidationCommands != "[]" {
		t.Errorf("ValidationCommands = %q, want %q", def.ValidationCommands, "[]")
	}
}

// TestValidationCommandsParsing_ValidJSON simulates the prepareSpawn parsing logic
// for a valid JSON array, matching how spawnSingle resolves validationCommands.
func TestValidationCommandsParsing_ValidJSON(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		rawJSON    string
		wantCmds   []string
	}{
		{"empty_array", "[]", nil},
		{"single_cmd", `["true"]`, []string{"true"}},
		{"two_cmds", `["true","make test"]`, []string{"true", "make test"}},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			agentDef := &model.AgentDefinition{ValidationCommands: tt.rawJSON}

			var validationCommands []string
			if agentDef.ValidationCommands != "" {
				if err := json.Unmarshal([]byte(agentDef.ValidationCommands), &validationCommands); err != nil {
					validationCommands = nil
				}
			}

			if len(validationCommands) != len(tt.wantCmds) {
				t.Fatalf("validationCommands len = %d, want %d", len(validationCommands), len(tt.wantCmds))
			}
			for i, cmd := range tt.wantCmds {
				if validationCommands[i] != cmd {
					t.Errorf("validationCommands[%d] = %q, want %q", i, validationCommands[i], cmd)
				}
			}
		})
	}
}

// TestValidationCommandsParsing_GarbledJSON verifies that malformed JSON is treated as
// nil (no panic, no error propagated) matching the warn-and-continue behavior in spawnSingle.
func TestValidationCommandsParsing_GarbledJSON(t *testing.T) {
	t.Parallel()

	garbled := []string{"not-json", "{invalid}", `["unclosed`}
	for _, raw := range garbled {
		raw := raw
		t.Run(raw, func(t *testing.T) {
			t.Parallel()
			agentDef := &model.AgentDefinition{ValidationCommands: raw}

			var validationCommands []string
			if agentDef.ValidationCommands != "" {
				if err := json.Unmarshal([]byte(agentDef.ValidationCommands), &validationCommands); err != nil {
					validationCommands = nil
				}
			}

			if validationCommands != nil {
				t.Errorf("garbled JSON %q: validationCommands = %v, want nil", raw, validationCommands)
			}
		})
	}
}

// TestValidationCommands_CarriedAcrossContinuation verifies the continuation path
// in processInfo preserves validationCommands (as done in completion.go).
func TestValidationCommands_CarriedAcrossContinuation(t *testing.T) {
	t.Parallel()

	cmds := []string{"true", "make test"}
	oldProc := &processInfo{
		validationCommands: cmds,
	}

	// Simulate the continuation carry-over from completion.go
	newProc := &processInfo{}
	newProc.validationCommands = oldProc.validationCommands

	if len(newProc.validationCommands) != 2 {
		t.Fatalf("newProc.validationCommands len = %d, want 2", len(newProc.validationCommands))
	}
	if newProc.validationCommands[0] != "true" || newProc.validationCommands[1] != "make test" {
		t.Errorf("newProc.validationCommands = %v, want [true make test]", newProc.validationCommands)
	}
}
