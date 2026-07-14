package pty

import (
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// TestManager_Create_NoLaunchRegistered_ReturnsError verifies that Create
// returns an honest error (not a hardcoded claude fallback) when no launch
// was registered for a brand-new session ID.
func TestManager_Create_NoLaunchRegistered_ReturnsError(t *testing.T) {
	m := NewManager()

	sess, err := m.Create("sess-never-registered", t.TempDir(), buildTestEnv())
	if err == nil {
		t.Fatal("Create() with no registered launch should return an error")
	}
	if sess != nil {
		t.Errorf("Create() with no registered launch should return nil session, got %v", sess)
	}
	if !strings.Contains(err.Error(), "no PTY launch registered") {
		t.Errorf("error = %q, want to contain 'no PTY launch registered'", err.Error())
	}
}

// TestManager_Create_DirOverridesWorkDir verifies that a non-empty Launch.Dir
// wins over the workDir argument passed to Create.
func TestManager_Create_DirOverridesWorkDir(t *testing.T) {
	m := NewManager()

	passedWorkDir := t.TempDir()
	launchDir, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatalf("EvalSymlinks: %v", err)
	}

	m.RegisterLaunch("sess-dir", Launch{Command: "sh", Args: []string{"-c", "pwd"}, Dir: launchDir})

	sess, err := m.Create("sess-dir", passedWorkDir, buildTestEnv())
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	t.Cleanup(func() { _ = sess.Close() })

	buf := make([]byte, 512)
	done := make(chan struct{})
	var out string
	go func() {
		defer close(done)
		n, _ := sess.Read(buf)
		out = string(buf[:n])
	}()
	select {
	case <-done:
	case <-time.After(3 * time.Second):
		t.Fatal("Read timed out")
	}

	if !strings.Contains(out, launchDir) {
		t.Errorf("pwd output = %q, want to contain launch Dir %q", out, launchDir)
	}
	if strings.Contains(out, passedWorkDir) {
		t.Errorf("pwd output = %q, should not contain the overridden workDir %q", out, passedWorkDir)
	}
}

// TestManager_Create_EnvMergedWithOverrides verifies that Create merges
// Launch.Env on top of the env argument (override wins, base entries preserved).
func TestManager_Create_EnvMergedWithOverrides(t *testing.T) {
	m := NewManager()

	base := append(buildTestEnv(), "FOO=base-value", "KEEPME=still-here")
	m.RegisterLaunch("sess-env", Launch{
		Command: "sh",
		Args:    []string{"-c", "echo FOO=$FOO KEEPME=$KEEPME"},
		Env:     []string{"FOO=overridden-value"},
	})

	sess, err := m.Create("sess-env", t.TempDir(), base)
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	t.Cleanup(func() { _ = sess.Close() })

	buf := make([]byte, 512)
	done := make(chan struct{})
	var out string
	go func() {
		defer close(done)
		n, _ := sess.Read(buf)
		out = string(buf[:n])
	}()
	select {
	case <-done:
	case <-time.After(3 * time.Second):
		t.Fatal("Read timed out")
	}

	if !strings.Contains(out, "FOO=overridden-value") {
		t.Errorf("output = %q, want FOO overridden to 'overridden-value'", out)
	}
	if !strings.Contains(out, "KEEPME=still-here") {
		t.Errorf("output = %q, want base-only KEEPME preserved", out)
	}
}

// ── mergeLaunchEnv (unit) ─────────────────────────────────────────────────────

func TestMergeLaunchEnv_NoOverridesReturnsBase(t *testing.T) {
	base := []string{"A=1", "B=2"}
	got := mergeLaunchEnv(base, nil)
	if len(got) != 2 || got[0] != "A=1" || got[1] != "B=2" {
		t.Errorf("mergeLaunchEnv(base, nil) = %v, want unchanged base", got)
	}
}

func TestMergeLaunchEnv_OverrideReplacesExistingKey(t *testing.T) {
	base := []string{"A=1", "B=2"}
	got := mergeLaunchEnv(base, []string{"A=99"})

	if strings.Contains(strings.Join(got, ","), "A=1") {
		t.Errorf("mergeLaunchEnv() = %v, old A=1 should be dropped", got)
	}
	found := false
	for _, e := range got {
		if e == "A=99" {
			found = true
		}
	}
	if !found {
		t.Errorf("mergeLaunchEnv() = %v, want A=99 present", got)
	}
	if len(got) != 2 {
		t.Errorf("mergeLaunchEnv() = %v, want len 2 (B unchanged, A replaced)", got)
	}
}

func TestMergeLaunchEnv_MultipleOverridesSameKeyLastWins(t *testing.T) {
	base := []string{"A=1"}
	got := mergeLaunchEnv(base, []string{"A=2", "A=3"})

	if len(got) != 1 || got[0] != "A=3" {
		t.Errorf("mergeLaunchEnv() = %v, want [\"A=3\"]", got)
	}
}

func TestMergeLaunchEnv_PreservesUntouchedKeyOrder(t *testing.T) {
	base := []string{"A=1", "B=2", "C=3"}
	got := mergeLaunchEnv(base, []string{"B=99"})

	want := []string{"A=1", "C=3", "B=99"}
	if len(got) != len(want) {
		t.Fatalf("mergeLaunchEnv() = %v, want %v", got, want)
	}
	for i, v := range want {
		if got[i] != v {
			t.Errorf("mergeLaunchEnv()[%d] = %q, want %q (full: %v)", i, got[i], v, got)
		}
	}
}
