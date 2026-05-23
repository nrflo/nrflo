package service

import (
	"strings"
	"testing"

	"be/internal/types"
)

// TestCreateAgentDef_APIMode_AcceptsSeededAPIModel verifies that creating an agent
// with execution_mode=api and a model seeded in api_models succeeds.
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
		Model:         "opus_4_7",
	})
	if err != nil {
		t.Fatalf("CreateAgentDef with seeded api model: %v", err)
	}
	if def.Model != "opus_4_7" {
		t.Errorf("Model = %q, want opus_4_7", def.Model)
	}
}

// TestCreateAgentDef_APIMode_RejectsNonAPIModel verifies that creating an agent
// with execution_mode=api and a model ID not in api_models is rejected.
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
// from api_models is accepted for api execution mode.
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
		Model:               "opus_4_7",
		LowConsumptionModel: "haiku",
	})
	if err != nil {
		t.Fatalf("CreateAgentDef with valid lc model: %v", err)
	}
	if def.LowConsumptionModel != "haiku" {
		t.Errorf("LowConsumptionModel = %q, want haiku", def.LowConsumptionModel)
	}
}

// TestCreateAgentDef_APIMode_LCModel_Invalid verifies that a low_consumption_model
// not in api_models is rejected for api execution mode.
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
		Model:               "opus_4_7",
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
// cli_interactive mode uses cli_models for low_consumption_model validation.
// An ID that exists only in api_models must be rejected.
func TestCreateAgentDef_CLIInteractive_LCModel_APIModelID_Rejected(t *testing.T) {
	t.Parallel()
	svc, _, wfID := setupAgentDefAPIModeEnv(t)
	// api_mode_enabled is off — cli_interactive always works regardless

	// "gpt54_high" is seeded in api_models but does NOT exist in cli_models
	_, err := svc.CreateAgentDef("proj1", wfID, &types.AgentDefCreateRequest{
		ID:                  "agent-cli-lc-apionlyid",
		Prompt:              "do stuff",
		ExecutionMode:       "cli_interactive",
		LowConsumptionModel: "gpt54_high",
	})
	if err == nil {
		t.Fatal("cli_interactive with api-models-only id as lc model: expected error, got nil")
	}
	if !strings.Contains(err.Error(), "invalid low_consumption_model") {
		t.Errorf("error = %q, want to contain 'invalid low_consumption_model'", err.Error())
	}
}

// TestCreateAgentDef_CLIInteractive_PrimaryModel_NotValidated verifies that
// the primary model field is not validated for cli_interactive agents.
func TestCreateAgentDef_CLIInteractive_PrimaryModel_NotValidated(t *testing.T) {
	t.Parallel()
	svc, _, wfID := setupAgentDefAPIModeEnv(t)

	def, err := svc.CreateAgentDef("proj1", wfID, &types.AgentDefCreateRequest{
		ID:            "agent-cli-anymodel",
		Prompt:        "do stuff",
		ExecutionMode: "cli_interactive",
		Model:         "any-arbitrary-model-name",
	})
	if err != nil {
		t.Fatalf("cli_interactive with arbitrary model: expected success, got %v", err)
	}
	if def.Model != "any-arbitrary-model-name" {
		t.Errorf("Model = %q, want any-arbitrary-model-name", def.Model)
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
		Model:         "opus_4_7",
	}); err != nil {
		t.Fatalf("CreateAgentDef: %v", err)
	}

	// Try to update model to something not in api_models
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
