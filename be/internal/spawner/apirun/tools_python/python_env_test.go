package tools_python

import (
	"context"
	"strings"
	"testing"

	"be/internal/spawner/apirun"
)

// envMap collapses an env slice into a last-value-wins map, matching os/exec
// semantics (later entries override earlier ones for the same key).
func envMap(kvs []string) map[string]string {
	m := make(map[string]string, len(kvs))
	for _, kv := range kvs {
		if i := strings.IndexByte(kv, '='); i >= 0 {
			m[kv[:i]] = kv[i+1:]
		}
	}
	return m
}

// buildEnv must mirror spawner.prepareScriptSpawn: inherit the server env (so the
// SDK socket resolves via NRFLO_HOME/HOME), strip CLAUDECODE, export NRFLO_SDK_DIR
// (so the SDK imports), override identity vars, and let projectEnv win last.
func TestPython_BuildEnvMirrorsScriptSpawn(t *testing.T) {
	t.Setenv("CLAUDECODE", "1")
	t.Setenv("NRFLO_HOME", "/fake/home")
	t.Setenv("NRFLO_PROJECT", "server-proj")
	t.Setenv("MYVAR", "fromserver")

	h := New(pythonRow("t", "", "", 0), "", "/sdk/path", []string{"MYVAR=fromproj"})
	env := apirun.ToolEnv{ProjectID: "proj-1", SessionID: "sess-1", WorkflowInstanceID: "wfi-1"}

	m := envMap(h.buildEnv(context.Background(), env))

	if _, ok := m["CLAUDECODE"]; ok {
		t.Error("CLAUDECODE should be stripped from inherited env")
	}
	if m["NRFLO_HOME"] != "/fake/home" {
		t.Errorf("NRFLO_HOME = %q, want inherited /fake/home (socket resolution)", m["NRFLO_HOME"])
	}
	if m["NRFLO_SDK_DIR"] != "/sdk/path" {
		t.Errorf("NRFLO_SDK_DIR = %q, want /sdk/path", m["NRFLO_SDK_DIR"])
	}
	if m["NRFLO_PROJECT"] != "proj-1" {
		t.Errorf("NRFLO_PROJECT = %q, want identity override proj-1", m["NRFLO_PROJECT"])
	}
	if m["NRF_SESSION_ID"] != "sess-1" {
		t.Errorf("NRF_SESSION_ID = %q, want sess-1", m["NRF_SESSION_ID"])
	}
	if m["NRF_WORKFLOW_INSTANCE_ID"] != "wfi-1" {
		t.Errorf("NRF_WORKFLOW_INSTANCE_ID = %q, want wfi-1", m["NRF_WORKFLOW_INSTANCE_ID"])
	}
	if m["NRF_SPAWNED"] != "1" {
		t.Errorf("NRF_SPAWNED = %q, want 1", m["NRF_SPAWNED"])
	}
	if m["MYVAR"] != "fromproj" {
		t.Errorf("MYVAR = %q, want projectEnv last-wins fromproj", m["MYVAR"])
	}
}

func TestPython_BuildEnvNoSDKDirWhenEmpty(t *testing.T) {
	h := New(pythonRow("t", "", "", 0), "", "", nil)
	m := envMap(h.buildEnv(context.Background(), apirun.ToolEnv{ProjectID: "p"}))
	if _, ok := m["NRFLO_SDK_DIR"]; ok {
		t.Error("NRFLO_SDK_DIR should be absent when sdkDir is empty")
	}
}
