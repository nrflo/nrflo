package spawner

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestWriteCodexProfile_InheritsUserSettings verifies config.toml preserves the
// user's existing top-level settings (model, personality, custom providers).
func TestWriteCodexProfile_InheritsUserSettings(t *testing.T) {
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
	if err := writeCodexProfileForSession(dir, ""); err != nil {
		t.Fatalf("writeCodexProfileForSession() error: %v", err)
	}
	content := readFileString(t, filepath.Join(dir, "config.toml"))
	if !strings.Contains(content, `model = "gpt-5.4"`) {
		t.Errorf("config.toml does not inherit user model setting: %s", content)
	}
	if !strings.Contains(content, `personality = "pragmatic"`) {
		t.Errorf("config.toml does not inherit user personality setting: %s", content)
	}
}

// TestWriteCodexProfile_WritesWorkdirTrust verifies the profile config.toml
// carries a `[projects."<resolvedWorkDir>"] trust_level="trusted"` entry —
// codex 0.133 reads workdir trust from CODEX_HOME/config.toml and blocks on the
// directory-trust dialog without it. The path must be symlink-resolved.
func TestWriteCodexProfile_WritesWorkdirTrust(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	dir := t.TempDir()
	workDir := t.TempDir() // real dir so EvalSymlinks succeeds

	if err := writeCodexProfileForSession(dir, workDir); err != nil {
		t.Fatalf("writeCodexProfileForSession() error: %v", err)
	}
	content := readFileString(t, filepath.Join(dir, "config.toml"))
	resolved, _ := filepath.EvalSymlinks(workDir)
	want := fmt.Sprintf("[projects.%q]", resolved)
	if !strings.Contains(content, want) {
		t.Errorf("config.toml missing %s\nfull:\n%s", want, content)
	}
	if !strings.Contains(content, `trust_level = "trusted"`) {
		t.Errorf("config.toml missing trust_level = \"trusted\"\nfull:\n%s", content)
	}
}

// TestWriteCodexProfile_StripsHookTables verifies the user's own `[[hooks.…]]`
// definitions are stripped — codex 0.133 would otherwise raise a blocking
// "hooks need review" gate at startup for the spawned session.
func TestWriteCodexProfile_StripsHookTables(t *testing.T) {
	fakeHome := t.TempDir()
	t.Setenv("HOME", fakeHome)
	codexHome := filepath.Join(fakeHome, ".codex")
	if err := os.MkdirAll(codexHome, 0o755); err != nil {
		t.Fatalf("mkdir codex home: %v", err)
	}
	userConfig := "model = \"gpt-5.4\"\n\n[[hooks.PostToolUse]]\nmatcher = \"*\"\ncommand = \"echo hi\"\n"
	if err := os.WriteFile(filepath.Join(codexHome, "config.toml"), []byte(userConfig), 0o644); err != nil {
		t.Fatalf("write user config: %v", err)
	}

	dir := t.TempDir()
	if err := writeCodexProfileForSession(dir, ""); err != nil {
		t.Fatalf("writeCodexProfileForSession() error: %v", err)
	}
	content := readFileString(t, filepath.Join(dir, "config.toml"))
	if strings.Contains(content, "[[hooks.") || strings.Contains(content, "echo hi") {
		t.Errorf("config.toml should not contain user hook tables: %s", content)
	}
	if !strings.Contains(content, `model = "gpt-5.4"`) {
		t.Errorf("non-hook settings should survive stripping: %s", content)
	}
}

// TestWriteCodexProfile_NoHooksFeature verifies we no longer write the
// deprecated `[features] codex_hooks` flag (no hooks are wired at all).
func TestWriteCodexProfile_NoHooksFeature(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	dir := t.TempDir()
	if err := writeCodexProfileForSession(dir, ""); err != nil {
		t.Fatalf("writeCodexProfileForSession() error: %v", err)
	}
	content := readFileString(t, filepath.Join(dir, "config.toml"))
	if strings.Contains(content, "codex_hooks") {
		t.Errorf("config.toml should not contain deprecated codex_hooks flag: %s", content)
	}
}

// TestWriteCodexProfile_CopiesAuthJSON verifies the user's auth.json is copied
// into the per-session profile so the spawned codex stays logged in.
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
	if err := writeCodexProfileForSession(dir, ""); err != nil {
		t.Fatalf("writeCodexProfileForSession() error: %v", err)
	}
	if got := readFileString(t, filepath.Join(dir, "auth.json")); got != string(authPayload) {
		t.Errorf("auth.json content mismatch: got %q want %q", got, authPayload)
	}
}

