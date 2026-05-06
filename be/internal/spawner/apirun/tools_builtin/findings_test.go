package tools_builtin

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"be/internal/spawner/apirun"
	"be/internal/ws"
)

// invoke runs a builtin handler from the Builtins() map by name and returns
// the (output, isErr, err) tuple.
func invoke(t *testing.T, env apirun.ToolEnv, name string, input string) (string, bool, error) {
	t.Helper()
	h, ok := Builtins()[name]
	if !ok {
		t.Fatalf("builtin %q not registered", name)
	}
	return h.Invoke(context.Background(), env, json.RawMessage(input))
}

func TestFindingsAdd_PersistsAndBroadcasts(t *testing.T) {
	env := newBuiltinTestEnv(t)
	out, isErr, err := invoke(t, env.env, "findings_add", `{"key":"summary","value":"all good"}`)
	if err != nil {
		t.Fatalf("Invoke err: %v", err)
	}
	if isErr {
		t.Fatalf("isErr=true, output=%q", out)
	}
	if out != "ok" {
		t.Errorf("output = %q, want ok", out)
	}
	got := env.readSessionFindings(t)
	if !strings.Contains(got, `"summary":"all good"`) {
		t.Errorf("findings = %s, want contains summary:all good", got)
	}
	if len(env.hub.events) != 1 {
		t.Fatalf("hub events = %d, want 1", len(env.hub.events))
	}
	if env.hub.events[0].Type != ws.EventFindingsUpdated {
		t.Errorf("event type = %q, want %q", env.hub.events[0].Type, ws.EventFindingsUpdated)
	}
	if env.hub.events[0].Data["action"] != "add" {
		t.Errorf("event action = %v, want add", env.hub.events[0].Data["action"])
	}
}

func TestFindingsAddBulk_PersistsAndBroadcasts(t *testing.T) {
	env := newBuiltinTestEnv(t)
	_, _, err := invoke(t, env.env, "findings_add_bulk", `{"key_values":{"a":"1","b":"2"}}`)
	if err != nil {
		t.Fatalf("Invoke err: %v", err)
	}
	got := env.readSessionFindings(t)
	if !strings.Contains(got, `"a":1`) || !strings.Contains(got, `"b":2`) {
		t.Errorf("findings = %s, want a:1 and b:2", got)
	}
	if len(env.hub.events) != 1 || env.hub.events[0].Type != ws.EventFindingsUpdated {
		t.Errorf("expected single findings.updated event, got %+v", env.hub.events)
	}
	if env.hub.events[0].Data["action"] != "add-bulk" {
		t.Errorf("action = %v, want add-bulk", env.hub.events[0].Data["action"])
	}
}

func TestFindingsAppend_AppendsExistingValue(t *testing.T) {
	env := newBuiltinTestEnv(t)
	if _, _, err := invoke(t, env.env, "findings_add", `{"key":"items","value":"first"}`); err != nil {
		t.Fatalf("Invoke add: %v", err)
	}
	if _, _, err := invoke(t, env.env, "findings_append", `{"key":"items","value":"second"}`); err != nil {
		t.Fatalf("Invoke append: %v", err)
	}
	got := env.readSessionFindings(t)
	if !strings.Contains(got, `"first"`) || !strings.Contains(got, `"second"`) {
		t.Errorf("findings = %s, want both values present", got)
	}
}

func TestFindingsAppendBulk_PersistsAndBroadcasts(t *testing.T) {
	env := newBuiltinTestEnv(t)
	_, _, err := invoke(t, env.env, "findings_append_bulk", `{"key_values":{"x":"a","y":"b"}}`)
	if err != nil {
		t.Fatalf("Invoke err: %v", err)
	}
	got := env.readSessionFindings(t)
	if !strings.Contains(got, `"x":"a"`) || !strings.Contains(got, `"y":"b"`) {
		t.Errorf("findings = %s, want x:a y:b", got)
	}
	if len(env.hub.events) != 1 || env.hub.events[0].Data["action"] != "append-bulk" {
		t.Errorf("expected append-bulk event, got %+v", env.hub.events)
	}
}

func TestFindingsGet_OwnSession(t *testing.T) {
	env := newBuiltinTestEnv(t)
	if _, _, err := invoke(t, env.env, "findings_add", `{"key":"hi","value":"there"}`); err != nil {
		t.Fatalf("Invoke add: %v", err)
	}
	out, isErr, err := invoke(t, env.env, "findings_get", `{"key":"hi"}`)
	if err != nil {
		t.Fatalf("Invoke err: %v", err)
	}
	if isErr {
		t.Fatalf("isErr=true output=%q", out)
	}
	if out != `"there"` {
		t.Errorf("output = %q, want \"there\"", out)
	}
}

