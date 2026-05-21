package integration

import (
	"context"
	"testing"

	"be/internal/socket"
)

// fakeSocketWorkflowRunner implements socket.WorkflowOrchestrator for testing.
type fakeSocketWorkflowRunner struct {
	continueProjectID    string
	continueInstanceID   string
	continueInstructions string
	failProjectID        string
	failInstanceID       string
	failReason           string
	continueErr          error
	failErr              error
}

func (f *fakeSocketWorkflowRunner) StartWorkflow(_ context.Context, _, _, _, _, _ string) (string, error) {
	return "", nil
}

func (f *fakeSocketWorkflowRunner) RetryFailed(_ context.Context, _, _, _, _ string) error {
	return nil
}

func (f *fakeSocketWorkflowRunner) RetryFailedProject(_ context.Context, _, _, _, _ string) error {
	return nil
}

func (f *fakeSocketWorkflowRunner) ContinueWorkflow(_ context.Context, projectID, instanceID, instructions string) error {
	f.continueProjectID = projectID
	f.continueInstanceID = instanceID
	f.continueInstructions = instructions
	return f.continueErr
}

func (f *fakeSocketWorkflowRunner) FailWorkflow(_ context.Context, projectID, instanceID, reason string) error {
	f.failProjectID = projectID
	f.failInstanceID = instanceID
	f.failReason = reason
	return f.failErr
}

func (f *fakeSocketWorkflowRunner) Consult(_ context.Context, _, _, _ string) (string, error) {
	return "", nil
}

func TestWorkflowContinueSocket_OwnerSession_CallsRunner(t *testing.T) {
	env := NewTestEnv(t)
	env.CreateTicket(t, "WC-1", "continue ticket")
	env.InitWorkflow(t, "WC-1")
	wfiID := env.GetWorkflowInstanceID(t, "WC-1", "test")
	env.InsertAgentSession(t, "sess-wc-1", "WC-1", wfiID, "analyzer", "analyzer", "sonnet")

	runner := &fakeSocketWorkflowRunner{}
	env.Server.SetWorkflowRunner(runner)

	var result map[string]string
	env.MustExecute(t, "workflow.continue", map[string]interface{}{
		"session_id":   "sess-wc-1",
		"instance_id":  wfiID,
		"instructions": "keep going",
	}, &result)

	if result["status"] != "continuing" {
		t.Fatalf("expected status 'continuing', got %q", result["status"])
	}
	if runner.continueProjectID != env.ProjectID {
		t.Fatalf("expected projectID %q, got %q", env.ProjectID, runner.continueProjectID)
	}
	if runner.continueInstanceID != wfiID {
		t.Fatalf("expected instanceID %q, got %q", wfiID, runner.continueInstanceID)
	}
	if runner.continueInstructions != "keep going" {
		t.Fatalf("expected instructions 'keep going', got %q", runner.continueInstructions)
	}
}

func TestWorkflowContinueSocket_WrongSession_ValidationError(t *testing.T) {
	env := NewTestEnv(t)

	env.CreateTicket(t, "WC-2a", "ticket one")
	env.InitWorkflow(t, "WC-2a")
	wfi1 := env.GetWorkflowInstanceID(t, "WC-2a", "test")
	env.InsertAgentSession(t, "sess-wc-2a", "WC-2a", wfi1, "analyzer", "analyzer", "sonnet")

	env.CreateTicket(t, "WC-2b", "ticket two")
	env.InitWorkflow(t, "WC-2b")
	wfi2 := env.GetWorkflowInstanceID(t, "WC-2b", "test")

	runner := &fakeSocketWorkflowRunner{}
	env.Server.SetWorkflowRunner(runner)

	// session belongs to wfi1 but we pass wfi2 as instance_id
	env.ExpectError(t, "workflow.continue", map[string]interface{}{
		"session_id":  "sess-wc-2a",
		"instance_id": wfi2,
	}, socket.ErrCodeValidation)
}

