package service

import (
	"fmt"
	"os"
	"path/filepath"
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

// MergeAndCleanup merges the worktree branch into defaultBranch, removes the
// worktree, and deletes the branch. If merge fails, returns an error and
// preserves the branch/worktree for manual resolution.
func (s *WorktreeService) MergeAndCleanup(projectRoot, defaultBranch, branchName, worktreePath string) error {
	if err := validateRepoPath(projectRoot); err != nil {
		return fmt.Errorf("worktree merge: %w", err)
	}

	// Ensure we're on the default branch
	_, err := runGit(projectRoot, "checkout", defaultBranch)
	if err != nil {
		return fmt.Errorf("worktree merge: checkout %s failed: %w", defaultBranch, err)
	}

	// Merge the worktree branch
	_, err = runGit(projectRoot, "merge", branchName, "--no-edit")
	if err != nil {
		// Abort the merge to leave repo in a clean state
		runGit(projectRoot, "merge", "--abort")
		return fmt.Errorf("worktree merge: merge failed for branch '%s' — resolve manually: %w", branchName, err)
	}

	// Merge succeeded — clean up worktree and branch
	runGit(projectRoot, "worktree", "remove", worktreePath)
	runGit(projectRoot, "branch", "-d", branchName)
	return nil
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