func TestFindingsDelete_PersistsAndBroadcasts(t *testing.T) {
	env := newBuiltinTestEnv(t)
	if _, _, err := invoke(t, env.env, "findings_add", `{"key":"a","value":"1"}`); err != nil {
		t.Fatalf("Invoke add: %v", err)
	}
	env.hub.events = nil
	out, isErr, err := invoke(t, env.env, "findings_delete", `{"keys":["a","missing"]}`)
	if err != nil {
		t.Fatalf("Invoke err: %v", err)
	}
	if isErr {
		t.Fatalf("isErr=true output=%q", out)
	}
	if !strings.HasPrefix(out, "deleted ") {
		t.Errorf("output = %q, want prefix 'deleted '", out)
	}
	if len(env.hub.events) != 1 || env.hub.events[0].Data["action"] != "delete" {
		t.Errorf("expected delete event, got %+v", env.hub.events)
	}
	got := env.readSessionFindings(t)
	if strings.Contains(got, `"a":`) {
		t.Errorf("expected key 'a' deleted, findings=%s", got)
	}
}

func TestFindingsAdd_InvalidArgs(t *testing.T) {
	env := newBuiltinTestEnv(t)
	out, isErr, err := invoke(t, env.env, "findings_add", `not-json`)
	if err != nil {
		t.Fatalf("Invoke err: %v", err)
	}
	if !isErr {
		t.Errorf("isErr=false, want true for invalid JSON")
	}
	if !strings.Contains(out, "invalid arguments") {
		t.Errorf("output = %q, want invalid arguments prefix", out)
	}
	if len(env.hub.events) != 0 {
		t.Errorf("expected no broadcasts on validation error, got %+v", env.hub.events)
	}
}

func TestFindingsGet_Layer(t *testing.T) {
	env := newBuiltinTestEnv(t)

	const ts = "2026-01-01T00:00:00Z"
	// Seed two agent_definitions at layer 2: implementor (has a running session with findings)
	// and qa-verifier (sibling with no session → should appear as nil in result).
	mustExec(t, env.pool,
		`INSERT INTO agent_definitions (id, project_id, workflow_id, prompt, layer, created_at, updated_at)
		 VALUES (?, ?, ?, '', 2, ?, ?)`,
		testAgentType, testProjectID, testWorkflow, ts, ts)
	mustExec(t, env.pool,
		`INSERT INTO agent_definitions (id, project_id, workflow_id, prompt, layer, created_at, updated_at)
		 VALUES (?, ?, ?, '', 2, ?, ?)`,
		"qa-verifier", testProjectID, testWorkflow, ts, ts)

	// Give the running implementor session some findings.
	mustExec(t, env.pool,
		`UPDATE agent_sessions SET findings = '{"status":"ok","score":9}' WHERE id = ?`,
		testSessionID)

	out, isErr, err := invoke(t, env.env, "findings_get", `{"layer": 2}`)
	if err != nil {
		t.Fatalf("Invoke err: %v", err)
	}
	if isErr {
		t.Fatalf("isErr=true, output=%q", out)
	}

	var result map[string]interface{}
	if jsonErr := json.Unmarshal([]byte(out), &result); jsonErr != nil {
		t.Fatalf("unmarshal result: %v, raw=%q", jsonErr, out)
	}

	if len(result) != 2 {
		t.Fatalf("expected 2 entries in layer map, got %d: %v", len(result), result)
	}

	implF, ok := result[testAgentType]
	if !ok {
		t.Fatalf("result missing key %q", testAgentType)
	}
	if implF == nil {
		t.Errorf("%q findings should not be nil (running session with findings)", testAgentType)
	} else if fm, fok := implF.(map[string]interface{}); !fok {
		t.Errorf("%q value should be map, got %T", testAgentType, implF)
	} else if fm["status"] != "ok" {
		t.Errorf("%q[status] = %v, want \"ok\"", testAgentType, fm["status"])
	}

	siblingV, ok := result["qa-verifier"]
	if !ok {
		t.Error("result missing key \"qa-verifier\"")
	}
	if siblingV != nil {
		t.Errorf("\"qa-verifier\" should be nil (no session), got %v", siblingV)
	}

	// Layer reads do not broadcast events.
	if len(env.hub.events) != 0 {
		t.Errorf("expected no broadcasts for layer read, got %d events", len(env.hub.events))
	}
}

func TestFindingsGet_AgentTypeAndLayerMutuallyExclusive(t *testing.T) {
	env := newBuiltinTestEnv(t)

	out, isErr, err := invoke(t, env.env, "findings_get", `{"agent_type":"implementor","layer":1}`)
	if err != nil {
		t.Fatalf("Invoke err: %v", err)
	}
	if !isErr {
		t.Errorf("isErr=false, want true for agent_type+layer combination")
	}
	if !strings.Contains(out, "mutually exclusive") {
		t.Errorf("output = %q, want \"mutually exclusive\" substring", out)
	}
	if len(env.hub.events) != 0 {
		t.Errorf("expected no broadcasts for validation error, got %d events", len(env.hub.events))
	}
}
