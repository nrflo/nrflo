package socket

import (
	"testing"
)

// insertProjectWFIWithExternalRefs inserts a project-scoped WFI with external_id
// and external_context set.
func insertProjectWFIWithExternalRefs(t *testing.T, env *handlerTestEnv, id, extID, extCtx string) string {
	t.Helper()
	_, err := env.pool.Exec(`
		INSERT INTO workflow_instances (id, project_id, ticket_id, workflow_id, scope_type, status, external_id, external_context, created_at, updated_at)
		VALUES (?, ?, '', 'test', 'project', 'active', ?, ?, datetime('now'), datetime('now'))
	`, id, env.project, extID, extCtx)
	if err != nil {
		t.Fatalf("insertProjectWFIWithExternalRefs: %v", err)
	}
	return id
}

// insertProjectScopedSession inserts an agent_sessions row for a project-scoped WFI.
func insertProjectScopedSession(t *testing.T, env *handlerTestEnv, sessionID, wfiID string) {
	t.Helper()
	_, err := env.pool.Exec(`
		INSERT INTO agent_sessions (id, project_id, ticket_id, workflow_instance_id, phase, agent_type, model_id, status, created_at, updated_at)
		VALUES (?, ?, '', ?, 'analyzer', 'test-agent', 'claude-sonnet-4', 'running', datetime('now'), datetime('now'))
	`, sessionID, env.project, wfiID)
	if err != nil {
		t.Fatalf("insertProjectScopedSession: %v", err)
	}
}

// TestScriptContext_ExternalIDAndContext_Present verifies that external_id and
// external_context from the WFI row are returned by script.context.
func TestScriptContext_ExternalIDAndContext_Present(t *testing.T) {
	env := newHandlerTestEnv(t)

	wfiID := insertProjectWFIWithExternalRefs(t, env, "wfi-ext-1", "ext-abc-123", `{"source":"jira","key":"PROJ-42"}`)
	insertProjectScopedSession(t, env, "sess-ext-1", wfiID)

	resp, result := callScriptContext(t, env.handler, "sess-ext-1")
	if resp.Error != nil {
		t.Fatalf("expected no error, got: %v", resp.Error)
	}

	if v, _ := result["external_id"].(string); v != "ext-abc-123" {
		t.Errorf("external_id = %q, want %q", v, "ext-abc-123")
	}
	if v, _ := result["external_context"].(string); v != `{"source":"jira","key":"PROJ-42"}` {
		t.Errorf("external_context = %q, want %q", v, `{"source":"jira","key":"PROJ-42"}`)
	}
}

// TestScriptContext_ExternalIDAndContext_EmptyWhenUnset verifies that external_id
// and external_context are "" (not absent) when the WFI row has no values.
func TestScriptContext_ExternalIDAndContext_EmptyWhenUnset(t *testing.T) {
	env := newHandlerTestEnv(t)

	wfiID := insertProjectWFI(t, env, "wfi-ext-empty-1")
	insertProjectScopedSession(t, env, "sess-ext-empty-1", wfiID)

	resp, result := callScriptContext(t, env.handler, "sess-ext-empty-1")
	if resp.Error != nil {
		t.Fatalf("expected no error, got: %v", resp.Error)
	}

	if _, ok := result["external_id"]; !ok {
		t.Error("external_id key must be present in response")
	}
	if _, ok := result["external_context"]; !ok {
		t.Error("external_context key must be present in response")
	}
	if v, _ := result["external_id"].(string); v != "" {
		t.Errorf("external_id = %q, want empty string when not set on WFI", v)
	}
	if v, _ := result["external_context"].(string); v != "" {
		t.Errorf("external_context = %q, want empty string when not set on WFI", v)
	}
}

// TestScriptContext_ExternalIDAndContext_TicketScoped verifies that external_id
// and external_context are surfaced for ticket-scoped sessions too.
func TestScriptContext_ExternalIDAndContext_TicketScoped(t *testing.T) {
	env := newHandlerTestEnv(t)
	env.createTicketAndWorkflow(t, "EXT-TS-1")

	wfiID := queryWFIID(t, env, "EXT-TS-1")

	_, err := env.pool.Exec(
		`UPDATE workflow_instances SET external_id=?, external_context=? WHERE id=?`,
		"ticket-ext-id", "ticket-ext-ctx", wfiID,
	)
	if err != nil {
		t.Fatalf("update external refs: %v", err)
	}

	sessionID := "sess-ext-ts-1"
	insertAgentSession(t, env, "EXT-TS-1", sessionID, wfiID)

	resp, result := callScriptContext(t, env.handler, sessionID)
	if resp.Error != nil {
		t.Fatalf("expected no error, got: %v", resp.Error)
	}

	if v, _ := result["external_id"].(string); v != "ticket-ext-id" {
		t.Errorf("external_id = %q, want %q", v, "ticket-ext-id")
	}
	if v, _ := result["external_context"].(string); v != "ticket-ext-ctx" {
		t.Errorf("external_context = %q, want %q", v, "ticket-ext-ctx")
	}
}
