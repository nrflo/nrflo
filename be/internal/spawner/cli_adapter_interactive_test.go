package spawner

import (
	"strings"
	"testing"
)

// TestAllAdapters_SupportsInteractive pins each adapter's interactive policy.
// All three opt in: Claude via --settings hooks, Codex via -c hook injection,
// and Opencode via its embedded HTTP SSE server (--port/--hostname flags).
func TestAllAdapters_SupportsInteractive(t *testing.T) {
	cases := []struct {
		name    string
		adapter CLIAdapter
		want    bool
	}{
		{"claude", &ClaudeAdapter{}, true},
		{"opencode", &OpencodeAdapter{}, true},
		{"codex", &CodexAdapter{}, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := tc.adapter.SupportsInteractive(); got != tc.want {
				t.Errorf("%s.SupportsInteractive() = %v, want %v", tc.name, got, tc.want)
			}
		})
	}
}

// TestClaudeAdapter_BuildInteractiveCommand_NoBatchFlags verifies that the
// interactive command omits all batch-mode flags present in BuildCommand.
func TestClaudeAdapter_BuildInteractiveCommand_NoBatchFlags(t *testing.T) {
	a := &ClaudeAdapter{}
	opts := InteractiveSpawnOptions{
		SessionID: "sess-1",
		Model:     "claude-opus-4-7",
		WorkDir:   "/tmp",
	}
	cmd := a.BuildInteractiveCommand(opts)
	args := strings.Join(cmd.Args, " ")

	forbidden := []string{"--print", "--verbose", "--output-format", "--disallowed-tools"}
	for _, flag := range forbidden {
		if strings.Contains(args, flag) {
			t.Errorf("BuildInteractiveCommand must not contain batch flag %q: %s", flag, args)
		}
	}
}

// TestClaudeAdapter_BuildInteractiveCommand_HasRequiredFlags verifies that the
// interactive command includes session-id, model, and dangerously-skip-permissions.
func TestClaudeAdapter_BuildInteractiveCommand_HasRequiredFlags(t *testing.T) {
	a := &ClaudeAdapter{}
	opts := InteractiveSpawnOptions{
		SessionID: "sess-abc",
		Model:     "claude-sonnet",
		WorkDir:   "/work",
	}
	cmd := a.BuildInteractiveCommand(opts)
	args := strings.Join(cmd.Args, " ")

	for _, want := range []string{"--session-id", "sess-abc", "--model", "claude-sonnet", "--dangerously-skip-permissions"} {
		if !strings.Contains(args, want) {
			t.Errorf("BuildInteractiveCommand missing %q: %s", want, args)
		}
	}
	if cmd.Dir != "/work" {
		t.Errorf("cmd.Dir = %q, want /work", cmd.Dir)
	}
}

// TestClaudeAdapter_BuildInteractiveCommand_WithEffort verifies --effort is added
// when ReasoningEffort is non-empty.
func TestClaudeAdapter_BuildInteractiveCommand_WithEffort(t *testing.T) {
	a := &ClaudeAdapter{}
	opts := InteractiveSpawnOptions{
		SessionID:       "sess-1",
		Model:           "claude-opus-4-7",
		ReasoningEffort: "high",
		WorkDir:         "/tmp",
	}
	args := strings.Join(a.BuildInteractiveCommand(opts).Args, " ")
	if !strings.Contains(args, "--effort high") {
		t.Errorf("BuildInteractiveCommand missing --effort high: %s", args)
	}
}

// TestClaudeAdapter_BuildInteractiveCommand_EmptyEffortOmitsFlag verifies --effort
// is absent when ReasoningEffort is empty.
func TestClaudeAdapter_BuildInteractiveCommand_EmptyEffortOmitsFlag(t *testing.T) {
	a := &ClaudeAdapter{}
	opts := InteractiveSpawnOptions{SessionID: "s", Model: "claude-sonnet", WorkDir: "/tmp"}
	args := strings.Join(a.BuildInteractiveCommand(opts).Args, " ")
	if strings.Contains(args, "--effort") {
		t.Errorf("BuildInteractiveCommand with empty ReasoningEffort must not contain --effort: %s", args)
	}
}

// TestClaudeAdapter_BuildInteractiveCommand_WithSettings verifies --settings is
// included when SettingsJSON is non-empty.
func TestClaudeAdapter_BuildInteractiveCommand_WithSettings(t *testing.T) {
	a := &ClaudeAdapter{}
	settingsJSON := `{"hooks":{"PreToolUse":[]}}`
	opts := InteractiveSpawnOptions{
		SessionID:    "sess-1",
		Model:        "claude-sonnet",
		SettingsJSON: settingsJSON,
		WorkDir:      "/tmp",
	}
	args := strings.Join(a.BuildInteractiveCommand(opts).Args, " ")
	if !strings.Contains(args, "--settings") {
		t.Errorf("BuildInteractiveCommand with SettingsJSON missing --settings: %s", args)
	}
	if !strings.Contains(args, settingsJSON) {
		t.Errorf("BuildInteractiveCommand missing settings JSON value: %s", args)
	}
}

