package service

import (
	"encoding/json"
	"regexp"
	"strings"

	"be/internal/model"
)

// Prompt modes for agent_definitions.prompt_mode.
const (
	PromptModeFull     = "full"
	PromptModeStepwise = "stepwise"
)

var stepIDPattern = regexp.MustCompile(`^[a-z0-9][a-z0-9_-]{0,63}$`)

// validatePromptModeAndSteps resolves the incoming mode/steps pair into the
// stored (prompt_mode, steps JSON) values, enforcing the mode<->steps
// coupling: stepwise requires >=1 step, full must carry no steps, and
// execution_mode='script' (no prompt at all) cannot be stepwise.
func validatePromptModeAndSteps(executionMode, mode string, steps *[]model.StepDefinition) (string, *string, error) {
	if mode == "" {
		mode = PromptModeFull
	}
	if mode != PromptModeFull && mode != PromptModeStepwise {
		return "", nil, validationErrorf("invalid prompt_mode: %q", mode)
	}
	if mode == PromptModeStepwise && executionMode == "script" {
		return "", nil, validationErrorf("script mode agents cannot use prompt_mode=stepwise")
	}
	if mode == PromptModeFull {
		if steps != nil {
			return "", nil, validationErrorf("steps require prompt_mode=stepwise")
		}
		return PromptModeFull, nil, nil
	}
	if steps == nil || len(*steps) == 0 {
		return "", nil, validationErrorf("prompt_mode=stepwise requires at least one step")
	}
	if err := validateStepDefinitions(*steps); err != nil {
		return "", nil, err
	}
	b, err := json.Marshal(*steps)
	if err != nil {
		return "", nil, validationErrorf("failed to marshal steps: %v", err)
	}
	stepsJSON := string(b)
	return PromptModeStepwise, &stepsJSON, nil
}

// validateStepDefinitions checks well-formedness of a stepwise steps array.
// Per-value evidence validation (whether a finding's actual value satisfies
// its schema at run time) is owned by service/stepengine, not here.
func validateStepDefinitions(steps []model.StepDefinition) error {
	if len(steps) == 0 {
		return validationErrorf("steps: at least one step is required")
	}
	if len(steps) > 20 {
		return validationErrorf("steps: too many entries (max 20)")
	}
	seen := make(map[string]bool, len(steps))
	for _, step := range steps {
		if !stepIDPattern.MatchString(step.StepID) {
			return validationErrorf("steps: invalid step_id %q (must match ^[a-z0-9][a-z0-9_-]{0,63}$)", step.StepID)
		}
		if seen[step.StepID] {
			return validationErrorf("steps: duplicate step_id %q", step.StepID)
		}
		seen[step.StepID] = true
		if strings.TrimSpace(step.Title) == "" {
			return validationErrorf("steps[%s]: title is required", step.StepID)
		}
		if strings.TrimSpace(step.Instruction) == "" {
			return validationErrorf("steps[%s]: instruction is required", step.StepID)
		}
		if len(step.Instruction) > 16384 {
			return validationErrorf("steps[%s]: instruction exceeds 16384 bytes", step.StepID)
		}
		if err := validateRequiredFindings(step.StepID, step.RequiredFindings); err != nil {
			return err
		}
		if err := validateStepChecks(step.StepID, step.Checks); err != nil {
			return err
		}
		if err := validatePathOverlap(step.StepID, step.PathOverlap); err != nil {
			return err
		}
	}
	return nil
}

func validateRequiredFindings(stepID string, findings []model.RequiredFinding) error {
	if len(findings) > 20 {
		return validationErrorf("steps[%s]: too many required_findings (max 20)", stepID)
	}
	for _, f := range findings {
		if err := validateFindingKey(stepID, "required_findings", f.Key); err != nil {
			return err
		}
		if !model.ValidFindingSchema(f.Schema) {
			return validationErrorf("steps[%s]: invalid required_findings schema %q", stepID, f.Schema)
		}
	}
	return nil
}

// validateFindingKey applies the shared findings-key well-formedness rules
// (non-empty, whitespace-free, <=128 bytes) under a field label used in the
// error message.
func validateFindingKey(stepID, field, key string) error {
	trimmed := strings.TrimSpace(key)
	if trimmed == "" || trimmed != key || strings.ContainsAny(key, " \t\n") {
		return validationErrorf("steps[%s]: %s key must be non-empty and whitespace-free", stepID, field)
	}
	if len(key) > 128 {
		return validationErrorf("steps[%s]: %s key exceeds 128 bytes", stepID, field)
	}
	return nil
}

// validatePathOverlap checks a step's optional cross-key gate: nil is OK
// (no gate); both groups must be non-empty and hold at most 10 well-formed
// keys, with no key appearing in both groups.
func validatePathOverlap(stepID string, overlap *model.PathOverlap) error {
	if overlap == nil {
		return nil
	}
	if len(overlap.Left) == 0 || len(overlap.Right) == 0 {
		return validationErrorf("steps[%s]: path_overlap left and right must both be non-empty", stepID)
	}
	seen := make(map[string]string, len(overlap.Left)+len(overlap.Right))
	for _, side := range []struct {
		name string
		keys []string
	}{{"left", overlap.Left}, {"right", overlap.Right}} {
		if len(side.keys) > 10 {
			return validationErrorf("steps[%s]: path_overlap %s has too many keys (max 10)", stepID, side.name)
		}
		for _, key := range side.keys {
			if err := validateFindingKey(stepID, "path_overlap "+side.name, key); err != nil {
				return err
			}
			if other, ok := seen[key]; ok && other != side.name {
				return validationErrorf("steps[%s]: path_overlap key %q appears in both left and right", stepID, key)
			}
			seen[key] = side.name
		}
	}
	return nil
}

func validateStepChecks(stepID string, checks []string) error {
	if len(checks) > 20 {
		return validationErrorf("steps[%s]: too many checks (max 20)", stepID)
	}
	for _, c := range checks {
		if strings.TrimSpace(c) == "" {
			return validationErrorf("steps[%s]: checks entry is empty or whitespace-only", stepID)
		}
		if len(c) > 1024 {
			return validationErrorf("steps[%s]: checks entry exceeds 1024 bytes", stepID)
		}
	}
	return nil
}
