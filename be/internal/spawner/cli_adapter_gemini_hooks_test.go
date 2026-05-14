package spawner

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestBuildGeminiHookCommand_EnvPrefix(t *testing.T) {
	t.Parallel()
	got := buildGeminiHookCommand("/usr/local/bin/nrflo", "sess-abc", "inst-xyz", "proj-1")
	want := "/usr/bin/env NRF_SESSION_ID=sess-abc NRF_WORKFLOW_INSTANCE_ID=inst-xyz NRFLO_PROJECT=proj-1 /usr/local/bin/nrflo agent record-event"
	if got != want {
		t.Errorf("buildGeminiHookCommand = %q\nwant                               %q", got, want)
	}
}

func TestBuildGeminiHookCommand_EmptyIDs(t *testing.T) {
	t.Parallel()
	got := buildGeminiHookCommand("nrflo", "", "", "")
	want := "/usr/bin/env nrflo agent record-event"
	if got != want {
		t.Errorf("buildGeminiHookCommand (empty IDs) = %q\nwant %q", got, want)
	}
}

func TestPrepareGeminiHome_SettingsJSON(t *testing.T) {
	fakeHome := t.TempDir()
	t.Setenv("HOME", fakeHome)

	opts := InteractivePrepOptions{
		SessionID:          "test-sess",
		WorkflowInstanceID: "test-inst",
		ProjectID:          "test-proj",
	}
	dir, cleanup, err := prepareGeminiHome(opts)
	if err != nil {
		t.Fatalf("prepareGeminiHome() error: %v", err)
	}
	t.Cleanup(cleanup)

	settingsPath := filepath.Join(dir, ".gemini", "settings.json")
	data, err := os.ReadFile(settingsPath)
	if err != nil {
		t.Fatalf("settings.json not created: %v", err)
	}

	// File mode must be 0o600
	info, err := os.Stat(settingsPath)
	if err != nil {
		t.Fatalf("stat settings.json: %v", err)
	}
	if got := info.Mode().Perm(); got != 0o600 {
		t.Errorf("settings.json mode = %04o, want 0600", got)
	}

	var settings geminiSettings
	if err := json.Unmarshal(data, &settings); err != nil {
		t.Fatalf("json.Unmarshal settings.json: %v", err)
	}
	if got, want := len(settings.Hooks), len(geminiHookEvents); got != want {
		t.Errorf("hooks count = %d, want %d", got, want)
	}
	for _, ev := range geminiHookEvents {
		hooks, ok := settings.Hooks[ev]
		if !ok {
			t.Errorf("missing hook event %q in settings.json", ev)
			continue
		}
		if len(hooks) != 1 {
			t.Errorf("hook event %q: %d entries, want 1", ev, len(hooks))
			continue
		}
		h := hooks[0]
		if !strings.HasPrefix(h.Command, "/usr/bin/env NRF_SESSION_ID=") {
			t.Errorf("hook[%s].command missing /usr/bin/env NRF_SESSION_ID= prefix: %q", ev, h.Command)
		}
		if !strings.HasSuffix(h.Command, "agent record-event") {
			t.Errorf("hook[%s].command missing 'agent record-event' suffix: %q", ev, h.Command)
		}
		if h.Timeout != 5 {
			t.Errorf("hook[%s].timeout = %d, want 5", ev, h.Timeout)
		}
	}
}

func TestPrepareGeminiHome_SymlinksOAuthCreds(t *testing.T) {
	fakeHome := t.TempDir()
	t.Setenv("HOME", fakeHome)

	geminiDir := filepath.Join(fakeHome, ".gemini")
	if err := os.MkdirAll(geminiDir, 0o700); err != nil {
		t.Fatalf("mkdir .gemini: %v", err)
	}
	srcPath := filepath.Join(geminiDir, "oauth_creds.json")
	if err := os.WriteFile(srcPath, []byte(`{"token":"initial"}`), 0o600); err != nil {
		t.Fatalf("write oauth_creds.json: %v", err)
	}

	opts := InteractivePrepOptions{SessionID: "sess-sym"}
	dir, cleanup, err := prepareGeminiHome(opts)
	if err != nil {
		t.Fatalf("prepareGeminiHome() error: %v", err)
	}
	t.Cleanup(cleanup)

	symPath := filepath.Join(dir, ".gemini", "oauth_creds.json")
	target, err := os.Readlink(symPath)
	if err != nil {
		t.Fatalf("Readlink(%q): %v", symPath, err)
	}
	if target != srcPath {
		t.Errorf("symlink target = %q, want %q", target, srcPath)
	}

	// Write to source; read via symlink — content must match.
	updated := []byte(`{"token":"updated"}`)
	if err := os.WriteFile(srcPath, updated, 0o600); err != nil {
		t.Fatalf("write updated auth: %v", err)
	}
	got, err := os.ReadFile(symPath)
	if err != nil {
		t.Fatalf("ReadFile via symlink: %v", err)
	}
	if string(got) != string(updated) {
		t.Errorf("symlink read = %q, want %q", got, updated)
	}
}

