package socket

import (
	"encoding/json"
	"fmt"
	"testing"

	"be/internal/clock"
	"be/internal/types"
)

// fakeTerminalSignaler records RequestTerminalSignal calls for assertion.
// All calls happen synchronously inside Handler.Handle, so no mutex is needed.
type fakeTerminalSignaler struct {
	calls []terminalSignalCall
	err   error // if non-nil, returned from RequestTerminalSignal
}

type terminalSignalCall struct {
	projectID string
	ticketID  string
	workflow  string
	sessionID string
	result    string
}

func (f *fakeTerminalSignaler) RequestTerminalSignal(projectID, ticketID, workflow, sessionID, result string) error {
	f.calls = append(f.calls, terminalSignalCall{
		projectID: projectID,
		ticketID:  ticketID,
		workflow:  workflow,
		sessionID: sessionID,
		result:    result,
	})
	return f.err
}

// insertAgentSession inserts a running agent_sessions row for terminal signal tests.
func insertAgentSession(t *testing.T, env *handlerTestEnv, ticketID, sessionID, wfiID string) {
	t.Helper()
	_, err := env.pool.Exec(`
		INSERT INTO agent_sessions (id, project_id, ticket_id, workflow_instance_id, phase, agent_type, model_id, status, created_at, updated_at)
		VALUES (?, ?, ?, ?, 'analyzer', 'test-agent', 'claude-sonnet-4', 'running', datetime('now'), datetime('now'))
	`, sessionID, env.project, ticketID, wfiID)
	if err != nil {
		t.Fatalf("failed to insert agent session: %v", err)
	}
}

// queryWFIID returns the workflow_instances.id for the given project/ticket/workflow.
func queryWFIID(t *testing.T, env *handlerTestEnv, ticketID string) string {
	t.Helper()
	var id string
	err := env.pool.QueryRow(
		`SELECT id FROM workflow_instances WHERE LOWER(project_id) = LOWER(?) AND LOWER(ticket_id) = LOWER(?) AND LOWER(workflow_id) = LOWER(?)`,
		env.project, ticketID, "test",
	).Scan(&id)
	if err != nil {
		t.Fatalf("failed to query workflow instance ID: %v", err)
	}
	return id
}

// TestAgentFail_DispatchesTerminalSignal verifies that agent.fail dispatches a
// terminal signal with project, ticket, workflow, session, and result="fail".
func TestAgentFail_DispatchesTerminalSignal(t *testing.T) {
	env := newHandlerTestEnv(t)
	env.createTicketAndWorkflow(t, "TS-FAIL-1")
	wfiID := queryWFIID(t, env, "TS-FAIL-1")

	sessionID := "sess-ts-fail-1"
	insertAgentSession(t, env, "TS-FAIL-1", sessionID, wfiID)

	sig := &fakeTerminalSignaler{}
	h := NewHandler(env.pool, env.hub, clock.Real(), sig)

	params := types.AgentRequest{InstanceID: wfiID, SessionID: sessionID}
	paramsData, _ := json.Marshal(params)
	req := Request{ID: "req-1", Method: "agent.fail", Project: env.project, Params: paramsData}

	resp := h.Handle(req)
	if resp.Error != nil {
		t.Fatalf("expected no error, got: %v", resp.Error)
	}

	if len(sig.calls) != 1 {
		t.Fatalf("expected 1 signaler call, got %d", len(sig.calls))
	}
	got := sig.calls[0]
	if got.projectID != env.project {
		t.Errorf("projectID = %q, want %q", got.projectID, env.project)
	}
	if got.ticketID != "TS-FAIL-1" {
		t.Errorf("ticketID = %q, want %q", got.ticketID, "TS-FAIL-1")
	}
	if got.workflow != "test" {
		t.Errorf("workflow = %q, want %q", got.workflow, "test")
	}
	if got.sessionID != sessionID {
		t.Errorf("sessionID = %q, want %q", got.sessionID, sessionID)
	}
	if got.result != "fail" {
		t.Errorf("result = %q, want %q", got.result, "fail")
	}
}

