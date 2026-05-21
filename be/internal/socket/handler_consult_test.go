package socket

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
)

// stubConsultRunner is a minimal WorkflowOrchestrator stub for consult handler tests.
type stubConsultRunner struct {
	consultResult string
	consultErr    error
}

func (s *stubConsultRunner) StartWorkflow(_ context.Context, _, _, _, _, _ string) (string, error) {
	return "", nil
}

func (s *stubConsultRunner) RetryFailed(_ context.Context, _, _, _, _ string) error {
	return nil
}

func (s *stubConsultRunner) RetryFailedProject(_ context.Context, _, _, _, _ string) error {
	return nil
}

func (s *stubConsultRunner) ContinueWorkflow(_ context.Context, _, _, _ string) error {
	return nil
}

func (s *stubConsultRunner) FailWorkflow(_ context.Context, _, _, _ string) error {
	return nil
}

func (s *stubConsultRunner) Consult(_ context.Context, _, _, _ string) (string, error) {
	return s.consultResult, s.consultErr
}

// makeConsultReq builds an agent.consult socket Request.
func makeConsultReq(project string, params map[string]interface{}) Request {
	data, _ := json.Marshal(params)
	return Request{ID: "req-1", Method: "agent.consult", Project: project, Params: data}
}

// TestConsult_MissingSessionID verifies validation error when session_id is empty.
func TestConsult_MissingSessionID(t *testing.T) {
	t.Parallel()
	env := newHandlerTestEnv(t)

	resp := env.handler.Handle(makeConsultReq(env.project, map[string]interface{}{
		"consultant": "doc-consultant",
		"question":   "how?",
	}))

	if resp.Error == nil {
		t.Fatal("expected error, got nil")
	}
	if resp.Error.Code != ErrCodeValidation {
		t.Errorf("code = %d, want %d (validation)", resp.Error.Code, ErrCodeValidation)
	}
	if resp.Error.Message != "session_id is required" {
		t.Errorf("message = %q, want 'session_id is required'", resp.Error.Message)
	}
}

// TestConsult_MissingConsultant verifies validation error when consultant is empty.
func TestConsult_MissingConsultant(t *testing.T) {
	t.Parallel()
	env := newHandlerTestEnv(t)

	resp := env.handler.Handle(makeConsultReq(env.project, map[string]interface{}{
		"session_id": "sess-123",
		"question":   "how?",
	}))

	if resp.Error == nil {
		t.Fatal("expected error, got nil")
	}
	if resp.Error.Code != ErrCodeValidation {
		t.Errorf("code = %d, want %d (validation)", resp.Error.Code, ErrCodeValidation)
	}
	if resp.Error.Message != "consultant is required" {
		t.Errorf("message = %q, want 'consultant is required'", resp.Error.Message)
	}
}

// TestConsult_MissingQuestion verifies validation error when question is empty.
func TestConsult_MissingQuestion(t *testing.T) {
	t.Parallel()
	env := newHandlerTestEnv(t)

	resp := env.handler.Handle(makeConsultReq(env.project, map[string]interface{}{
		"session_id": "sess-123",
		"consultant": "doc-consultant",
	}))

	if resp.Error == nil {
		t.Fatal("expected error, got nil")
	}
	if resp.Error.Code != ErrCodeValidation {
		t.Errorf("code = %d, want %d (validation)", resp.Error.Code, ErrCodeValidation)
	}
	if resp.Error.Message != "question is required" {
		t.Errorf("message = %q, want 'question is required'", resp.Error.Message)
	}
}

// TestConsult_NilWorkflowRunner verifies internal error when workflowRunner is nil.
func TestConsult_NilWorkflowRunner(t *testing.T) {
	t.Parallel()
	env := newHandlerTestEnv(t)
	// workflowRunner is nil by default in newHandlerTestEnv.

	resp := env.handler.Handle(makeConsultReq(env.project, map[string]interface{}{
		"session_id": "sess-123",
		"consultant": "doc-consultant",
		"question":   "how?",
	}))

	if resp.Error == nil {
		t.Fatal("expected internal error for nil runner, got nil")
	}
	if resp.Error.Code != ErrCodeInternal {
		t.Errorf("code = %d, want %d (internal)", resp.Error.Code, ErrCodeInternal)
	}
	if resp.Error.Message != "workflow runner not available" {
		t.Errorf("message = %q, want 'workflow runner not available'", resp.Error.Message)
	}
}

// TestConsult_HappyPath verifies that a successful Consult returns {answer: "..."}.
func TestConsult_HappyPath(t *testing.T) {
	t.Parallel()
	env := newHandlerTestEnv(t)
	env.handler.workflowRunner = &stubConsultRunner{consultResult: "forty-two"}

	resp := env.handler.Handle(makeConsultReq(env.project, map[string]interface{}{
		"session_id": "sess-123",
		"consultant": "doc-consultant",
		"question":   "what is the answer?",
	}))

	if resp.Error != nil {
		t.Fatalf("unexpected error: %v", resp.Error)
	}
	var result map[string]string
	if err := json.Unmarshal(resp.Result, &result); err != nil {
		t.Fatalf("unmarshal result: %v", err)
	}
	if result["answer"] != "forty-two" {
		t.Errorf("answer = %q, want %q", result["answer"], "forty-two")
	}
}

// TestConsult_RunnerError verifies that a runner error surfaces as an internal error.
func TestConsult_RunnerError(t *testing.T) {
	t.Parallel()
	env := newHandlerTestEnv(t)
	env.handler.workflowRunner = &stubConsultRunner{
		consultErr: errors.New("consultant spawn failed"),
	}

	resp := env.handler.Handle(makeConsultReq(env.project, map[string]interface{}{
		"session_id": "sess-123",
		"consultant": "doc-consultant",
		"question":   "how?",
	}))

	if resp.Error == nil {
		t.Fatal("expected internal error, got nil")
	}
	if resp.Error.Code != ErrCodeInternal {
		t.Errorf("code = %d, want %d (internal)", resp.Error.Code, ErrCodeInternal)
	}
}
