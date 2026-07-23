package tools_builtin

import (
	"context"
	"encoding/json"
	"testing"

	"be/internal/model"
)

// TestCompleteStep_Rotate_AdvancesBeforeRequestingRotation verifies the
// cursor is already at the new index in the DB by the time
// RequestStepRotation observes it — Advance commits before the rotate leg
// asks the spawner to rotate — and that exactly one rotation request fires,
// with the "stop working" instruction returned.
func TestCompleteStep_Rotate_AdvancesBeforeRequestingRotation(t *testing.T) {
	env := newBuiltinTestEnv(t)
	env.seedStepCursor(t, completeStepTwoSteps(), 0, 1, nil)
	seedSummaryFinding(t, env, "did step one")

	var sawCurrentIndex int
	fake := &fakeStepSession{contextTokens: 300000, thresholdTokens: 250000}
	fake.onRequestRotation = func() {
		_, sawCurrentIndex, _, _ = env.readCursor(t)
	}
	env.env.Steps = fake

	out, isErr, err := completeStepHandler{}.Invoke(context.Background(), env.env, json.RawMessage(`{"step_id":"s1","revision":1,"evidence":{"finding_keys":["summary"]}}`))
	if err != nil {
		t.Fatalf("Invoke err: %v", err)
	}
	if isErr {
		t.Fatalf("isErr=true, output=%q", out)
	}
	if out == "" {
		t.Fatal("empty output on rotate")
	}

	if len(fake.rotationRequests) != 1 {
		t.Fatalf("rotationRequests = %v, want exactly 1", fake.rotationRequests)
	}
	if fake.rotationRequests[0] != testSessionID {
		t.Errorf("rotation requested for session %q, want %q", fake.rotationRequests[0], testSessionID)
	}
	if sawCurrentIndex != 1 {
		t.Errorf("current_index observed inside RequestStepRotation = %d, want 1 (Advance must commit before rotation is requested)", sawCurrentIndex)
	}
}

// TestCompleteStep_Rotate_FinalStepNeverRotates verifies completing the
// final step never upgrades to rotate even when over threshold — it's Done,
// and RequestStepRotation is never called.
func TestCompleteStep_Rotate_FinalStepNeverRotates(t *testing.T) {
	env := newBuiltinTestEnv(t)
	env.seedStepCursor(t, completeStepTwoSteps(), 1, 2, []model.CompletedStep{{StepID: "s1", CompletedAt: "2026-01-01T00:00:00Z"}})
	seedSummaryFinding(t, env, "did step two")

	fake := &fakeStepSession{contextTokens: 300000, thresholdTokens: 250000}
	env.env.Steps = fake

	out, isErr, err := completeStepHandler{}.Invoke(context.Background(), env.env, json.RawMessage(`{"step_id":"s2","revision":2,"evidence":{"finding_keys":["summary"]}}`))
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
	if payload["done"] != true {
		t.Errorf("done = %v, want true (final step never rotates)", payload["done"])
	}
	if len(fake.rotationRequests) != 0 {
		t.Errorf("rotationRequests = %v, want none on the final step", fake.rotationRequests)
	}
}

// TestCompleteStep_Rotate_RotationNotAllowedYieldsPlainNext verifies a step
// with rotation_allowed=false never upgrades to rotate even over threshold.
func TestCompleteStep_Rotate_RotationNotAllowedYieldsPlainNext(t *testing.T) {
	env := newBuiltinTestEnv(t)
	steps := []model.StepDefinition{
		{StepID: "s1", Title: "Step One", Instruction: "do step one",
			RequiredFindings: []model.RequiredFinding{{Key: "summary", Schema: model.FindingSchemaNonemptyText}},
			RotationAllowed:  false},
		{StepID: "s2", Title: "Step Two", Instruction: "do step two"},
	}
	env.seedStepCursor(t, steps, 0, 1, nil)
	seedSummaryFinding(t, env, "did step one")

	fake := &fakeStepSession{contextTokens: 300000, thresholdTokens: 250000}
	env.env.Steps = fake

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
		t.Errorf("step_id = %v, want s2 (plain next)", payload["step_id"])
	}
	if len(fake.rotationRequests) != 0 {
		t.Errorf("rotationRequests = %v, want none when rotation_allowed=false", fake.rotationRequests)
	}
	if len(fake.boundaryCalls) != 1 {
		t.Errorf("boundaryCalls = %v, want exactly 1 (NoteStepBoundary stamped on plain next)", fake.boundaryCalls)
	}
}