// TestClaudeAdapter_BuildInteractiveCommand_EmptySettingsOmitsFlag verifies
// --settings is absent when SettingsJSON is empty.
func TestClaudeAdapter_BuildInteractiveCommand_EmptySettingsOmitsFlag(t *testing.T) {
	a := &ClaudeAdapter{}
	opts := InteractiveSpawnOptions{SessionID: "s", Model: "claude-sonnet", WorkDir: "/tmp"}
	args := strings.Join(a.BuildInteractiveCommand(opts).Args, " ")
	if strings.Contains(args, "--settings") {
		t.Errorf("BuildInteractiveCommand with empty SettingsJSON must not contain --settings: %s", args)
	}
}

// TestClaudeAdapter_BuildInteractiveCommand_WithSystemPromptFile verifies
// --append-system-prompt-file is added when SystemPromptFile is non-empty.
func TestClaudeAdapter_BuildInteractiveCommand_WithSystemPromptFile(t *testing.T) {
	a := &ClaudeAdapter{}
	opts := InteractiveSpawnOptions{
		SessionID:        "sess-1",
		Model:            "claude-sonnet",
		SystemPromptFile: "/tmp/suffix.md",
		WorkDir:          "/tmp",
	}
	args := strings.Join(a.BuildInteractiveCommand(opts).Args, " ")
	if !strings.Contains(args, "--append-system-prompt-file") {
		t.Errorf("BuildInteractiveCommand missing --append-system-prompt-file: %s", args)
	}
	if !strings.Contains(args, "/tmp/suffix.md") {
		t.Errorf("BuildInteractiveCommand missing system prompt file path: %s", args)
	}
}

// TestClaudeAdapter_BuildInteractiveCommand_EmptySystemPromptFileOmitsFlag verifies
// --append-system-prompt-file is absent when SystemPromptFile is empty.
func TestClaudeAdapter_BuildInteractiveCommand_EmptySystemPromptFileOmitsFlag(t *testing.T) {
	a := &ClaudeAdapter{}
	opts := InteractiveSpawnOptions{SessionID: "s", Model: "claude-sonnet", WorkDir: "/tmp"}
	args := strings.Join(a.BuildInteractiveCommand(opts).Args, " ")
	if strings.Contains(args, "--append-system-prompt-file") {
		t.Errorf("BuildInteractiveCommand must not contain --append-system-prompt-file when not set: %s", args)
	}
}

// TestOpencodeAdapter_BuildInteractiveCommand_NoBatchFlags verifies that Opencode's
// interactive command omits run, --format, and json batch flags.
func TestOpencodeAdapter_BuildInteractiveCommand_NoBatchFlags(t *testing.T) {
	a := &OpencodeAdapter{}
	opts := InteractiveSpawnOptions{Model: "openai/gpt-5.4", WorkDir: "/tmp"}
	args := strings.Join(a.BuildInteractiveCommand(opts).Args, " ")

	for _, flag := range []string{"run", "--format json"} {
		if strings.Contains(args, flag) {
			t.Errorf("OpencodeAdapter.BuildInteractiveCommand must not contain %q: %s", flag, args)
		}
	}
}

// TestOpencodeAdapter_BuildInteractiveCommand_HasModel verifies --model is present.
func TestOpencodeAdapter_BuildInteractiveCommand_HasModel(t *testing.T) {
	a := &OpencodeAdapter{}
	opts := InteractiveSpawnOptions{Model: "openai/gpt-5.4", WorkDir: "/work"}
	cmd := a.BuildInteractiveCommand(opts)
	args := strings.Join(cmd.Args, " ")

	if !strings.Contains(args, "--model openai/gpt-5.4") {
		t.Errorf("BuildInteractiveCommand missing --model openai/gpt-5.4: %s", args)
	}
	if cmd.Dir != "/work" {
		t.Errorf("cmd.Dir = %q, want /work", cmd.Dir)
	}
}

// TestOpencodeAdapter_BuildInteractiveCommand_WithVariant verifies --variant is added
// when ReasoningEffort is non-empty.
func TestOpencodeAdapter_BuildInteractiveCommand_WithVariant(t *testing.T) {
	a := &OpencodeAdapter{}
	opts := InteractiveSpawnOptions{Model: "openai/gpt-5.4", ReasoningEffort: "high", WorkDir: "/tmp"}
	args := strings.Join(a.BuildInteractiveCommand(opts).Args, " ")
	if !strings.Contains(args, "--variant high") {
		t.Errorf("BuildInteractiveCommand missing --variant high: %s", args)
	}
}

