package console

import (
	"context"
	"strings"
	"testing"

	"be/internal/model"
	"be/internal/service"
)

func TestDynamicWorkflow_HappyPath_StartsTopLevelProjectRun(t *testing.T) {
	env := newConsoleTestEnv(t)
	fake := &fakeOrchestrator{startInstanceID: "wfi-dyn-1"}
	env.deps.Orch = fake
	reg, err := BuildRegistry(env.deps, nil)
	if err != nil {
		t.Fatalf("BuildRegistry: %v", err)
	}
	toolEnv := NewToolEnv(env.deps, "sess-1", testProjectID, model.AgentSessionKindConsole)

	out, isErr, err := invoke(t, reg, toolEnv, "dynamic_workflow", `{"instructions":"build the thing"}`)
	if err != nil || isErr {
		t.Fatalf("err=%v isErr=%v out=%s", err, isErr, out)
	}
	if !strings.Contains(out, "wfi-dyn-1") {
		t.Errorf("out=%q, want it to contain the instance id", out)
	}
	if fake.startProjectID != testProjectID {
		t.Errorf("startProjectID = %q, want %q", fake.startProjectID, testProjectID)
	}
	if fake.startWorkflow != service.DynamicWorkflow {
		t.Errorf("startWorkflow = %q, want %q", fake.startWorkflow, service.DynamicWorkflow)
	}
	if fake.startScopeType != "project" {
		t.Errorf("startScopeType = %q, want project", fake.startScopeType)
	}
	if fake.startInstructions != "build the thing" {
		t.Errorf("startInstructions = %q, want %q", fake.startInstructions, "build the thing")
	}
	if fake.startTicketID != "" {
		t.Errorf("startTicketID = %q, want empty (dynamic_workflow is always project-scoped)", fake.startTicketID)
	}
	if fake.startConsoleSessionID != "sess-1" {
		t.Errorf("startConsoleSessionID = %q, want the launching console session id %q", fake.startConsoleSessionID, "sess-1")
	}
}

func TestDynamicWorkflow_MissingInstructions_Errors(t *testing.T) {
	env := newConsoleTestEnv(t)
	env.deps.Orch = &fakeOrchestrator{}
	reg, err := BuildRegistry(env.deps, nil)
	if err != nil {
		t.Fatalf("BuildRegistry: %v", err)
	}
	toolEnv := NewToolEnv(env.deps, "sess-1", testProjectID, model.AgentSessionKindConsole)

	out, isErr, err := invoke(t, reg, toolEnv, "dynamic_workflow", `{"instructions":""}`)
	if err != nil {
		t.Fatalf("err = %v, want nil", err)
	}
	if !isErr || !strings.Contains(out, "instructions is required") {
		t.Errorf("out=%q isErr=%v, want instructions-required error", out, isErr)
	}
}

func TestDynamicWorkflow_NilOrchestrator_MissingService(t *testing.T) {
	env := newConsoleTestEnv(t)
	reg, err := BuildRegistry(env.deps, nil)
	if err != nil {
		t.Fatalf("BuildRegistry: %v", err)
	}
	toolEnv := NewToolEnv(env.deps, "sess-1", testProjectID, model.AgentSessionKindConsole)

	out, isErr, err := invoke(t, reg, toolEnv, "dynamic_workflow", `{"instructions":"do it"}`)
	if err != nil {
		t.Fatalf("err = %v, want nil", err)
	}
	if !isErr || !strings.Contains(out, "orchestrator") {
		t.Errorf("out=%q isErr=%v, want missing orchestrator error", out, isErr)
	}
}

func TestDynamicWorkflow_StartError_SurfacedAsToolError(t *testing.T) {
	env := newConsoleTestEnv(t)
	env.deps.Orch = &fakeOrchestrator{startErr: context.DeadlineExceeded}
	reg, err := BuildRegistry(env.deps, nil)
	if err != nil {
		t.Fatalf("BuildRegistry: %v", err)
	}
	toolEnv := NewToolEnv(env.deps, "sess-1", testProjectID, model.AgentSessionKindConsole)

	out, isErr, err := invoke(t, reg, toolEnv, "dynamic_workflow", `{"instructions":"do it"}`)
	if err != nil {
		t.Fatalf("err = %v, want nil", err)
	}
	if !isErr {
		t.Errorf("isErr = false, want true when StartWorkflow fails; out=%s", out)
	}
}

