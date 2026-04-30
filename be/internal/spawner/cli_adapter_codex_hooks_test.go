package spawner

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestWriteCodexProfile_ConfigInheritsUserSettings verifies that config.toml
// preserves the user's existing top-level settings (model, personality, etc).
func TestWriteCodexProfile_ConfigInheritsUserSettings(t *testing.T) {
	fakeHome := t.TempDir()
	t.Setenv("HOME", fakeHome)
	codexHome := filepath.Join(fakeHome, ".codex")
	if err := os.MkdirAll(codexHome, 0o755); err != nil {
		t.Fatalf("mkdir codex home: %v", err)
	}
	userConfig := "model = \"gpt-5.4\"\npersonality = \"pragmatic\"\n"
	if err := os.WriteFile(filepath.Join(codexHome, "config.toml"), []byte(userConfig), 0o644); err != nil {
		t.Fatalf("write user config: %v", err)
	}

	dir := t.TempDir()
	if err := writeCodexProfile(dir, "/usr/local/bin/nrflo"); err != nil {
		t.Fatalf("writeCodexProfile() error: %v", err)
	}
	data, err := os.ReadFile(filepath.Join(dir, "config.toml"))
	if err != nil {
		t.Fatalf("config.toml not created: %v", err)
	}
	content := string(data)
	if !strings.Contains(content, `model = "gpt-5.4"`) {
		t.Errorf("config.toml does not inherit user model setting: %s", content)
	}
	if !strings.Contains(content, `personality = "pragmatic"`) {
		t.Errorf("config.toml does not inherit user personality setting: %s", content)
	}
}

// TestWriteCodexProfile_EnablesCodexHooksFeature verifies that config.toml
// contains `[features] codex_hooks = true`. The hook tables themselves are
// NOT written to config.toml — codex's TUI ignores `<CODEX_HOME>/config.toml`
// hooks in PTY contexts, so they're injected via repeated `-c hooks.<event>=…`
// CLI flags from CodexAdapter.BuildInteractiveCommand instead.
func TestWriteCodexProfile_EnablesCodexHooksFeature(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	dir := t.TempDir()
	if err := writeCodexProfile(dir, "/custom/nrflo"); err != nil {
		t.Fatalf("writeCodexProfile() error: %v", err)
	}
	data, err := os.ReadFile(filepath.Join(dir, "config.toml"))
	if err != nil {
		t.Fatalf("config.toml not created: %v", err)
	}
	content := string(data)
	if !strings.Contains(content, "[features]") || !strings.Contains(content, "codex_hooks = true") {
		t.Errorf("config.toml missing [features] codex_hooks = true (required for TUI hook firing): %s", content)
	}
	if strings.Contains(content, "[[hooks.") {
		t.Errorf("config.toml should NOT contain [[hooks.…]] tables (hooks live in -c flags now): %s", content)
	}
}

// TestwriteCodexProfileForSession_TrustsWorkDir verifies that the agent's
// working directory is added to the project trust list. Without this, codex
// 0.125 blocks on its trust dialog and the agent exits during init when no
// one answers (the --dangerously-bypass-approvals-and-sandbox flag does NOT
// skip this prompt).
func TestWriteCodexProfileForSession_TrustsWorkDir(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	dir := t.TempDir()
	workDir := "/Users/x/projects/my-app"
	if err := writeCodexProfileForSession(dir, "/usr/local/bin/nrflo", "s", "i", "p", workDir); err != nil {
		t.Fatalf("writeCodexProfileForSession() error: %v", err)
	}
	body, _ := os.ReadFile(filepath.Join(dir, "config.toml"))
	want := `[projects."/Users/x/projects/my-app"]`
	if !strings.Contains(string(body), want) {
		t.Errorf("config.toml missing %s\nfull:\n%s", want, body)
	}
	if !strings.Contains(string(body), `trust_level = "trusted"`) {
		t.Errorf("config.toml missing trust_level = \"trusted\"")
	}
}

