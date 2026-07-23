package tools_builtin

import (
	"context"
	"encoding/json"
	"testing"

	"be/internal/model"
)

// completeStepTwoSteps is a canonical two-step sequence: each step requires a
// nonempty_text "summary" finding, both allow rotation.
func completeStepTwoSteps() []model.StepDefinition {
	return []model.StepDefinition{
		{StepID: "s1", Title: "Step One", Instruction: "do step one",
			RequiredFindings: []model.RequiredFinding{{Key: "summary", Schema: model.FindingSchemaNonemptyText}},
			RotationAllowed:  true},
		{StepID: "s2", Title: "Step Two", Instruction: "do step two",
			RequiredFindings: []model.RequiredFinding{{Key: "summary", Schema: model.FindingSchemaNonemptyText}},
			RotationAllowed:  true},
	}
}

// seedSummaryFinding writes a nonempty_text "summary" finding for the seeded
// session via the real findings_add handler.
func seedSummaryFinding(t *testing.T, env *builtinTestEnv, value string) {
	t.Helper()
	b, err := json.Marshal(value)
	if err != nil {
		t.Fatalf("marshal value: %v", err)
	}
	out, isErr, err := invoke(t, env.env, "findings_add", `{"key":"summary","value":`+string(b)+`}`)
	if err != nil || isErr {
		t.Fatalf("seed finding: out=%q isErr=%v err=%v", out, isErr, err)
	}
}

func TestCompleteStep_HappyPath_ReturnsNextStepAndAdvancesCursor(t *testing.T) {
	env := newBuiltinTestEnv(t)
	env.seedStepCursor(t, completeStepTwoSteps(), 0, 1, nil)
	seedSummaryFinding(t, env, "did step one")

	out, isErr, err := completeStepHandler{}.Invoke(context.Background(), env.env, json.RawMessage(`{"step_id":"s1","revision":1,"summary":"done","evidence":{"finding_keys":["summary"]}}`))
	if err != nil {
		t.Fatalf("Invoke err: %v", err)
	}
	if isErr {
		t.Fatalf("isErr=true, output=%q", out)
	}

	var payload map[string]interface{}
	if uerr := json.Unmarshal([]byte(out), &payload); uerr != nil {
		t.Fatalf("unmarshal output %q: %v", out, uerr)
	}
	if payload["step_id"] != "s2" {
		t.Errorf("step_id = %v, want s2", payload["step_id"])
	}
	if payload["revision"] != float64(2) {
		t.Errorf("revision = %v, want 2", payload["revision"])
	}
	if payload["step_index"] != float64(2) {
		t.Errorf("step_index = %v, want 2", payload["step_index"])
	}
	if payload["total"] != float64(2) {
		t.Errorf("total = %v, want 2", payload["total"])
	}
	if payload["title"] != "Step Two" {
		t.Errorf("title = %v, want Step Two", payload["title"])
	}
	if payload["instruction"] != "do step two" {
		t.Errorf("instruction = %v, want %q", payload["instruction"], "do step two")
	}

	revision, currentIndex, completed, _ := env.readCursor(t)
	if revision != 2 || currentIndex != 1 {
		t.Errorf("cursor after advance = revision=%d current_index=%d, want 2/1", revision, currentIndex)
	}
	if completed == "[]" {
		t.Errorf("completed still empty after advance")
	}
}

// TestCompleteStep_NilSteps_DegradesToNoRotationNoPanic verifies a nil
// env.Steps (console/tests) is treated as tokens=0/threshold=0 — never
// rotates and never panics on the boundary-stamp call.
func TestCompleteStep_NilSteps_DegradesToNoRotationNoPanic(t *testing.T) {
	env := newBuiltinTestEnv(t)
	env.seedStepCursor(t, completeStepTwoSteps(), 0, 1, nil)
	seedSummaryFinding(t, env, "did step one")

	if env.env.Steps != nil {
		t.Fatal("test setup: env.Steps must be nil for this case")
	}

	out, isErr, err := completeStepHandler{}.Invoke(context.Background(), env.env, json.RawMessage(`{"step_id":"s1","revision":1,"evidence":{"finding_keys":["summary"]}}`))
	if err != nil {
		t.Fatalf("Invoke err: %v", err)
	}
	if isErr {
		t.Fatalf("isErr=true, output=%q", out)
	}
	var payload map[string]interface{}
	if uerr := json.Unmarshal([]byte(out), &payload); uerr != nil {
		t.Fatalf("unmarshal output %q: %v", out, uerr)
	}
	if payload["step_id"] != "s2" {
		t.Errorf("step_id = %v, want s2 (plain next, no rotate) with nil Steps", payload["step_id"])
	}
}

func TestCompleteStep_LastStep_ReturnsDoneWithoutTerminalSignal(t *testing.T) {
	env := newBuiltinTestEnv(t)
	env.seedStepCursor(t, completeStepTwoSteps(), 1, 2, []model.CompletedStep{{StepID: "s1", CompletedAt: "2026-01-01T00:00:00Z"}})
	seedSummaryFinding(t, env, "did step two")

	out, isErr, err := completeStepHandler{}.Invoke(context.Background(), env.env, json.RawMessage(`{"step_id":"s2","revision":2,"evidence":{"finding_keys":["summary"]}}`))
	if err != nil {
		t.Fatalf("Invoke returned a Go error (must not self-signal completion): %v", err)
	}
	if isErr {
		t.Fatalf("isErr=true, output=%q", out)
	}
	var payload map[string]interface{}
	if uerr := json.Unmarshal([]byte(out), &payload); uerr != nil {
		t.Fatalf("unmarshal output %q: %v", out, uerr)
	}
	if payload["done"] != true {
		t.Errorf("done = %v, want true", payload["done"])
	}
	if payload["instruction"] == "" || payload["instruction"] == nil {
		t.Error("instruction missing on done payload")
	}
}

func TestCompleteStep_MissingPool_ReturnsErrorResult(t *testing.T) {
	env := newBuiltinTestEnv(t)
	badEnv := env.env
	badEnv.Pool = nil

	out, isErr, err := completeStepHandler{}.Invoke(context.Background(), badEnv, json.RawMessage(`{"step_id":"s1","revision":1}`))
	if err != nil {
		t.Fatalf("Invoke err: %v, want nil (service-missing is a result, not a Go error)", err)
	}
	if !isErr {
		t.Fatalf("isErr = false, want true when Pool is nil; output=%q", out)
	}
}

func TestCompleteStep_MissingNodeID_ReturnsErrorResult(t *testing.T) {
	env := newBuiltinTestEnv(t)
	badEnv := env.env
	badEnv.NodeID = ""

	out, isErr, err := completeStepHandler{}.Invoke(context.Background(), badEnv, json.RawMessage(`{"step_id":"s1","revision":1}`))
	if err != nil {
		t.Fatalf("Invoke err: %v", err)
	}
	if !isErr {
		t.Fatalf("isErr = false, want true when NodeID is empty; output=%q", out)
	}
}
