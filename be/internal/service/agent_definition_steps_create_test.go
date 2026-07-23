package service

// Tests for CreateAgentDef's prompt_mode/steps validation (P1's "STEP JSON
// RULES" list) — see agent_definition_steps.go.

import (
	"errors"
	"strings"
	"testing"

	"be/internal/model"
	"be/internal/types"
)

func validStep(id string) model.StepDefinition {
	return model.StepDefinition{
		StepID:      id,
		Title:       "Title for " + id,
		Instruction: "Do the thing for " + id,
	}
}

func TestCreateAgentDef_PromptMode_HappyPath(t *testing.T) {
	t.Parallel()
	_, svc, wfID := setupAgentDefTestEnv(t, nil)

	t.Run("full mode with no steps", func(t *testing.T) {
		def, err := svc.CreateAgentDef("proj1", wfID, &types.AgentDefCreateRequest{
			ID:     "step-full",
			Prompt: "do work",
		})
		if err != nil {
			t.Fatalf("CreateAgentDef: %v", err)
		}
		if def.PromptMode != PromptModeFull {
			t.Errorf("PromptMode = %q, want full", def.PromptMode)
		}
		if def.Steps != nil {
			t.Errorf("Steps = %v, want nil", def.Steps)
		}
	})

	t.Run("stepwise mode with canonical JSON round-trip", func(t *testing.T) {
		steps := []model.StepDefinition{
			{
				StepID:      "gather-evidence",
				Title:       "Gather Evidence",
				Instruction: "Collect the required findings.",
				RequiredFindings: []model.RequiredFinding{
					{Key: "evidence_key", Schema: model.FindingSchemaNonemptyText},
				},
				Checks:          []string{"make test-pkg PKG=repo"},
				RotationAllowed: true,
			},
			validStep("finalize"),
		}
		def, err := svc.CreateAgentDef("proj1", wfID, &types.AgentDefCreateRequest{
			ID:         "step-stepwise",
			Prompt:     "do work",
			PromptMode: PromptModeStepwise,
			Steps:      &steps,
		})
		if err != nil {
			t.Fatalf("CreateAgentDef: %v", err)
		}
		if def.PromptMode != PromptModeStepwise {
			t.Errorf("PromptMode = %q, want stepwise", def.PromptMode)
		}
		if def.Steps == nil {
			t.Fatal("Steps = nil, want canonical JSON")
		}
		if !strings.Contains(*def.Steps, "gather-evidence") || !strings.Contains(*def.Steps, "finalize") {
			t.Errorf("Steps = %q, want it to contain both step ids", *def.Steps)
		}
	})
}