// TestPrepareGeminiHome_MissingSourcesNonFatal verifies that absent auth files
// (google_accounts.json, installation_id) do not cause prepareGeminiHome to fail.
func TestPrepareGeminiHome_MissingSourcesNonFatal(t *testing.T) {
	fakeHome := t.TempDir()
	t.Setenv("HOME", fakeHome)
	// No .gemini dir; all auth sources are absent.

	opts := InteractivePrepOptions{SessionID: "sess-missing"}
	dir, cleanup, err := prepareGeminiHome(opts)
	if err != nil {
		t.Fatalf("prepareGeminiHome() must not error for missing auth files: %v", err)
	}
	t.Cleanup(cleanup)
	if dir == "" {
		t.Error("returned dir is empty")
	}
}

// TestGeminiAdapter_PrepareInteractive_DirContainsSessionID verifies that the
// created GeminiHome directory name embeds the sessionID.
func TestGeminiAdapter_PrepareInteractive_DirContainsSessionID(t *testing.T) {
	t.Parallel()
	extras, cleanup, err := (&GeminiAdapter{}).PrepareInteractive(InteractivePrepOptions{
		SessionID: "sess-x",
	})
	if err != nil {
		t.Fatalf("PrepareInteractive() error: %v", err)
	}
	t.Cleanup(cleanup)

	if _, statErr := os.Stat(extras.GeminiHome); statErr != nil {
		t.Errorf("GeminiHome dir does not exist: %v", statErr)
	}
	base := filepath.Base(extras.GeminiHome)
	if !strings.Contains(base, "sess-x") {
		t.Errorf("dir base %q does not contain sessionID 'sess-x'", base)
	}
}

// TestGeminiAdapter_PrepareInteractive_Cleanup verifies the cleanup func removes the dir.
func TestGeminiAdapter_PrepareInteractive_Cleanup(t *testing.T) {
	t.Parallel()
	extras, cleanup, err := (&GeminiAdapter{}).PrepareInteractive(InteractivePrepOptions{
		SessionID: "sess-cleanup",
	})
	if err != nil {
		t.Fatalf("PrepareInteractive() error: %v", err)
	}
	if _, statErr := os.Stat(extras.GeminiHome); statErr != nil {
		t.Fatalf("GeminiHome does not exist before cleanup: %v", statErr)
	}
	cleanup()
	if _, statErr := os.Stat(extras.GeminiHome); !os.IsNotExist(statErr) {
		t.Errorf("cleanup() did not remove GeminiHome %q (stat: %v)", extras.GeminiHome, statErr)
	}
}

// TestGeminiAdapter_PrepareInteractive_CleanupIdempotent verifies double-cleanup does not panic.
func TestGeminiAdapter_PrepareInteractive_CleanupIdempotent(t *testing.T) {
	t.Parallel()
	_, cleanup, err := (&GeminiAdapter{}).PrepareInteractive(InteractivePrepOptions{
		SessionID: "sess-idem",
	})
	if err != nil {
		t.Fatalf("PrepareInteractive() error: %v", err)
	}
	cleanup()
	cleanup() // must not panic
}

// TestGeminiAdapter_PrepareInteractive_FailureReturnsError verifies an error is returned
// when the temp directory cannot be created, and the cleanup noop does not panic.
func TestGeminiAdapter_PrepareInteractive_FailureReturnsError(t *testing.T) {
	t.Setenv("TMPDIR", "/nonexistent-nrflo-test-dir-xyz")
	_, cleanup, err := (&GeminiAdapter{}).PrepareInteractive(InteractivePrepOptions{
		SessionID: "sess-fail",
	})
	if err == nil {
		cleanup()
		t.Error("PrepareInteractive() should return error when TMPDIR is invalid")
		return
	}
	cleanup() // must not panic even on error path
}
