package service

import (
	"fmt"
	"path/filepath"
	"strings"
)

// MergeDelegateBranch merges an isolated delegation's server-committed branch
// into the live checkout's current branch (the branch was cut from live HEAD,
// so the current branch — not the default branch — is the merge target).
// Server-owned so callers never run git against the live tree themselves:
// refuses a dirty tree, treats an already-merged branch as success
// (mirroring the bc0cfc74 ancestry check), aborts cleanly on conflict keeping
// the branch for manual resolution, and deletes the branch after a
// successful merge. Returns the resulting HEAD commit.
func (s *WorktreeService) MergeDelegateBranch(projectRoot, branchName string) (mergeCommit string, alreadyMerged bool, err error) {
	repoPath, err := resolveRepoPath(projectRoot)
	if err != nil {
		return "", false, fmt.Errorf("delegate merge: %w", err)
	}
	if err := validateBranch(branchName); err != nil {
		return "", false, fmt.Errorf("delegate merge: invalid branch name: %w", err)
	}
	if _, err := runGit(repoPath, "rev-parse", "--verify", "refs/heads/"+branchName); err != nil {
		return "", false, fmt.Errorf("delegate merge: branch %q not found: %w", branchName, err)
	}

	headOut, err := runGit(repoPath, "rev-parse", "--abbrev-ref", "HEAD")
	if err != nil {
		return "", false, fmt.Errorf("delegate merge: resolve current branch: %w", err)
	}
	currentBranch := strings.TrimSpace(headOut)

	merged, err := s.BranchMerged(repoPath, currentBranch, branchName)
	if err != nil {
		return "", false, fmt.Errorf("delegate merge: %w", err)
	}
	if merged {
		_, _ = runGit(repoPath, "branch", "-d", branchName)
		sha, err := runGit(repoPath, "rev-parse", "HEAD")
		if err != nil {
			return "", true, fmt.Errorf("delegate merge: resolve HEAD: %w", err)
		}
		return strings.TrimSpace(sha), true, nil
	}

	statusOut, err := runGit(repoPath, "status", "--porcelain")
	if err != nil {
		return "", false, fmt.Errorf("delegate merge: status check: %w", err)
	}
	if strings.TrimSpace(statusOut) != "" {
		return "", false, fmt.Errorf("delegate merge: live tree has uncommitted changes — commit or stash them first, or merge %q manually", branchName)
	}

	removeStaleLock(filepath.Join(repoPath, ".git", "index.lock"))
	if _, err := runGit(repoPath, "merge", branchName, "--no-edit"); err != nil {
		conflicts, _ := runGit(repoPath, "diff", "--name-only", "--diff-filter=U")
		_, _ = runGit(repoPath, "merge", "--abort")
		detail := strings.TrimSpace(conflicts)
		if detail != "" {
			return "", false, fmt.Errorf("delegate merge: conflict on %s — branch %q preserved for manual resolution", strings.Join(strings.Fields(detail), ", "), branchName)
		}
		return "", false, fmt.Errorf("delegate merge: merge failed, branch %q preserved: %w", branchName, err)
	}

	sha, err := runGit(repoPath, "rev-parse", "HEAD")
	if err != nil {
		return "", false, fmt.Errorf("delegate merge: resolve merge commit: %w", err)
	}
	_, _ = runGit(repoPath, "branch", "-d", branchName)
	return strings.TrimSpace(sha), false, nil
}
