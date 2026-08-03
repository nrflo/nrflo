package console

import (
	"strings"
	"testing"

	"be/internal/model"
)

func TestGetSubworkflow_CrossProjectInstanceID_Rejected(t *testing.T) {
	env := newConsoleTestEnv(t)
	env.seedWorkflowInstance(t, testOtherProjectID, "wfi-plan-other")
	reg, err := BuildRegistry(env.deps, nil)
	if err != nil {
		t.Fatalf("BuildRegistry: %v", err)
	}
	toolEnv := NewToolEnv(env.deps, "sess-1", testProjectID, model.AgentSessionKindConsole)

	out, isErr, err := invoke(t, reg, toolEnv, "get_subworkflow", `{"instance_id":"wfi-plan-other"}`)
	if err != nil {
		t.Fatalf("err = %v, want nil", err)
	}
	if !isErr {
		t.Errorf("isErr = false, want true for cross-project instance_id; out=%s", out)
	}
}

func TestGetSubworkflow_MissingInstanceID_Errors(t *testing.T) {
	env := newConsoleTestEnv(t)
	reg, err := BuildRegistry(env.deps, nil)
	if err != nil {
		t.Fatalf("BuildRegistry: %v", err)
	}
	toolEnv := NewToolEnv(env.deps, "sess-1", testProjectID, model.AgentSessionKindConsole)

	out, isErr, err := invoke(t, reg, toolEnv, "get_subworkflow", `{}`)
	if err != nil {
		t.Fatalf("err = %v, want nil", err)
	}
	if !isErr || !strings.Contains(out, "instance_id is required") {
		t.Errorf("out=%q isErr=%v, want instance_id is required", out, isErr)
	}
}

func TestGetSubworkflow_UnknownInstanceID_Errors(t *testing.T) {
	env := newConsoleTestEnv(t)
	reg, err := BuildRegistry(env.deps, nil)
	if err != nil {
		t.Fatalf("BuildRegistry: %v", err)
	}
	toolEnv := NewToolEnv(env.deps, "sess-1", testProjectID, model.AgentSessionKindConsole)

	out, isErr, err := invoke(t, reg, toolEnv, "get_subworkflow", `{"instance_id":"no-such-instance"}`)
	if err != nil {
		t.Fatalf("err = %v, want nil", err)
	}
	if !isErr {
		t.Errorf("isErr = false, want true for unknown instance_id; out=%s", out)
	}
}

func TestGetSubworkflow_HappyPath_ReturnsRunningStatus(t *testing.T) {
	env := newConsoleTestEnv(t)
	env.seedWorkflowInstance(t, testProjectID, "wfi-plan-own")
	reg, err := BuildRegistry(env.deps, nil)
	if err != nil {
		t.Fatalf("BuildRegistry: %v", err)
	}
	toolEnv := NewToolEnv(env.deps, "sess-1", testProjectID, model.AgentSessionKindConsole)

	out, isErr, err := invoke(t, reg, toolEnv, "get_subworkflow", `{"instance_id":"wfi-plan-own"}`)
	if err != nil || isErr {
		t.Fatalf("err=%v isErr=%v out=%s", err, isErr, out)
	}
	if !strings.Contains(out, `"status":"running"`) {
		t.Errorf("out = %q, want status=running (seedWorkflowInstance inserts status='active')", out)
	}
}

func TestRevisePlan_MissingInstanceID_Errors(t *testing.T) {
	env := newConsoleTestEnv(t)
	env.deps.Orch = &fakeOrchestrator{}
	reg, err := BuildRegistry(env.deps, nil)
	if err != nil {
		t.Fatalf("BuildRegistry: %v", err)
	}
	toolEnv := NewToolEnv(env.deps, "sess-1", testProjectID, model.AgentSessionKindConsole)

	out, isErr, err := invoke(t, reg, toolEnv, "revise_plan", `{"revision":0}`)
	if err != nil {
		t.Fatalf("err = %v, want nil", err)
	}
	if !isErr || !strings.Contains(out, "instance_id is required") {
		t.Errorf("out=%q isErr=%v, want instance_id is required", out, isErr)
	}
}