func TestWorkflowContinueSocket_NilRunner_InternalError(t *testing.T) {
	env := NewTestEnv(t)
	env.CreateTicket(t, "WC-3", "nil runner")
	env.InitWorkflow(t, "WC-3")
	wfiID := env.GetWorkflowInstanceID(t, "WC-3", "test")
	env.InsertAgentSession(t, "sess-wc-3", "WC-3", wfiID, "analyzer", "analyzer", "sonnet")

	// Intentionally do NOT set a workflow runner

	env.ExpectError(t, "workflow.continue", map[string]interface{}{
		"session_id":  "sess-wc-3",
		"instance_id": wfiID,
	}, socket.ErrCodeInternal)
}

func TestWorkflowFailSocket_OwnerSession_CallsRunner(t *testing.T) {
	env := NewTestEnv(t)
	env.CreateTicket(t, "WF-1", "fail ticket")
	env.InitWorkflow(t, "WF-1")
	wfiID := env.GetWorkflowInstanceID(t, "WF-1", "test")
	env.InsertAgentSession(t, "sess-wf-1", "WF-1", wfiID, "analyzer", "analyzer", "sonnet")

	runner := &fakeSocketWorkflowRunner{}
	env.Server.SetWorkflowRunner(runner)

	var result map[string]string
	env.MustExecute(t, "workflow.fail", map[string]interface{}{
		"session_id":  "sess-wf-1",
		"instance_id": wfiID,
		"reason":      "test reason",
	}, &result)

	if result["status"] != "failing" {
		t.Fatalf("expected status 'failing', got %q", result["status"])
	}
	if runner.failProjectID != env.ProjectID {
		t.Fatalf("expected projectID %q, got %q", env.ProjectID, runner.failProjectID)
	}
	if runner.failInstanceID != wfiID {
		t.Fatalf("expected instanceID %q, got %q", wfiID, runner.failInstanceID)
	}
	if runner.failReason != "test reason" {
		t.Fatalf("expected reason 'test reason', got %q", runner.failReason)
	}
}

func TestWorkflowFailSocket_MissingReason_ValidationError(t *testing.T) {
	env := NewTestEnv(t)
	env.CreateTicket(t, "WF-2", "missing reason")
	env.InitWorkflow(t, "WF-2")
	wfiID := env.GetWorkflowInstanceID(t, "WF-2", "test")
	env.InsertAgentSession(t, "sess-wf-2", "WF-2", wfiID, "analyzer", "analyzer", "sonnet")

	runner := &fakeSocketWorkflowRunner{}
	env.Server.SetWorkflowRunner(runner)

	env.ExpectError(t, "workflow.fail", map[string]interface{}{
		"session_id":  "sess-wf-2",
		"instance_id": wfiID,
		// reason omitted
	}, socket.ErrCodeValidation)
}

func TestWorkflowFailSocket_WrongSession_ValidationError(t *testing.T) {
	env := NewTestEnv(t)

	env.CreateTicket(t, "WF-3a", "ticket one")
	env.InitWorkflow(t, "WF-3a")
	wfi1 := env.GetWorkflowInstanceID(t, "WF-3a", "test")
	env.InsertAgentSession(t, "sess-wf-3a", "WF-3a", wfi1, "analyzer", "analyzer", "sonnet")

	env.CreateTicket(t, "WF-3b", "ticket two")
	env.InitWorkflow(t, "WF-3b")
	wfi2 := env.GetWorkflowInstanceID(t, "WF-3b", "test")

	runner := &fakeSocketWorkflowRunner{}
	env.Server.SetWorkflowRunner(runner)

	// session belongs to wfi1 but we pass wfi2 as instance_id
	env.ExpectError(t, "workflow.fail", map[string]interface{}{
		"session_id":  "sess-wf-3a",
		"instance_id": wfi2,
		"reason":      "mismatch",
	}, socket.ErrCodeValidation)
}
