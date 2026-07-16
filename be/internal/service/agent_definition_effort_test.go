package service

import (
	"testing"

	"be/internal/types"
)

// TestCreateAgentDef_ReasoningEffort_ValidationMatrix exercises
// validateDefReasoningEffort (service/agent_definition_validate.go) through
// CreateAgentDef against each model row's supported_efforts list: an effort
// outside the row's list is rejected, one inside it is accepted. No API row is
// seeded with "ultra", so "ultra" on an api def is always a membership error.
func TestCreateAgentDef_ReasoningEffort_ValidationMatrix(t *testing.T) {
	t.Parallel()
	svc, settingsSvc, wfID := setupAgentDefAPIModeEnv(t)
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
		{"ultra on codex terra row accepted", "cli_interactive", "gpt-5.6-terra", strPtr("ultra"), false},
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
			id := "effort-agent-" + string(rune('a'+i))
			def, err := svc.CreateAgentDef("proj1", wfID, &types.AgentDefCreateRequest{
				ID:              id,
				Prompt:          "do work",
				ExecutionMode:   tc.executionMode,
				Model:           tc.model,
				ReasoningEffort: tc.effort,
			})
			if tc.wantErr {
				if err == nil {
					t.Fatalf("CreateAgentDef(mode=%s, model=%s, effort=%v): expected error, got nil", tc.executionMode, tc.model, tc.effort)
				}
				return
			}
			if err != nil {
				t.Fatalf("CreateAgentDef(mode=%s, model=%s, effort=%v): %v", tc.executionMode, tc.model, tc.effort, err)
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

// TestCreateAgentDef_ReasoningEffort_RoundTripsThroughGetAndList verifies a
// valid override survives Create, Get, and List, and that a nil override
// (inherit) round-trips as nil rather than an empty string.
func TestCreateAgentDef_ReasoningEffort_RoundTripsThroughGetAndList(t *testing.T) {
	t.Parallel()
	svc, _, wfID := setupAgentDefAPIModeEnv(t)

	created, err := svc.CreateAgentDef("proj1", wfID, &types.AgentDefCreateRequest{
		ID:              "effort-roundtrip",
		Prompt:          "do work",
		Model:           "opus-4-8",
		ReasoningEffort: strPtr("xhigh"),
	})
	if err != nil {
		t.Fatalf("CreateAgentDef: %v", err)
	}
	if created.ReasoningEffort == nil || *created.ReasoningEffort != "xhigh" {
		t.Fatalf("Create ReasoningEffort = %v, want xhigh", created.ReasoningEffort)
	}

	got, err := svc.GetAgentDef("proj1", wfID, "effort-roundtrip")
	if err != nil {
		t.Fatalf("GetAgentDef: %v", err)
	}
	if got.ReasoningEffort == nil || *got.ReasoningEffort != "xhigh" {
		t.Fatalf("Get ReasoningEffort = %v, want xhigh", got.ReasoningEffort)
	}

	list, err := svc.ListAgentDefs("proj1", wfID)
	if err != nil {
		t.Fatalf("ListAgentDefs: %v", err)
	}
	found := false
	for _, d := range list {
		if d.ID == "effort-roundtrip" {
			found = true
			if d.ReasoningEffort == nil || *d.ReasoningEffort != "xhigh" {
				t.Errorf("List ReasoningEffort = %v, want xhigh", d.ReasoningEffort)
			}
		}
	}
	if !found {
		t.Fatal("effort-roundtrip missing from ListAgentDefs")
	}

	noOverride, err := svc.CreateAgentDef("proj1", wfID, &types.AgentDefCreateRequest{
		ID:     "effort-no-override",
		Prompt: "do work",
		Model:  "sonnet-5",
	})
	if err != nil {
		t.Fatalf("CreateAgentDef (no override): %v", err)
	}
	if noOverride.ReasoningEffort != nil {
		t.Errorf("ReasoningEffort = %v, want nil for an unset override", noOverride.ReasoningEffort)
	}
}

// TestUpdateAgentDef_ReasoningEffort_PatchSafetyNet is acceptance case #2: a
// def created with a legal override (ultra, bound to a codex row) must be
// re-validated when a PATCH changes only `model` — swapping to a claude row
// strands the "ultra" override, which is illegal for claude, so the PATCH
// must fail even though it never touched reasoning_effort itself. This is
// exactly why revalidateConsultantAndNodeRole grew model+reasoning_effort.
func TestUpdateAgentDef_ReasoningEffort_PatchSafetyNet(t *testing.T) {
	t.Parallel()
	svc, _, wfID := setupAgentDefAPIModeEnv(t)

	if _, err := svc.CreateAgentDef("proj1", wfID, &types.AgentDefCreateRequest{
		ID:              "patch-safety-net",
		Prompt:          "do work",
		Model:           "gpt-5.6-sol",
		ReasoningEffort: strPtr("ultra"),
	}); err != nil {
		t.Fatalf("create: %v", err)
	}

	newModel := "sonnet-5"
	err := svc.UpdateAgentDef("proj1", wfID, "patch-safety-net", &types.AgentDefUpdateRequest{
		Model: &newModel,
	})
	if err == nil {
		t.Fatal("UpdateAgentDef(model=sonnet-5) on a def carrying an 'ultra' override: expected error, got nil")
	}

	// The def must be left untouched by the rejected PATCH.
	def, getErr := svc.GetAgentDef("proj1", wfID, "patch-safety-net")
	if getErr != nil {
		t.Fatalf("GetAgentDef: %v", getErr)
	}
	if def.Model != "gpt-5.6-sol" {
		t.Errorf("Model after rejected PATCH = %q, want unchanged %q", def.Model, "gpt-5.6-sol")
	}
}

// TestUpdateAgentDef_ReasoningEffort_PatchSafetyNet_ModelSwapToCompatibleRow
// verifies the counterpart: swapping model to another row that still
// supports the existing override succeeds.
func TestUpdateAgentDef_ReasoningEffort_PatchSafetyNet_ModelSwapToCompatibleRow(t *testing.T) {
	t.Parallel()
	svc, _, wfID := setupAgentDefAPIModeEnv(t)

	if _, err := svc.CreateAgentDef("proj1", wfID, &types.AgentDefCreateRequest{
		ID:              "patch-safety-net-ok",
		Prompt:          "do work",
		Model:           "gpt-5.6-sol",
		ReasoningEffort: strPtr("ultra"),
	}); err != nil {
		t.Fatalf("create: %v", err)
	}

	newModel := "gpt-5.6-terra"
	if err := svc.UpdateAgentDef("proj1", wfID, "patch-safety-net-ok", &types.AgentDefUpdateRequest{
		Model: &newModel,
	}); err != nil {
		t.Fatalf("UpdateAgentDef(model=codex_gpt56_terra_high) on a def carrying a still-legal 'ultra' override: %v", err)
	}

	def, getErr := svc.GetAgentDef("proj1", wfID, "patch-safety-net-ok")
	if getErr != nil {
		t.Fatalf("GetAgentDef: %v", getErr)
	}
	if def.Model != newModel {
		t.Errorf("Model after PATCH = %q, want %q", def.Model, newModel)
	}
	if def.ReasoningEffort == nil || *def.ReasoningEffort != "ultra" {
		t.Errorf("ReasoningEffort after PATCH = %v, want ultra (untouched)", def.ReasoningEffort)
	}
}

// TestUpdateAgentDef_ReasoningEffort_DirectPatchValidated verifies a PATCH
// that sets reasoning_effort directly is validated against the current model
// row.
func TestUpdateAgentDef_ReasoningEffort_DirectPatchValidated(t *testing.T) {
	t.Parallel()
	svc, _, wfID := setupAgentDefAPIModeEnv(t)

	if _, err := svc.CreateAgentDef("proj1", wfID, &types.AgentDefCreateRequest{
		ID:     "patch-direct",
		Prompt: "do work",
		Model:  "haiku-4-5",
	}); err != nil {
		t.Fatalf("create: %v", err)
	}

	bad := "xhigh"
	if err := svc.UpdateAgentDef("proj1", wfID, "patch-direct", &types.AgentDefUpdateRequest{
		ReasoningEffort: &bad,
	}); err == nil {
		t.Fatal("UpdateAgentDef(reasoning_effort=xhigh) on a haiku-4-5 def: expected error, got nil")
	}

	good := "high"
	if err := svc.UpdateAgentDef("proj1", wfID, "patch-direct", &types.AgentDefUpdateRequest{
		ReasoningEffort: &good,
	}); err != nil {
		t.Fatalf("UpdateAgentDef(reasoning_effort=high) on a haiku-4-5 def: %v", err)
	}
	def, err := svc.GetAgentDef("proj1", wfID, "patch-direct")
	if err != nil {
		t.Fatalf("GetAgentDef: %v", err)
	}
	if def.ReasoningEffort == nil || *def.ReasoningEffort != "high" {
		t.Errorf("ReasoningEffort = %v, want high", def.ReasoningEffort)
	}
}
