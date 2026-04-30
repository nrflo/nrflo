package spawner

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"time"

	"be/internal/logger"
)

// nrfloTempDir is the well-known scratch directory for prompt files,
// system-prompt-suffix tmp files, and context-saver prompts. It is shared
// with the worktrees subdirectory and the Unix socket so the cleanup walker
// must skip those.
const nrfloTempDir = "/tmp/nrflo"

// CleanupOrphanedTempFiles removes leftover prompt + suffix temp files in
// /tmp/nrflo and codex hook profile directories in os.TempDir() that are
// older than maxAge. Called once at server startup to reclaim files leaked
// when a previous server process was killed mid-run before the per-spawn
// wait goroutine could remove them.
//
// Safe to call concurrently with active runs: only files older than maxAge
// are touched, and a freshly-spawned agent's temp files are always newer
// than the threshold.
func CleanupOrphanedTempFiles(maxAge time.Duration) {
	ctx := context.Background()
	cutoff := time.Now().Add(-maxAge)

	// /tmp/nrflo: prompt-*.md and system-suffix-*.md files.
	if entries, err := os.ReadDir(nrfloTempDir); err == nil {
		removed := 0
		for _, entry := range entries {
			name := entry.Name()
			// Preserve the live socket and the worktrees subtree.
			if name == "nrflo.sock" || name == "worktrees" {
				continue
			}
			// Only touch regular files matching our temp-file naming. The
			// suffix files start with "system-suffix-"; prompt files end
			// with "-*.md" via os.CreateTemp's pattern.
			if entry.IsDir() {
				continue
			}
			if !strings.HasSuffix(name, ".md") {
				continue
			}
			info, err := entry.Info()
			if err != nil || info.ModTime().After(cutoff) {
				continue
			}
			if err := os.Remove(filepath.Join(nrfloTempDir, name)); err == nil {
				removed++
			}
		}
		if removed > 0 {
			logger.Info(ctx, "temp cleanup: orphaned prompt/suffix files", "deleted", removed, "dir", nrfloTempDir)
		}
	}

	// os.TempDir(): codex hook profile dirs (nrflo-codex-<session-id>-*).
	if entries, err := os.ReadDir(os.TempDir()); err == nil {
		removed := 0
		for _, entry := range entries {
			if !entry.IsDir() || !strings.HasPrefix(entry.Name(), "nrflo-codex-") {
				continue
			}
			info, err := entry.Info()
			if err != nil || info.ModTime().After(cutoff) {
				continue
			}
			if err := os.RemoveAll(filepath.Join(os.TempDir(), entry.Name())); err == nil {
				removed++
			}
		}
		if removed > 0 {
			logger.Info(ctx, "temp cleanup: orphaned codex profile dirs", "deleted", removed)
		}
	}
}
