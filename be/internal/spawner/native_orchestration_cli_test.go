//go:build clitools

package spawner

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"os/exec"
	"strings"
	"testing"
	"time"
)

// This file spawns real `claude`/`codex` binaries and is excluded from
// `make test` (no build tags) by design — CLAUDE.md rule 4 forbids real CLI
// execution in the default suite and caps it at 60s. Run deliberately after
// any CLAUDE_VERSION / codex version bump:
//
//	go test -tags clitools ./internal/spawner/ -run NativeOrchestration -v
//
// It is the version-drift alarm for the deny lists in
// cli_adapter_claude.go (claudeDisallowedNativeTools) and
// codex_appserver_client.go (codexDisabledFeatures): a renamed or removed
// tool/feature name would otherwise fail silently (claude: an unknown deny
// name is ignored without warning; codex: a stale --disable flag is a no-op).

// listToolsPrompt asks the CLI to enumerate its live tool registry. Enumerating
// and diffing the registry is deterministic in a way that asking the model
// "can you delegate?" is not — the answer is a list of names, not a judgement.
const listToolsPrompt = "List the exact names of every tool currently available to you, one per line, nothing else."

// claudeToolRegistry runs a real claude CLI with the given extra args and
// returns the set of tool names it reports.
func claudeToolRegistry(t *testing.T, extraArgs ...string) map[string]bool {
	t.Helper()

	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()

	args := append([]string{"--print", "--dangerously-skip-permissions"}, extraArgs...)
	cmd := exec.CommandContext(ctx, "claude", args...)
	// Prompt goes over stdin, never positionally: --disallowedTools is variadic
	// and would swallow a trailing positional prompt as a denied tool name.
	cmd.Stdin = strings.NewReader(listToolsPrompt)
	cmd.Env = append(cmd.Environ(), "DISABLE_AUTOUPDATER=1")

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		t.Fatalf("claude --print %v: %v\nstderr:\n%s", extraArgs, err, stderr.String())
	}

	tools := map[string]bool{}
	for _, line := range strings.Split(stdout.String(), "\n") {
		if name := strings.TrimSpace(line); name != "" {
			tools[name] = true
		}
	}
	if len(tools) == 0 {
		t.Fatalf("claude reported no tools at all; stdout:\n%s", stdout.String())
	}
	return tools
}

// TestNativeOrchestrationCLI_ClaudeDenyBlocksDelegation diffs claude's live
// tool registry with and without the production deny list.
//
// The baseline assertion is the actual drift alarm: if a future CLI renames the
// delegation tool (it was "Task" in older versions, "Agent" as of 2.1.178 and
// 2.1.207), "Agent" vanishes from the *undenied* registry and this fails loudly
// — whereas the deny-side assertion alone would still pass, vacuously.
func TestNativeOrchestrationCLI_ClaudeDenyBlocksDelegation(t *testing.T) {
	baseline := claudeToolRegistry(t)

	// Drift alarm: every name we deny that is meant to exist must actually
	// exist in this CLI version. "Task" is deliberately excluded — it is the
	// legacy pre-rename name, kept in the deny list for older CLIs and absent
	// from current ones.
	for _, tool := range []string{"Agent", "Workflow"} {
		if !baseline[tool] {
			t.Errorf("claudeDisallowedNativeTools has drifted stale: %q is not in this CLI's tool registry — the native delegation tool was likely renamed; update the deny list", tool)
		}
	}

	denied := claudeToolRegistry(t, "--disallowedTools", claudeDisallowedNativeTools)

	// The deny actually takes effect: delegation primitives are gone.
	for _, tool := range strings.Fields(claudeDisallowedNativeTools) {
		if denied[tool] {
			t.Errorf("tool %q survived --disallowedTools %q — a managed session could still spawn children invisible to nrflo", tool, claudeDisallowedNativeTools)
		}
	}

	// No collateral damage: deny matching is by exact name, so denying "Task"
	// must not take out the unrelated Task* background-task tools, and ordinary
	// coding tools must survive.
	for _, tool := range []string{"Bash", "Edit", "Read", "Write"} {
		if baseline[tool] && !denied[tool] {
			t.Errorf("coding tool %q was collaterally denied by --disallowedTools %q", tool, claudeDisallowedNativeTools)
		}
	}
	for _, tool := range []string{"TaskCreate", "TaskGet", "TaskList", "TaskOutput", "TaskStop", "TaskUpdate"} {
		if baseline[tool] && !denied[tool] {
			t.Errorf("denying %q prefix-matched and collaterally denied %q; the deny list must match exact tool names only", "Task", tool)
		}
	}
}

// TestNativeOrchestrationCLI_CodexDisableFlagsTakeEffect asserts the production
// --disable flags actually flip multi_agent off. `codex features list` prints
// each feature's stage and *effective* state, so this reads the real resolved
// state rather than trusting that the flag parsed.
//
// multi_agent is stage=stable and default-ON (codex 0.144.1), which is exactly
// why stripping the user's config.toml would not have been enough.
func TestNativeOrchestrationCLI_CodexDisableFlagsTakeEffect(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// `--disable` is documented as equivalent to `-c features.<name>=false` and
	// is accepted by every codex subcommand, so `features list` resolves the
	// same state the app-server will see for the same flags.
	args := append([]string{"features", "list"}, disableFeatureFlags()...)
	out, err := exec.CommandContext(ctx, "codex", args...).CombinedOutput()
	if err != nil {
		t.Fatalf("codex %v: %v\noutput:\n%s", args, err, out)
	}

	for _, feature := range codexDisabledFeatures {
		state, ok := codexFeatureState(string(out), feature)
		if !ok {
			t.Errorf("codexDisabledFeatures has drifted stale: %q is not a known codex feature — a --disable for it is a silent no-op; update the list", feature)
			continue
		}
		if state != "false" {
			t.Errorf("feature %q is still %q after --disable — a managed codex session could spawn children invisible to nrflo:\n%s", feature, state, out)
		}
	}
}

// codexFeatureState finds `<feature> <stage> <state>` in `codex features list`
// output, returning the trailing effective state.
func codexFeatureState(out, feature string) (string, bool) {
	for _, line := range strings.Split(out, "\n") {
		fields := strings.Fields(line)
		if len(fields) >= 2 && fields[0] == feature {
			return fields[len(fields)-1], true
		}
	}
	return "", false
}

// disableFeatureFlags returns just the `--disable <feature>` pairs, built
// directly from codexDisabledFeatures rather than sliced out of
// appServerArgs() — appServerArgs() now also carries the unrelated -c
// project-doc override, which `codex features list` does not accept as a
// feature-gating flag.
func disableFeatureFlags() []string {
	args := make([]string, 0, len(codexDisabledFeatures)*2)
	for _, f := range codexDisabledFeatures {
		args = append(args, "--disable", f)
	}
	return args
}

// TestNativeOrchestrationCLI_CodexAppServerAcceptsDisableFlags spawns a real
// `codex app-server` with the production appServerArgs() and asserts the same
// `initialize` handshake production performs still round-trips — guarding
// against codex rejecting a --disable flag outright at startup.
func TestNativeOrchestrationCLI_CodexAppServerAcceptsDisableFlags(t *testing.T) {
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
	// like a rejected --disable flag.
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
		t.Fatalf("initialize returned an error (a codexDisabledFeatures entry may be stale/rejected): code=%d message=%s\nstderr:\n%s", resp.Error.Code, resp.Error.Message, stderr.String())
	}
	if resp.Result == nil {
		t.Fatalf("initialize returned no result\nstderr:\n%s", stderr.String())
	}
}
