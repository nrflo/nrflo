package service

import (
	"strings"
	"testing"

	"be/internal/types"
)

// --- Update ---

func TestAPIModel_UpdatePartialFields(t *testing.T) {
	t.Parallel()
	svc, cleanup := setupAPIModelTestEnv(t)
	defer cleanup()

	if _, err := svc.Create(types.APIModelCreateRequest{
		ID:            "upd-api",
		Provider:      "anthropic",
		DisplayName:   "Original",
		MappedModel:   "claude-3-5-sonnet",
		ContextLength: 50000,
	}); err != nil {
		t.Fatalf("Create: %v", err)
	}

	newName := "Updated Name"
	updated, err := svc.Update("upd-api", types.APIModelUpdateRequest{
		DisplayName: &newName,
	})
	if err != nil {
		t.Fatalf("Update: %v", err)
	}
	if updated.DisplayName != "Updated Name" {
		t.Errorf("DisplayName = %q, want %q", updated.DisplayName, "Updated Name")
	}
	if updated.MappedModel != "claude-3-5-sonnet" {
		t.Errorf("MappedModel = %q after partial update, want unchanged", updated.MappedModel)
	}
	if updated.ContextLength != 50000 {
		t.Errorf("ContextLength = %d after partial update, want 50000", updated.ContextLength)
	}
}

func TestAPIModel_UpdateNotFound(t *testing.T) {
	t.Parallel()
	svc, cleanup := setupAPIModelTestEnv(t)
	defer cleanup()

	newName := "Whatever"
	_, err := svc.Update("nonexistent-model", types.APIModelUpdateRequest{
		DisplayName: &newName,
	})
	if err == nil {
		t.Fatal("expected not-found error, got nil")
	}
	if !strings.Contains(err.Error(), "not found") {
		t.Errorf("error = %q, want to contain %q", err.Error(), "not found")
	}
}

func TestAPIModel_UpdateReadOnly_LockedFields(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name string
		req  types.APIModelUpdateRequest
	}{
		{name: "display_name", req: types.APIModelUpdateRequest{DisplayName: strPtr("Foo")}},
		{name: "mapped_model", req: types.APIModelUpdateRequest{MappedModel: strPtr("gpt-5")}},
		{name: "context_length", req: types.APIModelUpdateRequest{ContextLength: intPtr(100000)}},
		{name: "enabled_false", req: types.APIModelUpdateRequest{Enabled: boolPtr(false)}},
		{name: "enabled_true", req: types.APIModelUpdateRequest{Enabled: boolPtr(true)}},
	}

	const wantMsg = "only reasoning_effort can be updated on built-in models"
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			svc, cleanup := setupAPIModelTestEnv(t)
			defer cleanup()

			_, err := svc.Update("opus_4_7", tc.req)
			if err == nil {
				t.Fatalf("Update %q on read-only row: expected error, got nil", tc.name)
			}
			if !strings.Contains(err.Error(), wantMsg) {
				t.Errorf("error = %q, want to contain %q", err.Error(), wantMsg)
			}
		})
	}
}

func TestAPIModel_UpdateReadOnly_ReasoningEffort_Succeeds(t *testing.T) {
	t.Parallel()
	svc, cleanup := setupAPIModelTestEnv(t)
	defer cleanup()

	effort := "xhigh"
	updated, err := svc.Update("opus_4_7", types.APIModelUpdateRequest{
		ReasoningEffort: &effort,
	})
	if err != nil {
		t.Fatalf("Update reasoning_effort on read-only row: %v", err)
	}
	if updated.ReasoningEffort != "xhigh" {
		t.Errorf("ReasoningEffort = %q, want xhigh", updated.ReasoningEffort)
	}
	if !updated.ReadOnly {
		t.Error("ReadOnly = false after reasoning_effort update, want true")
	}
}

