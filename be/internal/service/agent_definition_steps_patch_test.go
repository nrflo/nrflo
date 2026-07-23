package service

// Tests for UpdateAgentDef's PATCH merged-effective-value prompt_mode/steps
// rules — see agent_definition_update_meta.go resolvePromptModeUpdate.

import (
	"errors"
	"testing"

	"be/internal/model"
	"be/internal/types"
)

// TestUpdateAgentDef_PromptMode_BothOmitted_NoOp verifies a PATCH omitting
// both prompt_mode and steps leaves stored values untouched.
func TestUpdateAgentDef_PromptMode_BothOmitted_NoOp(t *testing.T) {
	t.Parallel()
	_, svc, wfID := setupAgentDefTestEnv(t, nil)
	steps := []model.StepDefinition{validStep("s1")}
	if _, err := svc.CreateAgentDef("proj1", wfID, &types.AgentDefCreateRequest{
		ID: "patch-noop", Prompt: "do work", PromptMode: PromptModeStepwise, Steps: &steps,
	}); err != nil {
		t.Fatalf("create: %v", err)
	}

	desc := "unrelated"
	if err := svc.UpdateAgentDef("proj1", wfID, "patch-noop", &types.AgentDefUpdateRequest{Description: &desc}); err != nil {
		t.Fatalf("UpdateAgentDef: %v", err)
	}
	def, err := svc.GetAgentDef("proj1", wfID, "patch-noop")
	if err != nil {
		t.Fatalf("GetAgentDef: %v", err)
	}
	if def.PromptMode != PromptModeStepwise || def.Steps == nil {
		t.Errorf("PromptMode/Steps = %q/%v, want unchanged stepwise/non-nil", def.PromptMode, def.Steps)
	}
}

// TestUpdateAgentDef_PromptMode_FullWithIncomingStepsRejected verifies
// effective mode 'full' + incoming steps is rejected.
func TestUpdateAgentDef_PromptMode_FullWithIncomingStepsRejected(t *testing.T) {
	t.Parallel()
	_, svc, wfID := setupAgentDefTestEnv(t, nil)
	if _, err := svc.CreateAgentDef("proj1", wfID, &types.AgentDefCreateRequest{
		ID: "patch-full-steps", Prompt: "do work",
	}); err != nil {
		t.Fatalf("create: %v", err)
	}

	steps := []model.StepDefinition{validStep("s1")}
	full := PromptModeFull
	err := svc.UpdateAgentDef("proj1", wfID, "patch-full-steps", &types.AgentDefUpdateRequest{
		PromptMode: &full, Steps: &steps,
	})
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !errors.Is(err, ErrValidation) {
		t.Errorf("error = %v, want errors.Is(err, ErrValidation)", err)
	}
}

// TestUpdateAgentDef_PromptMode_StepwiseToFullClearsSteps verifies switching
// to 'full' while stored is 'stepwise' clears steps (sets prompt_mode='full',
// steps=NULL).
func TestUpdateAgentDef_PromptMode_StepwiseToFullClearsSteps(t *testing.T) {
	t.Parallel()
	_, svc, wfID := setupAgentDefTestEnv(t, nil)
	steps := []model.StepDefinition{validStep("s1")}
	if _, err := svc.CreateAgentDef("proj1", wfID, &types.AgentDefCreateRequest{
		ID: "patch-to-full", Prompt: "do work", PromptMode: PromptModeStepwise, Steps: &steps,
	}); err != nil {
		t.Fatalf("create: %v", err)
	}

	full := PromptModeFull
	if err := svc.UpdateAgentDef("proj1", wfID, "patch-to-full", &types.AgentDefUpdateRequest{
		PromptMode: &full,
	}); err != nil {
		t.Fatalf("UpdateAgentDef: %v", err)
	}
	def, err := svc.GetAgentDef("proj1", wfID, "patch-to-full")
	if err != nil {
		t.Fatalf("GetAgentDef: %v", err)
	}
	if def.PromptMode != PromptModeFull {
		t.Errorf("PromptMode = %q, want full", def.PromptMode)
	}
	if def.Steps != nil {
		t.Errorf("Steps = %v, want nil (cleared)", def.Steps)
	}
}

// TestUpdateAgentDef_PromptMode_FullToStepwiseWithoutStepsRejected verifies
// effective 'stepwise' with neither incoming nor stored steps is rejected.
func TestUpdateAgentDef_PromptMode_FullToStepwiseWithoutStepsRejected(t *testing.T) {
	t.Parallel()
	_, svc, wfID := setupAgentDefTestEnv(t, nil)
	if _, err := svc.CreateAgentDef("proj1", wfID, &types.AgentDefCreateRequest{
		ID: "patch-full-to-stepwise", Prompt: "do work",
	}); err != nil {
		t.Fatalf("create: %v", err)
	}

	stepwise := PromptModeStepwise
	err := svc.UpdateAgentDef("proj1", wfID, "patch-full-to-stepwise", &types.AgentDefUpdateRequest{
		PromptMode: &stepwise,
	})
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !errors.Is(err, ErrValidation) {
		t.Errorf("error = %v, want errors.Is(err, ErrValidation)", err)
	}
}

