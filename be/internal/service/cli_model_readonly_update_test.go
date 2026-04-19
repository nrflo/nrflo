package service

import (
	"strings"
	"testing"

	"be/internal/types"
)

// These tests complete the TODO in TestCLIModel_UpdateReadonly by covering
// every locked field individually on built-in (read_only=1) rows, plus the
// happy-path reasoning_effort updates and a regression check for user-owned rows.

const readonlyUpdateErr = "only reasoning_effort can be updated on built-in models"

// --- Happy path: reasoning_effort IS editable on read_only rows ---

func TestCLIModel_UpdateReadonly_ReasoningEffort_High(t *testing.T) {
	svc, cleanup := setupCLIModelTestEnv(t)
	defer cleanup()

	effort := "high"
	updated, err := svc.Update("opus_4_7", types.CLIModelUpdateRequest{
		ReasoningEffort: &effort,
	})
	if err != nil {
		t.Fatalf("Update reasoning_effort=high on read_only row: %v", err)
	}
	if updated.ReasoningEffort != "high" {
		t.Errorf("ReasoningEffort = %q, want %q", updated.ReasoningEffort, "high")
	}
	if !updated.ReadOnly {
		t.Error("ReadOnly = false after reasoning_effort update, want true (flag preserved)")
	}

	// Persisted.
	got, err := svc.Get("opus_4_7")
	if err != nil {
		t.Fatalf("Get after Update: %v", err)
	}
	if got.ReasoningEffort != "high" {
		t.Errorf("persisted ReasoningEffort = %q, want %q", got.ReasoningEffort, "high")
	}
	if !got.ReadOnly {
		t.Error("persisted ReadOnly = false, want true")
	}
}

func TestCLIModel_UpdateReadonly_ReasoningEffort_XhighOnOpus47(t *testing.T) {
	svc, cleanup := setupCLIModelTestEnv(t)
	defer cleanup()

	effort := "xhigh"
	updated, err := svc.Update("opus_4_7_1m", types.CLIModelUpdateRequest{
		ReasoningEffort: &effort,
	})
	if err != nil {
		t.Fatalf("Update reasoning_effort=xhigh on read_only opus_4_7_1m: %v", err)
	}
	if updated.ReasoningEffort != "xhigh" {
		t.Errorf("ReasoningEffort = %q, want %q", updated.ReasoningEffort, "xhigh")
	}
}

func TestCLIModel_UpdateReadonly_ReasoningEffort_XhighOnSonnet_Rejected(t *testing.T) {
	// xhigh rule is still enforced on read_only rows. Validation error, NOT the new
	// "only reasoning_effort" guard, because the field allowed through is reasoning_effort itself.
	svc, cleanup := setupCLIModelTestEnv(t)
	defer cleanup()

	effort := "xhigh"
	_, err := svc.Update("sonnet", types.CLIModelUpdateRequest{
		ReasoningEffort: &effort,
	})
	if err == nil {
		t.Fatal("expected error for xhigh on sonnet (non-Opus-4.7), got nil")
	}
	if !strings.Contains(err.Error(), "only supported on Opus 4.7") {
		t.Errorf("error = %q, want to contain %q", err.Error(), "only supported on Opus 4.7")
	}
	// Must NOT surface the read_only guard message: the allowed field is reasoning_effort.
	if strings.Contains(err.Error(), readonlyUpdateErr) {
		t.Errorf("error = %q must not contain read_only guard text", err.Error())
	}
}

// --- Locked fields rejected individually on read_only rows ---

