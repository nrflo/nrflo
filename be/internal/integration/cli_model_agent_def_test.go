package integration

import (
	"testing"

	"be/internal/service"
	"be/internal/types"
)

// TestCustomCLIModel_ImmediatelyValidForAgentDef verifies that adding a new model
// with CLI support makes it immediately valid for agent definitions.
// This covers the acceptance criterion:
//
//	"Adding a custom model via POST /api/v1/models makes it immediately valid for agent definitions"
func TestCustomCLIModel_ImmediatelyValidForAgentDef(t *testing.T) {
	env := NewTestEnv(t)
	svc := env.getAgentDefService(t)
	modelSvc := service.NewModelService(env.Pool, env.Clock)

	customModelID := "my-custom-cli-model"

	// Before adding: custom model should be rejected
	_, err := svc.CreateAgentDef(env.ProjectID, "test", &types.AgentDefCreateRequest{
		ID:                  "pre-add-agent",
		Prompt:              "test",
		LowConsumptionModel: customModelID,
	})
	if err == nil {
		t.Fatal("expected error when using unknown model before adding it, got nil")
	}

	// Add the custom model with CLI mode enabled.
	_, err = modelSvc.Create(types.ModelCreateRequest{
		ID:          customModelID,
		Provider:    "anthropic",
		DisplayName: "My Custom Model",
		CLIModel:    "claude-custom",
	})
	if err != nil {
		t.Fatalf("ModelService.Create: %v", err)
	}

	// After adding: agent def creation with that model should succeed
	def, err := svc.CreateAgentDef(env.ProjectID, "test", &types.AgentDefCreateRequest{
		ID:                  "post-add-agent",
		Prompt:              "test",
		LowConsumptionModel: customModelID,
	})
	if err != nil {
		t.Fatalf("CreateAgentDef after adding custom model: %v", err)
	}
	if def.LowConsumptionModel != customModelID {
		t.Errorf("LowConsumptionModel = %q, want %q", def.LowConsumptionModel, customModelID)
	}
}

// TestDisabledCLIModel_RejectedForAgentDef verifies that creating an agent definition
// with a disabled custom model as low_consumption_model is rejected by IsValidModel.
func TestDisabledCLIModel_RejectedForAgentDef(t *testing.T) {
	env := NewTestEnv(t)
	svc := env.getAgentDefService(t)
	modelSvc := service.NewModelService(env.Pool, env.Clock)

	customModelID := "disabled-for-agentdef"

	// Add the custom model (not yet referenced by any agent def)
	_, err := modelSvc.Create(types.ModelCreateRequest{
		ID:          customModelID,
		Provider:    "anthropic",
		DisplayName: "Disabled For AgentDef",
		CLIModel:    "claude-custom",
	})
	if err != nil {
		t.Fatalf("ModelService.Create: %v", err)
	}

	// Disable the model while it has no agent def references
	falseVal := false
	_, err = modelSvc.Update(customModelID, types.ModelUpdateRequest{Enabled: &falseVal})
	if err != nil {
		t.Fatalf("ModelService.Update (disable): %v", err)
	}

	// Creating an agent def with the disabled model should be rejected
	_, err = svc.CreateAgentDef(env.ProjectID, "test", &types.AgentDefCreateRequest{
		ID:                  "disabled-model-agent",
		Prompt:              "test",
		LowConsumptionModel: customModelID,
	})
	if err == nil {
		t.Fatal("expected error when creating agent def with disabled model, got nil")
	}
}

// TestCustomCLIModel_UpdateImmediatelyValid verifies that UpdateAgentDef also
// accepts the new model immediately after it is added.
func TestCustomCLIModel_UpdateImmediatelyValid(t *testing.T) {
	env := NewTestEnv(t)
	svc := env.getAgentDefService(t)
	modelSvc := service.NewModelService(env.Pool, env.Clock)

	// Create a base agent without low_consumption_model
	_, err := svc.CreateAgentDef(env.ProjectID, "test", &types.AgentDefCreateRequest{
		ID:     "update-target",
		Prompt: "test",
	})
	if err != nil {
		t.Fatalf("create base agent: %v", err)
	}

	newModelID := "custom-update-model"

	// Update with unknown model should fail
	if err := svc.UpdateAgentDef(env.ProjectID, "test", "update-target", &types.AgentDefUpdateRequest{
		LowConsumptionModel: &newModelID,
	}); err == nil {
		t.Fatal("expected error when updating with unknown model, got nil")
	}

	// Add the model
	_, err = modelSvc.Create(types.ModelCreateRequest{
		ID:          newModelID,
		Provider:    "openai",
		DisplayName: "Custom Update Model",
		CLIModel:    "gpt-custom",
	})
	if err != nil {
		t.Fatalf("ModelService.Create: %v", err)
	}

	// Update should now succeed
	if err := svc.UpdateAgentDef(env.ProjectID, "test", "update-target", &types.AgentDefUpdateRequest{
		LowConsumptionModel: &newModelID,
	}); err != nil {
		t.Fatalf("UpdateAgentDef after adding model: %v", err)
	}

	def, err := svc.GetAgentDef(env.ProjectID, "test", "update-target")
	if err != nil {
		t.Fatalf("GetAgentDef: %v", err)
	}
	if def.LowConsumptionModel != newModelID {
		t.Errorf("LowConsumptionModel = %q, want %q", def.LowConsumptionModel, newModelID)
	}
}
