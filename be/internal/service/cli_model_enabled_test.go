package service

import (
	"strings"
	"testing"

	"be/internal/types"
)

// --- Enabled field in List ---

func TestCLIModel_List_EnabledField(t *testing.T) {
	svc, cleanup := setupCLIModelTestEnv(t)
	defer cleanup()

	models, err := svc.List()
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	for _, m := range models {
		if !m.Enabled {
			t.Errorf("seeded model %q: Enabled = false, want true", m.ID)
		}
	}
}

// --- ListEnabled ---

func TestCLIModel_ListEnabled_ReturnsAllByDefault(t *testing.T) {
	svc, cleanup := setupCLIModelTestEnv(t)
	defer cleanup()

	enabled, err := svc.ListEnabled()
	if err != nil {
		t.Fatalf("ListEnabled: %v", err)
	}
	if len(enabled) != 13 {
		t.Fatalf("ListEnabled len = %d, want 13 (all seeded models enabled)", len(enabled))
	}
}

func TestCLIModel_ListEnabled_ExcludesDisabledCustom(t *testing.T) {
	svc, cleanup := setupCLIModelTestEnv(t)
	defer cleanup()

	if _, err := svc.Create(types.CLIModelCreateRequest{
		ID:          "list-disabled",
		CLIType:     "claude",
		DisplayName: "List Disabled",
		MappedModel: "claude-custom",
	}); err != nil {
		t.Fatalf("Create: %v", err)
	}

	if _, err := svc.Update("list-disabled", types.CLIModelUpdateRequest{Enabled: boolPtr(false)}); err != nil {
		t.Fatalf("Update (disable): %v", err)
	}

	enabled, err := svc.ListEnabled()
	if err != nil {
		t.Fatalf("ListEnabled: %v", err)
	}
	if len(enabled) != 13 {
		t.Fatalf("ListEnabled len = %d, want 13 (disabled model excluded)", len(enabled))
	}
	for _, m := range enabled {
		if m.ID == "list-disabled" {
			t.Error("disabled model 'list-disabled' found in ListEnabled results")
		}
	}
}

func TestCLIModel_ListEnabled_IncludesEnabledCustom(t *testing.T) {
	svc, cleanup := setupCLIModelTestEnv(t)
	defer cleanup()

	if _, err := svc.Create(types.CLIModelCreateRequest{
		ID:          "list-enabled-custom",
		CLIType:     "claude",
		DisplayName: "List Enabled Custom",
		MappedModel: "claude-custom",
	}); err != nil {
		t.Fatalf("Create: %v", err)
	}

	enabled, err := svc.ListEnabled()
	if err != nil {
		t.Fatalf("ListEnabled: %v", err)
	}
	if len(enabled) != 14 {
		t.Fatalf("ListEnabled len = %d, want 14 (13 seeded + 1 custom)", len(enabled))
	}
}

// --- Get enabled field ---

func TestCLIModel_Get_EnabledField(t *testing.T) {
	svc, cleanup := setupCLIModelTestEnv(t)
	defer cleanup()

	m, err := svc.Get("sonnet")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if !m.Enabled {
		t.Error("Enabled = false for seeded model 'sonnet', want true")
	}
}

// --- Create returns enabled=true ---

func TestCLIModel_Create_EnabledByDefault(t *testing.T) {
	svc, cleanup := setupCLIModelTestEnv(t)
	defer cleanup()

	m, err := svc.Create(types.CLIModelCreateRequest{
		ID:          "created-enabled",
		CLIType:     "claude",
		DisplayName: "Created Enabled",
		MappedModel: "claude-3-5-sonnet",
	})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if !m.Enabled {
		t.Error("Create: Enabled = false, want true for new custom model")
	}

	got, err := svc.Get("created-enabled")
	if err != nil {
		t.Fatalf("Get after Create: %v", err)
	}
	if !got.Enabled {
		t.Error("Get Enabled = false after Create, want true")
	}
}

// --- Disable custom model ---

