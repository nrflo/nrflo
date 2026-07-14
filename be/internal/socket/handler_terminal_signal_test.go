package socket

import (
	"encoding/json"
	"fmt"
	"testing"
	"time"

	"be/internal/clock"
	"be/internal/types"
	"be/internal/ws"
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

func (f *fakeTerminalSignaler) BumpLastMessage(projectID, ticketID, workflow, sessionID string) error {
	return nil
}

func (f *fakeTerminalSignaler) SetLastMessage(projectID, ticketID, workflow, sessionID, content string) error {
	return nil
}

func (f *fakeTerminalSignaler) SignalSessionReady(sessionID string) error { return nil }

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
// terminal signal with project, ticket, workflow, session, and result="fail",
// and broadcasts agent.completed with the correct payload fields.
func TestAgentFail_DispatchesTerminalSignal(t *testing.T) {
	env := newHandlerTestEnv(t)
	env.createTicketAndWorkflow(t, "TS-FAIL-1")
	wfiID := queryWFIID(t, env, "TS-FAIL-1")

	sessionID := "sess-ts-fail-1"
	_, err := env.pool.Exec(`
		INSERT INTO agent_sessions (id, project_id, ticket_id, workflow_instance_id, phase, agent_type, model_id, status, created_at, updated_at)
		VALUES (?, ?, ?, ?, 'analyzer', 'test-agent', 'claude-sonnet-4', 'running', datetime('now'), datetime('now'))
	`, sessionID, env.project, "TS-FAIL-1", wfiID)
	if err != nil {
		t.Fatalf("failed to insert agent session: %v", err)
	}

	sig := &fakeTerminalSignaler{}
	h := NewHandler(env.pool, env.hub, clock.Real(), sig)

	client, sendCh := ws.NewTestClient(env.hub, "test-client-fail")
	env.hub.Register(client)
	env.hub.Subscribe(client, env.project, "TS-FAIL-1")

	params := types.AgentRequest{InstanceID: wfiID, SessionID: sessionID}
	paramsData, _ := json.Marshal(params)
	req := Request{ID: "req-1", Method: "agent.fail", Project: env.project, Params: paramsData}

	resp := h.Handle(req)
	if resp.Error != nil {
		t.Fatalf("expected no error, got: %v", resp.Error)
	}

	// Verify signaler call.
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

	// Verify broadcast payload.
	select {
	case msg := <-sendCh:
		var event ws.Event
		if err := json.Unmarshal(msg, &event); err != nil {
			t.Fatalf("failed to unmarshal event: %v", err)
		}
		if event.Type != ws.EventAgentCompleted {
			t.Errorf("event type = %s, want %s", event.Type, ws.EventAgentCompleted)
		}
		if sid, ok := event.Data["session_id"].(string); !ok || sid == "" {
			t.Errorf("session_id must be present in payload, got: %v", event.Data["session_id"])
		}
		if modelID, ok := event.Data["model_id"].(string); !ok || modelID != "claude-sonnet-4" {
			t.Errorf("model_id = %v, want claude-sonnet-4", event.Data["model_id"])
		}
		if result, ok := event.Data["result"].(string); !ok || result != "fail" {
			t.Errorf("result = %v, want fail", event.Data["result"])
		}
	case <-time.After(time.Second):
		t.Fatal("timeout waiting for agent.completed broadcast")
	}
}

// TestAgentContinue_DispatchesTerminalSignal verifies that agent.continue dispatches
// a terminal signal with result="continue" and broadcasts agent.continued with the
// correct payload fields.
func TestAgentContinue_DispatchesTerminalSignal(t *testing.T) {
	env := newHandlerTestEnv(t)
	env.createTicketAndWorkflow(t, "TS-CONT-1")
	wfiID := queryWFIID(t, env, "TS-CONT-1")

	sessionID := "sess-ts-continue-1"
	_, err := env.pool.Exec(`
		INSERT INTO agent_sessions (id, project_id, ticket_id, workflow_instance_id, phase, agent_type, model_id, status, created_at, updated_at)
		VALUES (?, ?, ?, ?, 'analyzer', 'test-agent', 'gpt-5.3', 'running', datetime('now'), datetime('now'))
	`, sessionID, env.project, "TS-CONT-1", wfiID)
	if err != nil {
		t.Fatalf("failed to insert agent session: %v", err)
	}

	sig := &fakeTerminalSignaler{}
	h := NewHandler(env.pool, env.hub, clock.Real(), sig)

	client, sendCh := ws.NewTestClient(env.hub, "test-client-continue")
	env.hub.Register(client)
	env.hub.Subscribe(client, env.project, "TS-CONT-1")

	params := types.AgentRequest{InstanceID: wfiID, SessionID: sessionID}
	paramsData, _ := json.Marshal(params)
	req := Request{ID: "req-2", Method: "agent.continue", Project: env.project, Params: paramsData}

	resp := h.Handle(req)
	if resp.Error != nil {
		t.Fatalf("expected no error, got: %v", resp.Error)
	}

	// Verify signaler call.
	if len(sig.calls) != 1 {
		t.Fatalf("expected 1 signaler call, got %d", len(sig.calls))
	}
	if got := sig.calls[0].result; got != "continue" {
		t.Errorf("result = %q, want %q", got, "continue")
	}
	if got := sig.calls[0].sessionID; got != sessionID {
		t.Errorf("sessionID = %q, want %q", got, sessionID)
	}

	// Verify broadcast payload.
	select {
	case msg := <-sendCh:
		var event ws.Event
		if err := json.Unmarshal(msg, &event); err != nil {
			t.Fatalf("failed to unmarshal event: %v", err)
		}
		if event.Type != ws.EventAgentContinued {
			t.Errorf("event type = %s, want %s", event.Type, ws.EventAgentContinued)
		}
		if sid, ok := event.Data["session_id"].(string); !ok || sid == "" {
			t.Errorf("session_id must be present in payload, got: %v", event.Data["session_id"])
		}
		if modelID, ok := event.Data["model_id"].(string); !ok || modelID != "gpt-5.3" {
			t.Errorf("model_id = %v, want gpt-5.3", event.Data["model_id"])
		}
	case <-time.After(time.Second):
		t.Fatal("timeout waiting for agent.continued broadcast")
	}
}

// TestAgentCallback_DispatchesTerminalSignal verifies that agent.callback dispatches
// a terminal signal with result="callback" and broadcasts agent.completed with the
// correct payload fields (model_id, result, level).
func TestAgentCallback_DispatchesTerminalSignal(t *testing.T) {
	env := newHandlerTestEnv(t)
	env.createTicketAndWorkflow(t, "TS-CB-1")
	wfiID := queryWFIID(t, env, "TS-CB-1")

	sessionID := "sess-ts-callback-1"
	_, err := env.pool.Exec(`
		INSERT INTO agent_sessions (id, project_id, ticket_id, workflow_instance_id, phase, agent_type, model_id, status, created_at, updated_at)
		VALUES (?, ?, ?, ?, 'analyzer', 'test-agent', 'claude-opus-4', 'running', datetime('now'), datetime('now'))
	`, sessionID, env.project, "TS-CB-1", wfiID)
	if err != nil {
		t.Fatalf("failed to insert agent session: %v", err)
	}

	sig := &fakeTerminalSignaler{}
	h := NewHandler(env.pool, env.hub, clock.Real(), sig)

	client, sendCh := ws.NewTestClient(env.hub, "test-client-callback")
	env.hub.Register(client)
	env.hub.Subscribe(client, env.project, "TS-CB-1")

	params := types.AgentCallbackRequest{
		AgentRequest: types.AgentRequest{InstanceID: wfiID, SessionID: sessionID},
		Level:        1,
	}
	paramsData, _ := json.Marshal(params)
	req := Request{ID: "req-3", Method: "agent.callback", Project: env.project, Params: paramsData}

	resp := h.Handle(req)
	if resp.Error != nil {
		t.Fatalf("expected no error, got: %v", resp.Error)
	}

	// Verify signaler call.
	if len(sig.calls) != 1 {
		t.Fatalf("expected 1 signaler call, got %d", len(sig.calls))
	}
	if got := sig.calls[0].result; got != "callback" {
		t.Errorf("result = %q, want %q", got, "callback")
	}
	if got := sig.calls[0].sessionID; got != sessionID {
		t.Errorf("sessionID = %q, want %q", got, sessionID)
	}

	// Verify broadcast payload.
	select {
	case msg := <-sendCh:
		var event ws.Event
		if err := json.Unmarshal(msg, &event); err != nil {
			t.Fatalf("failed to unmarshal event: %v", err)
		}
		if event.Type != ws.EventAgentCompleted {
			t.Errorf("event type = %s, want %s", event.Type, ws.EventAgentCompleted)
		}
		if modelID, ok := event.Data["model_id"].(string); !ok || modelID != "claude-opus-4" {
			t.Errorf("model_id = %v, want claude-opus-4", event.Data["model_id"])
		}
		if result, ok := event.Data["result"].(string); !ok || result != "callback" {
			t.Errorf("result = %v, want callback", event.Data["result"])
		}
		if level, ok := event.Data["level"].(float64); !ok || int(level) != 1 {
			t.Errorf("level = %v, want 1", event.Data["level"])
		}
	case <-time.After(time.Second):
		t.Fatal("timeout waiting for agent.completed broadcast")
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