func TestCreateAgentDef_PromptMode_RejectionMatrix(t *testing.T) {
	t.Parallel()
	_, svc, wfID := setupAgentDefTestEnv(t, nil)

	tooManySteps := make([]model.StepDefinition, 21)
	for i := range tooManySteps {
		tooManySteps[i] = validStep("step-" + string(rune('a'+i)))
	}

	longInstruction := strings.Repeat("x", 16385)
	longKey := strings.Repeat("k", 129)
	longCheck := strings.Repeat("c", 1025)

	tooManyFindings := make([]model.RequiredFinding, 21)
	for i := range tooManyFindings {
		tooManyFindings[i] = model.RequiredFinding{Key: "k" + string(rune('a'+i)), Schema: model.FindingSchemaNonemptyText}
	}
	tooManyChecks := make([]string, 21)
	for i := range tooManyChecks {
		tooManyChecks[i] = "check " + string(rune('a'+i))
	}

	cases := []struct {
		name  string
		mode  string
		steps []model.StepDefinition
	}{
		{"bogus prompt_mode", "bogus", nil},
		{"stepwise requires steps", PromptModeStepwise, []model.StepDefinition{}},
		{"too many steps", PromptModeStepwise, tooManySteps},
		{"non-slug step_id", PromptModeStepwise, []model.StepDefinition{{StepID: "Not_A_Slug!", Title: "t", Instruction: "i"}}},
		{"uppercase step_id rejected", PromptModeStepwise, []model.StepDefinition{{StepID: "Step1", Title: "t", Instruction: "i"}}},
		{"duplicate step_id", PromptModeStepwise, []model.StepDefinition{validStep("dup"), validStep("dup")}},
		{"empty title", PromptModeStepwise, []model.StepDefinition{{StepID: "s1", Title: "  ", Instruction: "i"}}},
		{"empty instruction", PromptModeStepwise, []model.StepDefinition{{StepID: "s1", Title: "t", Instruction: " "}}},
		{"instruction too long", PromptModeStepwise, []model.StepDefinition{{StepID: "s1", Title: "t", Instruction: longInstruction}}},
		{"too many required_findings", PromptModeStepwise, []model.StepDefinition{{StepID: "s1", Title: "t", Instruction: "i", RequiredFindings: tooManyFindings}}},
		{"required_findings empty key", PromptModeStepwise, []model.StepDefinition{{StepID: "s1", Title: "t", Instruction: "i", RequiredFindings: []model.RequiredFinding{{Key: "", Schema: model.FindingSchemaNonemptyText}}}}},
		{"required_findings whitespace key", PromptModeStepwise, []model.StepDefinition{{StepID: "s1", Title: "t", Instruction: "i", RequiredFindings: []model.RequiredFinding{{Key: "has space", Schema: model.FindingSchemaNonemptyText}}}}},
		{"required_findings key too long", PromptModeStepwise, []model.StepDefinition{{StepID: "s1", Title: "t", Instruction: "i", RequiredFindings: []model.RequiredFinding{{Key: longKey, Schema: model.FindingSchemaNonemptyText}}}}},
		{"required_findings invalid schema", PromptModeStepwise, []model.StepDefinition{{StepID: "s1", Title: "t", Instruction: "i", RequiredFindings: []model.RequiredFinding{{Key: "k1", Schema: "not_a_real_schema"}}}}},
		{"too many checks", PromptModeStepwise, []model.StepDefinition{{StepID: "s1", Title: "t", Instruction: "i", Checks: tooManyChecks}}},
		{"blank check entry", PromptModeStepwise, []model.StepDefinition{{StepID: "s1", Title: "t", Instruction: "i", Checks: []string{"  "}}}},
		{"check entry too long", PromptModeStepwise, []model.StepDefinition{{StepID: "s1", Title: "t", Instruction: "i", Checks: []string{longCheck}}}},
	}

	for i, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			req := &types.AgentDefCreateRequest{
				ID:         "steps-reject-" + string(rune('a'+i)),
				Prompt:     "do work",
				PromptMode: tc.mode,
			}
			if tc.steps != nil {
				req.Steps = &tc.steps
			}
			_, err := svc.CreateAgentDef("proj1", wfID, req)
			if err == nil {
				t.Fatalf("CreateAgentDef(%s): expected error, got nil", tc.name)
			}
			if !errors.Is(err, ErrValidation) {
				t.Errorf("CreateAgentDef(%s): error = %v, want errors.Is(err, ErrValidation)", tc.name, err)
			}
		})
	}
}

// TestCreateAgentDef_PromptMode_FullWithStepsRejected verifies full mode
// combined with a non-nil steps array is rejected.
func TestCreateAgentDef_PromptMode_FullWithStepsRejected(t *testing.T) {
	t.Parallel()
	_, svc, wfID := setupAgentDefTestEnv(t, nil)
	steps := []model.StepDefinition{validStep("s1")}
	_, err := svc.CreateAgentDef("proj1", wfID, &types.AgentDefCreateRequest{
		ID:         "full-with-steps",
		Prompt:     "do work",
		PromptMode: PromptModeFull,
		Steps:      &steps,
	})
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !errors.Is(err, ErrValidation) {
		t.Errorf("error = %v, want errors.Is(err, ErrValidation)", err)
	}
}

// TestCreateAgentDef_PromptMode_ScriptModeCannotBeStepwise verifies
// execution_mode='script' agents cannot use prompt_mode=stepwise.
func TestCreateAgentDef_PromptMode_ScriptModeCannotBeStepwise(t *testing.T) {
	t.Parallel()
	_, svc, wfID := setupAgentDefTestEnv(t, nil)
	steps := []model.StepDefinition{validStep("s1")}
	_, err := svc.CreateAgentDef("proj1", wfID, &types.AgentDefCreateRequest{
		ID:            "script-stepwise",
		ExecutionMode: "script",
		PromptMode:    PromptModeStepwise,
		Steps:         &steps,
		PythonScriptID: func() *string {
			v := "not-checked-before-prompt-mode"
			return &v
		}(),
	})
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !errors.Is(err, ErrValidation) {
		t.Errorf("error = %v, want errors.Is(err, ErrValidation)", err)
	}
}
