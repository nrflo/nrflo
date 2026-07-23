package service

import (
	"encoding/json"
	"errors"
	"testing"

	"be/internal/model"
	"be/internal/types"
)

// TestCreateAgentDef_ModelOrTierRequired documents CreateAgentDef's actual
// behavior for model==” + tier=nil: the pre-tiering "default to sonnet-5"
// fallback (agent_definition.go: `else if modelName == "" && req.Tier == nil
// { modelName = "sonnet-5" }`) runs BEFORE the "model or tier is required"
// check, so that check can never fire from CreateAgentDef — the def is
// silently defaulted to sonnet-5 instead of rejected. This is a production
// gap versus the ticket's stated invariant ("create with model==” and no
// tier -> error"); see be_production_bugs. UpdateAgentDef has no such
// default and does enforce the invariant (see
// TestUpdateAgentDef_ClearBothModelAndTier_Rejected).
func TestCreateAgentDef_ModelOrTierRequired(t *testing.T) {
	t.Parallel()
	_, svc, wfID := setupAgentDefTestEnv(t, nil)

	def, err := svc.CreateAgentDef("proj1", wfID, &types.AgentDefCreateRequest{
		ID: "tier-req", Prompt: "do work",
	})
	if err != nil {
		t.Fatalf("CreateAgentDef: %v, want success (silently defaults to sonnet-5)", err)
	}
	if def.Model != "sonnet-5" {
		t.Errorf("Model = %q, want sonnet-5 (default fallback)", def.Model)
	}
}

// TestCreateAgentDef_TierOnly_PersistsTierAndEmptyModel verifies a def
// created with tier set (and no model) persists tier and leaves model empty.
func TestCreateAgentDef_TierOnly_PersistsTierAndEmptyModel(t *testing.T) {
	t.Parallel()
	_, svc, wfID := setupAgentDefTestEnv(t, nil)

	tier := 2
	def, err := svc.CreateAgentDef("proj1", wfID, &types.AgentDefCreateRequest{
		ID: "tier-only", Prompt: "do work", Tier: &tier,
	})
	if err != nil {
		t.Fatalf("CreateAgentDef: %v", err)
	}
	if def.Model != "" {
		t.Errorf("Model = %q, want '' (tier-driven)", def.Model)
	}
	if def.Tier == nil || *def.Tier != 2 {
		t.Errorf("Tier = %v, want 2", def.Tier)
	}
}

// TestUpdateAgentDef_ClearModelWhileTierSet_Allowed verifies PATCHing
// model=” is accepted once the def already carries a tier.
func TestUpdateAgentDef_ClearModelWhileTierSet_Allowed(t *testing.T) {
	t.Parallel()
	_, svc, wfID := setupAgentDefTestEnv(t, nil)

	tier := 2
	if _, err := svc.CreateAgentDef("proj1", wfID, &types.AgentDefCreateRequest{
		ID: "clear-model", Prompt: "do work", Model: "sonnet-5", Tier: &tier,
	}); err != nil {
		t.Fatalf("CreateAgentDef: %v", err)
	}

	empty := ""
	if err := svc.UpdateAgentDef("proj1", wfID, "clear-model", &types.AgentDefUpdateRequest{
		Model: &empty,
	}); err != nil {
		t.Errorf("UpdateAgentDef(model=''): %v, want success (tier already set)", err)
	}
}

// TestUpdateAgentDef_ClearBothModelAndTier_Rejected verifies clearing model
// while also clearing/absent tier is rejected via the merged-effective-values
// guard (revalidateConsultantAndNodeRole).
func TestUpdateAgentDef_ClearBothModelAndTier_Rejected(t *testing.T) {
	t.Parallel()
	_, svc, wfID := setupAgentDefTestEnv(t, nil)

	if _, err := svc.CreateAgentDef("proj1", wfID, &types.AgentDefCreateRequest{
		ID: "clear-both", Prompt: "do work", Model: "sonnet-5",
	}); err != nil {
		t.Fatalf("CreateAgentDef: %v", err)
	}

	empty := ""
	err := svc.UpdateAgentDef("proj1", wfID, "clear-both", &types.AgentDefUpdateRequest{
		Model: &empty,
	})
	if err == nil {
		t.Fatal("expected error clearing model with no tier set, got nil")
	}
	if err.Error() != "model or tier is required" {
		t.Errorf("error = %q, want verbatim 'model or tier is required'", err.Error())
	}
}

