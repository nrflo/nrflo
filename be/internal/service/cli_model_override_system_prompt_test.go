package service

import (
	"strings"
	"testing"

	"be/internal/types"
)

// Tests for the OverrideSystemPrompt field added in migration 000126.

// --- Create with override_system_prompt ---

func TestCLIModel_Create_OverrideSystemPrompt_True(t *testing.T) {
	t.Parallel()
	svc, cleanup := setupCLIModelTestEnv(t)
	defer cleanup()

	m, err := svc.Create(types.CLIModelCreateRequest{
		ID:                   "osp-true",
		CLIType:              "claude",
		DisplayName:          "OSP True",
		MappedModel:          "claude-sonnet-4-5",
		OverrideSystemPrompt: true,
	})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if !m.OverrideSystemPrompt {
		t.Error("Create: OverrideSystemPrompt = false, want true")
	}

	// Round-trip via Get.
	got, err := svc.Get("osp-true")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if !got.OverrideSystemPrompt {
		t.Error("Get: OverrideSystemPrompt = false, want true")
	}

	// Present in List.
	models, err := svc.List()
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	for _, lm := range models {
		if lm.ID == "osp-true" {
			if !lm.OverrideSystemPrompt {
				t.Error("List: OverrideSystemPrompt = false, want true")
			}
			return
		}
	}
	t.Error("created model not found in List")
}

func TestCLIModel_Create_OverrideSystemPrompt_DefaultFalse(t *testing.T) {
	t.Parallel()
	svc, cleanup := setupCLIModelTestEnv(t)
	defer cleanup()

	m, err := svc.Create(types.CLIModelCreateRequest{
		ID:          "osp-default",
		CLIType:     "claude",
		DisplayName: "OSP Default",
		MappedModel: "claude-sonnet-4-5",
		// OverrideSystemPrompt not set → zero value false
	})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if m.OverrideSystemPrompt {
		t.Error("Create: OverrideSystemPrompt = true, want false (default)")
	}

	got, err := svc.Get("osp-default")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got.OverrideSystemPrompt {
		t.Error("Get: OverrideSystemPrompt = true, want false")
	}
}

func TestCLIModel_Create_OverrideSystemPrompt_ListEnabled(t *testing.T) {
	t.Parallel()
	svc, cleanup := setupCLIModelTestEnv(t)
	defer cleanup()

	if _, err := svc.Create(types.CLIModelCreateRequest{
		ID:                   "osp-enabled",
		CLIType:              "claude",
		DisplayName:          "OSP Enabled",
		MappedModel:          "claude-sonnet-4-5",
		OverrideSystemPrompt: true,
	}); err != nil {
		t.Fatalf("Create: %v", err)
	}

	models, err := svc.ListEnabled()
	if err != nil {
		t.Fatalf("ListEnabled: %v", err)
	}
	for _, lm := range models {
		if lm.ID == "osp-enabled" {
			if !lm.OverrideSystemPrompt {
				t.Error("ListEnabled: OverrideSystemPrompt = false, want true")
			}
			return
		}
	}
	t.Error("created model not found in ListEnabled")
}

// --- Update override_system_prompt on read-only built-in rows ---

func TestCLIModel_UpdateReadonly_OverrideSystemPrompt_SetTrue(t *testing.T) {
	t.Parallel()
	svc, cleanup := setupCLIModelTestEnv(t)
	defer cleanup()

	ospTrue := true
	updated, err := svc.Update("opus_4_7", types.CLIModelUpdateRequest{
		OverrideSystemPrompt: &ospTrue,
	})
	if err != nil {
		t.Fatalf("Update override_system_prompt=true on read_only row: %v", err)
	}
	if !updated.OverrideSystemPrompt {
		t.Error("OverrideSystemPrompt = false after update, want true")
	}
	// ReadOnly flag must be preserved.
	if !updated.ReadOnly {
		t.Error("ReadOnly = false after override_system_prompt update, want true (flag preserved)")
	}

	// Verify persistence.
	got, err := svc.Get("opus_4_7")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if !got.OverrideSystemPrompt {
		t.Error("persisted OverrideSystemPrompt = false, want true")
	}
	if !got.ReadOnly {
		t.Error("persisted ReadOnly = false, want true")
	}
}

func TestCLIModel_UpdateReadonly_OverrideSystemPrompt_ToggleFalse(t *testing.T) {
	t.Parallel()
	svc, cleanup := setupCLIModelTestEnv(t)
	defer cleanup()

	// Set true first.
	ospTrue := true
	if _, err := svc.Update("opus_4_7", types.CLIModelUpdateRequest{
		OverrideSystemPrompt: &ospTrue,
	}); err != nil {
		t.Fatalf("initial Update: %v", err)
	}

	// Toggle back to false.
	ospFalse := false
	updated, err := svc.Update("opus_4_7", types.CLIModelUpdateRequest{
		OverrideSystemPrompt: &ospFalse,
	})
	if err != nil {
		t.Fatalf("Update override_system_prompt=false on read_only row: %v", err)
	}
	if updated.OverrideSystemPrompt {
		t.Error("OverrideSystemPrompt = true after false update, want false")
	}
	if !updated.ReadOnly {
		t.Error("ReadOnly = false, want true (flag preserved)")
	}
}

// --- Update override_system_prompt on user-owned rows ---

func TestCLIModel_UpdateUserRow_OverrideSystemPrompt_Succeeds(t *testing.T) {
	t.Parallel()
	svc, cleanup := setupCLIModelTestEnv(t)
	defer cleanup()

	if _, err := svc.Create(types.CLIModelCreateRequest{
		ID:          "user-osp",
		CLIType:     "claude",
		DisplayName: "User OSP",
		MappedModel: "claude-sonnet-4-5",
	}); err != nil {
		t.Fatalf("Create: %v", err)
	}

	ospTrue := true
	updated, err := svc.Update("user-osp", types.CLIModelUpdateRequest{
		OverrideSystemPrompt: &ospTrue,
	})
	if err != nil {
		t.Fatalf("Update user row override_system_prompt=true: %v", err)
	}
	if !updated.OverrideSystemPrompt {
		t.Error("OverrideSystemPrompt = false, want true")
	}
	if updated.ReadOnly {
		t.Error("ReadOnly = true on user-owned row, want false")
	}
}

// --- Mixed: override_system_prompt + locked field is rejected wholesale ---

func TestCLIModel_UpdateReadonly_MixedOSPPlusLockedField_Rejected(t *testing.T) {
	t.Parallel()
	svc, cleanup := setupCLIModelTestEnv(t)
	defer cleanup()

	ospTrue := true
	newName := "Hack"
	_, err := svc.Update("opus_4_7", types.CLIModelUpdateRequest{
		OverrideSystemPrompt: &ospTrue,
		DisplayName:          &newName,
	})
	if err == nil {
		t.Fatal("expected error for mixed read_only update (OSP + display_name), got nil")
	}
	if !strings.Contains(err.Error(), readonlyUpdateErr) {
		t.Errorf("error = %q, want to contain %q", err.Error(), readonlyUpdateErr)
	}

	// Neither field must have been written.
	got, err := svc.Get("opus_4_7")
	if err != nil {
		t.Fatalf("Get after rejected update: %v", err)
	}
	if got.OverrideSystemPrompt {
		t.Error("OverrideSystemPrompt was persisted despite rejection")
	}
	if got.DisplayName == "Hack" {
		t.Error("DisplayName was persisted despite rejection")
	}
}
