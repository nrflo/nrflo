package service

import (
	"testing"

	"be/internal/types"
)

// TestSystemAgentDef_ReasoningEffort_ValidationMatrix mirrors
// TestCreateAgentDef_ReasoningEffort_ValidationMatrix (agent_definition_effort_test.go)
// for system_agent_definitions: same model-family gates from
// service/model_reasoning.go, reached via
// SystemAgentDefinitionService.Create -> validateDefReasoningEffort.
func TestSystemAgentDef_ReasoningEffort_ValidationMatrix(t *testing.T) {
	svc, cleanup := setupSysAgentDefTestEnv(t)
	t.Cleanup(cleanup)
	settingsSvc := NewGlobalSettingsService(svc.pool, svc.clock)
	if err := settingsSvc.Set("api_mode_enabled", "true"); err != nil {
		t.Fatalf("Set api_mode_enabled: %v", err)
	}

	cases := []struct {
		name          string
		executionMode string
		model         string
		effort        *string
		wantErr       bool
	}{
		{"ultra on claude cli row rejected", "cli_interactive", "sonnet-5", strPtr("ultra"), true},
		{"ultra on codex sol row accepted", "cli_interactive", "gpt-5.6-sol", strPtr("ultra"), false},
		{"xhigh on claude haiku rejected", "cli_interactive", "haiku-4-5", strPtr("xhigh"), true},
		{"xhigh on claude opus accepted", "cli_interactive", "opus-4-8", strPtr("xhigh"), false},
		{"xhigh on api openai gpt-5.6 accepted", "api", "gpt-5.6-sol", strPtr("xhigh"), false},
		{"max on api openai gpt-5.4 rejected", "api", "gpt-5.4", strPtr("max"), true},
		{"ultra on api def rejected regardless of model", "api", "opus-4-8", strPtr("ultra"), true},
		{"xhigh on api anthropic opus accepted", "api", "opus-4-8", strPtr("xhigh"), false},
		{"nil reasoning_effort accepted (inherit)", "cli_interactive", "sonnet-5", nil, false},
		{"garbage string rejected", "cli_interactive", "sonnet-5", strPtr("extreme"), true},
	}

	for i, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			id := "sys-effort-agent-" + string(rune('a'+i))
			def, err := svc.Create(&types.SystemAgentDefCreateRequest{
				ID:              id,
				Prompt:          "do work",
				ExecutionMode:   tc.executionMode,
				Model:           tc.model,
				ReasoningEffort: tc.effort,
			})
			if tc.wantErr {
				if err == nil {
					t.Fatalf("Create(mode=%s, model=%s, effort=%v): expected error, got nil", tc.executionMode, tc.model, tc.effort)
				}
				return
			}
			if err != nil {
				t.Fatalf("Create(mode=%s, model=%s, effort=%v): %v", tc.executionMode, tc.model, tc.effort, err)
			}
			if tc.effort == nil {
				if def.ReasoningEffort != nil {
					t.Errorf("ReasoningEffort = %v, want nil", def.ReasoningEffort)
				}
				return
			}
			if def.ReasoningEffort == nil || *def.ReasoningEffort != *tc.effort {
				t.Errorf("ReasoningEffort = %v, want %v", def.ReasoningEffort, *tc.effort)
			}
		})
	}
}

// TestSystemAgentDef_ReasoningEffort_RoundTripsThroughGetAndList verifies a
// valid override survives Create, Get, and List.
func TestSystemAgentDef_ReasoningEffort_RoundTripsThroughGetAndList(t *testing.T) {
	svc, cleanup := setupSysAgentDefTestEnv(t)
	t.Cleanup(cleanup)

	created, err := svc.Create(&types.SystemAgentDefCreateRequest{
		ID:              "sys-effort-roundtrip",
		Prompt:          "do work",
		Model:           "opus-4-8",
		ReasoningEffort: strPtr("xhigh"),
	})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if created.ReasoningEffort == nil || *created.ReasoningEffort != "xhigh" {
		t.Fatalf("Create ReasoningEffort = %v, want xhigh", created.ReasoningEffort)
	}

	got, err := svc.Get("sys-effort-roundtrip")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got.ReasoningEffort == nil || *got.ReasoningEffort != "xhigh" {
		t.Fatalf("Get ReasoningEffort = %v, want xhigh", got.ReasoningEffort)
	}

	list, err := svc.List()
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	found := false
	for _, d := range list {
		if d.ID == "sys-effort-roundtrip" {
			found = true
			if d.ReasoningEffort == nil || *d.ReasoningEffort != "xhigh" {
				t.Errorf("List ReasoningEffort = %v, want xhigh", d.ReasoningEffort)
			}
		}
	}
	if !found {
		t.Fatal("sys-effort-roundtrip missing from List")
	}
}

// TestSystemAgentDef_ReasoningEffort_DirectPatchValidated verifies a PATCH
// that sets reasoning_effort directly is validated against the current model
// row.
func TestSystemAgentDef_ReasoningEffort_DirectPatchValidated(t *testing.T) {
	svc, cleanup := setupSysAgentDefTestEnv(t)
	t.Cleanup(cleanup)

	if _, err := svc.Create(&types.SystemAgentDefCreateRequest{
		ID:     "sys-patch-direct",
		Prompt: "do work",
		Model:  "haiku-4-5",
	}); err != nil {
		t.Fatalf("create: %v", err)
	}

	bad := "xhigh"
	if err := svc.Update("sys-patch-direct", &types.SystemAgentDefUpdateRequest{
		ReasoningEffort: &bad,
	}); err == nil {
		t.Fatal("Update(reasoning_effort=xhigh) on a haiku-4-5 def: expected error, got nil")
	}

	good := "high"
	if err := svc.Update("sys-patch-direct", &types.SystemAgentDefUpdateRequest{
		ReasoningEffort: &good,
	}); err != nil {
		t.Fatalf("Update(reasoning_effort=high) on a haiku-4-5 def: %v", err)
	}
	def, err := svc.Get("sys-patch-direct")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if def.ReasoningEffort == nil || *def.ReasoningEffort != "high" {
		t.Errorf("ReasoningEffort = %v, want high", def.ReasoningEffort)
	}
}

func TestSystemAgentDef_ModelPatchRevalidatesExistingEffort(t *testing.T) {
	svc, cleanup := setupSysAgentDefTestEnv(t)
	t.Cleanup(cleanup)
	if _, err := svc.Create(&types.SystemAgentDefCreateRequest{
		ID: "sys-model-patch", Prompt: "do work", Model: "gpt-5.6-sol",
		ReasoningEffort: strPtr("ultra"),
	}); err != nil {
		t.Fatalf("create: %v", err)
	}
	modelName := "sonnet-5"
	if err := svc.Update("sys-model-patch", &types.SystemAgentDefUpdateRequest{Model: &modelName}); err == nil {
		t.Fatal("model patch stranded an unsupported effort")
	}
}
