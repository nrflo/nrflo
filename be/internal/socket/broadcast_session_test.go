package socket

import (
	"encoding/json"
	"testing"
	"time"

	"be/internal/clock"
	"be/internal/ws"
)

// insertConsoleChatSessionForBroadcastTest inserts a kind='console_chat' row
// with workflow_instance_id NULL — the shape that makes
// AgentService.RecordHookMessage's broadcast-id lookup (an INNER JOIN on
// workflow_instances) return empty ids, which is exactly why
// broadcastMessageEvent's unconditional session-channel push exists.
func insertConsoleChatSessionForBroadcastTest(t *testing.T, env *handlerTestEnv, sessionID string) {
	t.Helper()
	if _, err := env.pool.Exec(`
		INSERT INTO agent_sessions (id, project_id, ticket_id, workflow_instance_id, phase, agent_type, model_id, status, kind, created_at, updated_at)
		VALUES (?, ?, '', NULL, 'console_chat', 'console_chat', 'sonnet', 'user_interactive', 'console_chat', datetime('now'), datetime('now'))
	`, sessionID, env.project); err != nil {
		t.Fatalf("insert console_chat session: %v", err)
	}
}

// TestBroadcastMessageEvent_ConsoleChatSession_DeliversOnSessionChannelOnly
// asserts a PreToolUse hook message recorded for a console_chat session (no
// workflow_instance, so the id lookup resolves to "") still reaches a
// subscriber of that session's WS channel, and does NOT leak onto the
// project:ticket channel a workflow-agent hook would use (there is no ticket
// to scope it to).
func TestBroadcastMessageEvent_ConsoleChatSession_DeliversOnSessionChannelOnly(t *testing.T) {
	env := newHandlerTestEnv(t)
	sessionID := "sess-chat-broadcast"
	insertConsoleChatSessionForBroadcastTest(t, env, sessionID)

	sessionClient, sessionCh := ws.NewTestClient(env.hub, "chat-session-sub")
	env.hub.Register(sessionClient)
	env.hub.SubscribeSession(sessionClient, sessionID)

	projectClient, projectCh := ws.NewTestClient(env.hub, "chat-project-sub")
	env.hub.Register(projectClient)
	env.hub.Subscribe(projectClient, env.project, "")

	h := NewHandler(env.pool, env.hub, clock.Real(), nil)
	req := buildRecordEventReq(t, "req-chat-pretool", sessionID, map[string]interface{}{
		"hook_event_name": "PreToolUse",
		"tool_name":       "Bash",
		"tool_input":      map[string]interface{}{"command": "ls"},
	})

	resp := h.Handle(req)
	if resp.Error != nil {
		t.Fatalf("expected no error, got: %v", resp.Error)
	}
	if n := countAgentMessages(t, env, sessionID); n != 1 {
		t.Fatalf("agent_messages count = %d, want 1", n)
	}

	ev := awaitWSEvent(t, sessionCh, ws.EventMessagesUpdated)
	if ev.SessionID != sessionID {
		t.Errorf("session-channel event SessionID = %q, want %q", ev.SessionID, sessionID)
	}

	select {
	case msg := <-projectCh:
		var pev ws.Event
		_ = json.Unmarshal(msg, &pev)
		t.Errorf("project-scoped subscriber received a console_chat session's event, want none: %+v", pev)
	case <-time.After(150 * time.Millisecond):
		// expected: a console_chat session resolves no project scope, so
		// nothing is broadcast on the project:ticket channel.
	}
}

// TestBroadcastMessageEvent_WorkflowAgentSession_StillGetsProjectScopedBroadcast
// is the no-regression check: a normal workflow_agent session (which DOES
// resolve a project/ticket/workflow) must keep reaching its project-scoped
// subscriber exactly as before — the unconditional session-channel push added
// alongside it must not change that.
func TestBroadcastMessageEvent_WorkflowAgentSession_StillGetsProjectScopedBroadcast(t *testing.T) {
	env := newHandlerTestEnv(t)
	env.createTicketAndWorkflow(t, "RE-BCAST-WF")
	wfiID := queryWFIID(t, env, "RE-BCAST-WF")
	sessionID := "sess-wf-broadcast"
	insertAgentSession(t, env, "RE-BCAST-WF", sessionID, wfiID)

	projectClient, projectCh := ws.NewTestClient(env.hub, "wf-project-sub")
	env.hub.Register(projectClient)
	env.hub.Subscribe(projectClient, env.project, "RE-BCAST-WF")

	sessionClient, sessionCh := ws.NewTestClient(env.hub, "wf-session-sub")
	env.hub.Register(sessionClient)
	env.hub.SubscribeSession(sessionClient, sessionID)

	h := NewHandler(env.pool, env.hub, clock.Real(), nil)
	req := buildRecordEventReq(t, "req-wf-pretool", sessionID, map[string]interface{}{
		"hook_event_name": "PreToolUse",
		"tool_name":       "Bash",
		"tool_input":      map[string]interface{}{"command": "ls"},
	})

	resp := h.Handle(req)
	if resp.Error != nil {
		t.Fatalf("expected no error, got: %v", resp.Error)
	}

	projEv := awaitWSEvent(t, projectCh, ws.EventMessagesUpdated)
	if sid, _ := projEv.Data["session_id"].(string); sid != sessionID {
		t.Errorf("project-scoped broadcast session_id = %q, want %q", sid, sessionID)
	}

	// The additional session-channel push is a harmless no-op extension for a
	// workflow-agent session — no behavioural change to the project path.
	sessEv := awaitWSEvent(t, sessionCh, ws.EventMessagesUpdated)
	if sessEv.SessionID != sessionID {
		t.Errorf("session-channel event SessionID = %q, want %q", sessEv.SessionID, sessionID)
	}
}
