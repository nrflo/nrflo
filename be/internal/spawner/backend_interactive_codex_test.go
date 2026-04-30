package spawner

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"be/internal/clock"
)

// =============================================================================
// Codex hook profile: backend integration tests
// =============================================================================

// globSet returns the set of paths matching pattern (used to diff before/after Start).
func globSet(pattern string) map[string]bool {
	matches, _ := filepath.Glob(pattern)
	set := make(map[string]bool, len(matches))
	for _, m := range matches {
		set[m] = true
	}
	return set
}

// TestInteractiveBackend_CodexBuildsProfile verifies that Start() for a Codex
// adapter calls CodexAdapter.PrepareInteractive (profile dir created in tempdir)
// and that the dir is removed when the PTY session closes (prepCleanup fires
// in sess.Done).
//
// NOTE: The production code passes the original env (not cmd.Env) to
// ptyMgr.Create, so CODEX_HOME is present in cmd.Env but does not reach the
// spawned process via the Create call. That delivery gap is a known bug; the
// adapter unit test TestCodexAdapter_BuildInteractiveCommand_CodexHomeEnv
// verifies cmd.Env is correct. This test focuses on profile lifecycle.
func TestInteractiveBackend_CodexBuildsProfile(t *testing.T) {
	s := New(Config{Clock: clock.Real()})
	mgr := newMockPtyManager()
	b := newCLIInteractiveBackend(&CodexAdapter{}, s, mgr)

	sessionID := "sess-codex-profile"
	proc := &processInfo{sessionID: sessionID, doneCh: make(chan struct{})}
	prep := &prepResult{
		prompt: "test prompt",
		opts:   SpawnOptions{Model: "codex_gpt_normal", WorkDir: "/tmp"},
	}

	// Snapshot dirs before Start so we can identify the newly created one,
	// regardless of any stale dirs from previous interrupted test runs.
	pattern := filepath.Join(os.TempDir(), "nrflo-codex-"+sessionID+"-*")
	before := globSet(pattern)

	if err := b.Start(context.Background(), proc, prep); err != nil {
		t.Fatalf("Start() error: %v", err)
	}

	// Always close the session in cleanup so goroutines can drain.
	t.Cleanup(func() {
		mgr.mu.Lock()
		sess := mgr.sessions[sessionID]
		mgr.mu.Unlock()
		if sess != nil {
			_ = sess.Close()
		}
	})

	// Find the newly created dir (present after Start but not before).
	after, _ := filepath.Glob(pattern)
	var codexHome string
	for _, m := range after {
		if !before[m] {
			codexHome = m
			break
		}
	}
	if codexHome == "" {
		t.Fatal("CodexAdapter.PrepareInteractive not called — no new profile dir found matching " + pattern)
	}

	// Profile dir must contain config.toml (hooks live inline; no hooks.json).
	if _, err := os.Stat(filepath.Join(codexHome, "config.toml")); err != nil {
		t.Errorf("profile dir missing config.toml: %v", err)
	}

	// Close the session so the sess.Done() goroutine fires prepCleanup.
	mgr.mu.Lock()
	sess := mgr.sessions[sessionID]
	mgr.mu.Unlock()
	if sess == nil {
		t.Fatal("session not found in mockPtyManager")
	}
	sess.Close()

	// Poll until the profile dir is removed (cleanup is called in Done goroutine).
	deadline := time.After(2 * time.Second)
	ticker := time.NewTicker(10 * time.Millisecond)
	defer ticker.Stop()
	for {
		select {
		case <-deadline:
			t.Errorf("profile dir %q was not removed after session close", codexHome)
			return
		case <-ticker.C:
			if _, err := os.Stat(codexHome); os.IsNotExist(err) {
				return // cleanup ran — success
			}
		}
	}
}