// TestStripTOMLTables covers array-of-tables, single-table, and bare-table
// header forms for both hook and project tables, preserving surrounding content.
func TestStripTOMLTables(t *testing.T) {
	in := "model = \"x\"\n[[hooks.PreToolUse]]\nc = 1\n[features]\nfoo = true\n[hooks.Stop]\nc = 2\n[projects.\"/a/b\"]\ntrust_level = \"trusted\"\n[other]\nk = 1\n"
	out := string(stripTOMLTables([]byte(in), codexStripTablePrefixes))
	if strings.Contains(out, "hooks.") || strings.Contains(out, "[projects.") {
		t.Errorf("hook/project tables not stripped: %s", out)
	}
	for _, keep := range []string{`model = "x"`, "[features]", "foo = true", "[other]", "k = 1"} {
		if !strings.Contains(out, keep) {
			t.Errorf("stripTOMLTables dropped %q\nfull:\n%s", keep, out)
		}
	}
}

// TestWriteCodexProfile_NoDuplicateProjectKey guards the app-server regression:
// the user's config already trusts the spawn workdir, so a naive append would
// produce a duplicate `[projects."<dir>"]` table that the app-server's strict
// parser rejects (rpc -32600). The profile must contain exactly one.
func TestWriteCodexProfile_NoDuplicateProjectKey(t *testing.T) {
	fakeHome := t.TempDir()
	t.Setenv("HOME", fakeHome)
	codexHome := filepath.Join(fakeHome, ".codex")
	if err := os.MkdirAll(codexHome, 0o755); err != nil {
		t.Fatalf("mkdir codex home: %v", err)
	}
	workDir := t.TempDir()
	resolved, _ := filepath.EvalSymlinks(workDir)
	// User config already trusts the workdir (+ another project, + a hook).
	userConfig := fmt.Sprintf("model = \"gpt-5.4\"\n\n[projects.%q]\ntrust_level = \"trusted\"\n\n[projects.\"/other\"]\ntrust_level = \"trusted\"\n", resolved)
	if err := os.WriteFile(filepath.Join(codexHome, "config.toml"), []byte(userConfig), 0o644); err != nil {
		t.Fatalf("write user config: %v", err)
	}

	dir := t.TempDir()
	if err := writeCodexProfileForSession(dir, workDir); err != nil {
		t.Fatalf("writeCodexProfileForSession() error: %v", err)
	}
	content := readFileString(t, filepath.Join(dir, "config.toml"))
	if n := strings.Count(content, fmt.Sprintf("[projects.%q]", resolved)); n != 1 {
		t.Errorf("workdir project table appears %d times, want exactly 1\nfull:\n%s", n, content)
	}
	if strings.Contains(content, `[projects."/other"]`) {
		t.Errorf("stale user project entries should be stripped\nfull:\n%s", content)
	}
	if !strings.Contains(content, `model = "gpt-5.4"`) {
		t.Errorf("non-project settings should survive: %s", content)
	}
}

// TestCodexAdapter_PrepareInteractive_DirContainsSessionID verifies the created
// directory name embeds the sessionID and that no Hooks are returned.
func TestCodexAdapter_PrepareInteractive_DirContainsSessionID(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	workDir := t.TempDir()
	extras, cleanup, err := (&CodexAdapter{}).PrepareInteractive(InteractivePrepOptions{SessionID: "sess-x", WorkDir: workDir})
	if err != nil {
		t.Fatalf("PrepareInteractive() error: %v", err)
	}
	t.Cleanup(cleanup)

	if _, statErr := os.Stat(extras.CodexHome); statErr != nil {
		t.Errorf("profile dir does not exist: %v", statErr)
	}
	if !strings.Contains(filepath.Base(extras.CodexHome), "sess-x") {
		t.Errorf("dir base %q does not contain sessionID 'sess-x'", filepath.Base(extras.CodexHome))
	}
	// Trust for the workdir must land in the profile config.
	content := readFileString(t, filepath.Join(extras.CodexHome, "config.toml"))
	if !strings.Contains(content, `trust_level = "trusted"`) {
		t.Errorf("profile config missing workdir trust:\n%s", content)
	}
}

// TestCodexAdapter_PrepareInteractive_Cleanup verifies cleanup removes the dir.
func TestCodexAdapter_PrepareInteractive_Cleanup(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	extras, cleanup, err := (&CodexAdapter{}).PrepareInteractive(InteractivePrepOptions{SessionID: "sess-cleanup", WorkDir: t.TempDir()})
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
	cleanup() // second call must not panic
}

// TestCodexAdapter_PrepareInteractive_FailureReturnsError verifies an error is
// returned when the temp directory cannot be created and cleanup is a no-op.
func TestCodexAdapter_PrepareInteractive_FailureReturnsError(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv("TMPDIR", "/nonexistent-nrflo-test-dir-xyz")
	_, cleanup, err := (&CodexAdapter{}).PrepareInteractive(InteractivePrepOptions{SessionID: "sess-fail", WorkDir: t.TempDir()})
	if err == nil {
		cleanup()
		t.Error("PrepareInteractive() should return error when TMPDIR is invalid")
		return
	}
	cleanup() // must not panic even on error path
}

func readFileString(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	return string(data)
}
