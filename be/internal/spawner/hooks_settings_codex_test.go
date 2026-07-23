package spawner

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestBuildCodexHooksJSON_FiveEvents verifies the builder emits exactly the
// five validated codex hook events, each shaped as
// {matcher:"*", hooks:[{type:"command", command:<...agent record-event>}]}.
func TestBuildCodexHooksJSON_FiveEvents(t *testing.T) {
	t.Parallel()
	data, err := buildCodexHooksJSON()
	if err != nil {
		t.Fatalf("buildCodexHooksJSON() error: %v", err)
	}

	var parsed map[string]interface{}
	if err := json.Unmarshal(data, &parsed); err != nil {
		t.Fatalf("invalid JSON: %v\ngot: %s", err, data)
	}
	hooks, ok := parsed["hooks"].(map[string]interface{})
	if !ok {
		t.Fatalf("missing top-level 'hooks' key: %v", parsed)
	}
	if len(hooks) != 5 {
		t.Errorf("hooks has %d keys, want 5: %v", len(hooks), hooks)
	}

	for _, want := range []string{"SessionStart", "UserPromptSubmit", "PreToolUse", "PostToolUse", "Stop"} {
		entries, ok := hooks[want].([]interface{})
		if !ok || len(entries) != 1 {
			t.Fatalf("hooks.%s missing or not a single-entry array: %v", want, hooks[want])
		}
		entry, _ := entries[0].(map[string]interface{})
		if entry["matcher"] != "*" {
			t.Errorf("hooks.%s[0].matcher = %v, want '*'", want, entry["matcher"])
		}
		innerHooks, _ := entry["hooks"].([]interface{})
		if len(innerHooks) != 1 {
			t.Fatalf("hooks.%s[0].hooks should have exactly 1 entry: %v", want, innerHooks)
		}
		inner, _ := innerHooks[0].(map[string]interface{})
		if inner["type"] != "command" {
			t.Errorf("hooks.%s[0].hooks[0].type = %v, want 'command'", want, inner["type"])
		}
		cmd, _ := inner["command"].(string)
		if !strings.HasSuffix(cmd, "agent record-event") {
			t.Errorf("hooks.%s command = %q, want suffix 'agent record-event'", want, cmd)
		}
		for _, banned := range []string{"timeout", "async", "statusMessage"} {
			if _, has := inner[banned]; has {
				t.Errorf("hooks.%s[0].hooks[0] must not carry %q field: %v", want, banned, inner)
			}
		}
	}
}

// TestBuildCodexHooksJSON_NoUnknownTopLevelKeys guards against accidentally
// adding another top-level key beside "hooks" — codex rejects unrecognized
// keys in hooks.json.
func TestBuildCodexHooksJSON_NoUnknownTopLevelKeys(t *testing.T) {
	t.Parallel()
	data, err := buildCodexHooksJSON()
	if err != nil {
		t.Fatalf("buildCodexHooksJSON() error: %v", err)
	}
	var parsed map[string]interface{}
	if err := json.Unmarshal(data, &parsed); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}
	for key := range parsed {
		if key != "hooks" {
			t.Errorf("unexpected top-level key %q in hooks.json: %v", key, parsed)
		}
	}
}

// TestWriteCodexHooksForSession_CreatesParseableFile verifies the writer
// produces a valid hooks.json in the target dir.
func TestWriteCodexHooksForSession_CreatesParseableFile(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	if err := writeCodexHooksForSession(dir); err != nil {
		t.Fatalf("writeCodexHooksForSession() error: %v", err)
	}
	content := readFileString(t, filepath.Join(dir, "hooks.json"))
	var parsed map[string]interface{}
	if err := json.Unmarshal([]byte(content), &parsed); err != nil {
		t.Fatalf("hooks.json is not valid JSON: %v\ngot: %s", err, content)
	}
	if _, ok := parsed["hooks"]; !ok {
		t.Errorf("hooks.json missing 'hooks' key: %s", content)
	}
}

// TestWriteCodexHooksForSession_NonexistentDirErrors verifies the writer
// error path when the target directory doesn't exist.
func TestWriteCodexHooksForSession_NonexistentDirErrors(t *testing.T) {
	t.Parallel()
	err := writeCodexHooksForSession(filepath.Join(t.TempDir(), "does-not-exist"))
	if err == nil {
		t.Fatal("writeCodexHooksForSession() should error for a nonexistent dir")
	}
}

// TestWriteCodexProfileForSession_NoHooksJSON is a regression guard: the
// shared profile writer (app-server backend + console engine) must never
// produce a hooks.json — those profiles stay hook-free by design.
func TestWriteCodexProfileForSession_NoHooksJSON(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	dir := t.TempDir()
	if err := writeCodexProfileForSession(dir, ""); err != nil {
		t.Fatalf("writeCodexProfileForSession() error: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, "hooks.json")); !os.IsNotExist(err) {
		t.Errorf("writeCodexProfileForSession must not create hooks.json, stat err = %v", err)
	}
}

// TestWriteConsoleCodexProfile_NoHooksJSON is a regression guard: the console
// engine's profile writer must also stay hook-free.
func TestWriteConsoleCodexProfile_NoHooksJSON(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	dir := t.TempDir()
	workDir := t.TempDir()
	if err := WriteConsoleCodexProfile(dir, workDir, "/opt/nrflo_server", nil); err != nil {
		t.Fatalf("WriteConsoleCodexProfile() error: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, "hooks.json")); !os.IsNotExist(err) {
		t.Errorf("WriteConsoleCodexProfile must not create hooks.json, stat err = %v", err)
	}
}
