package socket

import (
	"encoding/json"
	"strings"
	"testing"
	"time"
)

const emitTestSchema = `[{"key":"security_issues","schema":{"type":"array","items":{"type":"object","properties":{"file":{"type":"string"},"severity":{"type":"string"}},"required":["file","severity"]}},"example":[{"file":"a.go","severity":"high"}]}]`

// setupEmitSession sets finding_schemas on the "test" workflow, creates an
// instance + running session, and returns the session id.
func setupEmitSession(t *testing.T, env *handlerTestEnv, ticketID string) string {
	t.Helper()
	if _, err := env.pool.Exec(`UPDATE workflows SET finding_schemas=? WHERE LOWER(id)='test' AND LOWER(project_id)=LOWER(?)`,
		emitTestSchema, env.project); err != nil {
		t.Fatalf("set finding_schemas: %v", err)
	}
	env.createTicketAndWorkflow(t, ticketID)

	var wfiID string
	if err := env.pool.QueryRow(
		`SELECT id FROM workflow_instances WHERE LOWER(project_id)=LOWER(?) AND LOWER(ticket_id)=LOWER(?) AND LOWER(workflow_id)='test'`,
		env.project, ticketID).Scan(&wfiID); err != nil {
		t.Fatalf("get wfi: %v", err)
	}
	now := time.Now().UTC().Format(time.RFC3339Nano)
	sessionID := "sess-emit-" + ticketID
	if _, err := env.pool.Exec(`
		INSERT INTO agent_sessions (id, project_id, ticket_id, workflow_instance_id, phase, agent_type, status, restart_count, created_at, updated_at)
		VALUES (?, ?, ?, ?, 'analyzer', 'analyzer', 'running', 0, ?, ?)`,
		sessionID, env.project, ticketID, wfiID, now, now); err != nil {
		t.Fatalf("insert session: %v", err)
	}
	return sessionID
}

func emitReq(t *testing.T, env *handlerTestEnv, sessionID, key string, value json.RawMessage) Response {
	t.Helper()
	params, _ := json.Marshal(map[string]interface{}{
		"session_id":  sessionID,
		"instance_id": "",
		"key":         key,
		"value":       value,
	})
	return env.handler.Handle(Request{ID: "emit", Method: "findings.emit", Project: env.project, Params: params})
}

func TestFindingsEmit_Valid(t *testing.T) {
	env := newHandlerTestEnv(t)
	sid := setupEmitSession(t, env, "EMIT-OK")

	resp := emitReq(t, env, sid, "security_issues", json.RawMessage(`[{"file":"a.go","severity":"high"}]`))
	if resp.Error != nil {
		t.Fatalf("unexpected error: %v", resp.Error)
	}
	var raw string
	if err := env.pool.QueryRow(`SELECT value FROM findings WHERE scope='session' AND scope_id=? AND key='security_issues'`, sid).Scan(&raw); err != nil {
		t.Fatalf("finding not stored: %v", err)
	}
}

func TestFindingsEmit_InvalidReturnsValidationError(t *testing.T) {
	env := newHandlerTestEnv(t)
	sid := setupEmitSession(t, env, "EMIT-BAD")

	resp := emitReq(t, env, sid, "security_issues", json.RawMessage(`[{"file":"a.go"}]`))
	if resp.Error == nil {
		t.Fatal("expected validation error, got success")
	}
	if resp.Error.Code != ErrCodeValidation {
		t.Errorf("code = %d, want %d", resp.Error.Code, ErrCodeValidation)
	}
	if !strings.Contains(resp.Error.Message, "Expected structure example") {
		t.Errorf("message missing example: %s", resp.Error.Message)
	}
}

func TestFindingsEmit_UnknownKey(t *testing.T) {
	env := newHandlerTestEnv(t)
	sid := setupEmitSession(t, env, "EMIT-UNK")

	resp := emitReq(t, env, sid, "notes", json.RawMessage(`[]`))
	if resp.Error == nil {
		t.Fatal("expected error for unknown key")
	}
	if !strings.Contains(resp.Error.Message, "no schema defined") || !strings.Contains(resp.Error.Message, "security_issues") {
		t.Errorf("message should list configured keys: %s", resp.Error.Message)
	}
}
