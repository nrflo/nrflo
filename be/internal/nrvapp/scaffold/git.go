package scaffold

import (
	"fmt"
	"os"
	"os/exec"
)

// initGit runs git init + git add + git commit in outDir.
// Best-effort: errors are returned but not fatal to the scaffold operation.
func initGit(outDir string) error {
	if _, err := exec.LookPath("git"); err != nil {
		return fmt.Errorf("git not found in PATH: %w", err)
	}

	run := func(args ...string) error {
		cmd := exec.Command("git", args...)
		cmd.Dir = outDir
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
		return cmd.Run()
	}

	if err := run("init"); err != nil {
		return fmt.Errorf("git init: %w", err)
	}
	if err := run("add", "."); err != nil {
		return fmt.Errorf("git add: %w", err)
	}
	if err := run("commit", "-m", "Initial customer config scaffold"); err != nil {
		return fmt.Errorf("git commit: %w", err)
	}
	return nil
}