func TestAPIModel_UpdateReasoningEffort_XhighOnAnthropicSonnet5_Allowed(t *testing.T) {
	t.Parallel()
	svc, cleanup := setupAPIModelTestEnv(t)
	defer cleanup()

	// "sonnet" is seeded as anthropic with mapped_model=claude-sonnet-5, which
	// supports xhigh.
	effort := "xhigh"
	updated, err := svc.Update("sonnet", types.APIModelUpdateRequest{
		ReasoningEffort: &effort,
	})
	if err != nil {
		t.Fatalf("Update reasoning_effort=xhigh on sonnet: %v", err)
	}
	if updated.ReasoningEffort != "xhigh" {
		t.Errorf("ReasoningEffort = %q, want xhigh", updated.ReasoningEffort)
	}
}

func TestAPIModel_UpdateReasoningEffort_XhighOnOpenAI_Rejected(t *testing.T) {
	t.Parallel()
	svc, cleanup := setupAPIModelTestEnv(t)
	defer cleanup()

	// Current implementation restricts xhigh to anthropic+opus-4.7 only;
	// openai models are also rejected.
	effort := "xhigh"
	_, err := svc.Update("gpt54_high", types.APIModelUpdateRequest{
		ReasoningEffort: &effort,
	})
	if err == nil {
		t.Fatal("expected error for xhigh on openai model, got nil")
	}
	if !strings.Contains(err.Error(), "only supported on Anthropic Opus 4.7") {
		t.Errorf("error = %q, want to contain 'only supported on Anthropic Opus 4.7'", err.Error())
	}
}

func TestAPIModel_UpdateReasoningEffort_Invalid(t *testing.T) {
	t.Parallel()
	svc, cleanup := setupAPIModelTestEnv(t)
	defer cleanup()

	effort := "nonsense"
	_, err := svc.Update("sonnet", types.APIModelUpdateRequest{
		ReasoningEffort: &effort,
	})
	if err == nil {
		t.Fatal("expected error for invalid reasoning_effort, got nil")
	}
	if !strings.Contains(err.Error(), "must be one of") {
		t.Errorf("error = %q, want to contain must be one of", err.Error())
	}
}

func TestAPIModel_UpdateNoFields(t *testing.T) {
	t.Parallel()
	svc, cleanup := setupAPIModelTestEnv(t)
	defer cleanup()

	if _, err := svc.Create(types.APIModelCreateRequest{
		ID:          "noop-api",
		Provider:    "anthropic",
		DisplayName: "Noop",
		MappedModel: "claude-sonnet",
	}); err != nil {
		t.Fatalf("Create: %v", err)
	}

	got, err := svc.Update("noop-api", types.APIModelUpdateRequest{})
	if err != nil {
		t.Fatalf("empty Update: %v", err)
	}
	if got.DisplayName != "Noop" {
		t.Errorf("DisplayName = %q after no-op update, want Noop", got.DisplayName)
	}
}

// --- Delete ---

func TestAPIModel_DeleteUserCreated(t *testing.T) {
	t.Parallel()
	svc, cleanup := setupAPIModelTestEnv(t)
	defer cleanup()

	if _, err := svc.Create(types.APIModelCreateRequest{
		ID:          "deletable-api",
		Provider:    "openai",
		DisplayName: "Deletable",
		MappedModel: "gpt-4",
	}); err != nil {
		t.Fatalf("Create: %v", err)
	}

	if err := svc.Delete("deletable-api"); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	if _, err := svc.Get("deletable-api"); err == nil {
		t.Error("Get after Delete: expected not-found error, got nil")
	}
}

func TestAPIModel_DeleteReadOnly(t *testing.T) {
	t.Parallel()
	svc, cleanup := setupAPIModelTestEnv(t)
	defer cleanup()

	err := svc.Delete("opus_4_7")
	if err == nil {
		t.Fatal("expected error deleting system model, got nil")
	}
	if !strings.Contains(err.Error(), "cannot delete system model") {
		t.Errorf("error = %q, want to contain %q", err.Error(), "cannot delete system model")
	}
}

func TestAPIModel_DeleteNotFound(t *testing.T) {
	t.Parallel()
	svc, cleanup := setupAPIModelTestEnv(t)
	defer cleanup()

	err := svc.Delete("nonexistent-model")
	if err == nil {
		t.Fatal("expected not-found error, got nil")
	}
	if !strings.Contains(err.Error(), "not found") {
		t.Errorf("error = %q, want to contain not found", err.Error())
	}
}
