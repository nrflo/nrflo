package tools_builtin

import (
	"context"
	"strings"
	"testing"

	"be/internal/spawner/apirun"
)

type fakeWorkflowControl struct {
	continueCalledProjectID    string
	continueCalledInstanceID   string
	continueCalledInstructions string
	failCalledProjectID        string
	failCalledInstanceID       string
	failCalledReason           string
	continueErr                error
	failErr                    error
}

func (f *fakeWorkflowControl) ContinueWorkflow(ctx context.Context, projectID, instanceID, instructions string) error {
	f.continueCalledProjectID = projectID
	f.continueCalledInstanceID = instanceID
	f.continueCalledInstructions = instructions
	return f.continueErr
}

func (f *fakeWorkflowControl) FailWorkflow(ctx context.Context, projectID, instanceID, reason string) error {
	f.failCalledProjectID = projectID
	f.failCalledInstanceID = instanceID
	f.failCalledReason = reason
	return f.failErr
}

func TestWorkflowContinue_NilControl_ReturnsServiceError(t *testing.T) {
	env := newBuiltinTestEnv(t)
	// WorkflowControl is nil by default in newBuiltinTestEnv
	out, isErr, err := invoke(t, env.env, "workflow_continue", `{"instance_id":"wfi-1"}`)
	if err != nil {
		t.Fatalf("err = %v, want nil", err)
	}
	if !isErr {
		t.Errorf("isErr=false, want true")
	}
	if !strings.Contains(out, "workflow_control") {
		t.Errorf("output = %q, want contains 'workflow_control'", out)
	}
}

func TestWorkflowContinue_InvalidArgs(t *testing.T) {
	env := newBuiltinTestEnv(t)
	out, isErr, err := invoke(t, env.env, "workflow_continue", `not-valid-json`)
	if err != nil {
		t.Fatalf("err = %v, want nil", err)
	}
	if !isErr {
		t.Errorf("isErr=false, want true")
	}
	if !strings.Contains(out, "invalid arguments") {
		t.Errorf("output = %q, want contains 'invalid arguments'", out)
	}
}

func TestWorkflowContinue_HappyPath(t *testing.T) {
	env := newBuiltinTestEnv(t)
	fake := &fakeWorkflowControl{}
	env.env.WorkflowControl = fake

	out, isErr, err := invoke(t, env.env, "workflow_continue", `{"instance_id":"wfi-42","instructions":"carry on"}`)
	if err != nil {
		t.Fatalf("err = %v, want nil", err)
	}
	if isErr || out != "ok" {
		t.Errorf("output=%q isErr=%v, want ok/false", out, isErr)
	}
	if fake.continueCalledProjectID != testProjectID {
		t.Errorf("projectID = %q, want %q", fake.continueCalledProjectID, testProjectID)
	}
	if fake.continueCalledInstanceID != "wfi-42" {
		t.Errorf("instanceID = %q, want wfi-42", fake.continueCalledInstanceID)
	}
	if fake.continueCalledInstructions != "carry on" {
		t.Errorf("instructions = %q, want 'carry on'", fake.continueCalledInstructions)
	}
}

func TestWorkflowFail_NilControl_ReturnsServiceError(t *testing.T) {
	env := newBuiltinTestEnv(t)
	out, isErr, err := invoke(t, env.env, "workflow_fail", `{"instance_id":"wfi-1","reason":"broken"}`)
	if err != nil {
		t.Fatalf("err = %v, want nil", err)
	}
	if !isErr {
		t.Errorf("isErr=false, want true")
	}
	if !strings.Contains(out, "workflow_control") {
		t.Errorf("output = %q, want contains 'workflow_control'", out)
	}
}

func TestWorkflowFail_InvalidArgs(t *testing.T) {
	env := newBuiltinTestEnv(t)
	out, isErr, err := invoke(t, env.env, "workflow_fail", `not-valid-json`)
	if err != nil {
		t.Fatalf("err = %v, want nil", err)
	}
	if !isErr {
		t.Errorf("isErr=false, want true")
	}
	if !strings.Contains(out, "invalid arguments") {
		t.Errorf("output = %q, want contains 'invalid arguments'", out)
	}
}

func TestWorkflowFail_HappyPath(t *testing.T) {
	env := newBuiltinTestEnv(t)
	fake := &fakeWorkflowControl{}
	env.env.WorkflowControl = fake

	out, isErr, err := invoke(t, env.env, "workflow_fail", `{"instance_id":"wfi-99","reason":"catastrophic"}`)
	if err != nil {
		t.Fatalf("err = %v, want nil", err)
	}
	if isErr || out != "ok" {
		t.Errorf("output=%q isErr=%v, want ok/false", out, isErr)
	}
	if fake.failCalledProjectID != testProjectID {
		t.Errorf("projectID = %q, want %q", fake.failCalledProjectID, testProjectID)
	}
	if fake.failCalledInstanceID != "wfi-99" {
		t.Errorf("instanceID = %q, want wfi-99", fake.failCalledInstanceID)
	}
	if fake.failCalledReason != "catastrophic" {
		t.Errorf("reason = %q, want 'catastrophic'", fake.failCalledReason)
	}
}

// Compile-time check: fakeWorkflowControl satisfies apirun.WorkflowController.
var _ apirun.WorkflowController = (*fakeWorkflowControl)(nil)