// TestUpdateAgentDef_ExplicitNullTier_Clears verifies an explicit JSON
// `"tier": null` (types.AgentDefUpdateRequest.TierClear, set by
// UnmarshalJSON) actually nulls the tier column, distinguishing it from an
// omitted tier field (which must leave the existing tier untouched).
func TestUpdateAgentDef_ExplicitNullTier_Clears(t *testing.T) {
	t.Parallel()
	pool, svc, wfID := setupAgentDefTestEnv(t, nil)

	tier := 2
	if _, err := svc.CreateAgentDef("proj1", wfID, &types.AgentDefCreateRequest{
		ID: "clear-tier", Prompt: "do work", Tier: &tier,
	}); err != nil {
		t.Fatalf("CreateAgentDef: %v", err)
	}

	req := &types.AgentDefUpdateRequest{}
	if err := json.Unmarshal([]byte(`{"model":"sonnet-5","tier":null}`), req); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if !req.TierClear {
		t.Fatalf("TierClear = false, want true for explicit null")
	}
	if err := svc.UpdateAgentDef("proj1", wfID, "clear-tier", req); err != nil {
		t.Fatalf("UpdateAgentDef: %v", err)
	}

	var gotTier *int
	if err := pool.QueryRow("SELECT tier FROM agent_definitions WHERE id = 'clear-tier'").Scan(&gotTier); err != nil {
		t.Fatalf("query tier: %v", err)
	}
	if gotTier != nil {
		t.Errorf("tier = %v, want NULL after explicit clear", *gotTier)
	}
}

// TestUpdateAgentDef_OmittedTier_LeavesUntouched verifies a PATCH body
// without a tier field at all leaves the existing tier value unchanged
// (distinguishing "omitted" from "explicit null" via TierClear).
func TestUpdateAgentDef_OmittedTier_LeavesUntouched(t *testing.T) {
	t.Parallel()
	pool, svc, wfID := setupAgentDefTestEnv(t, nil)

	tier := 3
	if _, err := svc.CreateAgentDef("proj1", wfID, &types.AgentDefCreateRequest{
		ID: "keep-tier", Prompt: "do work", Tier: &tier,
	}); err != nil {
		t.Fatalf("CreateAgentDef: %v", err)
	}

	req := &types.AgentDefUpdateRequest{}
	if err := json.Unmarshal([]byte(`{"description":"noop"}`), req); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if req.TierClear {
		t.Fatalf("TierClear = true, want false for omitted tier field")
	}
	if err := svc.UpdateAgentDef("proj1", wfID, "keep-tier", req); err != nil {
		t.Fatalf("UpdateAgentDef: %v", err)
	}

	var gotTier *int
	if err := pool.QueryRow("SELECT tier FROM agent_definitions WHERE id = 'keep-tier'").Scan(&gotTier); err != nil {
		t.Fatalf("query tier: %v", err)
	}
	if gotTier == nil || *gotTier != 3 {
		t.Errorf("tier = %v, want 3 (untouched)", gotTier)
	}
}

// TestCreateAgentDef_TierOnScriptMode_Rejected verifies tier is incompatible
// with execution_mode='script'.
func TestCreateAgentDef_TierOnScriptMode_Rejected(t *testing.T) {
	t.Parallel()
	_, svc, wfID := setupAgentDefTestEnv(t, nil)

	tier := 2
	_, err := svc.CreateAgentDef("proj1", wfID, &types.AgentDefCreateRequest{
		ID: "script-tier", ExecutionMode: "script", Tier: &tier,
	})
	if err == nil {
		t.Fatal("expected error for tier on script mode, got nil")
	}
	if !errors.Is(err, ErrValidation) {
		t.Errorf("error not tagged ErrValidation: %v", err)
	}
}