// TestUpdateAgentDef_PromptMode_ScriptStepwiseRejected verifies effective
// stepwise + effective execution_mode='script' is rejected.
func TestUpdateAgentDef_PromptMode_ScriptStepwiseRejected(t *testing.T) {
	t.Parallel()
	_, svc, wfID := setupAgentDefTestEnv(t, nil)
	if _, err := svc.CreateAgentDef("proj1", wfID, &types.AgentDefCreateRequest{
		ID: "patch-script-stepwise", Prompt: "do work",
	}); err != nil {
		t.Fatalf("create: %v", err)
	}

	steps := []model.StepDefinition{validStep("s1")}
	scriptMode := "script"
	stepwise := PromptModeStepwise
	err := svc.UpdateAgentDef("proj1", wfID, "patch-script-stepwise", &types.AgentDefUpdateRequest{
		ExecutionMode: &scriptMode,
		PromptMode:    &stepwise,
		Steps:         &steps,
	})
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !errors.Is(err, ErrValidation) {
		t.Errorf("error = %v, want errors.Is(err, ErrValidation)", err)
	}
}

// TestUpdateAgentDef_PromptMode_CanonicalRoundTrip verifies the happy path:
// validate+canonicalize incoming steps and SET both columns.
func TestUpdateAgentDef_PromptMode_CanonicalRoundTrip(t *testing.T) {
	t.Parallel()
	_, svc, wfID := setupAgentDefTestEnv(t, nil)
	if _, err := svc.CreateAgentDef("proj1", wfID, &types.AgentDefCreateRequest{
		ID: "patch-roundtrip", Prompt: "do work",
	}); err != nil {
		t.Fatalf("create: %v", err)
	}

	steps := []model.StepDefinition{validStep("s1"), validStep("s2")}
	stepwise := PromptModeStepwise
	if err := svc.UpdateAgentDef("proj1", wfID, "patch-roundtrip", &types.AgentDefUpdateRequest{
		PromptMode: &stepwise, Steps: &steps,
	}); err != nil {
		t.Fatalf("UpdateAgentDef: %v", err)
	}
	def, err := svc.GetAgentDef("proj1", wfID, "patch-roundtrip")
	if err != nil {
		t.Fatalf("GetAgentDef: %v", err)
	}
	if def.PromptMode != PromptModeStepwise {
		t.Errorf("PromptMode = %q, want stepwise", def.PromptMode)
	}
	if def.Steps == nil {
		t.Fatal("Steps = nil, want canonical JSON")
	}

	// PATCH with only prompt_mode omitting steps, while already stepwise
	// with stored steps present: no-op on those two columns, but must not
	// error.
	if err := svc.UpdateAgentDef("proj1", wfID, "patch-roundtrip", &types.AgentDefUpdateRequest{
		PromptMode: &stepwise,
	}); err != nil {
		t.Fatalf("UpdateAgentDef (prompt_mode only, steps omitted): %v", err)
	}
}

// TestUpdateAgentDef_PromptMode_StepsOmittedStepwiseNoStoredStepsRejected
// verifies the (d) branch specifically via req.Steps==nil while nothing is
// stored yet (covered above by FullToStepwiseWithoutStepsRejected); this
// verifies invalid step content on PATCH is rejected the same as on create.
func TestUpdateAgentDef_PromptMode_InvalidStepContentRejected(t *testing.T) {
	t.Parallel()
	_, svc, wfID := setupAgentDefTestEnv(t, nil)
	if _, err := svc.CreateAgentDef("proj1", wfID, &types.AgentDefCreateRequest{
		ID: "patch-invalid-steps", Prompt: "do work",
	}); err != nil {
		t.Fatalf("create: %v", err)
	}

	badSteps := []model.StepDefinition{{StepID: "Bad Slug", Title: "t", Instruction: "i"}}
	stepwise := PromptModeStepwise
	err := svc.UpdateAgentDef("proj1", wfID, "patch-invalid-steps", &types.AgentDefUpdateRequest{
		PromptMode: &stepwise, Steps: &badSteps,
	})
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !errors.Is(err, ErrValidation) {
		t.Errorf("error = %v, want errors.Is(err, ErrValidation)", err)
	}
}

// TestUpdateAgentDef_PromptMode_InvalidEffectiveModeRejected verifies a
// bogus effective prompt_mode (from a bad req.PromptMode) is rejected.
func TestUpdateAgentDef_PromptMode_InvalidEffectiveModeRejected(t *testing.T) {
	t.Parallel()
	_, svc, wfID := setupAgentDefTestEnv(t, nil)
	if _, err := svc.CreateAgentDef("proj1", wfID, &types.AgentDefCreateRequest{
		ID: "patch-bogus-mode", Prompt: "do work",
	}); err != nil {
		t.Fatalf("create: %v", err)
	}

	bogus := "bogus"
	err := svc.UpdateAgentDef("proj1", wfID, "patch-bogus-mode", &types.AgentDefUpdateRequest{
		PromptMode: &bogus,
	})
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !errors.Is(err, ErrValidation) {
		t.Errorf("error = %v, want errors.Is(err, ErrValidation)", err)
	}
}
