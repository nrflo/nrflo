package spawner

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"be/internal/spawner/apirun"
	"be/internal/spawner/apirun/tools_builtin"
)

const safetyGateBlockedToken = "BLOCKED_TOKEN"

// writeSafetyScriptFixture writes a shell-parse safety-check script (no jq
// dependency, unlike CheckSafetyHook's generated hook) implementing the
// shared PreToolUse contract: read {"tool_name":...,"tool_input":{"command":
// ...}} from stdin, block (exit 2, reason on stderr) if the command contains
// safetyGateBlockedToken, else allow (exit 0).
func writeSafetyScriptFixture(t *testing.T) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "safety-check.sh")
	script := "#!/bin/sh\n" +
		"INPUT=$(cat)\n" +
		`CMD=$(echo "$INPUT" | sed -E 's/.*"command":"([^"]*)".*/\1/')` + "\n" +
		`case "$CMD" in` + "\n" +
		`  *` + safetyGateBlockedToken + `*) echo "Blocked: command contains ` + safetyGateBlockedToken + `" >&2; exit 2;;` + "\n" +
		`esac` + "\n" +
		"exit 0\n"
	if err := os.WriteFile(path, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	return path
}

// TestSafetyGate_SharedContract_BridgeExecPath drives a blocked and an
// allowed bash call through the real bridge path: bash's Invoke calling
// env.SafetyCheck, which execs the safety script via runSafetyScript — the
// exact same helper resolveSafetyCheck wires in production.
func TestSafetyGate_SharedContract_BridgeExecPath(t *testing.T) {
	fixture := writeSafetyScriptFixture(t)
	env := apirun.ToolEnv{
		WorkDir: t.TempDir(),
		FS:      apirun.NewFSSession(),
		SafetyCheck: func(command string) (bool, string, error) {
			return runSafetyScript(fixture, command)
		},
	}

	blocked, _ := json.Marshal(map[string]string{"command": "echo " + safetyGateBlockedToken + " here"})
	out, isErr, err := tools_builtin.FSTools()["bash"].Invoke(context.Background(), env, blocked)
	if err != nil {
		t.Fatalf("bash Invoke returned Go error: %v", err)
	}
	if !isErr || !strings.Contains(out, "Blocked: command contains "+safetyGateBlockedToken) {
		t.Errorf("blocked bash = (%q, %v), want isError with the script's reason", out, isErr)
	}

	allowed, _ := json.Marshal(map[string]string{"command": "echo safe-command"})
	out, isErr, err = tools_builtin.FSTools()["bash"].Invoke(context.Background(), env, allowed)
	if err != nil {
		t.Fatalf("bash Invoke returned Go error: %v", err)
	}
	if isErr || !strings.Contains(out, "safe-command") {
		t.Errorf("allowed bash = (%q, %v), want the command to actually run", out, isErr)
	}
}

// TestSafetyGate_SharedContract_DryRunHelper exercises runSafetyScript
// directly (the same helper the bridge path above wraps around), then proves
// CheckSafetyHook is built on the identical
// {"tool_name":"Bash","tool_input":{"command":...}} stdin contract: a
// DangerousPatterns rule matching the same token blocks the same command —
// only possible if CheckSafetyHook's generated hook script also receives
// tool_input.command on stdin exactly like runSafetyScript's target script
// does.
func TestSafetyGate_SharedContract_DryRunHelper(t *testing.T) {
	fixture := writeSafetyScriptFixture(t)
	command := "echo " + safetyGateBlockedToken + " here"

	allowed, reason, err := runSafetyScript(fixture, command)
	if err != nil {
		t.Fatalf("runSafetyScript error: %v", err)
	}
	if allowed || !strings.Contains(reason, safetyGateBlockedToken) {
		t.Fatalf("runSafetyScript(%q) = (%v, %q), want blocked with token in reason", command, allowed, reason)
	}

	allowedCmd := "echo safe-command"
	allowed, reason, err = runSafetyScript(fixture, allowedCmd)
	if err != nil {
		t.Fatalf("runSafetyScript error: %v", err)
	}
	if !allowed || reason != "" {
		t.Errorf("runSafetyScript(%q) = (%v, %q), want allowed with empty reason", allowedCmd, allowed, reason)
	}

	skipIfJqMissing(t)
	cfg := SafetyHookConfig{Enabled: true, AllowGit: true, DangerousPatterns: []string{safetyGateBlockedToken}}
	hookAllowed, hookReason, err := CheckSafetyHook(cfg, command)
	if err != nil {
		t.Fatalf("CheckSafetyHook error: %v", err)
	}
	if hookAllowed || hookReason == "" {
		t.Errorf("CheckSafetyHook(%q) = (%v, %q), want blocked — proves CheckSafetyHook's stdin carries tool_input.command identically to runSafetyScript's contract", command, hookAllowed, hookReason)
	}
}

// TestSafetyGate_SharedContract_NonBlockExitIsError covers runSafetyScript's
// non-block error path: a script that exits non-0/non-2 must surface as a Go
// error (bash.Invoke then reports it as isError, never turn-fatal — see
// fs_bash.go), not as a silent allow.
func TestSafetyGate_SharedContract_NonBlockExitIsError(t *testing.T) {
	path := filepath.Join(t.TempDir(), "explode.sh")
	if err := os.WriteFile(path, []byte("#!/bin/sh\nexit 1\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	allowed, reason, err := runSafetyScript(path, "echo hi")
	if err == nil {
		t.Fatalf("runSafetyScript with exit 1 = (%v, %q, nil), want a non-nil error", allowed, reason)
	}
	if allowed {
		t.Errorf("runSafetyScript with exit 1 = allowed=true, want false")
	}
}
