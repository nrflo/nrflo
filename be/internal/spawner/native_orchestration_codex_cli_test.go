//go:build clitools

package spawner

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// Codex-specific drift alarms, split out of native_orchestration_cli_test.go
// for the 300-line file cap. See that file's header for the shared
// listToolsPrompt constant and the CLAUDE.md rule 4 rationale (clitools-only,
// never in `make test`).

// codexToolRegistry runs a real `codex exec` with the given extra -c/args and
// returns the set of tool names it reports, using an isolated CODEX_HOME with
// the user's auth.json copied in (no network login needed) and no git-repo
// requirement via --skip-git-repo-check (codex refuses to run outside a
// trusted git dir otherwise). model_reasoning_effort must be "none" — gpt-5.6
// rejects "minimal".
func codexToolRegistry(t *testing.T, extraArgs ...string) map[string]bool {
	t.Helper()

	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()

	codexHome := t.TempDir()
	userHome := userCodexHome()
	if authBytes, err := os.ReadFile(filepath.Join(userHome, "auth.json")); err == nil {
		if err := os.WriteFile(filepath.Join(codexHome, "auth.json"), authBytes, 0o600); err != nil {
			t.Fatalf("copy auth.json: %v", err)
		}
	}

	args := append([]string{"exec", "--skip-git-repo-check", "--sandbox", "read-only", "-c", `model_reasoning_effort="none"`}, extraArgs...)
	args = append(args, listToolsPrompt)
	cmd := exec.CommandContext(ctx, "codex", args...)
	cmd.Env = append(cmd.Environ(), "CODEX_HOME="+codexHome)

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		t.Fatalf("codex %v: %v\nstderr:\n%s", args, err, stderr.String())
	}

	tools := map[string]bool{}
	for _, line := range strings.Split(stdout.String(), "\n") {
		if name := strings.TrimSpace(line); name != "" {
			tools[name] = true
		}
	}
	if len(tools) == 0 {
		t.Fatalf("codex reported no tools at all; stdout:\n%s", stdout.String())
	}
	return tools
}

// TestNativeOrchestrationCLI_CodexPromptInputDropsDelegationTools is the free,
// deterministic stand-in for a live model turn: `codex debug prompt-input`
// renders the exact developer-message payload a real turn would send, no
// network/auth required. This is the baseline drift alarm — it fails loudly
// if codex renames/removes the delegation tool surface, and confirms
// codexAgentsArgs() actually suppresses it.
func TestNativeOrchestrationCLI_CodexPromptInputDropsDelegationTools(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	codexHome := t.TempDir()
	run := func(extraArgs ...string) string {
		args := append([]string{"debug", "prompt-input"}, extraArgs...)
		args = append(args, "hi")
		cmd := exec.CommandContext(ctx, "codex", args...)
		cmd.Env = append(cmd.Environ(), "CODEX_HOME="+codexHome)
		out, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("codex %v: %v\noutput:\n%s", args, err, out)
		}
		return string(out)
	}

	baseline := run()
	if !strings.Contains(baseline, "spawn_agent") {
		t.Fatalf("codexAgentsArgs has drifted stale: baseline prompt-input no longer mentions spawn_agent — the native delegation tool was likely renamed:\n%s", baseline)
	}

	blocked := run(codexAgentsArgs()...)
	if strings.Contains(blocked, "spawn_agent") {
		t.Errorf("prompt-input still mentions spawn_agent with agents.enabled=false applied — delegation is not actually blocked:\n%s", blocked)
	}
}

// TestNativeOrchestrationCLI_CodexAgentsEnabledBlocksDelegationTools is the
// strongest guard: it runs `codex exec` for real and diffs the live tool
// registry with and without codexAgentsArgs(), mirroring the claude
// equivalent in native_orchestration_cli_test.go. Skipped when no codex auth
// is available locally.
func TestNativeOrchestrationCLI_CodexAgentsEnabledBlocksDelegationTools(t *testing.T) {
	userHome := userCodexHome()
	if _, err := os.Stat(filepath.Join(userHome, "auth.json")); err != nil {
		t.Skip("no local codex auth.json; skipping live tool-registry probe")
	}

	baseline := codexToolRegistry(t)
	for _, tool := range []string{"spawn_agent", "followup_task", "send_message", "wait_agent", "interrupt_agent", "list_agents"} {
		if !baseline[tool] {
			t.Errorf("codexAgentsArgs has drifted stale: %q is not in codex's baseline tool registry — update codex_delegation.go", tool)
		}
	}

	blocked := codexToolRegistry(t, codexAgentsArgs()...)
	for _, tool := range []string{"spawn_agent", "followup_task", "send_message", "wait_agent", "interrupt_agent", "list_agents"} {
		if blocked[tool] {
			t.Errorf("tool %q survived -c agents.enabled=false — a managed session could still spawn children invisible to nrflo", tool)
		}
	}
	for _, tool := range []string{"exec", "apply_patch", "update_plan"} {
		if baseline[tool] && !blocked[tool] {
			t.Errorf("coding tool %q was collaterally denied by agents.enabled=false", tool)
		}
	}
}

