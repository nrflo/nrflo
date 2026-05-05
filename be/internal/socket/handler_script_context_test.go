package socket

import (
	"encoding/json"
	"testing"
)

// insertProjectWFI inserts a project-scoped workflow_instance row for context tests.
func insertProjectWFI(t *testing.T, env *handlerTestEnv, id string) string {
	t.Helper()
	_, err := env.pool.Exec(`
		INSERT INTO workflow_instances (id, project_id, ticket_id, workflow_id, scope_type, status, created_at, updated_at)
		VALUES (?, ?, '', 'test', 'project', 'active', datetime('now'), datetime('now'))
	`, id, env.project)
	if err != nil {
		t.Fatalf("insertProjectWFI: %v", err)
	}
	return id
}

// setWFIFindings updates the findings JSON on a workflow_instances row.
func setWFIFindings(t *testing.T, env *handlerTestEnv, wfiID string, findings map[string]interface{}) {
	t.Helper()
	data, _ := json.Marshal(findings)
	_, err := env.pool.Exec(`UPDATE workflow_instances SET findings = ? WHERE id = ?`, string(data), wfiID)
	if err != nil {
		t.Fatalf("setWFIFindings: %v", err)
	}
}

// setSessionFindings updates the findings JSON on an agent_sessions row.
func setSessionFindings(t *testing.T, env *handlerTestEnv, sessionID string, findings map[string]interface{}) {
	t.Helper()
	data, _ := json.Marshal(findings)
	_, err := env.pool.Exec(`UPDATE agent_sessions SET findings = ? WHERE id = ?`, string(data), sessionID)
	if err != nil {
		t.Fatalf("setSessionFindings: %v", err)
	}
}

// callScriptContext sends a script.context request and returns the response and
// the parsed result map (nil on error; check resp.Error).
func callScriptContext(t *testing.T, h *Handler, sessionID string) (Response, map[string]interface{}) {
	t.Helper()
	params, _ := json.Marshal(map[string]string{"session_id": sessionID})
	req := Request{
		ID:     "req-sc-" + sessionID,
		Method: "script.context",
		Params: params,
	}
	resp := h.Handle(req)
	if resp.Error != nil {
		return resp, nil
	}
	var result map[string]interface{}
	if err := json.Unmarshal(resp.Result, &result); err != nil {
		t.Fatalf("callScriptContext: unmarshal result: %v", err)
	}
	return resp, result
}

// TestScriptContext_HappyPath verifies all 12 keys are returned with seeded values.
func TestScriptContext_HappyPath(t *testing.T) {
	env := newHandlerTestEnv(t)
	env.createTicketAndWorkflow(t, "SC-HP-1")

	// Seed ticket description via direct SQL (createTicketAndWorkflow sets title only)
	_, err := env.pool.Exec(
		`UPDATE tickets SET description = ? WHERE LOWER(id) = LOWER(?) AND LOWER(project_id) = LOWER(?)`,
		"Happy path description", "SC-HP-1", env.project,
	)
	if err != nil {
		t.Fatalf("seed ticket description: %v", err)
	}

	wfiID := queryWFIID(t, env, "SC-HP-1")
	setWFIFindings(t, env, wfiID, map[string]interface{}{
		"user_instructions": "do the thing",
		"_callback": map[string]interface{}{
			"instructions": "fix it",
			"from_agent":   "qa-verifier",
			"level":        float64(1),
		},
	})

	sessionID := "sess-sc-hp-1"
	insertAgentSession(t, env, "SC-HP-1", sessionID, wfiID)
	setSessionFindings(t, env, sessionID, map[string]interface{}{
		"to_resume": "previous state here",
	})

	resp, result := callScriptContext(t, env.handler, sessionID)
	if resp.Error != nil {
		t.Fatalf("expected no error, got: %v", resp.Error)
	}

	checks := []struct {
		key  string
		want interface{}
	}{
		{"session_id", sessionID},
		{"instance_id", wfiID},
		{"project_id", env.project},
		{"agent_type", "test-agent"},
		{"workflow_id", "test"},
		{"ticket_id", "SC-HP-1"},
		{"ticket_title", "Test ticket"},
		{"ticket_description", "Happy path description"},
		{"user_instructions", "do the thing"},
		{"previous_data", "previous state here"},
	}
	for _, c := range checks {
		got, _ := result[c.key].(string)
		want, _ := c.want.(string)
		if got != want {
			t.Errorf("%s = %q, want %q", c.key, got, want)
		}
	}

	// callback must be non-nil map when _callback is seeded
	if result["callback"] == nil {
		t.Error("callback must be non-nil when _callback is seeded with non-empty instructions")
	}
	cb, ok := result["callback"].(map[string]interface{})
	if !ok {
		t.Errorf("callback is not a map, got %T", result["callback"])
	} else {
		if cb["instructions"] != "fix it" {
			t.Errorf("callback.instructions = %v, want %q", cb["instructions"], "fix it")
		}
		if cb["from_agent"] != "qa-verifier" {
			t.Errorf("callback.from_agent = %v, want %q", cb["from_agent"], "qa-verifier")
		}
	}

	// scope_type key must be present
	if _, ok := result["scope_type"]; !ok {
		t.Error("scope_type key must be present in response")
	}
}

// TestScriptContext_UnknownSession verifies that an unknown session_id returns NOT_FOUND.
func TestScriptContext_UnknownSession(t *testing.T) {
	env := newHandlerTestEnv(t)

	resp, _ := callScriptContext(t, env.handler, "no-such-session-id")
	if resp.Error == nil {
		t.Fatal("expected NOT_FOUND error for unknown session_id")
	}
	if resp.Error.Code != ErrCodeNotFound {
		t.Errorf("error code = %d, want %d (NOT_FOUND)", resp.Error.Code, ErrCodeNotFound)
	}
}

