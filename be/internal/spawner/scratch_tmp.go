package spawner

import (
	"fmt"
	"os"
)

// scratchDir is the shared scratch directory for agent prompt / system-prompt
// temp files written before a spawn. It is a fixed path so the spawner does not
// depend on NRFLO_HOME, and is intentionally outside any per-server home.
const scratchDir = "/tmp/nrflo"

// createScratchTemp creates a temp file under scratchDir, creating the directory
// first. os.CreateTemp does not create parent directories, so on a fresh machine
// (or after a /tmp reaper sweep) the first spawn would otherwise fail with
// "no such file or directory". MkdirAll is idempotent, so this is also
// self-healing if the directory is removed under a long-running server.
func createScratchTemp(pattern string) (*os.File, error) {
	return createTempInDir(scratchDir, pattern)
}

// createTempInDir is createScratchTemp with an explicit base directory, so the
// dir-creating behavior can be exercised in tests without touching scratchDir.
func createTempInDir(dir, pattern string) (*os.File, error) {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, fmt.Errorf("create scratch dir %s: %w", dir, err)
	}
	return os.CreateTemp(dir, pattern)
}
