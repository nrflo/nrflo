package integration

// TestStepwiseFullLoop exercises the stepwise cross-layer wiring end to end:
// the real complete_step builtin (stepengine.Advance) driving the real
// agent_step_cursors row + FindingRepo, WorkflowService.BuildStepCursors'
// read model, and a real WS hub broadcast — none of it mocked. Unit tests
// elsewhere cover Advance/rejection/rotation logic in isolation; this test
// exists to catch bugs at the seams between those layers.

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"be/internal/spawner/apirun"
	"be/internal/spawner/apirun/tools_builtin"
	"be/internal/ws"
)

func completeStep(env *TestEnv, steps *fakeStepSession, sessionID, ticketID, wfiID, nodeID, stepID string, revision int, evidenceKeys []string) (string, bool, error) {
	handler := tools_builtin.StepwiseBuiltins()["complete_step"]
	toolEnv := apirun.ToolEnv{
		Pool:               env.Pool,
		WSHub:              env.Hub,
		Clock:              env.Clock,
		SessionID:          sessionID,
		ProjectID:          env.ProjectID,
		TicketID:           ticketID,
		WorkflowName:       "test",
		WorkflowInstanceID: wfiID,
		NodeID:             nodeID,
		Steps:              steps,
	}
	input, _ := json.Marshal(map[string]interface{}{
		"step_id":  stepID,
		"revision": revision,
		"summary":  "done",
		"evidence": map[string]interface{}{"finding_keys": evidenceKeys},
	})
	return handler.Invoke(context.Background(), toolEnv, input)
}

func TestStepwiseFullLoop(t *testing.T) {
	env := NewTestEnv(t)

	ticketID := "SW-FULL-1"
	env.CreateTicket(t, ticketID, "Stepwise full loop")
	env.InitWorkflow(t, ticketID)
	wfiID := env.GetWorkflowInstanceID(t, ticketID, "test")
	const nodeID = "stepper"

	def := createStepwiseAgentDef(t, env, nodeID, 2, stepwiseTwoSteps())
	snapshotCursor(t, env, def, wfiID, nodeID)

	// Read model before any advance: current step s1, not done.
	before := env.WorkflowSvc.BuildStepCursors(wfiID)
	prog := before[nodeID]
	if prog == nil {
		t.Fatalf("BuildStepCursors: no progress for node %q", nodeID)
	}
	if prog.Done || prog.CurrentStepID != "s1" || prog.Total != 2 {
		t.Fatalf("initial progress = %+v, want current_step_id=s1 total=2 done=false", prog)
	}

	sessionID := "sw-sess-1"
	env.InsertAgentSession(t, sessionID, ticketID, wfiID, nodeID, nodeID, "")

	_, ch := env.NewWSClient(t, "ws-stepwise", ticketID)
	steps := &fakeStepSession{}

	// --- Leg 1: complete s1 -> advances to s2 ---
	seedSummaryFinding(t, env, sessionID, "did step one")
	out, isErr, err := completeStep(env, steps, sessionID, ticketID, wfiID, nodeID, "s1", 1, []string{"summary"})
	if err != nil || isErr {
		t.Fatalf("complete_step(s1) failed: out=%q isErr=%v err=%v", out, isErr, err)
	}
	var payload map[string]interface{}
	if uerr := json.Unmarshal([]byte(out), &payload); uerr != nil {
		t.Fatalf("unmarshal complete_step(s1) output %q: %v", out, uerr)
	}
	if payload["step_id"] != "s2" {
		t.Errorf("complete_step(s1) next step_id = %v, want s2", payload["step_id"])
	}
	if len(steps.boundaryCalls) != 1 || steps.boundaryCalls[0] != sessionID {
		t.Errorf("NoteStepBoundary calls = %v, want one call for %s", steps.boundaryCalls, sessionID)
	}

	event := expectEvent(t, ch, ws.EventStepAdvanced, 2*time.Second)
	if event.Data["node_id"] != nodeID {
		t.Errorf("step.advanced node_id = %v, want %q", event.Data["node_id"], nodeID)
	}
	if event.Data["step_id"] != "s2" {
		t.Errorf("step.advanced step_id = %v, want s2", event.Data["step_id"])
	}
	if event.Data["step_index"] != float64(1) {
		t.Errorf("step.advanced step_index = %v, want 1", event.Data["step_index"])
	}
	if event.Data["total"] != float64(2) {
		t.Errorf("step.advanced total = %v, want 2", event.Data["total"])
	}
	if event.Data["rejected_count"] != float64(0) {
		t.Errorf("step.advanced rejected_count = %v, want 0", event.Data["rejected_count"])
	}
	if event.Data["rotated"] != false {
		t.Errorf("step.advanced rotated = %v, want false", event.Data["rotated"])
	}

	mid := env.WorkflowSvc.BuildStepCursors(wfiID)
	midProg := mid[nodeID]
	if midProg.Done || midProg.CurrentStepID != "s2" || midProg.CurrentIndex != 1 {
		t.Fatalf("mid-loop progress = %+v, want current_step_id=s2 current_index=1 done=false", midProg)
	}
	if midProg.Steps[0].Status != "done" {
		t.Errorf("step s1 status = %q, want done", midProg.Steps[0].Status)
	}

	// --- Leg 2: complete s2 -> done ---
	seedSummaryFinding(t, env, sessionID, "did step two")
	out, isErr, err = completeStep(env, steps, sessionID, ticketID, wfiID, nodeID, "s2", 2, []string{"summary"})
	if err != nil || isErr {
		t.Fatalf("complete_step(s2) failed: out=%q isErr=%v err=%v", out, isErr, err)
	}
	if uerr := json.Unmarshal([]byte(out), &payload); uerr != nil {
		t.Fatalf("unmarshal complete_step(s2) output %q: %v", out, uerr)
	}
	if payload["done"] != true {
		t.Errorf("complete_step(s2) done = %v, want true", payload["done"])
	}

	event = expectEvent(t, ch, ws.EventStepAdvanced, 2*time.Second)
	if event.Data["step_id"] != "" {
		t.Errorf("step.advanced (done leg) step_id = %v, want empty", event.Data["step_id"])
	}
	if event.Data["step_index"] != float64(2) || event.Data["total"] != float64(2) {
		t.Errorf("step.advanced (done leg) step_index/total = %v/%v, want 2/2", event.Data["step_index"], event.Data["total"])
	}

	after := env.WorkflowSvc.BuildStepCursors(wfiID)
	afterProg := after[nodeID]
	if !afterProg.Done {
		t.Errorf("final progress Done = false, want true (current_index %d >= total %d)", afterProg.CurrentIndex, afterProg.Total)
	}
	if afterProg.CurrentStepID != "" {
		t.Errorf("final progress CurrentStepID = %q, want empty once done", afterProg.CurrentStepID)
	}
	for _, sp := range afterProg.Steps {
		if sp.Status != "done" {
			t.Errorf("final step %s status = %q, want done", sp.StepID, sp.Status)
		}
	}
}

