package tools_builtin

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"be/internal/clock"
)

// bgShellIDFromOutput extracts the shell_id from bash's
// "started background shell <id>" success message.
func bgShellIDFromOutput(t *testing.T, out string) string {
	t.Helper()
	const prefix = "started background shell "
	if !strings.HasPrefix(out, prefix) {
		t.Fatalf("bash run_in_background output = %q, want %q prefix", out, prefix)
	}
	return strings.TrimSpace(strings.TrimPrefix(out, prefix))
}

func TestFSTools_BashBG_RoundTrip(t *testing.T) {
	env := fsEnv(t)

	out, isErr := invokeFS(t, "bash", env, `{"command":"echo hello","run_in_background":true}`)
	if isErr {
		t.Fatalf("bash run_in_background = (%q, %v)", out, isErr)
	}
	id := bgShellIDFromOutput(t, out)

	sh, ok := env.FS.GetShell(id)
	if !ok {
		t.Fatalf("shell %q not registered on FS session", id)
	}
	select {
	case <-sh.Done():
	case <-time.After(5 * time.Second):
		t.Fatal("background shell did not finish")
	}

	args, _ := jsonArgs(map[string]any{"shell_id": id})
	out, isErr = invokeFS(t, "bash_output", env, args)
	if isErr {
		t.Fatalf("bash_output = (%q, %v)", out, isErr)
	}
	if !strings.Contains(out, "status: completed") || !strings.Contains(out, "exit_code: 0") || !strings.Contains(out, "hello") {
		t.Errorf("bash_output = %q, want completed/exit 0/hello", out)
	}

	// A second poll returns no new output (already consumed).
	out, isErr = invokeFS(t, "bash_output", env, args)
	if isErr || !strings.Contains(out, "no new output") {
		t.Errorf("second bash_output = (%q, %v), want no-new-output", out, isErr)
	}
}

func TestFSTools_BashBG_StartedAtUsesInjectedClock(t *testing.T) {
	env := fsEnv(t)
	fixed := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)
	env.Clock = clock.NewTest(fixed)

	out, isErr := invokeFS(t, "bash", env, `{"command":"echo hi","run_in_background":true}`)
	if isErr {
		t.Fatalf("bash run_in_background = (%q, %v)", out, isErr)
	}
	id := bgShellIDFromOutput(t, out)
	sh, ok := env.FS.GetShell(id)
	if !ok {
		t.Fatalf("shell %q not registered", id)
	}
	if !sh.StartedAt.Equal(fixed) {
		t.Errorf("StartedAt = %v, want %v (from injected clock)", sh.StartedAt, fixed)
	}
	<-sh.Done()
}

func TestFSTools_BashBG_KillRunningShell(t *testing.T) {
	env := fsEnv(t)
	out, isErr := invokeFS(t, "bash", env, `{"command":"sleep 30","run_in_background":true}`)
	if isErr {
		t.Fatalf("bash run_in_background = (%q, %v)", out, isErr)
	}
	id := bgShellIDFromOutput(t, out)
	sh, ok := env.FS.GetShell(id)
	if !ok {
		t.Fatalf("shell %q not registered", id)
	}

	args, _ := jsonArgs(map[string]any{"shell_id": id})
	out, isErr = invokeFS(t, "kill_shell", env, args)
	if isErr || !strings.Contains(out, "killed "+id) {
		t.Fatalf("kill_shell = (%q, %v)", out, isErr)
	}

	select {
	case <-sh.Done():
	case <-time.After(5 * time.Second):
		t.Fatal("killed shell did not finish")
	}
}

func TestFSTools_BashBG_KillOnFinishedShellIsNoop(t *testing.T) {
	env := fsEnv(t)
	out, isErr := invokeFS(t, "bash", env, `{"command":"echo done","run_in_background":true}`)
	if isErr {
		t.Fatalf("bash run_in_background = (%q, %v)", out, isErr)
	}
	id := bgShellIDFromOutput(t, out)
	sh, _ := env.FS.GetShell(id)
	<-sh.Done()

	args, _ := jsonArgs(map[string]any{"shell_id": id})
	// Killing an already-finished shell, and killing it a second time, must
	// both succeed without error.
	for i := 0; i < 2; i++ {
		out, isErr = invokeFS(t, "kill_shell", env, args)
		if isErr || !strings.Contains(out, "killed "+id) {
			t.Errorf("kill_shell[%d] on finished shell = (%q, %v), want success no-op", i, out, isErr)
		}
	}
}

func TestFSTools_BashBG_KillAllReaps(t *testing.T) {
	env := fsEnv(t)
	out, isErr := invokeFS(t, "bash", env, `{"command":"sleep 30","run_in_background":true}`)
	if isErr {
		t.Fatalf("bash run_in_background = (%q, %v)", out, isErr)
	}
	id := bgShellIDFromOutput(t, out)
	sh, _ := env.FS.GetShell(id)

	env.FS.KillAll()

	select {
	case <-sh.Done():
	case <-time.After(5 * time.Second):
		t.Fatal("KillAll did not reap the running shell")
	}
}

func TestFSTools_BashBG_NilFSErrorsCleanly(t *testing.T) {
	env := fsEnv(t)
	env.FS = nil

	if out, isErr := invokeFS(t, "bash", env, `{"command":"echo hi","run_in_background":true}`); !isErr || !strings.Contains(out, "not available") {
		t.Errorf("bash run_in_background with nil FS = (%q, %v), want clean error", out, isErr)
	}
	if out, isErr := invokeFS(t, "bash_output", env, `{"shell_id":"bg_1"}`); !isErr || !strings.Contains(out, "not available") {
		t.Errorf("bash_output with nil FS = (%q, %v), want clean error", out, isErr)
	}
	if out, isErr := invokeFS(t, "kill_shell", env, `{"shell_id":"bg_1"}`); !isErr || !strings.Contains(out, "not available") {
		t.Errorf("kill_shell with nil FS = (%q, %v), want clean error", out, isErr)
	}
}

func TestFSTools_BashBG_UnknownShellID(t *testing.T) {
	env := fsEnv(t)
	if out, isErr := invokeFS(t, "bash_output", env, `{"shell_id":"bg_missing"}`); !isErr || !strings.Contains(out, "unknown shell_id") {
		t.Errorf("bash_output unknown id = (%q, %v), want unknown-shell error", out, isErr)
	}
	if out, isErr := invokeFS(t, "kill_shell", env, `{"shell_id":"bg_missing"}`); !isErr || !strings.Contains(out, "unknown shell_id") {
		t.Errorf("kill_shell unknown id = (%q, %v), want unknown-shell error", out, isErr)
	}
}

// jsonArgs is a tiny helper to build tool input JSON from a map, avoiding
// manual string formatting for shell_id-keyed calls.
func jsonArgs(m map[string]any) (string, error) {
	b, err := json.Marshal(m)
	return string(b), err
}
