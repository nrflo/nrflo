package service

import (
	"strings"
	"testing"

	"be/internal/types"
)

// seedCustomProvider creates an enabled custom_providers row via the same
// pool/clock as svc, for use in ModelService provider-validation tests.
func seedCustomProvider(t *testing.T, svc *ModelService, name string) {
	t.Helper()
	cpSvc := NewCustomProviderService(svc.pool, svc.clock)
	if _, err := cpSvc.Create(types.CustomProviderCreateRequest{
		Name:    name,
		BaseURL: "http://localhost:11434/v1",
	}); err != nil {
		t.Fatalf("seed custom provider %q: %v", name, err)
	}
}

// TestModelServiceCreate_CustomProvider_Accepted verifies a model row can
// reference a registered custom provider.
func TestModelServiceCreate_CustomProvider_Accepted(t *testing.T) {
	t.Parallel()
	svc := setupModelService(t)
	seedCustomProvider(t, svc, "local-ollama")

	created, err := svc.Create(types.ModelCreateRequest{
		ID: "local-model", Provider: "local-ollama", DisplayName: "Local Model",
		APIModel: "llama3",
	})
	if err != nil {
		t.Fatalf("Create(custom provider): %v", err)
	}
	if created.Provider != "local-ollama" {
		t.Errorf("Provider = %q, want local-ollama", created.Provider)
	}
}

// TestModelServiceCreate_UnregisteredCustomProvider_Rejected verifies a
// provider name that is neither builtin nor a registered custom_providers row
// is rejected with "invalid provider".
func TestModelServiceCreate_UnregisteredCustomProvider_Rejected(t *testing.T) {
	t.Parallel()
	svc := setupModelService(t)
	_, err := svc.Create(types.ModelCreateRequest{
		ID: "ghost-model", Provider: "never-registered", DisplayName: "Ghost",
		APIModel: "x",
	})
	if err == nil {
		t.Fatal("Create(unregistered provider) succeeded, want error")
	}
	if !strings.Contains(err.Error(), "invalid provider") {
		t.Errorf("error = %v, want mention of invalid provider", err)
	}
}

// TestModelServiceCreate_CustomProvider_RejectsCLIModel verifies custom
// providers are API-only: a non-empty cli_model is rejected on Create.
func TestModelServiceCreate_CustomProvider_RejectsCLIModel(t *testing.T) {
	t.Parallel()
	svc := setupModelService(t)
	seedCustomProvider(t, svc, "local-ollama")

	_, err := svc.Create(types.ModelCreateRequest{
		ID: "local-model-cli", Provider: "local-ollama", DisplayName: "Local Model",
		CLIModel: "llama3", APIModel: "llama3",
	})
	if err == nil {
		t.Fatal("Create(custom provider, cli_model set) succeeded, want error")
	}
	if !strings.Contains(err.Error(), "API-mode only") {
		t.Errorf("error = %v, want mention of API-mode only", err)
	}
}

// TestModelServiceUpdate_CustomProvider_RejectsSettingCLIModel mirrors the
// openrouter update-time rejection for a custom provider row.
func TestModelServiceUpdate_CustomProvider_RejectsSettingCLIModel(t *testing.T) {
	t.Parallel()
	svc := setupModelService(t)
	seedCustomProvider(t, svc, "local-ollama")
	created, err := svc.Create(types.ModelCreateRequest{
		ID: "local-model-upd", Provider: "local-ollama", DisplayName: "Local Model",
		APIModel: "llama3",
	})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	cliModel := "llama3"
	if _, err := svc.Update(created.ID, types.ModelUpdateRequest{CLIModel: &cliModel}); err == nil {
		t.Fatal("Update(custom provider, set cli_model) succeeded, want error")
	} else if !strings.Contains(err.Error(), "API-mode only") {
		t.Errorf("error = %v, want mention of API-mode only", err)
	}
}

// TestModelServiceIsValidModelForMode_CustomProvider verifies a custom
// provider row (api_model only) validates for api mode but not cli mode.
func TestModelServiceIsValidModelForMode_CustomProvider(t *testing.T) {
	t.Parallel()
	svc := setupModelService(t)
	seedCustomProvider(t, svc, "local-ollama")
	created, err := svc.Create(types.ModelCreateRequest{
		ID: "local-model-mode", Provider: "local-ollama", DisplayName: "Local Model",
		APIModel: "llama3",
	})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	validAPI, err := svc.IsValidModelForMode(created.ID, "api")
	if err != nil || !validAPI {
		t.Errorf("IsValidModelForMode(api) = %v, %v; want true, nil", validAPI, err)
	}
	validCLI, err := svc.IsValidModelForMode(created.ID, "cli")
	if err != nil || validCLI {
		t.Errorf("IsValidModelForMode(cli) = %v, %v; want false, nil", validCLI, err)
	}
}

// TestModelServiceResolveProvider_CustomProviderDisabled verifies
// resolveProvider still reports exists=true for a disabled custom provider
// (Exists ignores enabled state — only BuildAPIProvider's GetEnabled cares).
func TestModelServiceResolveProvider_CustomProviderDisabled(t *testing.T) {
	t.Parallel()
	svc := setupModelService(t)
	seedCustomProvider(t, svc, "local-ollama")
	disabled := false
	cpSvc := NewCustomProviderService(svc.pool, svc.clock)
	if _, err := cpSvc.Update("local-ollama", types.CustomProviderUpdateRequest{Enabled: &disabled}); err != nil {
		t.Fatalf("disable provider: %v", err)
	}

	apiOnly, exists, err := svc.resolveProvider("local-ollama")
	if err != nil {
		t.Fatalf("resolveProvider: %v", err)
	}
	if !exists {
		t.Error("resolveProvider exists = false, want true (disabled row still exists)")
	}
	if !apiOnly {
		t.Error("resolveProvider apiOnly = false, want true for custom providers")
	}
}