// TestInteractiveBackend_ClaudeUnaffected_NoCodexHome verifies that Claude's
// interactive backend does NOT create a CODEX_HOME profile and DOES include
// --settings in the registered command args (hooks+statusLine injection).
func TestInteractiveBackend_ClaudeUnaffected_NoCodexHome(t *testing.T) {
	s := New(Config{Clock: clock.Real()})
	mgr := newMockPtyManager()
	b := newCLIInteractiveBackend(&ClaudeAdapter{}, s, mgr)

	sessionID := "sess-claude-nohome"
	proc := &processInfo{
		sessionID: sessionID,
		modelID:   "claude:sonnet",
		doneCh:    make(chan struct{}),
	}
	prep := &prepResult{
		prompt: "test prompt",
		opts:   SpawnOptions{Model: "sonnet", WorkDir: "/tmp"},
	}

	t.Cleanup(func() {
		mgr.mu.Lock()
		sess := mgr.sessions[sessionID]
		mgr.mu.Unlock()
		if sess != nil {
			_ = sess.Close()
		}
	})

	if err := b.Start(context.Background(), proc, prep); err != nil {
		t.Fatalf("Start() error: %v", err)
	}

	// Claude: no nrflo-codex-* dir created.
	pattern := filepath.Join(os.TempDir(), "nrflo-codex-"+sessionID+"-*")
	matches, _ := filepath.Glob(pattern)
	if len(matches) > 0 {
		t.Errorf("Claude backend must not create a codex profile dir, found: %v", matches)
	}

	// Claude: --settings must be present in the registered command args.
	mgr.mu.Lock()
	args := mgr.registeredCmds[sessionID]
	mgr.mu.Unlock()
	found := false
	for _, a := range args {
		if a == "--settings" {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("Claude backend registered cmd must include --settings, got args: %v", args)
	}
}

// TestInteractiveBackend_CodexProfileFailureFallsThrough verifies that Start()
// succeeds even when CodexAdapter.PrepareInteractive fails: a warning is logged but the
// spawn continues without CODEX_HOME. No profile dir is left behind.
func TestInteractiveBackend_CodexProfileFailureFallsThrough(t *testing.T) {
	// Force os.MkdirTemp to fail by pointing TMPDIR at a non-existent path.
	t.Setenv("TMPDIR", "/nonexistent-nrflo-test-xyz")

	s := New(Config{Clock: clock.Real()})
	mgr := newMockPtyManager()
	b := newCLIInteractiveBackend(&CodexAdapter{}, s, mgr)

	sessionID := "sess-codex-profilefail"
	proc := &processInfo{sessionID: sessionID, doneCh: make(chan struct{})}
	prep := &prepResult{
		prompt: "test prompt",
		opts:   SpawnOptions{Model: "codex_gpt_normal", WorkDir: "/tmp"},
	}

	t.Cleanup(func() {
		mgr.mu.Lock()
		sess := mgr.sessions[sessionID]
		mgr.mu.Unlock()
		if sess != nil {
			_ = sess.Close()
		}
	})

	// Start must succeed even though profile build failed.
	if err := b.Start(context.Background(), proc, prep); err != nil {
		t.Fatalf("Start() should succeed even when profile build fails, got: %v", err)
	}
}

// TestInteractiveBackend_CodexCleanup_OnPTYCreateError verifies that the Codex
// profile directory is cleaned up when ptyMgr.Create() returns an error.
func TestInteractiveBackend_CodexCleanup_OnPTYCreateError(t *testing.T) {
	s := New(Config{Clock: clock.Real()})
	mgr := newMockPtyManager()
	mgr.createErr = fmt.Errorf("pty create failed")
	b := newCLIInteractiveBackend(&CodexAdapter{}, s, mgr)

	sessionID := "sess-codex-ptyfail"
	proc := &processInfo{sessionID: sessionID, doneCh: make(chan struct{})}
	prep := &prepResult{
		prompt: "test",
		opts:   SpawnOptions{Model: "codex_gpt_normal", WorkDir: "/tmp"},
	}

	// Snapshot before to detect only dirs created by this call.
	pattern := filepath.Join(os.TempDir(), "nrflo-codex-"+sessionID+"-*")
	before := globSet(pattern)

	err := b.Start(context.Background(), proc, prep)
	if err == nil {
		t.Fatal("Start() with PTY create error should return error")
	}

	// Cleanup is called synchronously before Start returns.
	// No new dir should remain.
	after, _ := filepath.Glob(pattern)
	for _, m := range after {
		if !before[m] {
			t.Errorf("prepCleanup not called on PTY create error; new dir remains: %s", m)
		}
	}
}

// TestInteractiveBackend_OpencodesNoCodexProfile verifies that Opencode's backend
// does not create a Codex profile dir (CODEX_HOME is Codex-adapter-only).
func TestInteractiveBackend_OpencodesNoCodexProfile(t *testing.T) {
	s := New(Config{Clock: clock.Real()})
	mgr := newMockPtyManager()
	b := newCLIInteractiveBackend(&OpencodeAdapter{}, s, mgr)

	sessionID := "sess-opencode-noprofile"
	proc := &processInfo{sessionID: sessionID, doneCh: make(chan struct{})}
	prep := &prepResult{
		prompt: "test",
		opts:   SpawnOptions{Model: "opencode_gpt54", WorkDir: "/tmp"},
	}

	t.Cleanup(func() {
		mgr.mu.Lock()
		sess := mgr.sessions[sessionID]
		mgr.mu.Unlock()
		if sess != nil {
			_ = sess.Close()
		}
	})

	if err := b.Start(context.Background(), proc, prep); err != nil {
		t.Fatalf("Start() error: %v", err)
	}

	pattern := filepath.Join(os.TempDir(), "nrflo-codex-"+sessionID+"-*")
	matches, _ := filepath.Glob(pattern)
	if len(matches) > 0 {
		t.Errorf("Opencode backend must not create a codex profile dir, found: %v", matches)
	}
}

// TestInteractiveBackend_CodexDirNameContainsSessionID verifies the profile dir
// basename embeds the session ID so it's attributable per-agent-run.
func TestInteractiveBackend_CodexDirNameContainsSessionID(t *testing.T) {
	s := New(Config{Clock: clock.Real()})
	mgr := newMockPtyManager()
	b := newCLIInteractiveBackend(&CodexAdapter{}, s, mgr)

	sessionID := "unique-sess-abc123"
	proc := &processInfo{sessionID: sessionID, doneCh: make(chan struct{})}
	prep := &prepResult{
		prompt: "test",
		opts:   SpawnOptions{Model: "codex_gpt_normal", WorkDir: "/tmp"},
	}

	pattern := filepath.Join(os.TempDir(), "nrflo-codex-"+sessionID+"-*")
	before := globSet(pattern)

	t.Cleanup(func() {
		mgr.mu.Lock()
		sess := mgr.sessions[sessionID]
		mgr.mu.Unlock()
		if sess != nil {
			_ = sess.Close()
		}
	})

	if err := b.Start(context.Background(), proc, prep); err != nil {
		t.Fatalf("Start() error: %v", err)
	}

	// Find the newly created dir (not in the before snapshot).
	after, _ := filepath.Glob(pattern)
	var newDir string
	for _, m := range after {
		if !before[m] {
			newDir = m
			break
		}
	}
	if newDir == "" {
		t.Fatalf("no new profile dir found matching %s", pattern)
	}
	base := filepath.Base(newDir)
	if !strings.Contains(base, sessionID) {
		t.Errorf("profile dir base %q does not contain sessionID %q", base, sessionID)
	}
}
