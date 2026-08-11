package service

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestCommitAndCollect_WorkerSelfCommit_KeepsBranch guards against orphaning a
// worker's own commits: a worker that ran `git commit` itself leaves a clean
// tree, but the branch advanced past baseCommit — CommitAndCollect must report
// Committed=true, keep the branch, and collect the diff, never branch -D it
// (which would leave the commits dangling).
func TestCommitAndCollect_WorkerSelfCommit_KeepsBranch(t *testing.T) {
	t.Parallel()
	repoPath := setupWorktreeTestRepo(t)
	defer os.RemoveAll(repoPath)

	svc := &WorktreeService{}
	worktreePath, baseCommit, err := svc.SetupFromHEAD(repoPath, "nrdelegate/self-commit")
	if err != nil {
		t.Fatalf("SetupFromHEAD: %v", err)
	}

	// The worker does its work and commits it itself, leaving a clean tree.
	if err := os.WriteFile(filepath.Join(worktreePath, "worker.txt"), []byte("worker payload"), 0o644); err != nil {
		t.Fatalf("write worker.txt: %v", err)
	}
	runOrFatal(t, worktreePath, "add", "-A")
	runOrFatal(t, worktreePath, "commit", "-m", "worker self-commit")
	workerHead := strings.TrimSpace(runOutOrFatal(t, worktreePath, "rev-parse", "HEAD"))

	summary, err := svc.CommitAndCollect(repoPath, worktreePath, "nrdelegate/self-commit", baseCommit, "deleg-self", "self-committing worker")
	if err != nil {
		t.Fatalf("CommitAndCollect: %v", err)
	}

	if !summary.Committed {
		t.Fatal("Committed = false, want true (branch advanced past baseCommit by worker commit)")
	}
	if len(summary.ChangedFiles) != 1 || summary.ChangedFiles[0] != "worker.txt" {
		t.Errorf("ChangedFiles = %v, want [worker.txt]", summary.ChangedFiles)
	}
	if !branchExists(t, repoPath, "nrdelegate/self-commit") {
		t.Fatal("branch was deleted despite carrying the worker's commit")
	}
	branchHead := strings.TrimSpace(runOutOrFatal(t, repoPath, "rev-parse", "nrdelegate/self-commit"))
	if branchHead != workerHead {
		t.Errorf("branch head = %s, want the worker's commit %s", branchHead, workerHead)
	}
	if worktreeExists(worktreePath) {
		t.Error("worktree still exists after CommitAndCollect")
	}

	svc.Cleanup(repoPath, "nrdelegate/self-commit", worktreePath)
}
