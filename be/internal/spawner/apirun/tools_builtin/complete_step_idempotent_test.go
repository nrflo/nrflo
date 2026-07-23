package tools_builtin

import (
	"context"
	"encoding/json"
	"testing"
)

// TestCompleteStep_Idempotent_ReplaySameStepRevisionReturnsSamePayload
// verifies invoking complete_step twice with the same (step_id, revision)
// after a successful advance returns the identical next payload the second
// time, without double-advancing the cursor.
func TestCompleteStep_Idempotent_ReplaySameStepRevisionReturnsSamePayload(t *testing.T) {
	env := newBuiltinTestEnv(t)
	env.seedStepCursor(t, completeStepTwoSteps(), 0, 1, nil)
	seedSummaryFinding(t, env, "did step one")

	input := json.RawMessage(`{"step_id":"s1","revision":1,"evidence":{"finding_keys":["summary"]}}`)

	first, isErr1, err1 := completeStepHandler{}.Invoke(context.Background(), env.env, input)
	if err1 != nil || isErr1 {
		t.Fatalf("first Invoke: out=%q isErr=%v err=%v", first, isErr1, err1)
	}

	revisionAfterFirst, indexAfterFirst, completedAfterFirst, _ := env.readCursor(t)

	second, isErr2, err2 := completeStepHandler{}.Invoke(context.Background(), env.env, input)
	if err2 != nil || isErr2 {
		t.Fatalf("second (replay) Invoke: out=%q isErr=%v err=%v", second, isErr2, err2)
	}

	if first != second {
		t.Errorf("replay payload = %q, want identical to first call's %q", second, first)
	}

	revisionAfterSecond, indexAfterSecond, completedAfterSecond, _ := env.readCursor(t)
	if revisionAfterSecond != revisionAfterFirst || indexAfterSecond != indexAfterFirst {
		t.Errorf("replay mutated the cursor: revision=%d/%d index=%d/%d, want unchanged",
			revisionAfterFirst, revisionAfterSecond, indexAfterFirst, indexAfterSecond)
	}
	if completedAfterSecond != completedAfterFirst {
		t.Errorf("replay double-advanced completed: %q -> %q", completedAfterFirst, completedAfterSecond)
	}
}

// TestCompleteStep_Idempotent_RotateOutcomeReplayReturnsSameRotate verifies
// replaying the exact call that produced OutcomeRotate returns the identical
// "stop working" message the second time — stepengine.Advance's isReplay()
// branch reapplies the rotate upgrade off the durable CompletedStep.Rotated
// stamp (replayWithSignals) — WITHOUT re-requesting rotation: renderRotate
// suppresses the side effect on a replay, so the spawner is never asked to
// kill->relaunch twice for one completion.
func TestCompleteStep_Idempotent_RotateOutcomeReplayReturnsSameRotate(t *testing.T) {
	env := newBuiltinTestEnv(t)
	env.seedStepCursor(t, completeStepTwoSteps(), 0, 1, nil)
	seedSummaryFinding(t, env, "did step one")

	fake := &fakeStepSession{contextTokens: 300000, thresholdTokens: 250000}
	env.env.Steps = fake

	input := json.RawMessage(`{"step_id":"s1","revision":1,"evidence":{"finding_keys":["summary"]}}`)

	first, isErr1, err1 := completeStepHandler{}.Invoke(context.Background(), env.env, input)
	if err1 != nil || isErr1 {
		t.Fatalf("first Invoke: out=%q isErr=%v err=%v", first, isErr1, err1)
	}
	if len(fake.rotationRequests) != 1 {
		t.Fatalf("rotationRequests after first call = %v, want exactly 1 (first call rotates)", fake.rotationRequests)
	}

	second, isErr2, err2 := completeStepHandler{}.Invoke(context.Background(), env.env, input)
	if err2 != nil || isErr2 {
		t.Fatalf("second (replay) Invoke: out=%q isErr=%v err=%v", second, isErr2, err2)
	}

	if first != second {
		t.Errorf("replay message = %q, want identical to the original rotate message %q (replay must reapply the rotate upgrade)", second, first)
	}
	if len(fake.rotationRequests) != 1 {
		t.Errorf("rotationRequests after replay = %v, want still 1 (replay must not re-request rotation)", fake.rotationRequests)
	}
}
