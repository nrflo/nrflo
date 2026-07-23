package tools_builtin

import (
	"context"
	"encoding/json"
	"testing"

	"be/internal/model"
	"be/internal/ws"
)

// lastStepAdvancedEvent returns the last ws.EventStepAdvanced broadcast (and
// its decoded data map), failing the test if none was seen.
func lastStepAdvancedEvent(t *testing.T, env *builtinTestEnv) (*ws.Event, map[string]interface{}) {
	t.Helper()
	for i := len(env.hub.events) - 1; i >= 0; i-- {
		e := env.hub.events[i]
		if e.Type == ws.EventStepAdvanced {
			return e, e.Data
		}
	}
	t.Fatal("no step.advanced event broadcast")
	return nil, nil
}

func countStepAdvancedEvents(env *builtinTestEnv) int {
	n := 0
	for _, e := range env.hub.events {
		if e.Type == ws.EventStepAdvanced {
			n++
		}
	}
	return n
}

// TestCompleteStepEvent_AcceptedAdvanceEmitsOnceWithNewStepFields is case 1:
// step.advanced fires exactly once on a plain accepted advance, carrying the
// NEW current step's id/index and the total step count.
func TestCompleteStepEvent_AcceptedAdvanceEmitsOnceWithNewStepFields(t *testing.T) {
	env := newBuiltinTestEnv(t)
	env.seedStepCursor(t, completeStepTwoSteps(), 0, 1, nil)
	seedSummaryFinding(t, env, "did step one")

	_, isErr, err := completeStepHandler{}.Invoke(context.Background(), env.env, json.RawMessage(`{"step_id":"s1","revision":1,"evidence":{"finding_keys":["summary"]}}`))
	if err != nil || isErr {
		t.Fatalf("Invoke: isErr=%v err=%v", isErr, err)
	}

	if got := countStepAdvancedEvents(env); got != 1 {
		t.Fatalf("step.advanced emitted %d times, want exactly 1", got)
	}
	_, data := lastStepAdvancedEvent(t, env)
	if data["step_id"] != "s2" {
		t.Errorf("data[step_id] = %v, want s2 (the NEW current step)", data["step_id"])
	}
	if data["step_index"] != 1 {
		t.Errorf("data[step_index] = %v, want 1 (cursor's new current_index)", data["step_index"])
	}
	if data["total"] != 2 {
		t.Errorf("data[total] = %v, want 2", data["total"])
	}
	if data["rejected_count"] != 0 {
		t.Errorf("data[rejected_count] = %v, want 0", data["rejected_count"])
	}
	if data["rotated"] != false {
		t.Errorf("data[rotated] = %v, want false", data["rotated"])
	}
	if data["workflow_instance_id"] != testWFIID {
		t.Errorf("data[workflow_instance_id] = %v, want %q", data["workflow_instance_id"], testWFIID)
	}
	if data["node_id"] != testAgentType {
		t.Errorf("data[node_id] = %v, want %q", data["node_id"], testAgentType)
	}
}

// TestCompleteStepEvent_DoneLegCarriesTotalAndEmptyStepID is case 2:
// completing the final step emits step.advanced with step_index==total and
// step_id=="".
func TestCompleteStepEvent_DoneLegCarriesTotalAndEmptyStepID(t *testing.T) {
	env := newBuiltinTestEnv(t)
	env.seedStepCursor(t, completeStepTwoSteps(), 1, 2, []model.CompletedStep{{StepID: "s1", CompletedAt: "2026-01-01T00:00:00Z"}})
	seedSummaryFinding(t, env, "did step two")

	_, isErr, err := completeStepHandler{}.Invoke(context.Background(), env.env, json.RawMessage(`{"step_id":"s2","revision":2,"evidence":{"finding_keys":["summary"]}}`))
	if err != nil || isErr {
		t.Fatalf("Invoke: isErr=%v err=%v", isErr, err)
	}

	if got := countStepAdvancedEvents(env); got != 1 {
		t.Fatalf("step.advanced emitted %d times, want exactly 1", got)
	}
	_, data := lastStepAdvancedEvent(t, env)
	if data["step_id"] != "" {
		t.Errorf("data[step_id] = %v, want empty on done", data["step_id"])
	}
	if data["step_index"] != 2 || data["total"] != 2 {
		t.Errorf("data[step_index]/data[total] = %v/%v, want 2/2 (done: step_index==total)", data["step_index"], data["total"])
	}
}

// TestCompleteStepEvent_RotateLegCarriesRotatedTrue is case 3: the rotate leg
// emits step.advanced with rotated==true, and RequestStepRotation still fires.
func TestCompleteStepEvent_RotateLegCarriesRotatedTrue(t *testing.T) {
	env := newBuiltinTestEnv(t)
	env.seedStepCursor(t, completeStepTwoSteps(), 0, 1, nil)
	seedSummaryFinding(t, env, "did step one")

	fake := &fakeStepSession{contextTokens: 300000, thresholdTokens: 250000}
	env.env.Steps = fake

	_, isErr, err := completeStepHandler{}.Invoke(context.Background(), env.env, json.RawMessage(`{"step_id":"s1","revision":1,"evidence":{"finding_keys":["summary"]}}`))
	if err != nil || isErr {
		t.Fatalf("Invoke: isErr=%v err=%v", isErr, err)
	}

	if got := countStepAdvancedEvents(env); got != 1 {
		t.Fatalf("step.advanced emitted %d times, want exactly 1", got)
	}
	_, data := lastStepAdvancedEvent(t, env)
	if data["rotated"] != true {
		t.Errorf("data[rotated] = %v, want true", data["rotated"])
	}
	if len(fake.rotationRequests) != 1 {
		t.Errorf("rotationRequests = %v, want exactly 1 (rotate leg still requests rotation)", fake.rotationRequests)
	}
}

