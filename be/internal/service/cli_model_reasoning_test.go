package service

import (
	"fmt"
	"strings"
	"testing"

	"be/internal/types"
)

// --- ReasoningEffort validation ---

func TestCLIModel_CreateReasoningEffort(t *testing.T) {
	t.Parallel()
	// Capability is driven solely by the row's supported_efforts list, not the
	// mapped_model name. An empty list + a non-empty effort defaults the list to
	// [effort]; an effort outside the list is a membership error.
	tests := []struct {
		name      string
		cliType   string
		supported []string
		effort    string
		wantErr   string // substring; "" means success expected
	}{
		{name: "empty effort no list", cliType: "claude", effort: ""},
		{name: "low defaults list to [low]", cliType: "claude", effort: "low"},
		{name: "high in list", cliType: "claude", supported: []string{"low", "medium", "high"}, effort: "high"},
		{name: "xhigh in list", cliType: "claude", supported: []string{"low", "high", "xhigh"}, effort: "xhigh"},
		{name: "ultra in codex list", cliType: "codex", supported: []string{"low", "ultra"}, effort: "ultra"},

		{name: "nonsense rejected", cliType: "claude", effort: "nonsense", wantErr: "must be one of low, medium, high, xhigh, max, ultra"},
		{name: "uppercase rejected", cliType: "claude", effort: "HIGH", wantErr: "invalid reasoning_effort"},
		{name: "xhigh outside list rejected", cliType: "claude", supported: []string{"low", "medium", "high"}, effort: "xhigh", wantErr: "is not supported by this model"},
		{name: "ultra outside list rejected", cliType: "codex", supported: []string{"low", "medium", "high", "xhigh"}, effort: "ultra", wantErr: "is not supported by this model"},
		{name: "invalid supported entry rejected", cliType: "claude", supported: []string{"low", "bogus"}, effort: "low", wantErr: "invalid supported_efforts entry"},
	}

	for i, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			svc, cleanup := setupCLIModelTestEnv(t)
			defer cleanup()

			req := types.CLIModelCreateRequest{
				ID:               fmt.Sprintf("re-test-%d", i),
				CLIType:          tt.cliType,
				DisplayName:      "RE Test",
				MappedModel:      "some-model",
				ReasoningEffort:  tt.effort,
				SupportedEfforts: tt.supported,
			}
			m, err := svc.Create(req)
			if tt.wantErr == "" {
				if err != nil {
					t.Fatalf("Create: unexpected error: %v", err)
				}
				if m.ReasoningEffort != tt.effort {
					t.Errorf("ReasoningEffort = %q, want %q", m.ReasoningEffort, tt.effort)
				}
				return
			}
			if err == nil {
				t.Fatalf("Create: expected error containing %q, got nil", tt.wantErr)
			}
			if !strings.Contains(err.Error(), tt.wantErr) {
				t.Errorf("error = %q, want to contain %q", err.Error(), tt.wantErr)
			}
		})
	}
}

// TestCLIModel_CreateSupportedEfforts_NormalizesAndSorts verifies an explicit
// supported_efforts list is deduped and sorted weakest→strongest.
func TestCLIModel_CreateSupportedEfforts_NormalizesAndSorts(t *testing.T) {
	t.Parallel()
	svc, cleanup := setupCLIModelTestEnv(t)
	defer cleanup()

	m, err := svc.Create(types.CLIModelCreateRequest{
		ID:               "norm-cli",
		CLIType:          "claude",
		DisplayName:      "Norm",
		MappedModel:      "some-model",
		SupportedEfforts: []string{"high", "low", "high"},
	})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if got := fmt.Sprintf("%v", m.SupportedEfforts); got != "[low high]" {
		t.Errorf("SupportedEfforts = %v, want [low high]", m.SupportedEfforts)
	}
	got, err := svc.Get("norm-cli")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if fmt.Sprintf("%v", got.SupportedEfforts) != "[low high]" {
		t.Errorf("persisted SupportedEfforts = %v, want [low high]", got.SupportedEfforts)
	}
}

// TestCLIModel_CreateDefaultsSupportedToEffort verifies that when the request
// omits supported_efforts but sets reasoning_effort, the list defaults to
// [reasoning_effort].
func TestCLIModel_CreateDefaultsSupportedToEffort(t *testing.T) {
	t.Parallel()
	svc, cleanup := setupCLIModelTestEnv(t)
	defer cleanup()

	m, err := svc.Create(types.CLIModelCreateRequest{
		ID:              "default-cli",
		CLIType:         "claude",
		DisplayName:     "Default",
		MappedModel:     "some-model",
		ReasoningEffort: "high",
	})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if fmt.Sprintf("%v", m.SupportedEfforts) != "[high]" {
		t.Errorf("SupportedEfforts = %v, want [high]", m.SupportedEfforts)
	}
}