func TestCLIModel_UpdateReadonly_LockedFields_Rejected(t *testing.T) {
	newName := "My Opus"
	newModel := "claude-opus-4-7"
	newCtx := 100000
	enabledTrue := true
	enabledFalse := false

	cases := []struct {
		name string
		req  types.CLIModelUpdateRequest
	}{
		{name: "display_name", req: types.CLIModelUpdateRequest{DisplayName: &newName}},
		{name: "mapped_model (same value)", req: types.CLIModelUpdateRequest{MappedModel: &newModel}},
		{name: "context_length", req: types.CLIModelUpdateRequest{ContextLength: &newCtx}},
		{name: "enabled=false", req: types.CLIModelUpdateRequest{Enabled: &enabledFalse}},
		{name: "enabled=true", req: types.CLIModelUpdateRequest{Enabled: &enabledTrue}},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			svc, cleanup := setupCLIModelTestEnv(t)
			defer cleanup()

			before, err := svc.Get("opus_4_7")
			if err != nil {
				t.Fatalf("Get before: %v", err)
			}

			_, err = svc.Update("opus_4_7", tc.req)
			if err == nil {
				t.Fatalf("Update %s on read_only row: expected error, got nil", tc.name)
			}
			if !strings.Contains(err.Error(), readonlyUpdateErr) {
				t.Errorf("error = %q, want to contain %q", err.Error(), readonlyUpdateErr)
			}

			// Verify state was NOT mutated (read_only guard fires before any write).
			after, err := svc.Get("opus_4_7")
			if err != nil {
				t.Fatalf("Get after rejected Update: %v", err)
			}
			if after.DisplayName != before.DisplayName {
				t.Errorf("DisplayName changed: before=%q after=%q", before.DisplayName, after.DisplayName)
			}
			if after.MappedModel != before.MappedModel {
				t.Errorf("MappedModel changed: before=%q after=%q", before.MappedModel, after.MappedModel)
			}
			if after.ContextLength != before.ContextLength {
				t.Errorf("ContextLength changed: before=%d after=%d", before.ContextLength, after.ContextLength)
			}
			if after.Enabled != before.Enabled {
				t.Errorf("Enabled changed: before=%v after=%v", before.Enabled, after.Enabled)
			}
			if !after.ReadOnly {
				t.Error("ReadOnly flag got cleared by rejected Update")
			}
		})
	}
}

// Mixed request: reasoning_effort alongside a locked field is still rejected wholesale.
func TestCLIModel_UpdateReadonly_MixedLockedPlusEffort_Rejected(t *testing.T) {
	svc, cleanup := setupCLIModelTestEnv(t)
	defer cleanup()

	newName := "Mixed"
	effort := "high"
	_, err := svc.Update("opus_4_7", types.CLIModelUpdateRequest{
		DisplayName:     &newName,
		ReasoningEffort: &effort,
	})
	if err == nil {
		t.Fatal("expected error for mixed read_only update, got nil")
	}
	if !strings.Contains(err.Error(), readonlyUpdateErr) {
		t.Errorf("error = %q, want to contain %q", err.Error(), readonlyUpdateErr)
	}

	// reasoning_effort must NOT have been written.
	got, err := svc.Get("opus_4_7")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got.ReasoningEffort == "high" {
		t.Error("ReasoningEffort was persisted despite rejection")
	}
}

// --- Regression: user-owned rows are NOT affected by the new guard ---

func TestCLIModel_UpdateUserRow_DisplayName_Succeeds(t *testing.T) {
	svc, cleanup := setupCLIModelTestEnv(t)
	defer cleanup()

	if _, err := svc.Create(types.CLIModelCreateRequest{
		ID:          "user-row",
		CLIType:     "claude",
		DisplayName: "Original",
		MappedModel: "claude-sonnet-4-5",
	}); err != nil {
		t.Fatalf("Create user row: %v", err)
	}

	newName := "Foo"
	updated, err := svc.Update("user-row", types.CLIModelUpdateRequest{
		DisplayName: &newName,
	})
	if err != nil {
		t.Fatalf("Update user row display_name: %v", err)
	}
	if updated.DisplayName != "Foo" {
		t.Errorf("DisplayName = %q, want %q", updated.DisplayName, "Foo")
	}
	if updated.ReadOnly {
		t.Error("ReadOnly = true on user-owned row, want false")
	}
}

func TestCLIModel_UpdateUserRow_AllFields_Succeeds(t *testing.T) {
	svc, cleanup := setupCLIModelTestEnv(t)
	defer cleanup()

	if _, err := svc.Create(types.CLIModelCreateRequest{
		ID:          "user-all",
		CLIType:     "claude",
		DisplayName: "Orig",
		MappedModel: "claude-sonnet-4-5",
	}); err != nil {
		t.Fatalf("Create: %v", err)
	}

	newName := "New"
	newModel := "claude-opus-4-7"
	newCtx := 500000
	effort := "xhigh"
	disabled := false
	updated, err := svc.Update("user-all", types.CLIModelUpdateRequest{
		DisplayName:     &newName,
		MappedModel:     &newModel,
		ContextLength:   &newCtx,
		ReasoningEffort: &effort,
		Enabled:         &disabled,
	})
	if err != nil {
		t.Fatalf("Update user row with all fields: %v", err)
	}
	if updated.DisplayName != "New" || updated.MappedModel != "claude-opus-4-7" ||
		updated.ContextLength != 500000 || updated.ReasoningEffort != "xhigh" || updated.Enabled {
		t.Errorf("update did not apply expected values: %+v", updated)
	}
}
