package service

import (
	"strings"
	"testing"

	"be/internal/types"
)

// seedOllamaNativeProvider creates an enabled ollama_native custom_providers
// row via the same pool/clock as svc, the only provider kind that can carry
// effort="none".
func seedOllamaNativeProvider(t *testing.T, svc *ModelService, name string) {
	t.Helper()
	cpSvc := NewCustomProviderService(svc.pool, svc.clock)
	if _, err := cpSvc.Create(types.CustomProviderCreateRequest{
		Name: name, BaseURL: "http://localhost:11434", APIWire: APIWireOllamaNative,
	}); err != nil {
		t.Fatalf("seed ollama_native provider %q: %v", name, err)
	}
}

// TestModelServiceCreate_NoneEffort_AcceptedForOllamaNative verifies
// cli_efforts/api_efforts/default_effort="none" are accepted on Create when
// the provider is a registered ollama_native custom provider.
func TestModelServiceCreate_NoneEffort_AcceptedForOllamaNative(t *testing.T) {
	t.Parallel()
	svc := setupModelService(t)
	seedOllamaNativeProvider(t, svc, "local-ollama")

	created, err := svc.Create(types.ModelCreateRequest{
		ID: "ollama-model", Provider: "local-ollama", DisplayName: "Ollama Model",
		APIModel: "llama3", APIEfforts: []string{"none", "low"}, DefaultEffort: "none",
	})
	if err != nil {
		t.Fatalf("Create(none effort, ollama_native provider): %v", err)
	}
	if created.DefaultEffort != "none" {
		t.Errorf("DefaultEffort = %q, want none", created.DefaultEffort)
	}
	found := false
	for _, e := range created.APIEfforts {
		if e == "none" {
			found = true
		}
	}
	if !found {
		t.Errorf("APIEfforts = %v, want to contain none", created.APIEfforts)
	}
}

// TestModelServiceCreate_NoneEffort_RejectedForOtherProviders verifies
// effort="none" is rejected on Create for every non-ollama_native provider:
// builtins (anthropic, openai) and a chat_completions-wire custom provider.
func TestModelServiceCreate_NoneEffort_RejectedForOtherProviders(t *testing.T) {
	t.Parallel()
	svc := setupModelService(t)
	cpSvc := NewCustomProviderService(svc.pool, svc.clock)
	if _, err := cpSvc.Create(types.CustomProviderCreateRequest{
		Name: "local-llamacpp", BaseURL: "http://localhost:8080/v1", APIWire: APIWireChatCompletions,
	}); err != nil {
		t.Fatalf("seed chat_completions provider: %v", err)
	}

	tests := []struct {
		name     string
		provider string
	}{
		{"anthropic builtin", "anthropic"},
		{"openai builtin", "openai"},
		{"chat_completions custom provider", "local-llamacpp"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			_, err := svc.Create(types.ModelCreateRequest{
				ID: "reject-" + tc.provider, Provider: tc.provider, DisplayName: "Reject",
				APIModel: "x", DefaultEffort: "none",
			})
			if err == nil {
				t.Fatalf("Create(provider=%s, default_effort=none) succeeded, want error", tc.provider)
			}
			if !strings.Contains(err.Error(), "only supported by an ollama_native custom provider") {
				t.Errorf("error = %v, want mention of ollama_native gate", err)
			}
		})
	}
}

// TestModelServiceCreate_NoneEffort_RejectedInCLIOrAPIEfforts verifies the
// gate also fires when "none" appears in cli_efforts/api_efforts (not just
// default_effort) for a non-ollama_native provider.
func TestModelServiceCreate_NoneEffort_RejectedInCLIOrAPIEfforts(t *testing.T) {
	t.Parallel()
	svc := setupModelService(t)

	t.Run("api_efforts", func(t *testing.T) {
		_, err := svc.Create(types.ModelCreateRequest{
			ID: "reject-api-efforts", Provider: "openai", DisplayName: "Reject",
			APIModel: "x", APIEfforts: []string{"none"},
		})
		if err == nil || !strings.Contains(err.Error(), "api_efforts") {
			t.Errorf("error = %v, want api_efforts gate error", err)
		}
	})
}

// TestModelServiceUpdate_NoneEffort_ProviderImmutableGate verifies Update
// re-checks the none-effort gate using the model's (immutable) provider: an
// ollama_native-backed model can adopt "none" via Update, and a builtin
// model cannot.
func TestModelServiceUpdate_NoneEffort_ProviderImmutableGate(t *testing.T) {
	t.Parallel()

	t.Run("accepted for ollama_native provider", func(t *testing.T) {
		svc := setupModelService(t)
		seedOllamaNativeProvider(t, svc, "local-ollama")
		created, err := svc.Create(types.ModelCreateRequest{
			ID: "ollama-upd", Provider: "local-ollama", DisplayName: "Ollama Model",
			APIModel: "llama3", APIEfforts: []string{"none", "low"},
		})
		if err != nil {
			t.Fatalf("Create: %v", err)
		}
		none := "none"
		updated, err := svc.Update(created.ID, types.ModelUpdateRequest{DefaultEffort: &none})
		if err != nil {
			t.Fatalf("Update(default_effort=none): %v", err)
		}
		if updated.DefaultEffort != "none" {
			t.Errorf("DefaultEffort = %q, want none", updated.DefaultEffort)
		}
	})

	t.Run("rejected for openai builtin", func(t *testing.T) {
		svc := setupModelService(t)
		created, err := svc.Get("gpt-5.6-sol")
		if err != nil {
			t.Fatalf("Get seeded model: %v", err)
		}
		none := "none"
		_, err = svc.Update(created.ID, types.ModelUpdateRequest{DefaultEffort: &none})
		if err == nil {
			t.Fatal("Update(builtin, default_effort=none) succeeded, want error")
		}
		if !strings.Contains(err.Error(), "only supported by an ollama_native custom provider") {
			t.Errorf("error = %v, want mention of ollama_native gate", err)
		}
	})
}
