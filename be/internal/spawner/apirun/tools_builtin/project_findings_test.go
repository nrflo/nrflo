package tools_builtin

import (
	"strings"
	"testing"

	"be/internal/ws"
)

func TestProjectFindingsAdd_PersistsAndBroadcasts(t *testing.T) {
	env := newBuiltinTestEnv(t)
	out, isErr, err := invoke(t, env.env, "project_findings_add", `{"key":"arch","value":"clean"}`)
	if err != nil {
		t.Fatalf("Invoke: %v", err)
	}
	if isErr || out != "ok" {
		t.Fatalf("output=%q isErr=%v want ok", out, isErr)
	}
	var stored string
	row := env.pool.QueryRow(
		`SELECT value FROM project_findings WHERE project_id = ? AND key = ?`,
		testProjectID, "arch")
	if err := row.Scan(&stored); err != nil {
		t.Fatalf("read project_findings: %v", err)
	}
	if !strings.Contains(stored, "clean") {
		t.Errorf("stored = %s, want contains 'clean'", stored)
	}
	if len(env.hub.events) != 1 || env.hub.events[0].Type != ws.EventProjectFindingsUpdated {
		t.Errorf("expected single project_findings.updated event, got %+v", env.hub.events)
	}
}

func TestProjectFindingsAddBulk_PersistsAndBroadcasts(t *testing.T) {
	env := newBuiltinTestEnv(t)
	_, _, err := invoke(t, env.env, "project_findings_add_bulk", `{"key_values":{"a":"1","b":"2"}}`)
	if err != nil {
		t.Fatalf("Invoke: %v", err)
	}
	rows, err := env.pool.Query(`SELECT key FROM project_findings WHERE project_id = ?`, testProjectID)
	if err != nil {
		t.Fatalf("query: %v", err)
	}
	defer rows.Close()
	got := map[string]bool{}
	for rows.Next() {
		var k string
		_ = rows.Scan(&k)
		got[k] = true
	}
	if !got["a"] || !got["b"] {
		t.Errorf("expected keys a,b stored, got %v", got)
	}
	if env.hub.events[0].Data["action"] != "add-bulk" {
		t.Errorf("action = %v, want add-bulk", env.hub.events[0].Data["action"])
	}
}

func TestProjectFindingsAppend_PersistsAndBroadcasts(t *testing.T) {
	env := newBuiltinTestEnv(t)
	if _, _, err := invoke(t, env.env, "project_findings_add", `{"key":"items","value":"first"}`); err != nil {
		t.Fatalf("add: %v", err)
	}
	env.hub.events = nil
	if _, _, err := invoke(t, env.env, "project_findings_append", `{"key":"items","value":"second"}`); err != nil {
		t.Fatalf("append: %v", err)
	}
	var stored string
	if err := env.pool.QueryRow(
		`SELECT value FROM project_findings WHERE project_id = ? AND key = ?`,
		testProjectID, "items").Scan(&stored); err != nil {
		t.Fatalf("read: %v", err)
	}
	if !strings.Contains(stored, "first") || !strings.Contains(stored, "second") {
		t.Errorf("stored = %s, want both first and second", stored)
	}
	if len(env.hub.events) != 1 || env.hub.events[0].Data["action"] != "append" {
		t.Errorf("expected append event, got %+v", env.hub.events)
	}
}

func TestProjectFindingsGet_AllKeys(t *testing.T) {
	env := newBuiltinTestEnv(t)
	if _, _, err := invoke(t, env.env, "project_findings_add", `{"key":"k1","value":"v1"}`); err != nil {
		t.Fatalf("add: %v", err)
	}
	out, isErr, err := invoke(t, env.env, "project_findings_get", `{"key":"k1"}`)
	if err != nil {
		t.Fatalf("Invoke: %v", err)
	}
	if isErr {
		t.Fatalf("isErr=true output=%q", out)
	}
	if !strings.Contains(out, "v1") {
		t.Errorf("output = %q, want contains v1", out)
	}
}

func TestProjectFindingsDelete_PersistsAndBroadcasts(t *testing.T) {
	env := newBuiltinTestEnv(t)
	if _, _, err := invoke(t, env.env, "project_findings_add", `{"key":"k","value":"v"}`); err != nil {
		t.Fatalf("add: %v", err)
	}
	env.hub.events = nil
	out, isErr, err := invoke(t, env.env, "project_findings_delete", `{"keys":["k"]}`)
	if err != nil {
		t.Fatalf("Invoke: %v", err)
	}
	if isErr {
		t.Fatalf("isErr=true output=%q", out)
	}
	if !strings.HasPrefix(out, "deleted ") {
		t.Errorf("output = %q, want prefix 'deleted '", out)
	}
	var count int
	_ = env.pool.QueryRow(
		`SELECT COUNT(*) FROM project_findings WHERE project_id = ? AND key = ?`,
		testProjectID, "k").Scan(&count)
	if count != 0 {
		t.Errorf("expected 0 rows after delete, got %d", count)
	}
	if len(env.hub.events) != 1 || env.hub.events[0].Data["action"] != "delete" {
		t.Errorf("expected delete event, got %+v", env.hub.events)
	}
}

func TestProjectFindingsAppendBulk(t *testing.T) {
	env := newBuiltinTestEnv(t)
	_, _, err := invoke(t, env.env, "project_findings_append_bulk", `{"key_values":{"x":"a","y":"b"}}`)
	if err != nil {
		t.Fatalf("Invoke: %v", err)
	}
	if env.hub.events[0].Data["action"] != "append-bulk" {
		t.Errorf("action = %v, want append-bulk", env.hub.events[0].Data["action"])
	}
}
