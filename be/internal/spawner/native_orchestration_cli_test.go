//go:build clitools

package spawner

import (
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
// codex_delegation.go (codexAgentsArgs): a renamed or removed tool/config key
// would otherwise fail silently (claude: an unknown deny name is ignored
// without warning; codex: a stale -c override is a no-op). Codex-specific
// drift alarms live in native_orchestration_codex_cli_test.go (split for the
// 300-line file cap).

// listToolsPrompt gives each CLI a minimal turn to initialize its tool surface.
const listToolsPrompt = "List the exact names of every tool currently available to you, one per line, nothing else."

// claudeToolRegistry reads the CLI's deterministic stream-json init event and
// kills the process before a model response. This avoids relying on the model
// to reproduce the registry without omissions.
func claudeToolRegistry(t *testing.T, extraArgs ...string) map[string]bool {
	t.Helper()

	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()

	args := append([]string{"--print", "--verbose", "--output-format", "stream-json", "--dangerously-skip-permissions"}, extraArgs...)
	cmd := exec.CommandContext(ctx, "claude", args...)
	cmd.Stdin = strings.NewReader(listToolsPrompt)
	cmd.Env = append(cmd.Environ(), "DISABLE_AUTOUPDATER=1")

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		t.Fatalf("claude stdout pipe: %v", err)
	}
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Start(); err != nil {
		t.Fatalf("claude --print %v: %v", extraArgs, err)
	}

	dec := json.NewDecoder(stdout)
	for {
		var ev struct {
			Type    string   `json:"type"`
			Subtype string   `json:"subtype"`
			Tools   []string `json:"tools"`
		}
		if err := dec.Decode(&ev); err != nil {
			_ = cmd.Wait()
			t.Fatalf("decode claude init event: %v\nstderr:\n%s", err, stderr.String())
		}
		if ev.Type != "system" || ev.Subtype != "init" {
			continue
		}
		_ = cmd.Process.Kill()
		_ = cmd.Wait()
		tools := make(map[string]bool, len(ev.Tools))
		for _, name := range ev.Tools {
			tools[name] = true
		}
		if len(tools) == 0 {
			t.Fatal("claude init event reported no tools")
		}
		return tools
	}
}

// TestNativeOrchestrationCLI_ClaudeDenyBlocksDelegation diffs claude's live
// tool registry with and without the production deny list.
//
// The baseline assertion keeps the deny list from passing vacuously when a
// future CLI renames or removes an orchestration tool.
func TestNativeOrchestrationCLI_ClaudeDenyBlocksDelegation(t *testing.T) {
	baseline := claudeToolRegistry(t)

	for _, tool := range strings.Fields(claudeDisallowedNativeTools) {
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

	// No collateral damage: exact matching must preserve ordinary coding and
	// background-task control tools.
	for _, tool := range []string{"Bash", "Edit", "Read", "Write"} {
		if baseline[tool] && !denied[tool] {
			t.Errorf("coding tool %q was collaterally denied by --disallowedTools %q", tool, claudeDisallowedNativeTools)
		}
	}
	for _, tool := range []string{"TaskCreate", "TaskGet", "TaskList", "TaskOutput", "TaskStop", "TaskUpdate"} {
		if baseline[tool] && !denied[tool] {
			t.Errorf("denying Task prefix-matched and collaterally denied %q; the deny list must match exact tool names only", tool)
		}
	}
}
