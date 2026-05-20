package orchestrator

import (
	"context"
	"testing"
	"time"

	"be/internal/ws"
)

// TestRunFinalize_NoOp verifies that runFinalize is a no-op when no slot is configured
// and also when only the wrong-outcome slot is configured.
func TestRunFinalize_NoOp(t *testing.T) {
	cases := []struct {
		name    string
		req     func(projectID string) RunRequest
		outcome finalizeOutcome
	}{
		{
			name:    "both_slots_empty",
			req:     func(p string) RunRequest { return RunRequest{ProjectID: p, WorkflowName: "test"} },
			outcome: outcomeSuccess,
		},
		{
			name: "success_outcome_only_failure_slot_set",
			req: func(p string) RunRequest {
				return RunRequest{ProjectID: p, WorkflowName: "test", FinalizeFailureCommand: "exit 0"}
			},
			outcome: outcomeSuccess,
		},
		{
			name: "failure_outcome_only_success_slot_set",
			req: func(p string) RunRequest {
				return RunRequest{ProjectID: p, WorkflowName: "test", FinalizeSuccessCommand: "exit 0"}
			},
			outcome: outcomeFailure,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			env := newTestEnv(t)
			env.createTicket(t, "FN-1", "noop")
			wfiID := env.initWorkflow(t, "FN-1")

			req := tc.req(env.project)
			req.TicketID = "FN-1"

			env.orch.runFinalize(context.Background(), wfiID, req, tc.outcome, "detail")

			findings := getWFIFindings(t, env, wfiID)
			if _, ok := findings["_finalize"]; ok {
				t.Errorf("_finalize finding present for no-op case %q, want absent", tc.name)
			}
		})
	}
}

// TestRunFinalize_SuccessCommand verifies the success command path:
// exit 0 → EventWorkflowFinalizeSucceeded + _finalize finding with required keys, no RecordError.
func TestRunFinalize_SuccessCommand(t *testing.T) {
	env := newTestEnv(t)
	mock := &mockErrorRecorder{}
	env.orch.errorSvc = mock

	env.createTicket(t, "FS-1", "success cmd")
	wfiID := env.initWorkflow(t, "FS-1")
	ch := env.subscribeWSClient(t, "ws-sc", "FS-1")

	req := RunRequest{
		ProjectID:              env.project,
		TicketID:               "FS-1",
		WorkflowName:           "test",
		FinalizeSuccessCommand: "exit 0",
	}
	env.orch.runFinalize(context.Background(), wfiID, req, outcomeSuccess, "done")

	ev := expectEvent(t, ch, ws.EventWorkflowFinalizeSucceeded, 2*time.Second)
	if ev.Data["instance_id"] != wfiID {
		t.Errorf("event instance_id = %v, want %v", ev.Data["instance_id"], wfiID)
	}

	findings := getWFIFindings(t, env, wfiID)
	raw, ok := findings["_finalize"]
	if !ok {
		t.Fatalf("_finalize finding absent after success command")
	}
	m, _ := raw.(map[string]interface{})
	for _, key := range []string{"slot", "kind", "target", "exit_code", "status", "output_tail", "timestamp"} {
		if _, ok := m[key]; !ok {
			t.Errorf("_finalize finding missing key %q", key)
		}
	}
	if m["status"] != "ok" {
		t.Errorf("_finalize status = %v, want ok", m["status"])
	}
	if m["slot"] != "success" {
		t.Errorf("_finalize slot = %v, want success", m["slot"])
	}
	if got := mock.callCount(); got != 0 {
		t.Errorf("RecordError called %d times on success, want 0", got)
	}
}

// TestRunFinalize_FailureCommand verifies the failure command path:
// exit 1 → EventWorkflowFinalizeFailed + RecordError(type=workflow) + _finalize finding.
// Also verifies that workflow_instances.status is not changed by runFinalize.
func TestRunFinalize_FailureCommand(t *testing.T) {
	env := newTestEnv(t)
	mock := &mockErrorRecorder{}
	env.orch.errorSvc = mock

	env.createTicket(t, "FF-1", "failure cmd")
	wfiID := env.initWorkflow(t, "FF-1")
	statusBefore := env.getWorkflowInstance(t, wfiID).Status
	ch := env.subscribeWSClient(t, "ws-fc", "FF-1")

	req := RunRequest{
		ProjectID:              env.project,
		TicketID:               "FF-1",
		WorkflowName:           "test",
		FinalizeFailureCommand: "exit 1",
	}
	env.orch.runFinalize(context.Background(), wfiID, req, outcomeFailure, "agent error")

	expectEvent(t, ch, ws.EventWorkflowFinalizeFailed, 2*time.Second)

	if got := mock.callCount(); got != 1 {
		t.Fatalf("RecordError calls = %d, want 1", got)
	}
	call := mock.getCall(0)
	if call.projectID != env.project {
		t.Errorf("RecordError projectID = %q, want %q", call.projectID, env.project)
	}
	if call.errorType != "workflow" {
		t.Errorf("RecordError errorType = %q, want workflow", call.errorType)
	}
	if call.instanceID != wfiID {
		t.Errorf("RecordError instanceID = %q, want %q", call.instanceID, wfiID)
	}

	findings := getWFIFindings(t, env, wfiID)
	m, ok := findings["_finalize"].(map[string]interface{})
	if !ok {
		t.Fatalf("_finalize finding absent after failure command")
	}
	if m["slot"] != "failure" {
		t.Errorf("_finalize slot = %v, want failure", m["slot"])
	}
	if m["status"] == "ok" {
		t.Errorf("_finalize status = ok after exit 1, want failed")
	}

	statusAfter := env.getWorkflowInstance(t, wfiID).Status
	if statusAfter != statusBefore {
		t.Errorf("workflow status changed from %q to %q by runFinalize, want unchanged", statusBefore, statusAfter)
	}
}

