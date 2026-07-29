package console

import (
	"strings"
	"testing"
)

func TestWorkflowRun_TicketScoped_ValidatesAndStarts(t *testing.T) {
	env := newConsoleTestEnv(t)
	fake := &fakeOrchestrator{}
	env.deps.Orch = fake
	reg, err := BuildRegistry(env.deps, nil)
	if err != nil {
		t.Fatalf("BuildRegistry: %v", err)
	}
	toolEnv := NewToolEnv(env.deps, "sess-1", testProjectID)

	out, isErr, err := invoke(t, reg, toolEnv, "workflow_run", `{"workflow":"feature","ticket_id":"`+testTicketID+`"}`)
	if err != nil || isErr {
		t.Fatalf("err=%v isErr=%v out=%s", err, isErr, out)
	}
	if fake.startScopeType != "ticket" {
		t.Errorf("scopeType = %q, want ticket", fake.startScopeType)
	}
	if fake.startTicketID != testTicketID {
		t.Errorf("ticketID = %q, want %q", fake.startTicketID, testTicketID)
	}
	if !strings.Contains(out, "instance_id") {
		t.Errorf("out = %q, want contains instance_id", out)
	}
	if fake.startConsoleSessionID != "sess-1" {
		t.Errorf("startConsoleSessionID = %q, want the launching console session id %q", fake.startConsoleSessionID, "sess-1")
	}
}

func TestWorkflowRun_TicketScoped_UnknownTicket_Errors(t *testing.T) {
	env := newConsoleTestEnv(t)
	fake := &fakeOrchestrator{}
	env.deps.Orch = fake
	reg, err := BuildRegistry(env.deps, nil)
	if err != nil {
		t.Fatalf("BuildRegistry: %v", err)
	}
	toolEnv := NewToolEnv(env.deps, "sess-1", testProjectID)

	_, isErr, err := invoke(t, reg, toolEnv, "workflow_run", `{"workflow":"feature","ticket_id":"no-such-ticket"}`)
	if err != nil {
		t.Fatalf("err = %v, want nil", err)
	}
	if !isErr {
		t.Errorf("isErr = false, want true for unknown ticket")
	}
	if fake.startWorkflow != "" {
		t.Errorf("orchestrator should not have been called; startWorkflow = %q", fake.startWorkflow)
	}
}

func TestWorkflowRun_ProjectScoped_NoTicketID(t *testing.T) {
	env := newConsoleTestEnv(t)
	fake := &fakeOrchestrator{}
	env.deps.Orch = fake
	reg, err := BuildRegistry(env.deps, nil)
	if err != nil {
		t.Fatalf("BuildRegistry: %v", err)
	}
	toolEnv := NewToolEnv(env.deps, "sess-1", testProjectID)

	_, isErr, err := invoke(t, reg, toolEnv, "workflow_run", `{"workflow":"some-project-wf"}`)
	if err != nil || isErr {
		t.Fatalf("err=%v isErr=%v", err, isErr)
	}
	if fake.startScopeType != "project" {
		t.Errorf("scopeType = %q, want project", fake.startScopeType)
	}
	if fake.startTicketID != "" {
		t.Errorf("ticketID = %q, want empty", fake.startTicketID)
	}
}

func TestWorkflowRun_MissingWorkflow_Errors(t *testing.T) {
	env := newConsoleTestEnv(t)
	reg, err := BuildRegistry(env.deps, nil)
	if err != nil {
		t.Fatalf("BuildRegistry: %v", err)
	}
	toolEnv := NewToolEnv(env.deps, "sess-1", testProjectID)

	out, isErr, err := invoke(t, reg, toolEnv, "workflow_run", `{}`)
	if err != nil {
		t.Fatalf("err = %v, want nil", err)
	}
	if !isErr || !strings.Contains(out, "workflow is required") {
		t.Errorf("out=%q isErr=%v, want error mentioning workflow is required", out, isErr)
	}
}

func TestWorkflowRun_NilOrchestrator_MissingService(t *testing.T) {
	env := newConsoleTestEnv(t)
	reg, err := BuildRegistry(env.deps, nil)
	if err != nil {
		t.Fatalf("BuildRegistry: %v", err)
	}
	toolEnv := NewToolEnv(env.deps, "sess-1", testProjectID)

	out, isErr, err := invoke(t, reg, toolEnv, "workflow_run", `{"workflow":"feature"}`)
	if err != nil || !isErr || !strings.Contains(out, "orchestrator") {
		t.Errorf("err=%v isErr=%v out=%q, want orchestrator not configured", err, isErr, out)
	}
}

