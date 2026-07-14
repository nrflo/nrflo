package cli

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

// setConsoleFlags saves and restores the consoleCmd package-level flag vars
// around a test — runConsole reads them directly (Cobra flag vars), so a test
// mutating them must not leak state into other tests sharing this package.
func setConsoleFlags(t *testing.T, cliName, model, project, server, token string) {
	t.Helper()
	origCLI, origModel, origProject, origServer, origToken :=
		consoleCLIFlag, consoleModelFlag, consoleProjectFlag, consoleServerFlag, consoleTokenFlag
	consoleCLIFlag, consoleModelFlag, consoleProjectFlag, consoleServerFlag, consoleTokenFlag =
		cliName, model, project, server, token
	t.Cleanup(func() {
		consoleCLIFlag, consoleModelFlag, consoleProjectFlag, consoleServerFlag, consoleTokenFlag =
			origCLI, origModel, origProject, origServer, origToken
	})
}

// fakeBinDir creates a temp dir containing a (never-executed) file for each
// given name and points PATH at it for the test's duration, so
// console.GetDriver(name).Probe()'s exec.LookPath succeeds without a real CLI
// installed. runConsoleChild is always stubbed alongside this (Rule 4: no
// real CLI execution in tests), so the fake binary's content never matters.
func fakeBinDir(t *testing.T, names ...string) {
	t.Helper()
	dir := t.TempDir()
	for _, name := range names {
		if err := os.WriteFile(filepath.Join(dir, name), []byte("#!/bin/sh\n"), 0o755); err != nil {
			t.Fatalf("write fake binary %s: %v", name, err)
		}
	}
	t.Setenv("PATH", dir)
}

// stubRunConsoleChild replaces runConsoleChild for the test's duration,
// restoring the original on cleanup.
func stubRunConsoleChild(t *testing.T, fn func(cmd *exec.Cmd) (int, error)) {
	t.Helper()
	orig := runConsoleChild
	runConsoleChild = fn
	t.Cleanup(func() { runConsoleChild = orig })
}
