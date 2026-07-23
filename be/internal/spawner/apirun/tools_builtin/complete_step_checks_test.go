package tools_builtin

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"be/internal/model"
	"be/internal/service"
	"be/internal/spawner/apirun"
)

// completeStepStepsWithChecks mirrors completeStepTwoSteps but adds a
// `checks` command to step one so the checks-wiring path is exercised.
func completeStepStepsWithChecks(cmds []string) []model.StepDefinition {
	steps := completeStepTwoSteps()
	steps[0].Checks = cmds
	return steps
}

// TestCompleteStep_ChecksFail_RejectsWithoutAdvancingCursor is case 15: a
// scripted check failure surfaces as a check_failed rejection carrying the
// tail, and the cursor/rejections counter reflect exactly one rejection.
func TestCompleteStep_ChecksFail_RejectsWithoutAdvancingCursor(t *testing.T) {
	env := newBuiltinTestEnv(t)
	env.seedStepCursor(t, completeStepStepsWithChecks([]string{"make test"}), 0, 1, nil)
	seedSummaryFinding(t, env, "did step one")

	fake := &fakeStepSession{checksConfigured: true, checksFailedIdx: 0, checksExitCode: 1, checksOutputTail: "FAILURE TAIL"}
	env.env.Steps = fake

	out, isErr, err := completeStepHandler{}.Invoke(context.Background(), env.env, json.RawMessage(`{"step_id":"s1","revision":1,"evidence":{"finding_keys":["summary"]}}`))
	if err != nil {
		t.Fatalf("Invoke err: %v", err)
	}
	if !isErr {
		t.Fatalf("isErr = false, want true (check_failed rejection); output=%q", out)
	}
	if !strings.Contains(out, "check failed") || !strings.Contains(out, "FAILURE TAIL") {
		t.Errorf("rejection message = %q, want it to carry the check failure + tail", out)
	}
	if len(fake.checksCmds) != 1 || fake.checksCmds[0] != "make test" {
		t.Errorf("checksCmds = %v, want [\"make test\"]", fake.checksCmds)
	}

	revision, currentIndex, _, rejections := env.readCursor(t)
	if revision != 1 || currentIndex != 0 {
		t.Errorf("cursor mutated by a check_failed rejection: revision=%d current_index=%d", revision, currentIndex)
	}
	var counts map[string]int
	if uerr := json.Unmarshal([]byte(rejections), &counts); uerr != nil {
		t.Fatalf("unmarshal rejections %q: %v", rejections, uerr)
	}
	if counts["s1"] != 1 {
		t.Errorf("rejections[s1] = %d, want 1 (check_failed counts toward the evidence cap)", counts["s1"])
	}
}

// TestCompleteStep_ChecksPass_AdvancesAndRunsSnapshotCommandsOnce is case 16:
// a scripted pass advances the cursor and the checks executor is invoked
// exactly once with the snapshot's commands (not the live def's).
func TestCompleteStep_ChecksPass_AdvancesAndRunsSnapshotCommandsOnce(t *testing.T) {
	env := newBuiltinTestEnv(t)
	env.seedStepCursor(t, completeStepStepsWithChecks([]string{"make test", "make lint"}), 0, 1, nil)
	seedSummaryFinding(t, env, "did step one")

	fake := &fakeStepSession{checksConfigured: true, checksFailedIdx: -1}
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
		t.Errorf("step_id = %v, want s2 (advance on checks pass)", payload["step_id"])
	}
	if len(fake.checksCmds) != 2 || fake.checksCmds[0] != "make test" || fake.checksCmds[1] != "make lint" {
		t.Errorf("checksCmds = %v, want the snapshot's [\"make test\",\"make lint\"]", fake.checksCmds)
	}

	revision, currentIndex, _, _ := env.readCursor(t)
	if revision != 2 || currentIndex != 1 {
		t.Errorf("cursor after advance = revision=%d current_index=%d, want 2/1", revision, currentIndex)
	}
}

// TestCompleteStep_NilSteps_ChecksSkippedStepStillAdvances is case 17: a nil
// env.Steps must not be passed to Advance as a typed-nil CheckRunner (which
// would defeat its `checks != nil` skip) — checks are skipped and the step
// still advances normally.
func TestCompleteStep_NilSteps_ChecksSkippedStepStillAdvances(t *testing.T) {
	env := newBuiltinTestEnv(t)
	env.seedStepCursor(t, completeStepStepsWithChecks([]string{"make test"}), 0, 1, nil)
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
		t.Errorf("step_id = %v, want s2 (checks skipped, step advances) with nil env.Steps", payload["step_id"])
	}
}

// TestCompleteStep_RepeatedCheckFailuresReachRejectionCap is case 18:
// repeated check_failed rejections reach service.StepRejectionCap and
// force-fail the session with step_evidence_exhausted, exercised through
// the checks path (P4's cap enforcement is otherwise only reached via
// missing/invalid evidence).
func TestCompleteStep_RepeatedCheckFailuresReachRejectionCap(t *testing.T) {
	env := newBuiltinTestEnv(t)
	env.seedStepCursor(t, completeStepStepsWithChecks([]string{"make test"}), 0, 1, nil)
	seedSummaryFinding(t, env, "did step one")
	if err := env.pool.SetProjectConfig(testProjectID, service.StepRejectionCapKey, "2"); err != nil {
		t.Fatalf("SetProjectConfig: %v", err)
	}

	fake := &fakeStepSession{checksConfigured: true, checksFailedIdx: 0, checksExitCode: 1, checksOutputTail: "boom"}
	env.env.Steps = fake

	out1, isErr1, err1 := completeStepHandler{}.Invoke(context.Background(), env.env, json.RawMessage(`{"step_id":"s1","revision":1,"evidence":{"finding_keys":["summary"]}}`))
	if err1 != nil {
		t.Fatalf("first Invoke err: %v", err1)
	}
	if !isErr1 {
		t.Fatalf("first call isErr = false, want true; output=%q", out1)
	}

	out2, isErr2, err2 := completeStepHandler{}.Invoke(context.Background(), env.env, json.RawMessage(`{"step_id":"s1","revision":1,"evidence":{"finding_keys":["summary"]}}`))
	if isErr2 {
		t.Errorf("second call isErr = true, want false (TerminalSignal path); output=%q", out2)
	}
	if err2 == nil {
		t.Fatal("second call error = nil, want a FAIL TerminalSignal")
	}
	var term apirun.TerminalSignal
	if !errors.As(err2, &term) {
		t.Fatalf("second call error = %v, want errors.As to unwrap a TerminalSignal", err2)
	}
	if term.Status != "FAIL" {
		t.Errorf("TerminalSignal.Status = %q, want FAIL", term.Status)
	}
	if got := env.readSessionResultReason(t); got != service.ResultReasonStepEvidenceExhausted {
		t.Errorf("session result_reason = %q, want %q", got, service.ResultReasonStepEvidenceExhausted)
	}
}