// TestwriteCodexProfileForSession_SkipsTrustWhenUserHasIt prevents a duplicate
// `[projects."<path>"]` table when the user's main config already trusts the
// directory.
func TestWriteCodexProfileForSession_SkipsTrustWhenUserHasIt(t *testing.T) {
	fakeHome := t.TempDir()
	t.Setenv("HOME", fakeHome)
	codexHome := filepath.Join(fakeHome, ".codex")
	if err := os.MkdirAll(codexHome, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	workDir := "/Users/x/projects/my-app"
	userConfig := "model = \"gpt-5.4\"\n\n[projects.\"/Users/x/projects/my-app\"]\ntrust_level = \"trusted\"\n"
	if err := os.WriteFile(filepath.Join(codexHome, "config.toml"), []byte(userConfig), 0o644); err != nil {
		t.Fatalf("write user config: %v", err)
	}

	dir := t.TempDir()
	if err := writeCodexProfileForSession(dir, "/usr/local/bin/nrflo", "s", "i", "p", workDir); err != nil {
		t.Fatalf("writeCodexProfileForSession() error: %v", err)
	}
	body, _ := os.ReadFile(filepath.Join(dir, "config.toml"))
	count := strings.Count(string(body), `[projects."/Users/x/projects/my-app"]`)
	if count != 1 {
		t.Errorf("trust entry appears %d times, want 1\nfull:\n%s", count, body)
	}
}

// TestBuildCodexHookCommand_EnvPrefix verifies the hook command string used
// in the inline `-c hooks.<event>=…` flag from
// CodexAdapter.BuildInteractiveCommand: it must wrap nrflo with `/usr/bin/env`
// + the per-session NRF_*/NRFLO_PROJECT vars, since codex strips most env vars
// from hook subprocesses.
func TestBuildCodexHookCommand_EnvPrefix(t *testing.T) {
	got := buildCodexHookCommand("/usr/local/bin/nrflo", "sess-abc", "inst-xyz", "proj-1")
	want := "/usr/bin/env NRF_SESSION_ID=sess-abc NRF_WORKFLOW_INSTANCE_ID=inst-xyz NRFLO_PROJECT=proj-1 /usr/local/bin/nrflo agent record-event"
	if got != want {
		t.Errorf("buildCodexHookCommand = %q\nwant %q", got, want)
	}
}

// TestWriteCodexProfile_NoSeparateHooksFile verifies that we no longer write a
// separate hooks.json file — codex 0.125 expects hooks inline in config.toml.
func TestWriteCodexProfile_NoSeparateHooksFile(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	dir := t.TempDir()
	if err := writeCodexProfile(dir, "/usr/local/bin/nrflo"); err != nil {
		t.Fatalf("writeCodexProfile() error: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, "hooks.json")); !os.IsNotExist(err) {
		t.Errorf("hooks.json should not exist (codex uses inline config.toml hooks): %v", err)
	}
}

// TestWriteCodexProfile_CopiesAuthJSON verifies that the user's auth.json is
// copied into the per-session profile so the spawned codex stays logged in.
func TestWriteCodexProfile_CopiesAuthJSON(t *testing.T) {
	fakeHome := t.TempDir()
	t.Setenv("HOME", fakeHome)
	codexHome := filepath.Join(fakeHome, ".codex")
	if err := os.MkdirAll(codexHome, 0o755); err != nil {
		t.Fatalf("mkdir codex home: %v", err)
	}
	authPayload := []byte(`{"token":"sk-test"}`)
	if err := os.WriteFile(filepath.Join(codexHome, "auth.json"), authPayload, 0o600); err != nil {
		t.Fatalf("write user auth: %v", err)
	}

	dir := t.TempDir()
	if err := writeCodexProfile(dir, "/usr/local/bin/nrflo"); err != nil {
		t.Fatalf("writeCodexProfile() error: %v", err)
	}
	got, err := os.ReadFile(filepath.Join(dir, "auth.json"))
	if err != nil {
		t.Fatalf("auth.json not copied: %v", err)
	}
	if string(got) != string(authPayload) {
		t.Errorf("auth.json content mismatch: got %q want %q", got, authPayload)
	}
}

// TestWriteCodexProfile_SkipsFeaturesWhenUserHasIt guards against a TOML
// duplicate-table error: when the user's config.toml already opens
// `[features]`, our appended block must NOT add a second `[features]` table
// (codex silently rejects the whole config and disables hooks otherwise).
func TestWriteCodexProfile_SkipsFeaturesWhenUserHasIt(t *testing.T) {
	fakeHome := t.TempDir()
	t.Setenv("HOME", fakeHome)
	codexHome := filepath.Join(fakeHome, ".codex")
	if err := os.MkdirAll(codexHome, 0o755); err != nil {
		t.Fatalf("mkdir codex home: %v", err)
	}
	userConfig := "model = \"gpt-5.4\"\n\n[features]\ncodex_hooks = true\n"
	if err := os.WriteFile(filepath.Join(codexHome, "config.toml"), []byte(userConfig), 0o644); err != nil {
		t.Fatalf("write user config: %v", err)
	}

	dir := t.TempDir()
	if err := writeCodexProfile(dir, "/usr/local/bin/nrflo"); err != nil {
		t.Fatalf("writeCodexProfile() error: %v", err)
	}
	data, err := os.ReadFile(filepath.Join(dir, "config.toml"))
	if err != nil {
		t.Fatalf("config.toml not created: %v", err)
	}
	count := strings.Count(string(data), "[features]")
	if count != 1 {
		t.Errorf("config.toml has %d [features] sections, want exactly 1; full body:\n%s", count, data)
	}
}

// TestWriteCodexProfile_NoUserConfig verifies that WriteCodexProfile writes
// a minimal config.toml (with [features] codex_hooks=true, no hook tables)
// when the user has no ~/.codex/config.toml.
func TestWriteCodexProfile_NoUserConfig(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	dir := t.TempDir()
	if err := writeCodexProfile(dir, "/usr/local/bin/nrflo"); err != nil {
		t.Fatalf("writeCodexProfile() error: %v", err)
	}
	data, err := os.ReadFile(filepath.Join(dir, "config.toml"))
	if err != nil {
		t.Fatalf("config.toml not created: %v", err)
	}
	if !strings.Contains(string(data), "codex_hooks = true") {
		t.Errorf("config.toml missing codex_hooks = true: %s", data)
	}
}

// TestCodexAdapter_PrepareInteractive_DirContainsSessionID verifies the
// created directory name embeds the processInfo sessionID.
func TestCodexAdapter_PrepareInteractive_DirContainsSessionID(t *testing.T) {
	proc := &processInfo{sessionID: "sess-x", doneCh: make(chan struct{})}
	extras, cleanup, err := (&CodexAdapter{}).PrepareInteractive(InteractivePrepOptions{SessionID: proc.sessionID, WorkflowInstanceID: proc.workflowInstanceID, ProjectID: proc.projectID})
	if err != nil {
		t.Fatalf("PrepareInteractive() error: %v", err)
	}
	t.Cleanup(cleanup)

	if _, statErr := os.Stat(extras.CodexHome); statErr != nil {
		t.Errorf("profile dir does not exist: %v", statErr)
	}
	base := filepath.Base(extras.CodexHome)
	if !strings.Contains(base, "sess-x") {
		t.Errorf("dir base %q does not contain sessionID 'sess-x'", base)
	}
}

// TestCodexAdapter_PrepareInteractive_Cleanup verifies that calling the
// returned cleanup func removes the profile directory.
func TestCodexAdapter_PrepareInteractive_Cleanup(t *testing.T) {
	proc := &processInfo{sessionID: "sess-cleanup", doneCh: make(chan struct{})}
	extras, cleanup, err := (&CodexAdapter{}).PrepareInteractive(InteractivePrepOptions{SessionID: proc.sessionID, WorkflowInstanceID: proc.workflowInstanceID, ProjectID: proc.projectID})
	if err != nil {
		t.Fatalf("PrepareInteractive() error: %v", err)
	}
	if _, statErr := os.Stat(extras.CodexHome); statErr != nil {
		t.Fatalf("dir does not exist before cleanup: %v", statErr)
	}
	cleanup()
	if _, statErr := os.Stat(extras.CodexHome); !os.IsNotExist(statErr) {
		t.Errorf("cleanup() did not remove dir %q (stat: %v)", extras.CodexHome, statErr)
	}
}

// TestCodexAdapter_PrepareInteractive_CleanupIdempotent verifies that calling
// cleanup twice does not panic.
func TestCodexAdapter_PrepareInteractive_CleanupIdempotent(t *testing.T) {
	proc := &processInfo{sessionID: "sess-idempotent", doneCh: make(chan struct{})}
	_, cleanup, err := (&CodexAdapter{}).PrepareInteractive(InteractivePrepOptions{SessionID: proc.sessionID, WorkflowInstanceID: proc.workflowInstanceID, ProjectID: proc.projectID})
	if err != nil {
		t.Fatalf("PrepareInteractive() error: %v", err)
	}
	cleanup()
	cleanup() // second call must not panic
}

// TestCodexAdapter_PrepareInteractive_FailureReturnsError verifies that
// PrepareInteractive returns an error when the temp directory cannot be
// created, and the returned cleanup is a no-op (does not panic).
func TestCodexAdapter_PrepareInteractive_FailureReturnsError(t *testing.T) {
	t.Setenv("TMPDIR", "/nonexistent-nrflo-test-dir-xyz")
	proc := &processInfo{sessionID: "sess-fail", doneCh: make(chan struct{})}
	_, cleanup, err := (&CodexAdapter{}).PrepareInteractive(InteractivePrepOptions{SessionID: proc.sessionID, WorkflowInstanceID: proc.workflowInstanceID, ProjectID: proc.projectID})
	if err == nil {
		cleanup()
		t.Error("PrepareInteractive() should return error when TMPDIR is invalid")
		return
	}
	cleanup() // must not panic even on error path
}

// TestCodexAdapter_PrepareInteractive_HookEvents verifies the returned
// InteractiveExtras carries one HookEvent per canonical codex event with the
// env-prefixed nrflo command.
func TestCodexAdapter_PrepareInteractive_HookEvents(t *testing.T) {
	proc := &processInfo{
		sessionID:          "sess-h",
		workflowInstanceID: "inst-h",
		projectID:          "proj-h",
		doneCh:             make(chan struct{}),
	}
	extras, cleanup, err := (&CodexAdapter{}).PrepareInteractive(InteractivePrepOptions{SessionID: proc.sessionID, WorkflowInstanceID: proc.workflowInstanceID, ProjectID: proc.projectID})
	if err != nil {
		t.Fatalf("PrepareInteractive() error: %v", err)
	}
	t.Cleanup(cleanup)
	if got, want := len(extras.Hooks), len(codexHookEvents); got != want {
		t.Fatalf("hook events count = %d, want %d", got, want)
	}
	wantEvents := map[string]bool{}
	for _, ev := range codexHookEvents {
		wantEvents[ev] = true
	}
	for _, h := range extras.Hooks {
		if !wantEvents[h.Event] {
			t.Errorf("unexpected hook event %q", h.Event)
		}
		if !strings.Contains(h.Command, "NRF_SESSION_ID=sess-h") {
			t.Errorf("hook command missing session id: %q", h.Command)
		}
		if !strings.Contains(h.Command, "agent record-event") {
			t.Errorf("hook command missing nrflo agent record-event: %q", h.Command)
		}
		if h.TimeoutSec != 5 {
			t.Errorf("hook timeout = %d, want 5", h.TimeoutSec)
		}
	}
}