// TestScriptContext_MissingSessionID verifies that an empty session_id returns a validation error.
func TestScriptContext_MissingSessionID(t *testing.T) {
	env := newHandlerTestEnv(t)

	req := Request{
		ID:     "req-sc-missing",
		Method: "script.context",
		Params: []byte(`{}`),
	}
	resp := env.handler.Handle(req)
	if resp.Error == nil {
		t.Fatal("expected validation error when session_id is missing")
	}
	if resp.Error.Code != ErrCodeValidation {
		t.Errorf("error code = %d, want %d (VALIDATION)", resp.Error.Code, ErrCodeValidation)
	}
}

// TestScriptContext_AbsentCallback verifies that absent _callback yields JSON null,
// not an omitted key — the spec mandates the key is always present.
func TestScriptContext_AbsentCallback(t *testing.T) {
	env := newHandlerTestEnv(t)
	env.createTicketAndWorkflow(t, "SC-CB-1")

	wfiID := queryWFIID(t, env, "SC-CB-1")
	setWFIFindings(t, env, wfiID, map[string]interface{}{})

	sessionID := "sess-sc-cb-1"
	insertAgentSession(t, env, "SC-CB-1", sessionID, wfiID)

	resp, result := callScriptContext(t, env.handler, sessionID)
	if resp.Error != nil {
		t.Fatalf("expected no error, got: %v", resp.Error)
	}

	// Key must be present (json.Unmarshal maps "null" → nil for interface{}).
	val, ok := result["callback"]
	if !ok {
		t.Error("callback key must be present in response even when _callback absent")
	}
	if val != nil {
		t.Errorf("callback = %v, want nil (JSON null) when _callback absent", val)
	}
}

// TestScriptContext_AbsentUserInstructions verifies user_instructions is "" when absent.
func TestScriptContext_AbsentUserInstructions(t *testing.T) {
	env := newHandlerTestEnv(t)
	env.createTicketAndWorkflow(t, "SC-UI-1")

	wfiID := queryWFIID(t, env, "SC-UI-1")
	setWFIFindings(t, env, wfiID, map[string]interface{}{})

	sessionID := "sess-sc-ui-1"
	insertAgentSession(t, env, "SC-UI-1", sessionID, wfiID)

	_, result := callScriptContext(t, env.handler, sessionID)
	if result == nil {
		t.Fatal("expected non-nil result")
	}

	v, _ := result["user_instructions"].(string)
	if v != "" {
		t.Errorf("user_instructions = %q, want empty string when absent in WFI findings", v)
	}
}

// TestScriptContext_AbsentPreviousData verifies previous_data is "" when to_resume absent.
func TestScriptContext_AbsentPreviousData(t *testing.T) {
	env := newHandlerTestEnv(t)
	env.createTicketAndWorkflow(t, "SC-PD-1")

	wfiID := queryWFIID(t, env, "SC-PD-1")
	sessionID := "sess-sc-pd-1"
	insertAgentSession(t, env, "SC-PD-1", sessionID, wfiID)
	// Intentionally do not set session findings

	_, result := callScriptContext(t, env.handler, sessionID)
	if result == nil {
		t.Fatal("expected non-nil result")
	}

	v, _ := result["previous_data"].(string)
	if v != "" {
		t.Errorf("previous_data = %q, want empty string when to_resume absent", v)
	}
}

// TestScriptContext_ProjectScoped verifies that a project-scoped session returns
// empty ticket fields without triggering a DB error.
func TestScriptContext_ProjectScoped(t *testing.T) {
	env := newHandlerTestEnv(t)

	wfiID := insertProjectWFI(t, env, "proj-wfi-sc-1")

	sessionID := "sess-sc-proj-1"
	_, err := env.pool.Exec(`
		INSERT INTO agent_sessions (id, project_id, ticket_id, workflow_instance_id, phase, agent_type, model_id, status, created_at, updated_at)
		VALUES (?, ?, '', ?, 'analyzer', 'test-agent', 'claude-sonnet-4', 'running', datetime('now'), datetime('now'))
	`, sessionID, env.project, wfiID)
	if err != nil {
		t.Fatalf("insert project-scoped session: %v", err)
	}

	resp, result := callScriptContext(t, env.handler, sessionID)
	if resp.Error != nil {
		t.Fatalf("expected no error for project-scoped session, got: %v", resp.Error)
	}

	if v, _ := result["ticket_id"].(string); v != "" {
		t.Errorf("ticket_id = %q, want empty string for project-scoped session", v)
	}
	if v, _ := result["ticket_title"].(string); v != "" {
		t.Errorf("ticket_title = %q, want empty string for project-scoped session", v)
	}
	if v, _ := result["ticket_description"].(string); v != "" {
		t.Errorf("ticket_description = %q, want empty string for project-scoped session", v)
	}
	if v, _ := result["scope_type"].(string); v != "project" {
		t.Errorf("scope_type = %q, want %q", v, "project")
	}
}

// TestScriptContext_UnknownAction verifies that unknown script.* actions return METHOD_NOT_FOUND.
func TestScriptContext_UnknownAction(t *testing.T) {
	env := newHandlerTestEnv(t)

	req := Request{
		ID:     "req-sc-unknown",
		Method: "script.bogus_action",
		Params: []byte(`{}`),
	}
	resp := env.handler.Handle(req)
	if resp.Error == nil {
		t.Fatal("expected error for unknown script.* action")
	}
	if resp.Error.Code != ErrCodeMethodNotFound {
		t.Errorf("error code = %d, want %d (METHOD_NOT_FOUND)", resp.Error.Code, ErrCodeMethodNotFound)
	}
}