// TestNativeOrchestrationCLI_CodexAppServerAcceptsArgs spawns a real
// `codex app-server` with the production appServerArgs() and asserts the same
// `initialize` handshake production performs still round-trips — guarding
// against codex rejecting the agents/-c overrides outright at startup.
func TestNativeOrchestrationCLI_CodexAppServerAcceptsArgs(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	cmd := exec.CommandContext(ctx, "codex", appServerArgs()...)
	stdin, err := cmd.StdinPipe()
	if err != nil {
		t.Fatalf("stdin pipe: %v", err)
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		t.Fatalf("stdout pipe: %v", err)
	}
	var stderr bytes.Buffer
	cmd.Stderr = &stderr

	if err := cmd.Start(); err != nil {
		t.Fatalf("start codex app-server: %v", err)
	}
	defer func() {
		_ = cmd.Process.Kill()
		_ = cmd.Wait()
	}()

	// Mirror codex_appserver_backend.go's handshake exactly — clientInfo is a
	// required field, and omitting it yields a -32600 that looks deceptively
	// like a rejected -c override.
	req := map[string]any{
		"jsonrpc": "2.0",
		"id":      1,
		"method":  "initialize",
		"params": map[string]any{
			"clientInfo": map[string]string{"name": appServerClientName, "version": "1"},
		},
	}
	if err := json.NewEncoder(stdin).Encode(req); err != nil {
		t.Fatalf("write initialize request: %v", err)
	}

	line, err := bufio.NewReader(stdout).ReadBytes('\n')
	if err != nil {
		t.Fatalf("read initialize response: %v\nstderr:\n%s", err, stderr.String())
	}

	var resp struct {
		ID     int64           `json:"id"`
		Result json.RawMessage `json:"result"`
		Error  *struct {
			Code    int    `json:"code"`
			Message string `json:"message"`
		} `json:"error"`
	}
	if err := json.Unmarshal(line, &resp); err != nil {
		t.Fatalf("unmarshal initialize response: %v\nline: %s", err, line)
	}
	if resp.Error != nil {
		t.Fatalf("initialize returned an error (an appServerArgs() -c override may be stale/rejected): code=%d message=%s\nstderr:\n%s", resp.Error.Code, resp.Error.Message, stderr.String())
	}
	if resp.Result == nil {
		t.Fatalf("initialize returned no result\nstderr:\n%s", stderr.String())
	}
}

// TestNativeOrchestrationCLI_StrictConfigCatchesRenamedKeys is the drift
// alarm for the -c override key names (auto-compact + agents.enabled):
// `--strict-config` is the only mode where codex actually validates a `-c`
// override against its known ConfigToml fields (a plain `-c` silently
// accepts and ignores an unknown key). CODEX_HOME points at an empty
// t.TempDir() so a developer's own ~/.codex/config.toml (which may itself
// carry unrelated legacy keys) cannot false-fail the strict parse.
// Production must NEVER pass --strict-config — it would hard-fail every
// spawn for a user with any unrecognized key in their own config.toml.
func TestNativeOrchestrationCLI_StrictConfigCatchesRenamedKeys(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	codexHome := t.TempDir()
	// An immediately-EOF stdin lets `codex app-server` run its startup config
	// validation and exit on its own once the JSON-RPC stdio loop hits EOF,
	// instead of blocking on a handshake this test doesn't perform.
	runStrict := func(kv string) (string, error) {
		// --strict-config is an `app-server` subcommand option, not a global
		// codex flag — it must follow "app-server", so it cannot ride
		// appServerArgs() itself (production never passes it).
		args := append([]string{"app-server", "--strict-config", "-c", kv}, appServerArgs()[1:]...)
		cmd := exec.CommandContext(ctx, "codex", args...)
		cmd.Env = append(cmd.Environ(), "CODEX_HOME="+codexHome)
		cmd.Stdin = bytes.NewReader(nil)
		out, err := cmd.CombinedOutput()
		return string(out), err
	}

	// Control: a deliberately bogus key must be rejected under --strict-config
	// (proves the flag is actually enforcing, not just accepted/ignored).
	if out, err := runStrict("bogus_key_xyz=1"); err == nil {
		t.Fatalf("--strict-config -c bogus_key_xyz=1 unexpectedly succeeded (strict validation not enforcing):\n%s", out)
	} else if !strings.Contains(out, "bogus_key_xyz") {
		t.Errorf("expected rejection to mention the unknown key, got:\n%s", out)
	}

	// The actual drift alarm: codexAutoCompactTokenLimit's key must still be
	// recognized. A codex rename of model_auto_compact_token_limit would fail
	// here exactly like the bogus-key control above.
	kv := codexAutoCompactArgs()[1]
	if out, err := runStrict(kv); err != nil {
		t.Fatalf("--strict-config rejected %q — model_auto_compact_token_limit may have been renamed/removed by codex:\n%s\nerr: %v", kv, out, err)
	}

	// Same drift alarm for the delegation-blocking key: a rename/removal of
	// agents.enabled would be silently ignored under a plain -c, but
	// --strict-config catches it here.
	agentsKV := codexAgentsArgs()[1]
	if out, err := runStrict(agentsKV); err != nil {
		t.Fatalf("--strict-config rejected %q — agents.enabled may have been renamed/removed by codex:\n%s\nerr: %v", agentsKV, out, err)
	}
}