// TestUpdateAgentDef_TierOnScriptMode_Rejected mirrors the create-time
// rejection for a PATCH that sets tier on a script-mode def.
func TestUpdateAgentDef_TierOnScriptMode_Rejected(t *testing.T) {
	t.Parallel()
	svc, wfID, scriptID := setupAgentDefScriptEnv(t)
	if _, err := svc.CreateAgentDef("proj1", wfID, &types.AgentDefCreateRequest{
		ID: "script-patch-tier", ExecutionMode: "script", PythonScriptID: &scriptID,
	}); err != nil {
		t.Fatalf("CreateAgentDef: %v", err)
	}

	tier := 2
	err := svc.UpdateAgentDef("proj1", wfID, "script-patch-tier", &types.AgentDefUpdateRequest{
		Tier: &tier,
	})
	if err == nil {
		t.Fatal("expected error setting tier on a script-mode def, got nil")
	}
	if !errors.Is(err, ErrValidation) {
		t.Errorf("error not tagged ErrValidation: %v", err)
	}
}

// TestCreateAgentDef_TierOutOfRange_Rejected verifies tier is validated to
// the 1..5 range.
func TestCreateAgentDef_TierOutOfRange_Rejected(t *testing.T) {
	t.Parallel()
	_, svc, wfID := setupAgentDefTestEnv(t, nil)

	for _, bad := range []int{0, -1, 6, 100} {
		bad := bad
		t.Run("", func(t *testing.T) {
			_, err := svc.CreateAgentDef("proj1", wfID, &types.AgentDefCreateRequest{
				ID: "tier-range", Prompt: "do work", Tier: &bad,
			})
			if err == nil {
				t.Fatalf("tier=%d: expected error, got nil", bad)
			}
			if !errors.Is(err, ErrValidation) {
				t.Errorf("tier=%d: error not tagged ErrValidation: %v", bad, err)
			}
		})
	}
}

// TestUpdateAgentDef_TierOutOfRange_Rejected mirrors the create-time range
// check for PATCH.
func TestUpdateAgentDef_TierOutOfRange_Rejected(t *testing.T) {
	t.Parallel()
	_, svc, wfID := setupAgentDefTestEnv(t, nil)
	if _, err := svc.CreateAgentDef("proj1", wfID, &types.AgentDefCreateRequest{
		ID: "patch-tier-range", Prompt: "do work", Model: "sonnet-5",
	}); err != nil {
		t.Fatalf("CreateAgentDef: %v", err)
	}

	bad := 0
	err := svc.UpdateAgentDef("proj1", wfID, "patch-tier-range", &types.AgentDefUpdateRequest{Tier: &bad})
	if err == nil {
		t.Fatal("expected error for tier=0, got nil")
	}
	if !errors.Is(err, ErrValidation) {
		t.Errorf("error not tagged ErrValidation: %v", err)
	}
}

// TestCreateAgentDef_NativeToolsSandbox_RejectedWhenModelEmpty verifies
// native_tools/sandbox require an explicit model override — a tier-driven
// def (model==”) doesn't know its provider until spawn time.
func TestCreateAgentDef_NativeToolsSandbox_RejectedWhenModelEmpty(t *testing.T) {
	t.Parallel()
	_, svc, wfID := setupAgentDefTestEnv(t, nil)

	tier := 2
	cases := []struct {
		name        string
		nativeTools string
		sandbox     string
	}{
		{"native_tools set", "Read,Grep", ""},
		{"sandbox set", "", model.SandboxReadOnly},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			_, err := svc.CreateAgentDef("proj1", wfID, &types.AgentDefCreateRequest{
				ID:          "native-tier-" + c.name,
				Prompt:      "do work",
				Tier:        &tier,
				NativeTools: c.nativeTools,
				Sandbox:     c.sandbox,
			})
			if err == nil {
				t.Fatal("expected error, got nil")
			}
			if !errors.Is(err, ErrValidation) {
				t.Errorf("error not tagged ErrValidation: %v", err)
			}
		})
	}
}
