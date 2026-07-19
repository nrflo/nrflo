package tools_builtin

import (
	"errors"
	"strings"
	"testing"
)

var errBoom = errors.New("safety script exploded")

func TestFSTools_Bash_TimeoutMSRespected(t *testing.T) {
	env := fsEnv(t)
	// A command that outlives a 100ms timeout must be reported as timed out,
	// not run to completion.
	out, isErr := invokeFS(t, "bash", env, `{"command":"sleep 5","timeout_ms":100}`)
	if !isErr || !strings.Contains(out, "timed out") {
		t.Errorf("bash with tight timeout_ms = (%q, %v), want timeout error", out, isErr)
	}
}

func TestFSTools_Bash_TimeoutMSCappedAtMax(t *testing.T) {
	env := fsEnv(t)
	// A timeout_ms above the 600000 cap must not error; it's silently capped,
	// so a fast command still completes normally.
	out, isErr := invokeFS(t, "bash", env, `{"command":"echo ok","timeout_ms":999999999}`)
	if isErr || !strings.Contains(out, "ok") {
		t.Errorf("bash with oversized timeout_ms = (%q, %v), want success", out, isErr)
	}
}

func TestFSTools_Bash_SafetyCheckBlocks(t *testing.T) {
	env := fsEnv(t)
	env.SafetyCheck = func(command string) (bool, string, error) {
		return false, "blocked by policy: " + command, nil
	}
	out, isErr := invokeFS(t, "bash", env, `{"command":"rm -rf /tmp/whatever"}`)
	if !isErr || !strings.Contains(out, "blocked by policy") {
		t.Errorf("bash with blocking SafetyCheck = (%q, %v), want blocked-by-policy error", out, isErr)
	}
}

func TestFSTools_Bash_SafetyCheckAllows(t *testing.T) {
	env := fsEnv(t)
	called := false
	env.SafetyCheck = func(command string) (bool, string, error) {
		called = true
		return true, "", nil
	}
	out, isErr := invokeFS(t, "bash", env, `{"command":"echo safe"}`)
	if isErr || !strings.Contains(out, "safe") {
		t.Errorf("bash with allowing SafetyCheck = (%q, %v), want success", out, isErr)
	}
	if !called {
		t.Error("SafetyCheck was never invoked")
	}
}

func TestFSTools_Bash_SafetyCheckErrorSurfacesAsToolError(t *testing.T) {
	env := fsEnv(t)
	env.SafetyCheck = func(command string) (bool, string, error) {
		return false, "", errBoom
	}
	out, isErr := invokeFS(t, "bash", env, `{"command":"echo hi"}`)
	if !isErr || !strings.Contains(out, errBoom.Error()) {
		t.Errorf("bash with erroring SafetyCheck = (%q, %v), want isError tool result (not a Go error)", out, isErr)
	}
}

func TestFSTools_Bash_NilSafetyCheckRuns(t *testing.T) {
	env := fsEnv(t)
	env.SafetyCheck = nil
	out, isErr := invokeFS(t, "bash", env, `{"command":"echo unrestricted"}`)
	if isErr || !strings.Contains(out, "unrestricted") {
		t.Errorf("bash with nil SafetyCheck = (%q, %v), want success", out, isErr)
	}
}
