package cli

import (
	"context"
	"errors"
	"os/exec"
	"testing"
)

// TestRunConsole_ProbeMissingBinary_NoSessionCreated covers case 4: a missing
// CLI binary errors before any console session is created.
func TestRunConsole_ProbeMissingBinary_NoSessionCreated(t *testing.T) {
	f := newFakeConsoleServer(t)
	setConsoleFlags(t, "claude", "", "p1", f.url(), f.serviceToken)
	t.Setenv("PATH", t.TempDir()) // empty dir: claude is not found

	exitCode, err := runConsole(context.Background())
	if err == nil {
		t.Fatal("runConsole() expected an error when the CLI binary is missing")
	}
	if exitCode != -1 {
		t.Errorf("exitCode = %d, want -1", exitCode)
	}
	if len(f.createReqs) != 0 {
		t.Errorf("createReqs = %d, want 0 (no session should be opened before Probe succeeds)", len(f.createReqs))
	}
}

// TestRunConsole_SessionLifecycle_CreateAndCloseOnSuccess covers case 5: a
// clean child exit yields exactly one create and one close, with the
// resolved project on the create request.
func TestRunConsole_SessionLifecycle_CreateAndCloseOnSuccess(t *testing.T) {
	f := newFakeConsoleServer(t)
	fakeBinDir(t, "claude")
	setConsoleFlags(t, "claude", "", "p1", f.url(), f.serviceToken)
	stubRunConsoleChild(t, func(cmd *exec.Cmd) (int, error) { return 0, nil })

	exitCode, err := runConsole(context.Background())
	if err != nil {
		t.Fatalf("runConsole() error: %v", err)
	}
	if exitCode != 0 {
		t.Errorf("exitCode = %d, want 0", exitCode)
	}
	if len(f.createReqs) != 1 {
		t.Fatalf("createReqs = %d, want 1", len(f.createReqs))
	}
	if f.createReqs[0].project != "p1" {
		t.Errorf("create X-Project = %q, want p1", f.createReqs[0].project)
	}
	if len(f.closeReqs) != 1 {
		t.Errorf("closeReqs = %d, want 1", len(f.closeReqs))
	}
}

// TestRunConsole_SessionLifecycle_CloseOnNonZeroExit covers case 5: a
// non-zero child exit code still closes the session, and the exit code is
// propagated to the caller.
func TestRunConsole_SessionLifecycle_CloseOnNonZeroExit(t *testing.T) {
	f := newFakeConsoleServer(t)
	fakeBinDir(t, "claude")
	setConsoleFlags(t, "claude", "", "p1", f.url(), f.serviceToken)
	stubRunConsoleChild(t, func(cmd *exec.Cmd) (int, error) { return 7, nil })

	exitCode, err := runConsole(context.Background())
	if err != nil {
		t.Fatalf("runConsole() error: %v", err)
	}
	if exitCode != 7 {
		t.Errorf("exitCode = %d, want 7 (propagated from the child)", exitCode)
	}
	if len(f.closeReqs) != 1 {
		t.Errorf("closeReqs = %d, want 1 (session must close even on a non-zero exit)", len(f.closeReqs))
	}
}

// TestRunConsole_SessionLifecycle_CloseOnChildError covers case 5: an error
// starting/waiting on the child still closes the session via the deferred
// close.
func TestRunConsole_SessionLifecycle_CloseOnChildError(t *testing.T) {
	f := newFakeConsoleServer(t)
	fakeBinDir(t, "claude")
	setConsoleFlags(t, "claude", "", "p1", f.url(), f.serviceToken)
	stubRunConsoleChild(t, func(cmd *exec.Cmd) (int, error) { return -1, errors.New("boom") })

	_, err := runConsole(context.Background())
	if err == nil || err.Error() != "boom" {
		t.Fatalf("runConsole() error = %v, want boom", err)
	}
	if len(f.closeReqs) != 1 {
		t.Errorf("closeReqs = %d, want 1 (session must close even when the child errors)", len(f.closeReqs))
	}
}

// TestRunConsole_ExplicitProjectWinsOverCwd covers the one end-to-end cwd
// case named in the plan: an explicit --project short-circuits cwd
// auto-detect entirely, even when the cwd matches a different project.
func TestRunConsole_ExplicitProjectWinsOverCwd(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)
	f := newFakeConsoleServer(t)
	f.projects = []projRoot{{ID: "cwdproj", RootPath: dir}}
	fakeBinDir(t, "claude")
	setConsoleFlags(t, "claude", "", "explicit-proj", f.url(), f.serviceToken)
	stubRunConsoleChild(t, func(cmd *exec.Cmd) (int, error) { return 0, nil })

	if _, err := runConsole(context.Background()); err != nil {
		t.Fatalf("runConsole() error: %v", err)
	}
	if len(f.createReqs) != 1 || f.createReqs[0].project != "explicit-proj" {
		t.Errorf("create X-Project = %+v, want explicit-proj (explicit --project must beat cwd auto-detect)", f.createReqs)
	}
}
