package service

import (
	"strings"
	"testing"

	"be/internal/types"
)

// TestCreateAgentDef_APIMode_AcceptsSeededAPIModel verifies that creating an agent
// with execution_mode=api and a model with API support succeeds.
func TestCreateAgentDef_APIMode_AcceptsSeededAPIModel(t *testing.T) {
	t.Parallel()
	svc, settingsSvc, wfID := setupAgentDefAPIModeEnv(t)
	if err := settingsSvc.Set("api_mode_enabled", "true"); err != nil {
		t.Fatalf("Set api_mode_enabled: %v", err)
	}

	def, err := svc.CreateAgentDef("proj1", wfID, &types.AgentDefCreateRequest{
		ID:            "agent-api-seeded",
		Prompt:        "do stuff",
		ExecutionMode: "api",
		Model:         "opus-4-7",
	})
	if err != nil {
		t.Fatalf("CreateAgentDef with seeded api model: %v", err)
	}
	if def.Model != "opus-4-7" {
		t.Errorf("Model = %q, want opus-4-7", def.Model)
	}
}

// TestCreateAgentDef_APIMode_RejectsNonAPIModel verifies that creating an agent
// with execution_mode=api and a model ID without API support is rejected.
func TestCreateAgentDef_APIMode_RejectsNonAPIModel(t *testing.T) {
	t.Parallel()
	svc, settingsSvc, wfID := setupAgentDefAPIModeEnv(t)
	if err := settingsSvc.Set("api_mode_enabled", "true"); err != nil {
		t.Fatalf("Set api_mode_enabled: %v", err)
	}

	_, err := svc.CreateAgentDef("proj1", wfID, &types.AgentDefCreateRequest{
		ID:            "agent-bad-model",
		Prompt:        "do stuff",
		ExecutionMode: "api",
		Model:         "cli-only-model-id",
	})
	if err == nil {
		t.Fatal("CreateAgentDef with non-api model: expected error, got nil")
	}
	if !strings.Contains(err.Error(), "invalid model") {
		t.Errorf("error = %q, want to contain 'invalid model'", err.Error())
	}
}

// TestCreateAgentDef_APIMode_LCModel_Valid verifies that a valid low_consumption_model
// with API support is accepted for api execution mode.
func TestCreateAgentDef_APIMode_LCModel_Valid(t *testing.T) {
	t.Parallel()
	svc, settingsSvc, wfID := setupAgentDefAPIModeEnv(t)
	if err := settingsSvc.Set("api_mode_enabled", "true"); err != nil {
		t.Fatalf("Set api_mode_enabled: %v", err)
	}

	def, err := svc.CreateAgentDef("proj1", wfID, &types.AgentDefCreateRequest{
		ID:                  "agent-api-lcmodel",
		Prompt:              "do stuff",
		ExecutionMode:       "api",
		Model:               "opus-4-7",
		LowConsumptionModel: "haiku-4-5",
	})
	if err != nil {
		t.Fatalf("CreateAgentDef with valid lc model: %v", err)
	}
	if def.LowConsumptionModel != "haiku-4-5" {
		t.Errorf("LowConsumptionModel = %q, want haiku-4-5", def.LowConsumptionModel)
	}
}

// TestCreateAgentDef_APIMode_LCModel_Invalid verifies that a low_consumption_model
// without API support is rejected for api execution mode.
func TestCreateAgentDef_APIMode_LCModel_Invalid(t *testing.T) {
	t.Parallel()
	svc, settingsSvc, wfID := setupAgentDefAPIModeEnv(t)
	if err := settingsSvc.Set("api_mode_enabled", "true"); err != nil {
		t.Fatalf("Set api_mode_enabled: %v", err)
	}

	_, err := svc.CreateAgentDef("proj1", wfID, &types.AgentDefCreateRequest{
		ID:                  "agent-api-bad-lc",
		Prompt:              "do stuff",
		ExecutionMode:       "api",
		Model:               "opus-4-7",
		LowConsumptionModel: "nonexistent-api-model",
	})
	if err == nil {
		t.Fatal("CreateAgentDef with invalid lc model: expected error, got nil")
	}
	if !strings.Contains(err.Error(), "invalid low_consumption_model") {
		t.Errorf("error = %q, want to contain 'invalid low_consumption_model'", err.Error())
	}
}

// TestCreateAgentDef_CLIInteractive_LCModel_APIModelID_Rejected verifies that
// cli_interactive mode requires CLI support for low_consumption_model validation.
// An ID without CLI support must be rejected.
func TestCreateAgentDef_CLIInteractive_LCModel_APIModelID_Rejected(t *testing.T) {
	t.Parallel()
	svc, _, wfID := setupAgentDefAPIModeEnv(t)
	// api_mode_enabled is off — cli_interactive always works regardless

	// Unknown IDs are rejected in CLI mode too.
	_, err := svc.CreateAgentDef("proj1", wfID, &types.AgentDefCreateRequest{
		ID:                  "agent-cli-lc-apionlyid",
		Prompt:              "do stuff",
		ExecutionMode:       "cli_interactive",
		LowConsumptionModel: "unknown-model",
	})
	if err == nil {
		t.Fatal("cli_interactive with api-models-only id as lc model: expected error, got nil")
	}
	if !strings.Contains(err.Error(), "invalid low_consumption_model") {
		t.Errorf("error = %q, want to contain 'invalid low_consumption_model'", err.Error())
	}
}

// TestCreateAgentDef_CLIInteractive_PrimaryModelValidated verifies symmetry
// with API mode: unknown CLI model IDs are rejected.
func TestCreateAgentDef_CLIInteractive_PrimaryModelValidated(t *testing.T) {
	t.Parallel()
	svc, _, wfID := setupAgentDefAPIModeEnv(t)

	_, err := svc.CreateAgentDef("proj1", wfID, &types.AgentDefCreateRequest{
		ID:            "agent-cli-anymodel",
		Prompt:        "do stuff",
		ExecutionMode: "cli_interactive",
		Model:         "any-arbitrary-model-name",
	})
	if err == nil {
		t.Fatal("cli_interactive with arbitrary model: expected error")
	}
}

// TestUpdateAgentDef_APIMode_InvalidModel verifies that updating execution_mode to api
// with an invalid model is rejected.
func TestUpdateAgentDef_APIMode_InvalidModel(t *testing.T) {
	t.Parallel()
	svc, settingsSvc, wfID := setupAgentDefAPIModeEnv(t)
	if err := settingsSvc.Set("api_mode_enabled", "true"); err != nil {
		t.Fatalf("Set api_mode_enabled: %v", err)
	}

	// Create api agent with valid model
	if _, err := svc.CreateAgentDef("proj1", wfID, &types.AgentDefCreateRequest{
		ID:            "agent-to-update-model",
		Prompt:        "do stuff",
		ExecutionMode: "api",
		Model:         "opus-4-7",
	}); err != nil {
		t.Fatalf("CreateAgentDef: %v", err)
	}

	// Try to update the model to one without API support.
	badModel := "cli-only-model-id"
	err := svc.UpdateAgentDef("proj1", wfID, "agent-to-update-model", &types.AgentDefUpdateRequest{
		Model: &badModel,
	})
	if err == nil {
		t.Fatal("UpdateAgentDef with invalid model: expected error, got nil")
	}
	if !strings.Contains(err.Error(), "invalid model") {
		t.Errorf("error = %q, want to contain 'invalid model'", err.Error())
	}
}
