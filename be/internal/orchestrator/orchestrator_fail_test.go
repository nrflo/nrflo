package orchestrator

import (
	"context"
	"testing"
	"time"

	"be/internal/model"
	"be/internal/ws"
)

// TestFailWorkflow_TerminalStatus_Error verifies an error is returned for each terminal status.
func TestFailWorkflow_TerminalStatus_Error(t *testing.T) {
	cases := []struct {
		name   string
		status model.WorkflowInstanceStatus
	}{
		{"completed", model.WorkflowInstanceCompleted},
		{"failed", model.WorkflowInstanceFailed},
		{"project_completed", model.WorkflowInstanceProjectCompleted},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			env := newTestEnv(t)
			env.createTicket(t, "FW-T1", "terminal "+tc.name)
			wfiID := env.initWorkflow(t, "FW-T1")
			setWFIStatus(t, env, wfiID, tc.status)

			err := env.orch.FailWorkflow(context.Background(), env.project, "FW-T1", "test", wfiID, "reason")
			if err == nil {
				t.Errorf("FailWorkflow(%s): want error, got nil", tc.name)
			}
		})
	}
}

// TestFailWorkflow_WaitingInstance verifies that a waiting instance is marked failed
// and the failure-finalize slot fires (with status→failed + _failure_reason finding).
func TestFailWorkflow_WaitingInstance(t *testing.T) {
	env := newTestEnv(t)
	env.createTicket(t, "FW-W1", "waiting fail")
	wfiID := env.initWorkflow(t, "FW-W1")
	setWFIStatus(t, env, wfiID, model.WorkflowInstanceWaiting)
	ch := env.subscribeWSClient(t, "ws-fw-w1", "FW-W1")

	// Set finalize_failure_command so finalize fires and broadcasts an event.
	_, err := env.pool.Exec(
		`UPDATE workflows SET finalize_failure_command = 'exit 0' WHERE LOWER(project_id) = LOWER(?) AND id = 'test'`,
		env.project)
	if err != nil {
		t.Fatalf("set finalize_failure_command: %v", err)
	}

	err = env.orch.FailWorkflow(context.Background(), env.project, "FW-W1", "test", wfiID, "manual fail reason")
	if err != nil {
		t.Fatalf("FailWorkflow: %v", err)
	}

	wi := env.getWorkflowInstance(t, wfiID)
	if wi.Status != model.WorkflowInstanceFailed {
		t.Errorf("status = %v, want failed", wi.Status)
	}

	findings := getWFIFindings(t, env, wfiID)
	if _, ok := findings["_failure_reason"]; !ok {
		t.Errorf("_failure_reason finding absent after FailWorkflow on waiting instance")
	}

	// Finalize slot fires (exit 0 → EventWorkflowFinalizeSucceeded).
	expectEvent(t, ch, ws.EventWorkflowFinalizeSucceeded, 2*time.Second)
}

// TestFailWorkflow_WaitingInstance_WritesFailureReason verifies the custom reason is stored.
func TestFailWorkflow_WaitingInstance_WritesFailureReason(t *testing.T) {
	env := newTestEnv(t)
	env.createTicket(t, "FW-W2", "failure reason")
	wfiID := env.initWorkflow(t, "FW-W2")
	setWFIStatus(t, env, wfiID, model.WorkflowInstanceWaiting)

	err := env.orch.FailWorkflow(context.Background(), env.project, "FW-W2", "test", wfiID, "my custom reason")
	if err != nil {
		t.Fatalf("FailWorkflow: %v", err)
	}

	findings := getWFIFindings(t, env, wfiID)
	raw, ok := findings["_failure_reason"]
	if !ok {
		t.Fatalf("_failure_reason absent")
	}
	m, _ := raw.(map[string]interface{})
	reason, _ := m["reason"].(string)
	if reason != "my custom reason" {
		t.Errorf("_failure_reason.reason = %q, want %q", reason, "my custom reason")
	}
}

// TestFailWorkflow_DefaultReason verifies empty reason defaults to "manual_fail".
func TestFailWorkflow_DefaultReason(t *testing.T) {
	env := newTestEnv(t)
	env.createTicket(t, "FW-D1", "default reason")
	wfiID := env.initWorkflow(t, "FW-D1")
	setWFIStatus(t, env, wfiID, model.WorkflowInstanceWaiting)

	err := env.orch.FailWorkflow(context.Background(), env.project, "FW-D1", "test", wfiID, "")
	if err != nil {
		t.Fatalf("FailWorkflow: %v", err)
	}

	findings := getWFIFindings(t, env, wfiID)
	m, _ := findings["_failure_reason"].(map[string]interface{})
	reason, _ := m["reason"].(string)
	if reason != "manual_fail" {
		t.Errorf("_failure_reason.reason = %q, want %q", reason, "manual_fail")
	}
}

// TestFailWorkflow_RunningInstance verifies failReason is injected and cancel is called.
func TestFailWorkflow_RunningInstance(t *testing.T) {
	env := newTestEnv(t)
	env.createTicket(t, "FW-R1", "running fail")
	wfiID := env.initWorkflow(t, "FW-R1")

	cancelCalled := make(chan struct{}, 1)
	env.orch.mu.Lock()
	env.orch.runs[wfiID] = &runState{
		cancel: func() { cancelCalled <- struct{}{} },
		done:   make(chan struct{}),
	}
	env.orch.mu.Unlock()
	t.Cleanup(func() {
		env.orch.mu.Lock()
		delete(env.orch.runs, wfiID)
		env.orch.mu.Unlock()
	})

	err := env.orch.FailWorkflow(context.Background(), env.project, "FW-R1", "test", wfiID, "custom fail reason")
	if err != nil {
		t.Fatalf("FailWorkflow on running instance: %v", err)
	}

	select {
	case <-cancelCalled:
	case <-time.After(2 * time.Second):
		t.Fatal("cancel not called within timeout")
	}

	// failReason must be set on the runState before cancel is called.
	env.orch.mu.Lock()
	rs := env.orch.runs[wfiID]
	gotReason := ""
	if rs != nil {
		gotReason = rs.failReason
	}
	env.orch.mu.Unlock()
	if gotReason != "custom fail reason" {
		t.Errorf("rs.failReason = %q, want %q", gotReason, "custom fail reason")
	}
}

// TestFailWorkflow_UnexpectedStatus_Error verifies error for active-but-not-running instance.
func TestFailWorkflow_UnexpectedStatus_Error(t *testing.T) {
	env := newTestEnv(t)
	env.createTicket(t, "FW-U1", "unexpected status")
	wfiID := env.initWorkflow(t, "FW-U1")
	// Status is 'active' but not in o.runs → unexpected

	err := env.orch.FailWorkflow(context.Background(), env.project, "FW-U1", "test", wfiID, "reason")
	if err == nil {
		t.Errorf("FailWorkflow on active-not-running: want error, got nil")
	}
}