// TestCompleteStepEvent_CountingRejectionCarriesIncrementedRejectedCount is
// case 4 (first half): a rejection that counts toward the evidence cap
// (missing_evidence) emits step.advanced with the incremented rejected_count.
func TestCompleteStepEvent_CountingRejectionCarriesIncrementedRejectedCount(t *testing.T) {
	env := newBuiltinTestEnv(t)
	env.seedStepCursor(t, completeStepTwoSteps(), 0, 1, nil)
	// No "summary" finding seeded -> missing_evidence rejection, counts toward the cap.

	_, isErr, err := completeStepHandler{}.Invoke(context.Background(), env.env, json.RawMessage(`{"step_id":"s1","revision":1}`))
	if err != nil {
		t.Fatalf("Invoke err: %v", err)
	}
	if !isErr {
		t.Fatal("isErr = false, want true (missing_evidence rejection)")
	}

	if got := countStepAdvancedEvents(env); got != 1 {
		t.Fatalf("step.advanced emitted %d times, want exactly 1 (counting rejection)", got)
	}
	_, data := lastStepAdvancedEvent(t, env)
	if data["rejected_count"] != 1 {
		t.Errorf("data[rejected_count] = %v, want 1", data["rejected_count"])
	}
	if data["step_id"] != "s1" {
		t.Errorf("data[step_id] = %v, want s1 (still the current step)", data["step_id"])
	}
	if data["rotated"] != false {
		t.Errorf("data[rotated] = %v, want false", data["rotated"])
	}
}

// TestCompleteStepEvent_GuardMissRejectionEmitsNoEvent is case 4 (second
// half): stale_revision / step_mismatch guard-miss rejections never count
// toward the evidence cap and must never emit step.advanced.
func TestCompleteStepEvent_GuardMissRejectionEmitsNoEvent(t *testing.T) {
	cases := []struct {
		name  string
		input string
	}{
		{"stale_revision", `{"step_id":"s1","revision":99}`},
		{"step_mismatch", `{"step_id":"wrong-step","revision":1}`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			env := newBuiltinTestEnv(t)
			env.seedStepCursor(t, completeStepTwoSteps(), 0, 1, nil)

			_, isErr, err := completeStepHandler{}.Invoke(context.Background(), env.env, json.RawMessage(tc.input))
			if err != nil {
				t.Fatalf("Invoke err: %v", err)
			}
			if !isErr {
				t.Fatal("isErr = false, want true (guard-miss rejection)")
			}
			if got := countStepAdvancedEvents(env); got != 0 {
				t.Errorf("step.advanced emitted %d times, want 0 for a guard-miss rejection", got)
			}
		})
	}
}

// TestCompleteStepEvent_EnvelopeCarriesBroadcastCtxFields is case 5: the
// ws.Event envelope (not just the data payload) carries project/ticket/
// workflow/session identity unpacked from ToolEnv via BroadcastCtx.
func TestCompleteStepEvent_EnvelopeCarriesBroadcastCtxFields(t *testing.T) {
	env := newBuiltinTestEnv(t)
	env.seedStepCursor(t, completeStepTwoSteps(), 0, 1, nil)
	seedSummaryFinding(t, env, "did step one")

	_, isErr, err := completeStepHandler{}.Invoke(context.Background(), env.env, json.RawMessage(`{"step_id":"s1","revision":1,"evidence":{"finding_keys":["summary"]}}`))
	if err != nil || isErr {
		t.Fatalf("Invoke: isErr=%v err=%v", isErr, err)
	}

	event, _ := lastStepAdvancedEvent(t, env)
	if event.ProjectID != testProjectID {
		t.Errorf("event.ProjectID = %q, want %q", event.ProjectID, testProjectID)
	}
	if event.TicketID != testTicketID {
		t.Errorf("event.TicketID = %q, want %q", event.TicketID, testTicketID)
	}
	if event.Workflow != testWorkflow {
		t.Errorf("event.Workflow = %q, want %q", event.Workflow, testWorkflow)
	}
	if event.SessionID != testSessionID {
		t.Errorf("event.SessionID = %q, want %q", event.SessionID, testSessionID)
	}
}

// TestCompleteStepEvent_NilWSHubIsNoOp is case 6: a nil env.WSHub must not
// panic and the tool result must be unchanged.
func TestCompleteStepEvent_NilWSHubIsNoOp(t *testing.T) {
	env := newBuiltinTestEnv(t)
	env.seedStepCursor(t, completeStepTwoSteps(), 0, 1, nil)
	seedSummaryFinding(t, env, "did step one")
	env.env.WSHub = nil

	out, isErr, err := completeStepHandler{}.Invoke(context.Background(), env.env, json.RawMessage(`{"step_id":"s1","revision":1,"evidence":{"finding_keys":["summary"]}}`))
	if err != nil {
		t.Fatalf("Invoke err (nil WSHub must not panic): %v", err)
	}
	if isErr {
		t.Fatalf("isErr = true, want false; output=%q", out)
	}
	var payload map[string]interface{}
	if uerr := json.Unmarshal([]byte(out), &payload); uerr != nil {
		t.Fatalf("unmarshal output %q: %v", out, uerr)
	}
	if payload["step_id"] != "s2" {
		t.Errorf("step_id = %v, want s2 (result unchanged by nil WSHub)", payload["step_id"])
	}
}