func TestCLIModel_UpdateReasoningEffort_Valid(t *testing.T) {
	t.Parallel()
	svc, cleanup := setupCLIModelTestEnv(t)
	defer cleanup()

	// Seeded row: opus_4_7 → claude + claude-opus-4-7.
	effort := "xhigh"
	updated, err := svc.Update("opus_4_7", types.CLIModelUpdateRequest{
		ReasoningEffort: &effort,
	})
	if err != nil {
		t.Fatalf("Update: %v", err)
	}
	if updated.ReasoningEffort != "xhigh" {
		t.Errorf("ReasoningEffort = %q, want %q", updated.ReasoningEffort, "xhigh")
	}
}

func TestCLIModel_UpdateReasoningEffort_XhighAllowedOnSonnet5(t *testing.T) {
	t.Parallel()
	svc, cleanup := setupCLIModelTestEnv(t)
	defer cleanup()

	// sonnet is seeded as claude CLI with mapped_model=claude-sonnet-5, xhigh-capable.
	effort := "xhigh"
	updated, err := svc.Update("sonnet", types.CLIModelUpdateRequest{
		ReasoningEffort: &effort,
	})
	if err != nil {
		t.Fatalf("Update reasoning_effort=xhigh on sonnet: %v", err)
	}
	if updated.ReasoningEffort != "xhigh" {
		t.Errorf("ReasoningEffort = %q, want %q", updated.ReasoningEffort, "xhigh")
	}
}

func TestCLIModel_UpdateReasoningEffort_InvalidValue(t *testing.T) {
	t.Parallel()
	svc, cleanup := setupCLIModelTestEnv(t)
	defer cleanup()

	effort := "nonsense"
	_, err := svc.Update("sonnet", types.CLIModelUpdateRequest{
		ReasoningEffort: &effort,
	})
	if err == nil {
		t.Fatal("expected error for invalid reasoning_effort, got nil")
	}
	if !strings.Contains(err.Error(), "must be one of") {
		t.Errorf("error = %q, want to contain %q", err.Error(), "must be one of")
	}
}

func TestCLIModel_UpdateSupportedEfforts_InvalidatesStoredXhigh(t *testing.T) {
	t.Parallel()
	// User stored xhigh (in the row's supported list), then shrinks the list to
	// drop xhigh without clearing effort. The Update must be rejected.
	svc, cleanup := setupCLIModelTestEnv(t)
	defer cleanup()

	if _, err := svc.Create(types.CLIModelCreateRequest{
		ID:               "user-opus",
		CLIType:          "claude",
		DisplayName:      "User Opus",
		MappedModel:      "some-model",
		ReasoningEffort:  "xhigh",
		SupportedEfforts: []string{"low", "medium", "high", "xhigh"},
	}); err != nil {
		t.Fatalf("Create user-owned row: %v", err)
	}

	// Now shrink the supported list so xhigh is no longer offered.
	shrunk := []string{"low", "medium", "high"}
	_, err := svc.Update("user-opus", types.CLIModelUpdateRequest{
		SupportedEfforts: &shrunk,
	})
	if err == nil {
		t.Fatal("expected error: stored xhigh no longer in supported list, got nil")
	}
	if !strings.Contains(err.Error(), "is not supported by this model") {
		t.Errorf("error = %q, want to contain %q", err.Error(), "is not supported by this model")
	}

	// Verify state was not mutated.
	got, err := svc.Get("user-opus")
	if err != nil {
		t.Fatalf("Get after failed Update: %v", err)
	}
	if got.ReasoningEffort != "xhigh" {
		t.Errorf("ReasoningEffort = %q after failed Update, want %q (unchanged)", got.ReasoningEffort, "xhigh")
	}
	if fmt.Sprintf("%v", got.SupportedEfforts) != "[low medium high xhigh]" {
		t.Errorf("SupportedEfforts = %v after failed Update, want unchanged", got.SupportedEfforts)
	}
}

func TestCLIModel_UpdateSupportedEfforts_AndClearEffort(t *testing.T) {
	t.Parallel()
	// Shrinking the supported list WHILE also clearing effort must succeed.
	svc, cleanup := setupCLIModelTestEnv(t)
	defer cleanup()

	if _, err := svc.Create(types.CLIModelCreateRequest{
		ID:               "user-opus-2",
		CLIType:          "claude",
		DisplayName:      "User Opus 2",
		MappedModel:      "some-model",
		ReasoningEffort:  "xhigh",
		SupportedEfforts: []string{"low", "medium", "high", "xhigh"},
	}); err != nil {
		t.Fatalf("Create user-owned row: %v", err)
	}

	// Shrink list AND clear effort in same request.
	shrunk := []string{"low", "medium", "high"}
	empty := ""
	updated, err := svc.Update("user-opus-2", types.CLIModelUpdateRequest{
		SupportedEfforts: &shrunk,
		ReasoningEffort:  &empty,
	})
	if err != nil {
		t.Fatalf("Update: %v", err)
	}
	if updated.ReasoningEffort != "" {
		t.Errorf("ReasoningEffort = %q, want empty", updated.ReasoningEffort)
	}
	if fmt.Sprintf("%v", updated.SupportedEfforts) != "[low medium high]" {
		t.Errorf("SupportedEfforts = %v, want [low medium high]", updated.SupportedEfforts)
	}
}