// TestOpencodeAdapter_BuildInteractiveCommand_NoVariantWhenEmpty verifies --variant
// is absent when ReasoningEffort is empty.
func TestOpencodeAdapter_BuildInteractiveCommand_NoVariantWhenEmpty(t *testing.T) {
	a := &OpencodeAdapter{}
	opts := InteractiveSpawnOptions{Model: "opencode/minimax-m2.5-free", WorkDir: "/tmp"}
	args := strings.Join(a.BuildInteractiveCommand(opts).Args, " ")
	if strings.Contains(args, "--variant") {
		t.Errorf("BuildInteractiveCommand must not contain --variant when ReasoningEffort empty: %s", args)
	}
}

// TestCodexAdapter_BuildInteractiveCommand_NoBatchFlags verifies that Codex's
// interactive command omits exec and --json batch flags.
func TestCodexAdapter_BuildInteractiveCommand_NoBatchFlags(t *testing.T) {
	a := &CodexAdapter{}
	opts := InteractiveSpawnOptions{Model: "gpt-5.3-codex", WorkDir: "/tmp"}
	args := strings.Join(a.BuildInteractiveCommand(opts).Args, " ")

	for _, flag := range []string{"exec", "--json"} {
		if strings.Contains(args, flag) {
			t.Errorf("CodexAdapter.BuildInteractiveCommand must not contain %q: %s", flag, args)
		}
	}
}

// TestCodexAdapter_BuildInteractiveCommand_HasRequiredFlags verifies Codex includes
// --model and --dangerously-bypass-approvals-and-sandbox.
func TestCodexAdapter_BuildInteractiveCommand_HasRequiredFlags(t *testing.T) {
	a := &CodexAdapter{}
	opts := InteractiveSpawnOptions{Model: "gpt-5.3-codex", WorkDir: "/work"}
	cmd := a.BuildInteractiveCommand(opts)
	args := strings.Join(cmd.Args, " ")

	for _, want := range []string{"--model", "gpt-5.3-codex", "--dangerously-bypass-approvals-and-sandbox"} {
		if !strings.Contains(args, want) {
			t.Errorf("CodexAdapter.BuildInteractiveCommand missing %q: %s", want, args)
		}
	}
	if cmd.Dir != "/work" {
		t.Errorf("cmd.Dir = %q, want /work", cmd.Dir)
	}
}

// TestCodexAdapter_BuildInteractiveCommand_CodexHomeEnv verifies that CODEX_HOME is
// appended to cmd.Env when InteractiveSpawnOptions.CodexHome is non-empty, and that
// caller-supplied env vars are preserved.
func TestCodexAdapter_BuildInteractiveCommand_CodexHomeEnv(t *testing.T) {
	a := &CodexAdapter{}
	opts := InteractiveSpawnOptions{
		Model:     "gpt-5.3-codex",
		WorkDir:   "/tmp",
		Env:       []string{"FOO=bar", "NRF_SESSION_ID=sess-1"},
		CodexHome: "/tmp/codex-home-abc",
	}
	cmd := a.BuildInteractiveCommand(opts)

	envSet := make(map[string]bool, len(cmd.Env))
	for _, e := range cmd.Env {
		envSet[e] = true
	}
	if !envSet["CODEX_HOME=/tmp/codex-home-abc"] {
		t.Errorf("cmd.Env missing CODEX_HOME=/tmp/codex-home-abc: %v", cmd.Env)
	}
	if !envSet["FOO=bar"] {
		t.Errorf("cmd.Env missing caller-supplied FOO=bar: %v", cmd.Env)
	}
	if !envSet["NRF_SESSION_ID=sess-1"] {
		t.Errorf("cmd.Env missing caller-supplied NRF_SESSION_ID=sess-1: %v", cmd.Env)
	}
}

// TestCodexAdapter_BuildInteractiveCommand_EmptyCodexHome_NoCodexHomeEnv verifies
// that no CODEX_HOME entry appears in cmd.Env when CodexHome is empty.
func TestCodexAdapter_BuildInteractiveCommand_EmptyCodexHome_NoCodexHomeEnv(t *testing.T) {
	a := &CodexAdapter{}
	opts := InteractiveSpawnOptions{
		Model:   "gpt-5.3-codex",
		WorkDir: "/tmp",
		Env:     []string{"FOO=bar"},
		// CodexHome intentionally left empty
	}
	cmd := a.BuildInteractiveCommand(opts)
	for _, e := range cmd.Env {
		if strings.HasPrefix(e, "CODEX_HOME=") {
			t.Errorf("cmd.Env must not contain CODEX_HOME when CodexHome is empty: %v", cmd.Env)
		}
	}
}
