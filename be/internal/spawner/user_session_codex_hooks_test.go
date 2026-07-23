package spawner

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// codexHomeFromEnv extracts CODEX_HOME from a pty.Launch env slice.
func codexHomeFromEnv(t *testing.T, env []string) string {
	t.Helper()
	for _, e := range env {
		if strings.HasPrefix(e, "CODEX_HOME=") {
			return strings.TrimPrefix(e, "CODEX_HOME=")
		}
	}
	t.Fatalf("env missing CODEX_HOME: %v", env)
	return ""
}

// assertHooksWiredToRecordEvent verifies codexHome/hooks.json exists, is
// valid JSON, and every hook command is suffixed "agent record-event".
func assertHooksWiredToRecordEvent(t *testing.T, codexHome string) {
	t.Helper()
	content := readFileString(t, filepath.Join(codexHome, "hooks.json"))
	var parsed map[string]interface{}
	if err := json.Unmarshal([]byte(content), &parsed); err != nil {
		t.Fatalf("hooks.json invalid JSON: %v\ngot: %s", err, content)
	}
	hooks, ok := parsed["hooks"].(map[string]interface{})
	if !ok || len(hooks) == 0 {
		t.Fatalf("hooks.json missing/empty 'hooks': %s", content)
	}
	for name, raw := range hooks {
		entries, _ := raw.([]interface{})
		if len(entries) == 0 {
			t.Fatalf("hooks.%s empty: %s", name, content)
		}
		entry, _ := entries[0].(map[string]interface{})
		innerHooks, _ := entry["hooks"].([]interface{})
		if len(innerHooks) == 0 {
			t.Fatalf("hooks.%s[0].hooks empty: %s", name, content)
		}
		inner, _ := innerHooks[0].(map[string]interface{})
		cmd, _ := inner["command"].(string)
		if !strings.HasSuffix(cmd, "agent record-event") {
			t.Errorf("hooks.%s command = %q, want suffix 'agent record-event'", name, cmd)
		}
	}
}

// TestCodexAdapter_PrepareUserSession_HooksWired_Interactive verifies an
// interactive human PTY session gets a CODEX_HOME/hooks.json wired to
// `agent record-event`.
func TestCodexAdapter_PrepareUserSession_HooksWired_Interactive(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	adapter := &CodexAdapter{}
	workDir := t.TempDir()

	launch, cleanup, err := adapter.PrepareUserSession(UserSessionOptions{
		SessionID: "sess-hooks-int",
		Model:     "gpt-5-codex",
		WorkDir:   workDir,
	})
	if err != nil {
		t.Fatalf("PrepareUserSession() error: %v", err)
	}
	t.Cleanup(cleanup)

	codexHome := codexHomeFromEnv(t, launch.Env)
	assertHooksWiredToRecordEvent(t, codexHome)
}

// TestCodexAdapter_PrepareUserSession_HooksWired_PlanMode verifies plan mode
// also gets a hooks.json wired to `agent record-event`.
func TestCodexAdapter_PrepareUserSession_HooksWired_PlanMode(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	adapter := &CodexAdapter{}
	workDir := t.TempDir()

	launch, cleanup, err := adapter.PrepareUserSession(UserSessionOptions{
		SessionID: "sess-hooks-plan",
		Model:     "gpt-5-codex",
		WorkDir:   workDir,
		PlanMode:  true,
	})
	if err != nil {
		t.Fatalf("PrepareUserSession() error: %v", err)
	}
	t.Cleanup(cleanup)

	codexHome := codexHomeFromEnv(t, launch.Env)
	assertHooksWiredToRecordEvent(t, codexHome)
}

// TestCodexAdapter_PrepareUserSession_NoHookLeakFromUserHome is the no-leak
// regression: the user's own ~/.codex/hooks.json (which writeCodexHooksForSession
// never reads — it always writes our own hooks.json) must never appear in the
// generated session file.
func TestCodexAdapter_PrepareUserSession_NoHookLeakFromUserHome(t *testing.T) {
	fakeHome := t.TempDir()
	t.Setenv("HOME", fakeHome)
	codexHome := filepath.Join(fakeHome, ".codex")
	if err := os.MkdirAll(codexHome, 0o755); err != nil {
		t.Fatalf("mkdir codex home: %v", err)
	}
	marker := "echo THIS-IS-THE-USERS-OWN-HOOK-MARKER"
	userHooks := `{"hooks":{"SessionStart":[{"matcher":"*","hooks":[{"type":"command","command":"` + marker + `"}]}]}}`
	if err := os.WriteFile(filepath.Join(codexHome, "hooks.json"), []byte(userHooks), 0o644); err != nil {
		t.Fatalf("write user hooks.json: %v", err)
	}

	adapter := &CodexAdapter{}
	workDir := t.TempDir()
	launch, cleanup, err := adapter.PrepareUserSession(UserSessionOptions{
		SessionID: "sess-no-leak",
		Model:     "gpt-5-codex",
		WorkDir:   workDir,
	})
	if err != nil {
		t.Fatalf("PrepareUserSession() error: %v", err)
	}
	t.Cleanup(cleanup)

	sessionCodexHome := codexHomeFromEnv(t, launch.Env)
	if sessionCodexHome == codexHome {
		t.Fatalf("session CODEX_HOME must be a separate temp dir, not the user's real ~/.codex")
	}
	content := readFileString(t, filepath.Join(sessionCodexHome, "hooks.json"))
	if strings.Contains(content, marker) {
		t.Errorf("session hooks.json leaked the user's own hook marker: %s", content)
	}
}

// TestCodexAdapter_PrepareUserSession_CleanupRemovesHooksJSON verifies
// cleanup() removes hooks.json along with the rest of the temp dir.
func TestCodexAdapter_PrepareUserSession_CleanupRemovesHooksJSON(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	adapter := &CodexAdapter{}
	workDir := t.TempDir()

	launch, cleanup, err := adapter.PrepareUserSession(UserSessionOptions{
		SessionID: "sess-hooks-cleanup",
		Model:     "gpt-5-codex",
		WorkDir:   workDir,
	})
	if err != nil {
		t.Fatalf("PrepareUserSession() error: %v", err)
	}
	codexHome := codexHomeFromEnv(t, launch.Env)
	hooksPath := filepath.Join(codexHome, "hooks.json")
	if _, err := os.Stat(hooksPath); err != nil {
		t.Fatalf("hooks.json should exist before cleanup: %v", err)
	}

	cleanup()
	if _, err := os.Stat(hooksPath); !os.IsNotExist(err) {
		t.Errorf("cleanup() should remove hooks.json, stat err = %v", err)
	}
	if _, err := os.Stat(codexHome); !os.IsNotExist(err) {
		t.Errorf("cleanup() should remove CODEX_HOME dir, stat err = %v", err)
	}
}
