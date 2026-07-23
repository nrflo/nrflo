//go:build clitools

package spawner

import (
	"bytes"
	"context"
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
// codex_delegation.go (codexAgentsArgs): a renamed or removed tool/config key
// would otherwise fail silently (claude: an unknown deny name is ignored
// without warning; codex: a stale -c override is a no-op). Codex-specific
// drift alarms live in native_orchestration_codex_cli_test.go (split for the
// 300-line file cap).

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
