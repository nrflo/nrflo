package spawner

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// =============================================================================
// WriteCodexProfile tests
// =============================================================================

// TestWriteCodexProfile_ConfigTOML verifies that config.toml is created with
// the [features] section and codex_hooks = true.
func TestWriteCodexProfile_ConfigTOML(t *testing.T) {
	dir := t.TempDir()
	if err := WriteCodexProfile(dir, "/usr/local/bin/nrflo"); err != nil {
		t.Fatalf("WriteCodexProfile() error: %v", err)
	}
	data, err := os.ReadFile(filepath.Join(dir, "config.toml"))
	if err != nil {
		t.Fatalf("config.toml not created: %v", err)
	}
	content := string(data)
	if !strings.Contains(content, "[features]") {
		t.Errorf("config.toml missing [features] section: %s", content)
	}
	if !strings.Contains(content, "codex_hooks = true") {
		t.Errorf("config.toml missing codex_hooks = true: %s", content)
	}
}

// TestWriteCodexProfile_HooksJSON_Structure verifies that hooks.json is valid
// JSON with PreToolUse and PostToolUse arrays, each containing one hook entry.
func TestWriteCodexProfile_HooksJSON_Structure(t *testing.T) {
	dir := t.TempDir()
	nrfloPath := "/usr/local/bin/nrflo"
	if err := WriteCodexProfile(dir, nrfloPath); err != nil {
		t.Fatalf("WriteCodexProfile() error: %v", err)
	}
	data, err := os.ReadFile(filepath.Join(dir, "hooks.json"))
	if err != nil {
		t.Fatalf("hooks.json not created: %v", err)
	}
	var hooks map[string]interface{}
	if err := json.Unmarshal(data, &hooks); err != nil {
		t.Fatalf("hooks.json invalid JSON: %v", err)
	}
	for _, event := range []string{"PreToolUse", "PostToolUse"} {
		arr, ok := hooks[event].([]interface{})
		if !ok || len(arr) != 1 {
			t.Errorf("hooks.json %s: expected 1-element array, got %v", event, hooks[event])
			continue
		}
		entry, ok := arr[0].(map[string]interface{})
		if !ok {
			t.Errorf("hooks.json %s[0] is not an object", event)
			continue
		}
		if entry["matcher"] != "*" {
			t.Errorf("hooks.json %s[0].matcher = %v, want *", event, entry["matcher"])
		}
		hooksArr, ok := entry["hooks"].([]interface{})
		if !ok || len(hooksArr) != 1 {
			t.Errorf("hooks.json %s[0].hooks has wrong shape: %v", event, entry["hooks"])
			continue
		}
		hookObj, ok := hooksArr[0].(map[string]interface{})
		if !ok {
			t.Errorf("hooks.json %s[0].hooks[0] is not an object", event)
			continue
		}
		wantCmd := nrfloPath + " agent record-event"
		if hookObj["command"] != wantCmd {
			t.Errorf("hooks.json %s command = %v, want %q", event, hookObj["command"], wantCmd)
		}
		if hookObj["type"] != "command" {
			t.Errorf("hooks.json %s type = %v, want command", event, hookObj["type"])
		}
		if timeout, _ := hookObj["timeout"].(float64); int(timeout) != 5 {
			t.Errorf("hooks.json %s timeout = %v, want 5", event, hookObj["timeout"])
		}
	}
}

// TestWriteCodexProfile_HooksJSON_CustomPath verifies the hook command uses the
// supplied nrfloPath value, not a hardcoded default.
func TestWriteCodexProfile_HooksJSON_CustomPath(t *testing.T) {
	dir := t.TempDir()
	nrfloPath := "/custom/path/to/nrflo"
	if err := WriteCodexProfile(dir, nrfloPath); err != nil {
		t.Fatalf("WriteCodexProfile() error: %v", err)
	}
	data, err := os.ReadFile(filepath.Join(dir, "hooks.json"))
	if err != nil {
		t.Fatalf("hooks.json not created: %v", err)
	}
	if !strings.Contains(string(data), nrfloPath+" agent record-event") {
		t.Errorf("hooks.json command does not use custom nrfloPath %q: %s", nrfloPath, data)
	}
}

