package integration

import (
	"testing"
	"time"

	"be/internal/clock"
	"be/internal/model"
	"be/internal/repo"
	"be/internal/spawner"
)

// insertConsoleChatSession inserts a raw agent_sessions row with
// kind='console_chat' — the only kind that actually fires the --console
// UserPromptSubmit hook (ChatService claude engine). agent_sessions has no
// FK on project_id/ticket_id, so no ticket/workflow needs to be seeded.
func insertConsoleChatSession(t *testing.T, env *TestEnv, sessionID, ticketID string) {
	t.Helper()
	now := env.Clock.Now().UTC().Format(time.RFC3339Nano)
	_, err := env.Pool.Exec(`
		INSERT INTO agent_sessions (id, project_id, ticket_id, phase, agent_type, status, kind, created_at, updated_at)
		VALUES (?, ?, ?, 'chat', 'console', 'running', ?, ?, ?)
	`, sessionID, env.ProjectID, ticketID, model.AgentSessionKindConsoleChat, now, now)
	if err != nil {
		t.Fatalf("failed to insert console_chat session: %v", err)
	}
}

// userPromptSubmitResult is the subset of agent.record_event's JSON-RPC
// result this test cares about.
type userPromptSubmitResult struct {
	Status            string `json:"status"`
	AdditionalContext string `json:"additional_context"`
}

// TestConsoleContextInjection_MultiTurn_StableAdditionalContext wires a real
// WorkingSetInjector onto the test server (mirroring serve.go's
// SetContextInjector call) and fires two UserPromptSubmit turns for the same
// console_chat session, advancing the clock between them. Both responses
// must carry the SAME additional_context: the injected working-set digest is
// structurally identical per turn — the cached system prefix is never
// touched, only the per-turn user message gets the appended context.
//
// Console claude sessions run over the PTY subscription path with no
// api-mode token accounting, so cache_read_input_tokens isn't observable at
// the socket layer; this asserts the structural invariant (stable, repeated
// injection) that the ticket's "cache_read stays high" claim depends on.
func TestConsoleContextInjection_MultiTurn_StableAdditionalContext(t *testing.T) {
	env := NewTestEnv(t)
	env.Server.SetContextInjector(spawner.NewWorkingSetInjector(env.Pool))

	_, err := env.Pool.Exec(`UPDATE default_templates SET template = ? WHERE id = 'working-set'`,
		"Working set for ${TICKET}: session=${SESSION_ID}")
	if err != nil {
		t.Fatalf("failed to seed working-set template: %v", err)
	}

	sessionID := "sess-console-multi-turn"
	insertConsoleChatSession(t, env, sessionID, "CHAT-TICKET-1")
	if _, err := repo.NewRefineryDigestRepo(env.Pool, clock.Real()).Upsert(sessionID, env.ProjectID, "digest content"); err != nil {
		t.Fatalf("seed refinery digest: %v", err)
	}

	var first userPromptSubmitResult
	env.MustExecute(t, "agent.record_event", map[string]interface{}{
		"session_id": sessionID,
		"event": map[string]interface{}{
			"hook_event_name": "UserPromptSubmit",
			"prompt":          "turn one",
		},
	}, &first)
	if first.AdditionalContext == "" {
		t.Fatal("expected non-empty additional_context on turn one")
	}

	env.Clock.Advance(5 * time.Second)

	var second userPromptSubmitResult
	env.MustExecute(t, "agent.record_event", map[string]interface{}{
		"session_id": sessionID,
		"event": map[string]interface{}{
			"hook_event_name": "UserPromptSubmit",
			"prompt":          "turn two",
		},
	}, &second)
	if second.AdditionalContext == "" {
		t.Fatal("expected non-empty additional_context on turn two")
	}

	if first.AdditionalContext != second.AdditionalContext {
		t.Errorf("additional_context differs per turn:\n turn1: %q\n turn2: %q", first.AdditionalContext, second.AdditionalContext)
	}
	want := "Working set for CHAT-TICKET-1: session=" + sessionID
	if first.AdditionalContext != want {
		t.Errorf("additional_context = %q, want %q", first.AdditionalContext, want)
	}
}

// TestConsoleContextInjection_WorkflowAgentSession_NoAdditionalContext is the
// scope guard: a plain workflow_agent session (the default kind, no
// console/console_chat) must NOT receive additional_context even with a real
// WorkingSetInjector wired — the ticket requires non-console spawned-agent
// hook behavior to stay untouched.
func TestConsoleContextInjection_WorkflowAgentSession_NoAdditionalContext(t *testing.T) {
	env := NewTestEnv(t)
	env.Server.SetContextInjector(spawner.NewWorkingSetInjector(env.Pool))

	_, err := env.Pool.Exec(`UPDATE default_templates SET template = ? WHERE id = 'working-set'`,
		"Working set: ${SESSION_ID}")
	if err != nil {
		t.Fatalf("failed to seed working-set template: %v", err)
	}

	env.CreateTicket(t, "WF-AGENT-1", "Workflow agent ticket")
	env.InitWorkflow(t, "WF-AGENT-1")
	wfiID := env.GetWorkflowInstanceID(t, "WF-AGENT-1", "test")
	sessionID := "sess-workflow-agent-1"
	env.InsertAgentSession(t, sessionID, "WF-AGENT-1", wfiID, "analyzer", "test-agent", "claude-sonnet-4")

	var result map[string]interface{}
	env.MustExecute(t, "agent.record_event", map[string]interface{}{
		"session_id": sessionID,
		"event": map[string]interface{}{
			"hook_event_name": "UserPromptSubmit",
			"prompt":          "hello",
		},
	}, &result)

	if _, has := result["additional_context"]; has {
		t.Errorf("workflow_agent session must not receive additional_context, got %v", result)
	}
}