// TestStepwiseFullLoop_RejectionSurfacesOnReadModel exercises a rejected
// complete_step call (missing evidence) end to end: the cursor's rejection
// counter is durable and BuildStepCursors reports rejected_retrying, without
// emitting a step.advanced event (rejections are not an advance).
func TestStepwiseFullLoop_RejectionSurfacesOnReadModel(t *testing.T) {
	env := NewTestEnv(t)

	ticketID := "SW-FULL-2"
	env.CreateTicket(t, ticketID, "Stepwise rejection")
	env.InitWorkflow(t, ticketID)
	wfiID := env.GetWorkflowInstanceID(t, ticketID, "test")
	const nodeID = "stepper"

	def := createStepwiseAgentDef(t, env, nodeID, 2, stepwiseTwoSteps())
	snapshotCursor(t, env, def, wfiID, nodeID)

	sessionID := "sw-sess-2"
	env.InsertAgentSession(t, sessionID, ticketID, wfiID, nodeID, nodeID, "")

	_, ch := env.NewWSClient(t, "ws-stepwise-2", ticketID)
	steps := &fakeStepSession{}

	// No "summary" finding recorded -> missing evidence rejection.
	out, isErr, err := completeStep(env, steps, sessionID, ticketID, wfiID, nodeID, "s1", 1, []string{"summary"})
	if err != nil {
		t.Fatalf("complete_step(s1) returned Go error: %v", err)
	}
	if !isErr {
		t.Fatalf("complete_step(s1) isErr = false, want true (missing evidence); out=%q", out)
	}

	event := expectEvent(t, ch, ws.EventStepAdvanced, 2*time.Second)
	if event.Data["rejected_count"] != float64(1) {
		t.Errorf("step.advanced (rejection leg) rejected_count = %v, want 1", event.Data["rejected_count"])
	}
	if event.Data["step_id"] != "s1" {
		t.Errorf("step.advanced (rejection leg) step_id = %v, want s1 (cursor did not move)", event.Data["step_id"])
	}

	progress := env.WorkflowSvc.BuildStepCursors(wfiID)
	prog := progress[nodeID]
	if prog == nil {
		t.Fatalf("BuildStepCursors: no progress for node %q", nodeID)
	}
	if prog.CurrentStepID != "s1" || prog.Done {
		t.Fatalf("progress after rejection = %+v, want current_step_id=s1 done=false", prog)
	}
	if prog.Steps[0].Status != "rejected_retrying" {
		t.Errorf("step s1 status after rejection = %q, want rejected_retrying", prog.Steps[0].Status)
	}
	if prog.Steps[0].Rejections != 1 {
		t.Errorf("step s1 rejections = %d, want 1", prog.Steps[0].Rejections)
	}

	if len(steps.boundaryCalls) != 0 {
		t.Errorf("NoteStepBoundary must not fire on a rejection, got %v", steps.boundaryCalls)
	}
}