// TestAgentContinue_DispatchesTerminalSignal verifies that agent.continue dispatches
// a terminal signal with result="continue".
func TestAgentContinue_DispatchesTerminalSignal(t *testing.T) {
	env := newHandlerTestEnv(t)
	env.createTicketAndWorkflow(t, "TS-CONT-1")
	wfiID := queryWFIID(t, env, "TS-CONT-1")

	sessionID := "sess-ts-continue-1"
	insertAgentSession(t, env, "TS-CONT-1", sessionID, wfiID)

	sig := &fakeTerminalSignaler{}
	h := NewHandler(env.pool, env.hub, clock.Real(), sig)

	params := types.AgentRequest{InstanceID: wfiID, SessionID: sessionID}
	paramsData, _ := json.Marshal(params)
	req := Request{ID: "req-2", Method: "agent.continue", Project: env.project, Params: paramsData}

	resp := h.Handle(req)
	if resp.Error != nil {
		t.Fatalf("expected no error, got: %v", resp.Error)
	}

	if len(sig.calls) != 1 {
		t.Fatalf("expected 1 signaler call, got %d", len(sig.calls))
	}
	if got := sig.calls[0].result; got != "continue" {
		t.Errorf("result = %q, want %q", got, "continue")
	}
	if got := sig.calls[0].sessionID; got != sessionID {
		t.Errorf("sessionID = %q, want %q", got, sessionID)
	}
}

// TestAgentCallback_DispatchesTerminalSignal verifies that agent.callback dispatches
// a terminal signal with result="callback".
func TestAgentCallback_DispatchesTerminalSignal(t *testing.T) {
	env := newHandlerTestEnv(t)
	env.createTicketAndWorkflow(t, "TS-CB-1")
	wfiID := queryWFIID(t, env, "TS-CB-1")

	sessionID := "sess-ts-callback-1"
	insertAgentSession(t, env, "TS-CB-1", sessionID, wfiID)

	sig := &fakeTerminalSignaler{}
	h := NewHandler(env.pool, env.hub, clock.Real(), sig)

	params := types.AgentCallbackRequest{
		AgentRequest: types.AgentRequest{InstanceID: wfiID, SessionID: sessionID},
		Level:        0,
	}
	paramsData, _ := json.Marshal(params)
	req := Request{ID: "req-3", Method: "agent.callback", Project: env.project, Params: paramsData}

	resp := h.Handle(req)
	if resp.Error != nil {
		t.Fatalf("expected no error, got: %v", resp.Error)
	}

	if len(sig.calls) != 1 {
		t.Fatalf("expected 1 signaler call, got %d", len(sig.calls))
	}
	if got := sig.calls[0].result; got != "callback" {
		t.Errorf("result = %q, want %q", got, "callback")
	}
	if got := sig.calls[0].sessionID; got != sessionID {
		t.Errorf("sessionID = %q, want %q", got, sessionID)
	}
}

// TestTerminalSignal_ErrorDoesNotAffectResponse verifies that when RequestTerminalSignal
// returns an error the handler response is still success — the signal is best-effort.
func TestTerminalSignal_ErrorDoesNotAffectResponse(t *testing.T) {
	env := newHandlerTestEnv(t)
	env.createTicketAndWorkflow(t, "TS-ERR-1")
	wfiID := queryWFIID(t, env, "TS-ERR-1")

	sessionID := "sess-ts-err-1"
	insertAgentSession(t, env, "TS-ERR-1", sessionID, wfiID)

	sig := &fakeTerminalSignaler{err: fmt.Errorf("signaler unavailable")}
	h := NewHandler(env.pool, env.hub, clock.Real(), sig)

	params := types.AgentRequest{InstanceID: wfiID, SessionID: sessionID}
	paramsData, _ := json.Marshal(params)
	req := Request{ID: "req-4", Method: "agent.fail", Project: env.project, Params: paramsData}

	resp := h.Handle(req)
	if resp.Error != nil {
		t.Errorf("expected success even when signaler returns error, got: %v", resp.Error)
	}
	// Signaler was still called despite the error.
	if len(sig.calls) != 1 {
		t.Errorf("expected signaler called once, got %d calls", len(sig.calls))
	}
}

// TestTerminalSignal_NotCalledOnHandlerError verifies that when the handler itself
// fails (e.g. missing session), the signaler is not called.
func TestTerminalSignal_NotCalledOnHandlerError(t *testing.T) {
	env := newHandlerTestEnv(t)
	// No ticket/workflow/session created — service call will fail.

	sig := &fakeTerminalSignaler{}
	h := NewHandler(env.pool, env.hub, clock.Real(), sig)

	params := types.AgentRequest{InstanceID: "no-such-wfi", SessionID: "no-such-session"}
	paramsData, _ := json.Marshal(params)
	req := Request{ID: "req-5", Method: "agent.fail", Project: env.project, Params: paramsData}

	resp := h.Handle(req)
	if resp.Error == nil {
		t.Fatal("expected handler error for missing session")
	}
	if len(sig.calls) != 0 {
		t.Errorf("expected signaler not called on handler error, got %d calls", len(sig.calls))
	}
}
