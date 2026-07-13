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
	tests := []struct {
		name        string
		cliType     string
		mappedModel string
		effort      string
		wantErr     string // substring; "" means success expected
	}{
		{name: "empty effort claude sonnet", cliType: "claude", mappedModel: "claude-sonnet-4-5", effort: ""},
		{name: "low effort claude sonnet", cliType: "claude", mappedModel: "claude-sonnet-4-5", effort: "low"},
		{name: "medium effort claude opus 4.6", cliType: "claude", mappedModel: "claude-opus-4-6", effort: "medium"},
		{name: "high effort claude opus 4.7", cliType: "claude", mappedModel: "claude-opus-4-7", effort: "high"},
		{name: "max effort claude sonnet", cliType: "claude", mappedModel: "claude-sonnet-4-5", effort: "max"},
		{name: "xhigh effort claude opus 4.7", cliType: "claude", mappedModel: "claude-opus-4-7", effort: "xhigh"},
		{name: "xhigh effort claude opus 4.7 1M", cliType: "claude", mappedModel: "claude-opus-4-7[1m]", effort: "xhigh"},
		{name: "xhigh effort codex ok", cliType: "codex", mappedModel: "gpt-5.3-codex", effort: "xhigh"},
		{name: "ultra effort codex sol ok", cliType: "codex", mappedModel: "gpt-5.6-sol", effort: "ultra"},
		{name: "ultra effort codex terra ok", cliType: "codex", mappedModel: "gpt-5.6-terra", effort: "ultra"},

		{name: "nonsense rejected", cliType: "claude", mappedModel: "claude-opus-4-7", effort: "nonsense", wantErr: "must be one of low, medium, high, xhigh, max, ultra"},
		{name: "uppercase rejected", cliType: "claude", mappedModel: "claude-opus-4-7", effort: "HIGH", wantErr: "invalid reasoning_effort"},
		{name: "xhigh on sonnet rejected", cliType: "claude", mappedModel: "claude-sonnet-4-5", effort: "xhigh", wantErr: "only supported on Opus 4.7"},
		{name: "xhigh on opus 4.6 rejected", cliType: "claude", mappedModel: "claude-opus-4-6", effort: "xhigh", wantErr: "only supported on Opus 4.7"},
		{name: "xhigh on opus 4.6 1M rejected", cliType: "claude", mappedModel: "claude-opus-4-6[1m]", effort: "xhigh", wantErr: "only supported on Opus 4.7"},
		{name: "ultra on codex luna rejected", cliType: "codex", mappedModel: "gpt-5.6-luna", effort: "ultra", wantErr: "only supported on Codex GPT-5.6 Sol/Terra"},
		{name: "ultra on codex gpt-5.5 rejected", cliType: "codex", mappedModel: "gpt-5.5", effort: "ultra", wantErr: "only supported on Codex GPT-5.6 Sol/Terra"},
		{name: "ultra on claude rejected", cliType: "claude", mappedModel: "claude-opus-4-8", effort: "ultra", wantErr: "only supported on Codex GPT-5.6 Sol/Terra"},
	}

	for i, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			svc, cleanup := setupCLIModelTestEnv(t)
			defer cleanup()

			req := types.CLIModelCreateRequest{
				ID:              fmt.Sprintf("re-test-%d", i),
				CLIType:         tt.cliType,
				DisplayName:     "RE Test",
				MappedModel:     tt.mappedModel,
				ReasoningEffort: tt.effort,
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

func TestCLIModel_UpdateMappedModel_InvalidatesStoredXhigh(t *testing.T) {
	t.Parallel()
	// User stored xhigh on a user-owned Opus-4.7 row, then changes mapped_model to sonnet
	// without clearing effort. Overlay logic must reject the Update.
	svc, cleanup := setupCLIModelTestEnv(t)
	defer cleanup()

	// Create a user-owned Opus-4.7 row (read_only rows block mapped_model edits).
	if _, err := svc.Create(types.CLIModelCreateRequest{
		ID:          "user-opus",
		CLIType:     "claude",
		DisplayName: "User Opus",
		MappedModel: "claude-opus-4-7",
	}); err != nil {
		t.Fatalf("Create user-owned row: %v", err)
	}

	// First set xhigh on the user-owned row (valid).
	xhigh := "xhigh"
	if _, err := svc.Update("user-opus", types.CLIModelUpdateRequest{
		ReasoningEffort: &xhigh,
	}); err != nil {
		t.Fatalf("initial Update: %v", err)
	}

	// Now try to switch mapped_model to a non-Opus-4.7 value without touching effort.
	newMapped := "claude-sonnet-4-5"
	_, err := svc.Update("user-opus", types.CLIModelUpdateRequest{
		MappedModel: &newMapped,
	})
	if err == nil {
		t.Fatal("expected error: overlay logic must reject xhigh + non-Opus-4.7 combination, got nil")
	}
	if !strings.Contains(err.Error(), "only supported on Opus 4.7") {
		t.Errorf("error = %q, want to contain %q", err.Error(), "only supported on Opus 4.7")
	}

	// Verify state was not mutated.
	got, err := svc.Get("user-opus")
	if err != nil {
		t.Fatalf("Get after failed Update: %v", err)
	}
	if got.MappedModel != "claude-opus-4-7" {
		t.Errorf("MappedModel = %q after failed Update, want %q (unchanged)", got.MappedModel, "claude-opus-4-7")
	}
	if got.ReasoningEffort != "xhigh" {
		t.Errorf("ReasoningEffort = %q after failed Update, want %q (unchanged)", got.ReasoningEffort, "xhigh")
	}
}

func TestCLIModel_UpdateMappedModel_AndClearEffort(t *testing.T) {
	t.Parallel()
	// Switching mapped_model to non-Opus-4.7 WHILE also clearing effort must succeed.
	// Uses a user-owned row because read_only rows block mapped_model edits.
	svc, cleanup := setupCLIModelTestEnv(t)
	defer cleanup()

	if _, err := svc.Create(types.CLIModelCreateRequest{
		ID:          "user-opus-2",
		CLIType:     "claude",
		DisplayName: "User Opus 2",
		MappedModel: "claude-opus-4-7",
	}); err != nil {
		t.Fatalf("Create user-owned row: %v", err)
	}

	// Seed xhigh.
	xhigh := "xhigh"
	if _, err := svc.Update("user-opus-2", types.CLIModelUpdateRequest{
		ReasoningEffort: &xhigh,
	}); err != nil {
		t.Fatalf("initial Update: %v", err)
	}

	// Switch mapped_model AND clear effort in same request.
	newMapped := "claude-sonnet-4-5"
	empty := ""
	updated, err := svc.Update("user-opus-2", types.CLIModelUpdateRequest{
		MappedModel:     &newMapped,
		ReasoningEffort: &empty,
	})
	if err != nil {
		t.Fatalf("Update: %v", err)
	}
	if updated.MappedModel != "claude-sonnet-4-5" {
		t.Errorf("MappedModel = %q, want %q", updated.MappedModel, "claude-sonnet-4-5")
	}
	if updated.ReasoningEffort != "" {
		t.Errorf("ReasoningEffort = %q, want empty", updated.ReasoningEffort)
	}
}