// fakeConsultant adapts a func field to apirun.ConsultantSpawner.
type fakeConsultant struct {
	consultFn func(ctx context.Context, callerSessionID, consultantID, question string) (string, error)

	lastCallerSessionID string
	lastConsultantID    string
	lastQuestion        string
}

func (f *fakeConsultant) Consult(ctx context.Context, callerSessionID, consultantID, question string) (string, error) {
	f.lastCallerSessionID = callerSessionID
	f.lastConsultantID = consultantID
	f.lastQuestion = question
	return f.consultFn(ctx, callerSessionID, consultantID, question)
}

func TestConsult_HappyPath_RoutesThroughDepsConsultant(t *testing.T) {
	env := newConsoleTestEnv(t)
	fake := &fakeConsultant{
		consultFn: func(context.Context, string, string, string) (string, error) {
			return "the consultant's answer", nil
		},
	}
	env.deps.Consultant = fake
	reg, err := BuildRegistry(env.deps, nil)
	if err != nil {
		t.Fatalf("BuildRegistry: %v", err)
	}
	toolEnv := NewToolEnv(env.deps, "sess-1", testProjectID, model.AgentSessionKindConsole)

	out, isErr, err := invoke(t, reg, toolEnv, "consult", `{"consultant":"security-expert","question":"is this safe?"}`)
	if err != nil || isErr {
		t.Fatalf("err=%v isErr=%v out=%s", err, isErr, out)
	}
	if out != "the consultant's answer" {
		t.Errorf("out = %q, want the consultant's raw answer", out)
	}
	if fake.lastCallerSessionID != "sess-1" {
		t.Errorf("lastCallerSessionID = %q, want sess-1", fake.lastCallerSessionID)
	}
	if fake.lastConsultantID != "security-expert" {
		t.Errorf("lastConsultantID = %q, want security-expert", fake.lastConsultantID)
	}
	if fake.lastQuestion != "is this safe?" {
		t.Errorf("lastQuestion = %q, want %q", fake.lastQuestion, "is this safe?")
	}
}

func TestConsult_NilConsultant_MissingService(t *testing.T) {
	env := newConsoleTestEnv(t)
	reg, err := BuildRegistry(env.deps, nil)
	if err != nil {
		t.Fatalf("BuildRegistry: %v", err)
	}
	toolEnv := NewToolEnv(env.deps, "sess-1", testProjectID, model.AgentSessionKindConsole)

	out, isErr, err := invoke(t, reg, toolEnv, "consult", `{"consultant":"x","question":"y"}`)
	if err != nil {
		t.Fatalf("err = %v, want nil", err)
	}
	if !isErr || !strings.Contains(out, "consultant") {
		t.Errorf("out=%q isErr=%v, want missing consultant error", out, isErr)
	}
}

func TestConsult_ConsultantError_SurfacedAsToolError(t *testing.T) {
	env := newConsoleTestEnv(t)
	env.deps.Consultant = &fakeConsultant{
		consultFn: func(context.Context, string, string, string) (string, error) {
			return "", context.DeadlineExceeded
		},
	}
	reg, err := BuildRegistry(env.deps, nil)
	if err != nil {
		t.Fatalf("BuildRegistry: %v", err)
	}
	toolEnv := NewToolEnv(env.deps, "sess-1", testProjectID, model.AgentSessionKindConsole)

	out, isErr, err := invoke(t, reg, toolEnv, "consult", `{"consultant":"x","question":"y"}`)
	if err != nil {
		t.Fatalf("err = %v, want nil", err)
	}
	if !isErr {
		t.Errorf("isErr = false, want true when Consult fails; out=%s", out)
	}
}
