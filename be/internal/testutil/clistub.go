// Package testutil holds helpers shared by test packages only.
package testutil

import (
	"os"
	"path/filepath"
)

// StubAgentCLIsOnPath writes non-runnable claude/codex stubs into dir/bin and
// prepends it to PATH, so service.CLIAvailable (a LookPath probe) reports both
// as installed regardless of the host. Call from TestMain.
func StubAgentCLIsOnPath(dir string) error {
	binDir := filepath.Join(dir, "bin")
	if err := os.MkdirAll(binDir, 0o755); err != nil {
		return err
	}
	for _, name := range []string{"claude", "codex"} {
		if err := os.WriteFile(filepath.Join(binDir, name), []byte("#!/bin/sh\nexit 1\n"), 0o755); err != nil {
			return err
		}
	}
	return os.Setenv("PATH", binDir+string(os.PathListSeparator)+os.Getenv("PATH"))
}