func TestWorkflowStop_CrossProjectInstanceID_Rejected(t *testing.T) {
	env := newConsoleTestEnv(t)
	env.seedWorkflowInstance(t, testOtherProjectID, "wfi-other-1")
	fake := &fakeOrchestrator{}
	env.deps.Orch = fake
	reg, err := BuildRegistry(env.deps, nil)
	if err != nil {
		t.Fatalf("BuildRegistry: %v", err)
	}
	toolEnv := NewToolEnv(env.deps, "sess-1", testProjectID)

	out, isErr, err := invoke(t, reg, toolEnv, "workflow_stop", `{"instance_id":"wfi-other-1"}`)
	if err != nil {
		t.Fatalf("err = %v, want nil", err)
	}
	if !isErr {
		t.Errorf("isErr = false, want true for cross-project instance_id")
	}
	if fake.stopProjectCalled != 0 {
		t.Errorf("orchestrator should not have been called; out=%q", out)
	}
}

func TestWorkflowStop_ProjectScoped_HappyPath(t *testing.T) {
	env := newConsoleTestEnv(t)
	env.seedWorkflowInstance(t, testProjectID, "wfi-own-1")
	fake := &fakeOrchestrator{}
	env.deps.Orch = fake
	reg, err := BuildRegistry(env.deps, nil)
	if err != nil {
		t.Fatalf("BuildRegistry: %v", err)
	}
	toolEnv := NewToolEnv(env.deps, "sess-1", testProjectID)

	out, isErr, err := invoke(t, reg, toolEnv, "workflow_stop", `{"instance_id":"wfi-own-1"}`)
	if err != nil || isErr {
		t.Fatalf("err=%v isErr=%v out=%s", err, isErr, out)
	}
	if fake.stopProjectCalled != 1 || fake.stopProjectInstanceID != "wfi-own-1" {
		t.Errorf("StopByProject not called with expected instance: %+v", fake)
	}
}

func TestWorkflowStop_TicketScopedInstance_StopsById(t *testing.T) {
	env := newConsoleTestEnv(t)
	env.seedWorkflowInstance(t, testProjectID, "wfi-own-2")
	fake := &fakeOrchestrator{}
	env.deps.Orch = fake
	reg, err := BuildRegistry(env.deps, nil)
	if err != nil {
		t.Fatalf("BuildRegistry: %v", err)
	}
	toolEnv := NewToolEnv(env.deps, "sess-1", testProjectID)

	out, isErr, err := invoke(t, reg, toolEnv, "workflow_stop", `{"instance_id":"wfi-own-2"}`)
	if err != nil || isErr {
		t.Fatalf("err=%v isErr=%v out=%s", err, isErr, out)
	}
	if fake.stopProjectCalled != 1 || fake.stopProjectInstanceID != "wfi-own-2" {
		t.Errorf("StopByProject not called with expected instance: %+v", fake)
	}
}

func TestWorkflowRetryFailed_CrossProjectInstanceID_Rejected(t *testing.T) {
	env := newConsoleTestEnv(t)
	env.seedWorkflowInstance(t, testOtherProjectID, "wfi-other-2")
	fake := &fakeOrchestrator{}
	env.deps.Orch = fake
	reg, err := BuildRegistry(env.deps, nil)
	if err != nil {
		t.Fatalf("BuildRegistry: %v", err)
	}
	toolEnv := NewToolEnv(env.deps, "sess-1", testProjectID)

	_, isErr, err := invoke(t, reg, toolEnv, "workflow_retry_failed",
		`{"workflow":"feature","session_id":"s1","instance_id":"wfi-other-2"}`)
	if err != nil {
		t.Fatalf("err = %v, want nil", err)
	}
	if !isErr {
		t.Errorf("isErr = false, want true for cross-project instance_id")
	}
}

func TestWorkflowRetryFailed_MissingTicketAndInstance_Errors(t *testing.T) {
	env := newConsoleTestEnv(t)
	reg, err := BuildRegistry(env.deps, nil)
	if err != nil {
		t.Fatalf("BuildRegistry: %v", err)
	}
	toolEnv := NewToolEnv(env.deps, "sess-1", testProjectID)

	out, isErr, err := invoke(t, reg, toolEnv, "workflow_retry_failed", `{"workflow":"feature","session_id":"s1"}`)
	if err != nil {
		t.Fatalf("err = %v, want nil", err)
	}
	if !isErr || !strings.Contains(out, "ticket_id or instance_id") {
		t.Errorf("out=%q isErr=%v, want ticket_id or instance_id error", out, isErr)
	}
}