// TestWriteCodexProfile_BothFilesExist verifies both config.toml and hooks.json
// are created in the target directory.
func TestWriteCodexProfile_BothFilesExist(t *testing.T) {
	dir := t.TempDir()
	if err := WriteCodexProfile(dir, "/usr/local/bin/nrflo"); err != nil {
		t.Fatalf("WriteCodexProfile() error: %v", err)
	}
	for _, name := range []string{"config.toml", "hooks.json"} {
		if _, err := os.Stat(filepath.Join(dir, name)); err != nil {
			t.Errorf("expected %s to exist: %v", name, err)
		}
	}
}

// TestWriteCodexProfile_InvalidDir verifies WriteCodexProfile returns an error
// when the target directory does not exist.
func TestWriteCodexProfile_InvalidDir(t *testing.T) {
	err := WriteCodexProfile("/nonexistent-dir-xyz/sub", "/usr/local/bin/nrflo")
	if err == nil {
		t.Error("WriteCodexProfile() to non-existent dir should return error")
	}
}

// =============================================================================
// BuildCodexHookProfile tests
// =============================================================================

// TestBuildCodexHookProfile_DirContainsSessionID verifies the created directory
// name embeds the processInfo sessionID.
func TestBuildCodexHookProfile_DirContainsSessionID(t *testing.T) {
	proc := &processInfo{sessionID: "sess-x", doneCh: make(chan struct{})}
	dir, cleanup, err := BuildCodexHookProfile(proc)
	if err != nil {
		t.Fatalf("BuildCodexHookProfile() error: %v", err)
	}
	t.Cleanup(cleanup)

	if _, statErr := os.Stat(dir); statErr != nil {
		t.Errorf("profile dir does not exist: %v", statErr)
	}
	base := filepath.Base(dir)
	if !strings.Contains(base, "sess-x") {
		t.Errorf("dir base %q does not contain sessionID 'sess-x'", base)
	}
}

// TestBuildCodexHookProfile_Cleanup verifies that calling the returned cleanup
// func removes the profile directory.
func TestBuildCodexHookProfile_Cleanup(t *testing.T) {
	proc := &processInfo{sessionID: "sess-cleanup", doneCh: make(chan struct{})}
	dir, cleanup, err := BuildCodexHookProfile(proc)
	if err != nil {
		t.Fatalf("BuildCodexHookProfile() error: %v", err)
	}
	if _, statErr := os.Stat(dir); statErr != nil {
		t.Fatalf("dir does not exist before cleanup: %v", statErr)
	}
	cleanup()
	if _, statErr := os.Stat(dir); !os.IsNotExist(statErr) {
		t.Errorf("cleanup() did not remove dir %q (stat: %v)", dir, statErr)
	}
}

// TestBuildCodexHookProfile_CleanupIdempotent verifies that calling cleanup twice
// does not panic.
func TestBuildCodexHookProfile_CleanupIdempotent(t *testing.T) {
	proc := &processInfo{sessionID: "sess-idempotent", doneCh: make(chan struct{})}
	_, cleanup, err := BuildCodexHookProfile(proc)
	if err != nil {
		t.Fatalf("BuildCodexHookProfile() error: %v", err)
	}
	cleanup()
	cleanup() // second call must not panic
}

// TestBuildCodexHookProfile_FailureReturnsError verifies that BuildCodexHookProfile
// returns an error when the temp directory cannot be created, and the returned
// cleanup is a no-op (does not panic).
func TestBuildCodexHookProfile_FailureReturnsError(t *testing.T) {
	t.Setenv("TMPDIR", "/nonexistent-nrflo-test-dir-xyz")
	proc := &processInfo{sessionID: "sess-fail", doneCh: make(chan struct{})}
	_, cleanup, err := BuildCodexHookProfile(proc)
	if err == nil {
		cleanup()
		t.Error("BuildCodexHookProfile() should return error when TMPDIR is invalid")
		return
	}
	cleanup() // must not panic even on error path
}