// TestMarkFailed_CancelledSkipsFinalize verifies that markFailed with reason="cancelled"
// writes _failure_reason but does NOT run finalize (no _finalize finding, no finalize WS event).
func TestMarkFailed_CancelledSkipsFinalize(t *testing.T) {
	env := newTestEnv(t)
	env.createTicket(t, "MC-1", "cancelled")
	wfiID := env.initWorkflow(t, "MC-1")

	req := RunRequest{
		ProjectID:              env.project,
		TicketID:               "MC-1",
		WorkflowName:           "test",
		FinalizeFailureCommand: "exit 1",
	}
	env.orch.markFailed(wfiID, req, reasonCancelled)

	findings := getWFIFindings(t, env, wfiID)
	if _, ok := findings["_finalize"]; ok {
		t.Errorf("_finalize finding present after cancelled markFailed, want absent")
	}
	if _, ok := findings["_failure_reason"]; !ok {
		t.Errorf("_failure_reason finding absent after markFailed")
	}
}

// TestMarkFailed_GenuineReasonRunsFinalize verifies that markFailed with a genuine reason
// runs finalize (writes _finalize finding, broadcasts WS event) and writes _failure_reason.
func TestMarkFailed_GenuineReasonRunsFinalize(t *testing.T) {
	env := newTestEnv(t)
	mock := &mockErrorRecorder{}
	env.orch.errorSvc = mock

	env.createTicket(t, "MG-1", "genuine failure")
	wfiID := env.initWorkflow(t, "MG-1")
	ch := env.subscribeWSClient(t, "ws-mg", "MG-1")

	req := RunRequest{
		ProjectID:              env.project,
		TicketID:               "MG-1",
		WorkflowName:           "test",
		FinalizeFailureCommand: "exit 0",
	}
	env.orch.markFailed(wfiID, req, "boom")

	// Finalize command exits 0 → succeeded event
	expectEvent(t, ch, ws.EventWorkflowFinalizeSucceeded, 2*time.Second)

	findings := getWFIFindings(t, env, wfiID)
	if _, ok := findings["_finalize"]; !ok {
		t.Errorf("_finalize finding absent after genuine-reason markFailed with finalize command")
	}
	if _, ok := findings["_failure_reason"]; !ok {
		t.Errorf("_failure_reason finding absent after markFailed")
	}
}

// TestPersistFinalizeFinding tests the persistence helper directly for subprocess-free
// success and failure path assertions.
func TestPersistFinalizeFinding(t *testing.T) {
	cases := []struct {
		name      string
		status    string
		exitCode  int
		wantEvent string
		wantErr   bool
	}{
		{"success", "ok", 0, ws.EventWorkflowFinalizeSucceeded, false},
		{"failure", "failed", 1, ws.EventWorkflowFinalizeFailed, true},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			env := newTestEnv(t)
			mock := &mockErrorRecorder{}
			env.orch.errorSvc = mock

			env.createTicket(t, "PF-1", "persist "+tc.name)
			wfiID := env.initWorkflow(t, "PF-1")
			ch := env.subscribeWSClient(t, "ws-pf-"+tc.name, "PF-1")

			req := RunRequest{
				ProjectID:    env.project,
				TicketID:     "PF-1",
				WorkflowName: "test",
			}
			persistFinalizeFinding(env.orch, env.pool, wfiID, req, tc.name, "command", "exit 0", tc.exitCode, tc.status, "output")

			expectEvent(t, ch, tc.wantEvent, 2*time.Second)

			if tc.wantErr {
				if got := mock.callCount(); got != 1 {
					t.Errorf("RecordError calls = %d, want 1", got)
				}
			} else {
				if got := mock.callCount(); got != 0 {
					t.Errorf("RecordError calls = %d on success, want 0", got)
				}
			}

			findings := getWFIFindings(t, env, wfiID)
			if _, ok := findings["_finalize"]; !ok {
				t.Errorf("_finalize finding absent after persistFinalizeFinding")
			}
		})
	}
}
