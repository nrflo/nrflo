package service

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// DelegateWorktreeSummary is the post-commit result of CommitAndCollect,
// persisted as JSON onto delegations.worktree_summary.
type DelegateWorktreeSummary struct {
	Committed       bool     `json:"committed"`
	ChangedFiles    []string `json:"changed_files,omitempty"`
	Diffstat        string   `json:"diffstat,omitempty"`
	LiveTreeMutated bool     `json:"live_tree_mutated,omitempty"`
	HeadBefore      string   `json:"head_before,omitempty"`
	HeadAfter       string   `json:"head_after,omitempty"`
}

// SetupFromHEAD branches branchName off the live checkout's current HEAD (no
// origin fetch — the console user's in-progress branch is the intended base,
// not the default branch) and creates a worktree for it under
// worktreeBasePath. Returns the absolute worktree path and the base commit
// sha the branch was cut from.
func (s *WorktreeService) SetupFromHEAD(projectRoot, branchName string) (string, string, error) {
	repoPath, err := resolveRepoPath(projectRoot)
	if err != nil {
		return "", "", fmt.Errorf("worktree setup from HEAD: %w", err)
	}
	if err := validateBranch(branchName); err != nil {
		return "", "", fmt.Errorf("worktree setup from HEAD: invalid branch name: %w", err)
	}

	headOut, err := runGit(repoPath, "rev-parse", "HEAD")
	if err != nil {
		return "", "", fmt.Errorf("worktree setup from HEAD: resolve HEAD: %w", err)
	}
	baseCommit := strings.TrimSpace(headOut)

	worktreePath := filepath.Join(worktreeBasePath, branchName)
	if err := os.MkdirAll(filepath.Dir(worktreePath), 0o755); err != nil {
		return "", "", fmt.Errorf("worktree setup from HEAD: failed to create parent dir: %w", err)
	}

	if _, err := runGit(repoPath, "worktree", "add", "-b", branchName, worktreePath, baseCommit); err != nil {
		_ = s.Cleanup(repoPath, branchName, worktreePath)
		return "", "", fmt.Errorf("worktree setup from HEAD: worktree creation failed: %w", err)
	}

	absPath, err := filepath.Abs(worktreePath)
	if err != nil {
		return "", "", fmt.Errorf("worktree setup from HEAD: failed to resolve path: %w", err)
	}

	seedAgentContext(repoPath, absPath)

	return absPath, baseCommit, nil
}

// LiveHead resolves the project live checkout's current HEAD sha, used by
// finalizeDelegateWorktree to detect whether the live tree moved underneath
// a no-commit delegation.
func (s *WorktreeService) LiveHead(projectRoot string) (string, error) {
	repoPath, err := resolveRepoPath(projectRoot)
	if err != nil {
		return "", fmt.Errorf("live head: %w", err)
	}
	headOut, err := runGit(repoPath, "rev-parse", "HEAD")
	if err != nil {
		return "", fmt.Errorf("live head: %w", err)
	}
	return strings.TrimSpace(headOut), nil
}

// CommitAndCollect commits the worktree's working tree (git add -A, skipped
// when the tree is clean relative to its index) tagged with the delegation
// id, diffs the result against baseCommit, removes the worktree, and — when
// nothing was committed — deletes the now-pointless branch. The branch is
// kept (and reported back to the caller) whenever a commit was made, even if
// individual workers failed or timed out: partial work stays recoverable.
func (s *WorktreeService) CommitAndCollect(projectRoot, worktreePath, branchName, baseCommit, delegationID, briefHead string) (*DelegateWorktreeSummary, error) {
	repoPath, err := resolveRepoPath(projectRoot)
	if err != nil {
		return nil, fmt.Errorf("commit and collect: %w", err)
	}

	if _, err := runGit(worktreePath, "add", "-A"); err != nil {
		return nil, fmt.Errorf("commit and collect: git add: %w", err)
	}

	summary := &DelegateWorktreeSummary{}
	_, cleanErr := runGit(worktreePath, "diff", "--cached", "--quiet")
	summary.Committed = cleanErr != nil // non-nil == staged changes exist

	if summary.Committed {
		msg := fmt.Sprintf("delegate(%s): %s", delegationID, briefHead)
		if _, err := runGit(worktreePath, "commit", "-m", msg); err != nil {
			return nil, fmt.Errorf("commit and collect: git commit: %w", err)
		}

		if out, err := runGit(worktreePath, "diff", "--name-only", baseCommit, "HEAD"); err == nil {
			for _, f := range strings.Split(strings.TrimSpace(out), "\n") {
				if f != "" {
					summary.ChangedFiles = append(summary.ChangedFiles, f)
				}
			}
		}
		if out, err := runGit(worktreePath, "diff", "--stat", baseCommit, "HEAD"); err == nil {
			summary.Diffstat = strings.TrimSpace(out)
		}
	}

	if _, err := runGit(repoPath, "worktree", "remove", worktreePath); err != nil {
		_, _ = runGit(repoPath, "worktree", "remove", "--force", worktreePath)
	}

	if !summary.Committed {
		_, _ = runGit(repoPath, "branch", "-D", branchName)
	}

	return summary, nil
}
