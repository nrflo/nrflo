package service

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"
)

// WorktreeService manages git worktree lifecycle for workflow isolation.
type WorktreeService struct{}

// worktreeBasePath is the base directory for all worktrees.
const worktreeBasePath = "/tmp/nrworkflow/worktrees"

// Setup creates a git branch from defaultBranch and a worktree for it.
// Returns the absolute path to the worktree directory.
func (s *WorktreeService) Setup(projectRoot, defaultBranch, branchName string) (string, error) {
	if err := validateRepoPath(projectRoot); err != nil {
		return "", fmt.Errorf("worktree setup: %w", err)
	}
	if err := validateBranch(defaultBranch); err != nil {
		return "", fmt.Errorf("worktree setup: invalid default branch: %w", err)
	}
	if err := validateBranch(branchName); err != nil {
		return "", fmt.Errorf("worktree setup: invalid branch name: %w", err)
	}

	worktreePath := filepath.Join(worktreeBasePath, branchName)

	// Create branch from defaultBranch
	_, err := runGit(projectRoot, "branch", branchName, defaultBranch)
	if err != nil {
		// Branch may exist from a previous crashed run — attempt cleanup and retry once
		cleanupErr := s.Cleanup(projectRoot, branchName, worktreePath)
		if cleanupErr != nil {
			return "", fmt.Errorf("worktree setup: branch creation failed and cleanup failed: %w (original: %v)", cleanupErr, err)
		}
		_, err = runGit(projectRoot, "branch", branchName, defaultBranch)
		if err != nil {
			return "", fmt.Errorf("worktree setup: branch creation failed after cleanup: %w", err)
		}
	}

	// Ensure parent directory exists
	if err := os.MkdirAll(filepath.Dir(worktreePath), 0o755); err != nil {
		return "", fmt.Errorf("worktree setup: failed to create parent dir: %w", err)
	}

	// Create worktree
	_, err = runGit(projectRoot, "worktree", "add", worktreePath, branchName)
	if err != nil {
		// Clean up the branch we just created
		runGit(projectRoot, "branch", "-D", branchName)
		return "", fmt.Errorf("worktree setup: worktree creation failed: %w", err)
	}

	absPath, err := filepath.Abs(worktreePath)
	if err != nil {
		return "", fmt.Errorf("worktree setup: failed to resolve path: %w", err)
	}
	return absPath, nil
}

// mergeRetryAttempts is the number of checkout+merge attempts before giving up.
const mergeRetryAttempts = 5

// mergeRetryDelay is the wait between retry attempts.
const mergeRetryDelay = 500 * time.Millisecond

// MergeAndCleanup removes the worktree, merges the branch into defaultBranch
// (with retry for transient index.lock contention), and deletes the branch.
// If merge fails, returns an error and preserves the branch for manual resolution.
func (s *WorktreeService) MergeAndCleanup(projectRoot, defaultBranch, branchName, worktreePath string) error {
	if err := validateRepoPath(projectRoot); err != nil {
		return fmt.Errorf("worktree merge: %w", err)
	}

	// Remove worktree BEFORE checkout/merge — commits live in .git/refs/heads,
	// not in the worktree dir, so this is safe and eliminates worktree-originated lock contention.
	runGit(projectRoot, "worktree", "remove", worktreePath)

	// Checkout + merge with retry for transient index.lock
	if err := s.checkoutAndMergeWithRetry(projectRoot, defaultBranch, branchName); err != nil {
		return err
	}

	// Merge succeeded — delete the branch
	runGit(projectRoot, "branch", "-d", branchName)
	return nil
}

// checkoutAndMergeWithRetry attempts checkout+merge up to mergeRetryAttempts times.
// Before each retry, it removes stale index.lock files (lock held by dead process).
func (s *WorktreeService) checkoutAndMergeWithRetry(projectRoot, defaultBranch, branchName string) error {
	var lastErr error
	lockPath := filepath.Join(projectRoot, ".git", "index.lock")

	for attempt := 0; attempt < mergeRetryAttempts; attempt++ {
		if attempt > 0 {
			removeStaleLock(lockPath)
			time.Sleep(mergeRetryDelay)
		}

		_, err := runGit(projectRoot, "checkout", defaultBranch)
		if err != nil {
			lastErr = fmt.Errorf("worktree merge: checkout %s failed (attempt %d/%d): %w",
				defaultBranch, attempt+1, mergeRetryAttempts, err)
			continue
		}

		_, err = runGit(projectRoot, "merge", branchName, "--no-edit")
		if err != nil {
			runGit(projectRoot, "merge", "--abort")
			return fmt.Errorf("worktree merge: merge failed for branch '%s' — resolve manually: %w", branchName, err)
		}

		return nil
	}

	return lastErr
}

// removeStaleLock removes .git/index.lock if the owning process is dead.
func removeStaleLock(lockPath string) {
	info, err := os.Stat(lockPath)
	if err != nil {
		return // no lock file
	}

	// If the lock is older than 2 seconds, the owning process is likely dead.
	// Git writes the PID into the lock on some platforms, but macOS doesn't
	// reliably do this, so we use age as the primary heuristic.
	if time.Since(info.ModTime()) > 2*time.Second {
		os.Remove(lockPath)
		return
	}

	// Try to read PID from lock content (some git versions write it)
	content, err := os.ReadFile(lockPath)
	if err != nil {
		return
	}
	pidStr := strings.TrimSpace(string(content))
	pid, err := strconv.Atoi(pidStr)
	if err != nil {
		return // not a PID, can't determine liveness
	}

	// Signal 0 tests process liveness without actually sending a signal
	if err := syscall.Kill(pid, 0); err != nil {
		// Process is dead — safe to remove
		os.Remove(lockPath)
	}
}

// Cleanup force-removes the worktree and branch without merging.
// Errors are logged but not propagated — this is best-effort cleanup.
func (s *WorktreeService) Cleanup(projectRoot, branchName, worktreePath string) error {
	// Force-remove worktree (ignore errors — may already be gone)
	runGit(projectRoot, "worktree", "remove", "--force", worktreePath)

	// Prune worktree metadata (handles case where directory was already deleted)
	runGit(projectRoot, "worktree", "prune")

	// Force-delete branch (ignore errors — may already be gone)
	runGit(projectRoot, "branch", "-D", branchName)
	return nil
}
