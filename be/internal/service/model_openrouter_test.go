package service

import (
	"strings"
	"testing"

	"be/internal/types"
)

// TestModelServiceCreate_OpenRouter_RejectsCLIModel verifies openrouter is
// API-mode only: a non-empty cli_model on Create is rejected.
func TestModelServiceCreate_OpenRouter_RejectsCLIModel(t *testing.T) {
	t.Parallel()
	svc := setupModelService(t)
	_, err := svc.Create(types.ModelCreateRequest{
		ID: "or-with-cli", Provider: "openrouter", DisplayName: "OR CLI",
		CLIModel: "openai/gpt-4o", APIModel: "openai/gpt-4o",
	})
	if err == nil {
		t.Fatal("Create(openrouter, cli_model set) succeeded, want error")
	}
	if !strings.Contains(err.Error(), "API-mode only") {
		t.Errorf("error = %v, want mention of API-mode only", err)
	}
}

// TestModelServiceCreate_OpenRouter_APIModelOnly_Accepted verifies an
// openrouter row with only api_model set is accepted.
func TestModelServiceCreate_OpenRouter_APIModelOnly_Accepted(t *testing.T) {
	t.Parallel()
	svc := setupModelService(t)
	created, err := svc.Create(types.ModelCreateRequest{
		ID: "or-api-only", Provider: "openrouter", DisplayName: "OR API",
		APIModel: "openai/gpt-4o",
	})
	if err != nil {
		t.Fatalf("Create(openrouter, api_model only): %v", err)
	}
	if created.Provider != "openrouter" {
		t.Errorf("Provider = %q, want openrouter", created.Provider)
	}
	if created.CLIModel != "" {
		t.Errorf("CLIModel = %q, want empty", created.CLIModel)
	}
}

// TestModelServiceUpdate_OpenRouter_RejectsSettingCLIModel verifies a PATCH
// that sets cli_model on an existing openrouter row is rejected (Update uses
// current.Provider since provider is immutable).
func TestModelServiceUpdate_OpenRouter_RejectsSettingCLIModel(t *testing.T) {
	t.Parallel()
	svc := setupModelService(t)
	created, err := svc.Create(types.ModelCreateRequest{
		ID: "or-update-cli", Provider: "openrouter", DisplayName: "OR Update",
		APIModel: "openai/gpt-4o",
	})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	cliModel := "openai/gpt-4o"
	_, err = svc.Update(created.ID, types.ModelUpdateRequest{CLIModel: &cliModel})
	if err == nil {
		t.Fatal("Update(openrouter, set cli_model) succeeded, want error")
	}
	if !strings.Contains(err.Error(), "API-mode only") {
		t.Errorf("error = %v, want mention of API-mode only", err)
	}
}

// TestModelServiceUpdate_OpenRouter_OtherFieldsStillUpdatable verifies a
// PATCH that leaves cli_model empty still succeeds on an openrouter row.
func TestModelServiceUpdate_OpenRouter_OtherFieldsStillUpdatable(t *testing.T) {
	t.Parallel()
	svc := setupModelService(t)
	created, err := svc.Create(types.ModelCreateRequest{
		ID: "or-update-ok", Provider: "openrouter", DisplayName: "OR Update OK",
		APIModel: "openai/gpt-4o",
	})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	name := "Renamed OR Model"
	updated, err := svc.Update(created.ID, types.ModelUpdateRequest{DisplayName: &name})
	if err != nil {
		t.Fatalf("Update(openrouter, display_name only): %v", err)
	}
	if updated.DisplayName != name {
		t.Errorf("DisplayName = %q, want %q", updated.DisplayName, name)
	}
}

// TestModelServiceIsValidModelForMode_OpenRouter verifies an openrouter row
// (api_model only) validates as api-mode-supported but not cli-mode.
func TestModelServiceIsValidModelForMode_OpenRouter(t *testing.T) {
	t.Parallel()
	svc := setupModelService(t)
	created, err := svc.Create(types.ModelCreateRequest{
		ID: "or-mode-check", Provider: "openrouter", DisplayName: "OR Mode",
		APIModel: "openai/gpt-4o",
	})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	validAPI, err := svc.IsValidModelForMode(created.ID, "api")
	if err != nil {
		t.Fatalf("IsValidModelForMode(api): %v", err)
	}
	if !validAPI {
		t.Error("IsValidModelForMode(api) = false, want true for openrouter api_model row")
	}

	validCLI, err := svc.IsValidModelForMode(created.ID, "cli")
	if err != nil {
		t.Fatalf("IsValidModelForMode(cli): %v", err)
	}
	if validCLI {
		t.Error("IsValidModelForMode(cli) = true, want false for openrouter row (no cli_model)")
	}
}

// TestModelServiceCreate_InvalidProvider_MentionsOpenRouter verifies the
// invalid-provider error message reflects the widened whitelist.
func TestModelServiceCreate_InvalidProvider_MentionsOpenRouter(t *testing.T) {
	t.Parallel()
	svc := setupModelService(t)
	_, err := svc.Create(types.ModelCreateRequest{
		ID: "bad-provider", Provider: "bogus", DisplayName: "Bad", APIModel: "x",
	})
	if err == nil {
		t.Fatal("Create(bogus provider) succeeded, want error")
	}
	if !strings.Contains(err.Error(), "openrouter") {
		t.Errorf("error = %v, want mention of openrouter", err)
	}
}