func TestCLIModel_DisableCustomModel(t *testing.T) {
	svc, cleanup := setupCLIModelTestEnv(t)
	defer cleanup()

	if _, err := svc.Create(types.CLIModelCreateRequest{
		ID:          "disable-me",
		CLIType:     "claude",
		DisplayName: "Disable Me",
		MappedModel: "claude-custom",
	}); err != nil {
		t.Fatalf("Create: %v", err)
	}

	updated, err := svc.Update("disable-me", types.CLIModelUpdateRequest{Enabled: boolPtr(false)})
	if err != nil {
		t.Fatalf("Update (disable): %v", err)
	}
	if updated.Enabled {
		t.Error("Enabled = true after disabling, want false")
	}

	got, err := svc.Get("disable-me")
	if err != nil {
		t.Fatalf("Get after disable: %v", err)
	}
	if got.Enabled {
		t.Error("Get Enabled = true after disable, want false")
	}
}

// --- Re-enable model ---

func TestCLIModel_EnableCustomModel(t *testing.T) {
	svc, cleanup := setupCLIModelTestEnv(t)
	defer cleanup()

	if _, err := svc.Create(types.CLIModelCreateRequest{
		ID:          "toggle-model",
		CLIType:     "opencode",
		DisplayName: "Toggle",
		MappedModel: "openai/gpt-4",
	}); err != nil {
		t.Fatalf("Create: %v", err)
	}

	if _, err := svc.Update("toggle-model", types.CLIModelUpdateRequest{Enabled: boolPtr(false)}); err != nil {
		t.Fatalf("Update (disable): %v", err)
	}

	updated, err := svc.Update("toggle-model", types.CLIModelUpdateRequest{Enabled: boolPtr(true)})
	if err != nil {
		t.Fatalf("Update (re-enable): %v", err)
	}
	if !updated.Enabled {
		t.Error("Enabled = false after re-enabling, want true")
	}
}

// --- Reject disable on read_only model ---

func TestCLIModel_DisableReadOnlyModel_Rejected(t *testing.T) {
	svc, cleanup := setupCLIModelTestEnv(t)
	defer cleanup()

	_, err := svc.Update("opus_4_7", types.CLIModelUpdateRequest{Enabled: boolPtr(false)})
	if err == nil {
		t.Fatal("expected error when disabling read_only model, got nil")
	}
	if !strings.Contains(err.Error(), "only reasoning_effort can be updated on built-in models") {
		t.Errorf("error = %q, want to contain %q", err.Error(), "only reasoning_effort can be updated on built-in models")
	}

	m, err := svc.Get("opus_4_7")
	if err != nil {
		t.Fatalf("Get after rejected disable: %v", err)
	}
	if !m.Enabled {
		t.Error("opus_4_7 Enabled = false after rejected disable, want true")
	}
}

// --- IsValidModel with enabled ---

func TestCLIModel_IsValidModel_DisabledModel(t *testing.T) {
	svc, cleanup := setupCLIModelTestEnv(t)
	defer cleanup()

	if _, err := svc.Create(types.CLIModelCreateRequest{
		ID:          "valid-disabled",
		CLIType:     "claude",
		DisplayName: "Disabled",
		MappedModel: "claude-custom",
	}); err != nil {
		t.Fatalf("Create: %v", err)
	}

	if _, err := svc.Update("valid-disabled", types.CLIModelUpdateRequest{Enabled: boolPtr(false)}); err != nil {
		t.Fatalf("Update (disable): %v", err)
	}

	ok, err := svc.IsValidModel("valid-disabled")
	if err != nil {
		t.Fatalf("IsValidModel: %v", err)
	}
	if ok {
		t.Error("IsValidModel = true for disabled model, want false")
	}
}

func TestCLIModel_IsValidModel_ReenabledModel(t *testing.T) {
	svc, cleanup := setupCLIModelTestEnv(t)
	defer cleanup()

	if _, err := svc.Create(types.CLIModelCreateRequest{
		ID:          "reenable-model",
		CLIType:     "codex",
		DisplayName: "Reenable",
		MappedModel: "gpt-4",
	}); err != nil {
		t.Fatalf("Create: %v", err)
	}

	if _, err := svc.Update("reenable-model", types.CLIModelUpdateRequest{Enabled: boolPtr(false)}); err != nil {
		t.Fatalf("Update (disable): %v", err)
	}

	if _, err := svc.Update("reenable-model", types.CLIModelUpdateRequest{Enabled: boolPtr(true)}); err != nil {
		t.Fatalf("Update (re-enable): %v", err)
	}

	ok, err := svc.IsValidModel("reenable-model")
	if err != nil {
		t.Fatalf("IsValidModel after re-enable: %v", err)
	}
	if !ok {
		t.Error("IsValidModel = false after re-enabling, want true")
	}
}