func TestRevisePlan_NilOrchestrator_MissingService(t *testing.T) {
	env := newConsoleTestEnv(t)
	env.seedWorkflowInstance(t, testProjectID, "wfi-revise-noorch")
	reg, err := BuildRegistry(env.deps, nil)
	if err != nil {
		t.Fatalf("BuildRegistry: %v", err)
	}
	toolEnv := NewToolEnv(env.deps, "sess-1", testProjectID, model.AgentSessionKindConsole)

	out, isErr, err := invoke(t, reg, toolEnv, "revise_plan", `{"instance_id":"wfi-revise-noorch","revision":0}`)
	if err != nil {
		t.Fatalf("err = %v, want nil", err)
	}
	if !isErr || !strings.Contains(out, "orchestrator") {
		t.Errorf("out=%q isErr=%v, want missing orchestrator error", out, isErr)
	}
}

func TestRevisePlan_CrossProjectInstanceID_Rejected(t *testing.T) {
	env := newConsoleTestEnv(t)
	env.deps.Orch = &fakeOrchestrator{}
	env.seedWorkflowInstance(t, testOtherProjectID, "wfi-revise-other")
	reg, err := BuildRegistry(env.deps, nil)
	if err != nil {
		t.Fatalf("BuildRegistry: %v", err)
	}
	toolEnv := NewToolEnv(env.deps, "sess-1", testProjectID, model.AgentSessionKindConsole)

	out, isErr, err := invoke(t, reg, toolEnv, "revise_plan", `{"instance_id":"wfi-revise-other","revision":0}`)
	if err != nil {
		t.Fatalf("err = %v, want nil", err)
	}
	if !isErr {
		t.Errorf("isErr = false, want true for cross-project instance_id; out=%s", out)
	}
}

func TestApprovePlan_MissingInstanceID_Errors(t *testing.T) {
	env := newConsoleTestEnv(t)
	env.deps.Orch = &fakeOrchestrator{}
	reg, err := BuildRegistry(env.deps, nil)
	if err != nil {
		t.Fatalf("BuildRegistry: %v", err)
	}
	toolEnv := NewToolEnv(env.deps, "sess-1", testProjectID, model.AgentSessionKindConsole)

	out, isErr, err := invoke(t, reg, toolEnv, "approve_plan", `{"revision":1}`)
	if err != nil {
		t.Fatalf("err = %v, want nil", err)
	}
	if !isErr || !strings.Contains(out, "instance_id is required") {
		t.Errorf("out=%q isErr=%v, want instance_id is required", out, isErr)
	}
}

func TestApprovePlan_NilOrchestrator_MissingService(t *testing.T) {
	env := newConsoleTestEnv(t)
	env.seedWorkflowInstance(t, testProjectID, "wfi-approve-noorch")
	reg, err := BuildRegistry(env.deps, nil)
	if err != nil {
		t.Fatalf("BuildRegistry: %v", err)
	}
	toolEnv := NewToolEnv(env.deps, "sess-1", testProjectID, model.AgentSessionKindConsole)

	out, isErr, err := invoke(t, reg, toolEnv, "approve_plan", `{"instance_id":"wfi-approve-noorch","revision":1}`)
	if err != nil {
		t.Fatalf("err = %v, want nil", err)
	}
	if !isErr || !strings.Contains(out, "orchestrator") {
		t.Errorf("out=%q isErr=%v, want missing orchestrator error", out, isErr)
	}
}

func TestApprovePlan_CrossProjectInstanceID_Rejected(t *testing.T) {
	env := newConsoleTestEnv(t)
	env.deps.Orch = &fakeOrchestrator{}
	env.seedWorkflowInstance(t, testOtherProjectID, "wfi-approve-other")
	reg, err := BuildRegistry(env.deps, nil)
	if err != nil {
		t.Fatalf("BuildRegistry: %v", err)
	}
	toolEnv := NewToolEnv(env.deps, "sess-1", testProjectID, model.AgentSessionKindConsole)

	out, isErr, err := invoke(t, reg, toolEnv, "approve_plan", `{"instance_id":"wfi-approve-other","revision":1}`)
	if err != nil {
		t.Fatalf("err = %v, want nil", err)
	}
	if !isErr {
		t.Errorf("isErr = false, want true for cross-project instance_id; out=%s", out)
	}
}
