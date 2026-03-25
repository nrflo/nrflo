package integration

import (
	"encoding/json"
	"testing"

	"be/internal/types"
)

// TestAgentDefCreateWithLowConsumptionModel verifies that creating an agent definition
// with low_consumption_model persists and returns it correctly.
func TestAgentDefCreateWithLowConsumptionModel(t *testing.T) {
	env := NewTestEnv(t)
	svc := env.getAgentDefService(t)

	def, err := svc.CreateAgentDef(env.ProjectID, "test", &types.AgentDefCreateRequest{
		ID:                  "lc-main",
		Prompt:              "main agent",
		LowConsumptionModel: "sonnet",
	})
	if err != nil {
		t.Fatalf("create main agent with low_consumption_model: %v", err)
	}

	if def.LowConsumptionModel != "sonnet" {
		t.Errorf("LowConsumptionModel = %q, want %q", def.LowConsumptionModel, "sonnet")
	}
}

// TestAgentDefGetLowConsumptionModel verifies that GET returns low_consumption_model correctly.
func TestAgentDefGetLowConsumptionModel(t *testing.T) {
	env := NewTestEnv(t)
	svc := env.getAgentDefService(t)

	_, err := svc.CreateAgentDef(env.ProjectID, "test", &types.AgentDefCreateRequest{
		ID:                  "get-main",
		Prompt:              "main",
		LowConsumptionModel: "haiku",
	})
	if err != nil {
		t.Fatalf("create main: %v", err)
	}

	got, err := svc.GetAgentDef(env.ProjectID, "test", "get-main")
	if err != nil {
		t.Fatalf("GetAgentDef: %v", err)
	}
	if got.LowConsumptionModel != "haiku" {
		t.Errorf("GetAgentDef LowConsumptionModel = %q, want %q", got.LowConsumptionModel, "haiku")
	}
}

// TestAgentDefEmptyLowConsumptionModelOmittedFromJSON verifies that when
// low_consumption_model is empty, it is omitted from the JSON output via omitempty.
func TestAgentDefEmptyLowConsumptionModelOmittedFromJSON(t *testing.T) {
	env := NewTestEnv(t)
	svc := env.getAgentDefService(t)

	def, err := svc.CreateAgentDef(env.ProjectID, "test", &types.AgentDefCreateRequest{
		ID:     "no-lc-agent",
		Prompt: "no low consumption",
	})
	if err != nil {
		t.Fatalf("create: %v", err)
	}

	if def.LowConsumptionModel != "" {
		t.Fatalf("expected LowConsumptionModel to be empty, got %q", def.LowConsumptionModel)
	}

	data, err := json.Marshal(def)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var result map[string]interface{}
	if err := json.Unmarshal(data, &result); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if _, exists := result["low_consumption_model"]; exists {
		t.Fatal("expected low_consumption_model to be omitted from JSON when empty")
	}
}

// TestAgentDefUpdateLowConsumptionModel verifies that updating low_consumption_model works end-to-end.
func TestAgentDefUpdateLowConsumptionModel(t *testing.T) {
	env := NewTestEnv(t)
	svc := env.getAgentDefService(t)

	_, err := svc.CreateAgentDef(env.ProjectID, "test", &types.AgentDefCreateRequest{
		ID:     "upd-lc-main",
		Prompt: "main",
	})
	if err != nil {
		t.Fatalf("create main: %v", err)
	}

	lcModel := "sonnet"
	if err := svc.UpdateAgentDef(env.ProjectID, "test", "upd-lc-main", &types.AgentDefUpdateRequest{
		LowConsumptionModel: &lcModel,
	}); err != nil {
		t.Fatalf("UpdateAgentDef: %v", err)
	}

	def, err := svc.GetAgentDef(env.ProjectID, "test", "upd-lc-main")
	if err != nil {
		t.Fatalf("GetAgentDef after update: %v", err)
	}
	if def.LowConsumptionModel != "sonnet" {
		t.Errorf("after update LowConsumptionModel = %q, want %q", def.LowConsumptionModel, "sonnet")
	}
}

// TestAgentDefInvalidLowConsumptionModelRejected verifies that creating or updating an
// agent definition with an invalid low_consumption_model returns an error.
func TestAgentDefInvalidLowConsumptionModelRejected(t *testing.T) {
	env := NewTestEnv(t)
	svc := env.getAgentDefService(t)

	// Create should fail for invalid model names
	invalidModels := []string{"invalid_model", "gpt-4", "lite-implementor", "opus3"}

	for _, m := range invalidModels {
		t.Run("create/"+m, func(t *testing.T) {
			_, err := svc.CreateAgentDef(env.ProjectID, "test", &types.AgentDefCreateRequest{
				ID:                  "inv-" + m,
				Prompt:              "p",
				LowConsumptionModel: m,
			})
			if err == nil {
				t.Errorf("CreateAgentDef(low_consumption_model=%q) = nil, want error", m)
			}
		})
	}

	// Update should also fail for invalid model names
	_, err := svc.CreateAgentDef(env.ProjectID, "test", &types.AgentDefCreateRequest{
		ID:     "upd-inv-base",
		Prompt: "base",
	})
	if err != nil {
		t.Fatalf("create base agent: %v", err)
	}

	for _, m := range invalidModels {
		t.Run("update/"+m, func(t *testing.T) {
			if err := svc.UpdateAgentDef(env.ProjectID, "test", "upd-inv-base", &types.AgentDefUpdateRequest{
				LowConsumptionModel: &m,
			}); err == nil {
				t.Errorf("UpdateAgentDef(low_consumption_model=%q) = nil, want error", m)
			}
		})
	}
}
