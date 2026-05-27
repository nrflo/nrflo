package tools_builtin

import (
	"strings"
	"testing"

	"be/internal/ws"
)

const emitBuiltinSchema = `[{"key":"security_issues","schema":{"type":"array","items":{"type":"object","properties":{"file":{"type":"string"},"severity":{"type":"string"}},"required":["file","severity"]}},"example":[{"file":"a.go","severity":"high"}]}]`

func setEmitSchema(t *testing.T, env *builtinTestEnv) {
	t.Helper()
	mustExec(t, env.pool, `UPDATE workflows SET finding_schemas=? WHERE id=? AND project_id=?`,
		emitBuiltinSchema, testWorkflow, testProjectID)
}

func TestEmitFindings_ValidPersistsAndBroadcasts(t *testing.T) {
	env := newBuiltinTestEnv(t)
	setEmitSchema(t, env)

	out, isErr, err := invoke(t, env.env, "emit_findings", `{"key":"security_issues","value":[{"file":"a.go","severity":"high"}]}`)
	if err != nil {
		t.Fatalf("Invoke err: %v", err)
	}
	if isErr {
		t.Fatalf("isErr=true, output=%q", out)
	}
	if !strings.Contains(env.readSessionFindings(t), "a.go") {
		t.Errorf("finding not stored: %s", env.readSessionFindings(t))
	}
	if len(env.hub.events) != 1 || env.hub.events[0].Type != ws.EventFindingsUpdated || env.hub.events[0].Data["action"] != "emit" {
		t.Fatalf("expected one emit broadcast, got %+v", env.hub.events)
	}
}

func TestEmitFindings_InvalidReturnsExample(t *testing.T) {
	env := newBuiltinTestEnv(t)
	setEmitSchema(t, env)

	out, isErr, err := invoke(t, env.env, "emit_findings", `{"key":"security_issues","value":[{"file":"a.go"}]}`)
	if err != nil {
		t.Fatalf("Invoke err: %v", err)
	}
	if !isErr {
		t.Fatalf("expected isErr=true, got ok: %s", out)
	}
	if !strings.Contains(out, "Expected structure example") {
		t.Errorf("output missing example: %s", out)
	}
	if len(env.hub.events) != 0 {
		t.Errorf("should not broadcast on validation failure: %+v", env.hub.events)
	}
}

func TestEmitFindings_ObjectValuePersists(t *testing.T) {
	env := newBuiltinTestEnv(t)
	// Object-typed schema (not an array): the tool's input schema no longer
	// constrains value to arrays, so the registered schema is the sole contract.
	mustExec(t, env.pool, `UPDATE workflows SET finding_schemas=? WHERE id=? AND project_id=?`,
		`[{"key":"owner_match","schema":{"type":"object","properties":{"result":{"type":"string","enum":["match","mismatch"]}},"required":["result"]},"example":{"result":"match"}}]`,
		testWorkflow, testProjectID)

	out, isErr, err := invoke(t, env.env, "emit_findings", `{"key":"owner_match","value":{"result":"match"}}`)
	if err != nil {
		t.Fatalf("Invoke err: %v", err)
	}
	if isErr {
		t.Fatalf("isErr=true, output=%q", out)
	}
	if !strings.Contains(env.readSessionFindings(t), "match") {
		t.Errorf("object finding not stored: %s", env.readSessionFindings(t))
	}
}

func TestEmitFindings_UnknownKey(t *testing.T) {
	env := newBuiltinTestEnv(t)
	setEmitSchema(t, env)

	out, isErr, _ := invoke(t, env.env, "emit_findings", `{"key":"notes","value":[]}`)
	if !isErr {
		t.Fatalf("expected isErr=true: %s", out)
	}
	if !strings.Contains(out, "no schema defined") || !strings.Contains(out, "security_issues") {
		t.Errorf("unexpected message: %s", out)
	}
}
