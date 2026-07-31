package service

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"
)

// TestSetupFromHEAD_BranchesOffCurrentHEAD_NotDefaultBranch verifies
// SetupFromHEAD cuts the delegate branch from the checkout's current HEAD —
// including work-in-progress commits on a non-default branch — rather than
// from "main"/default_branch (unlike Setup). No remote is configured on
// setupWorktreeTestRepo, so a clean SetupFromHEAD here also demonstrates no
// origin fetch is attempted.
func TestSetupFromHEAD_BranchesOffCurrentHEAD_NotDefaultBranch(t *testing.T) {
	t.Parallel()
	repoPath := setupWorktreeTestRepo(t)
	defer os.RemoveAll(repoPath)

	// Move HEAD onto a WIP branch, ahead of "main".
	runOrFatal(t, repoPath, "checkout", "-b", "wip-branch")
	createCommit(t, repoPath, "wip.txt", "work in progress", "WIP commit")
	wantHead := strings.TrimSpace(runOutOrFatal(t, repoPath, "rev-parse", "HEAD"))

	svc := &WorktreeService{}
	worktreePath, baseCommit, err := svc.SetupFromHEAD(repoPath, "nrdelegate/wip-test")
	if err != nil {
		t.Fatalf("SetupFromHEAD: %v", err)
	}
	defer svc.Cleanup(repoPath, "nrdelegate/wip-test", worktreePath)

	if baseCommit != wantHead {
		t.Errorf("baseCommit = %q, want current HEAD %q (not default_branch)", baseCommit, wantHead)
	}
	if !worktreeExists(worktreePath) {
		t.Fatalf("worktree directory does not exist at %s", worktreePath)
	}
	if !branchExists(t, repoPath, "nrdelegate/wip-test") {
		t.Error("branch was not created")
	}
	// The WIP file (committed after main diverged) must be visible in the
	// worktree, proving it branched from wip-branch's HEAD, not main.
	if _, err := os.Stat(filepath.Join(worktreePath, "wip.txt")); err != nil {
		t.Errorf("wip.txt not found in worktree (branched from wrong base): %v", err)
	}
}

// TestSetupFromHEAD_NonGitDir_ReturnsError verifies the resolveRepoPath guard
// fires before any worktree/branch is attempted.
func TestSetupFromHEAD_NonGitDir_ReturnsError(t *testing.T) {
	t.Parallel()
	tmpDir := t.TempDir()

	svc := &WorktreeService{}
	_, _, err := svc.SetupFromHEAD(tmpDir, "nrdelegate/nogit")
	if err == nil {
		t.Fatal("expected error for non-git directory")
	}
	if !strings.Contains(err.Error(), "not a git repository") {
		t.Errorf("error = %v, want to mention 'not a git repository'", err)
	}
}

// TestCommitAndCollect_CommitsChangesAndReportsDiffstat verifies a dirty
// worktree gets committed under the delegation id, the branch survives, and
// the reported changed files/diffstat reflect what was actually written.
func TestCommitAndCollect_CommitsChangesAndReportsDiffstat(t *testing.T) {
	t.Parallel()
	repoPath := setupWorktreeTestRepo(t)
	defer os.RemoveAll(repoPath)

	svc := &WorktreeService{}
	worktreePath, baseCommit, err := svc.SetupFromHEAD(repoPath, "nrdelegate/commit-test")
	if err != nil {
		t.Fatalf("SetupFromHEAD: %v", err)
	}

	if err := os.WriteFile(filepath.Join(worktreePath, "new.txt"), []byte("new file content"), 0o644); err != nil {
		t.Fatalf("write new.txt: %v", err)
	}
	if err := os.WriteFile(filepath.Join(worktreePath, "file1.txt"), []byte("modified content"), 0o644); err != nil {
		t.Fatalf("modify file1.txt: %v", err)
	}

	summary, err := svc.CommitAndCollect(repoPath, worktreePath, "nrdelegate/commit-test", baseCommit, "deleg-abc123", "Fix the thing\n\nlonger body")
	if err != nil {
		t.Fatalf("CommitAndCollect: %v", err)
	}

	if !summary.Committed {
		t.Fatal("Committed = false, want true (dirty worktree)")
	}
	wantFiles := map[string]bool{"new.txt": false, "file1.txt": false}
	for _, f := range summary.ChangedFiles {
		if _, ok := wantFiles[f]; ok {
			wantFiles[f] = true
		}
	}
	for f, seen := range wantFiles {
		if !seen {
			t.Errorf("ChangedFiles = %v, missing %q", summary.ChangedFiles, f)
		}
	}
	if summary.Diffstat == "" {
		t.Error("Diffstat is empty, want a non-empty git diff --stat summary")
	}

	// Worktree removed, branch kept (something was committed).
	if worktreeExists(worktreePath) {
		t.Error("worktree still exists after CommitAndCollect")
	}
	if !branchExists(t, repoPath, "nrdelegate/commit-test") {
		t.Error("branch should be kept when a commit landed")
	}

	logOut := runOutOrFatal(t, repoPath, "log", "nrdelegate/commit-test", "-1", "--format=%s")
	if !strings.Contains(logOut, "delegate(deleg-abc123): Fix the thing") {
		t.Errorf("commit subject = %q, want it to carry the delegation id and brief head", logOut)
	}

	svc.Cleanup(repoPath, "nrdelegate/commit-test", worktreePath)
}

// TestCommitAndCollect_CleanTree_DeletesBranchAndReportsNoChanges verifies a
// worktree with nothing to commit reports Committed=false and the
// now-pointless branch is deleted rather than left dangling.
func TestCommitAndCollect_CleanTree_DeletesBranchAndReportsNoChanges(t *testing.T) {
	t.Parallel()
	repoPath := setupWorktreeTestRepo(t)
	defer os.RemoveAll(repoPath)

	svc := &WorktreeService{}
	worktreePath, baseCommit, err := svc.SetupFromHEAD(repoPath, "nrdelegate/clean-test")
	if err != nil {
		t.Fatalf("SetupFromHEAD: %v", err)
	}

	summary, err := svc.CommitAndCollect(repoPath, worktreePath, "nrdelegate/clean-test", baseCommit, "deleg-clean", "no-op run")
	if err != nil {
		t.Fatalf("CommitAndCollect: %v", err)
	}

	if summary.Committed {
		t.Error("Committed = true, want false (clean worktree)")
	}
	if len(summary.ChangedFiles) != 0 {
		t.Errorf("ChangedFiles = %v, want empty", summary.ChangedFiles)
	}
	if summary.Diffstat != "" {
		t.Errorf("Diffstat = %q, want empty", summary.Diffstat)
	}
	if worktreeExists(worktreePath) {
		t.Error("worktree still exists after CommitAndCollect")
	}
	if branchExists(t, repoPath, "nrdelegate/clean-test") {
		t.Error("branch should be deleted when nothing was committed")
	}
}

// TestConcurrentDelegateWorktrees_DistinctBranchesLiveTreeUntouched runs two
// concurrent SetupFromHEAD+CommitAndCollect delegate flows against the same
// project checkout and verifies neither one dirties the live tree and both
// land on their own distinct branch.
func TestConcurrentDelegateWorktrees_DistinctBranchesLiveTreeUntouched(t *testing.T) {
	t.Parallel()
	repoPath := setupWorktreeTestRepo(t)
	defer os.RemoveAll(repoPath)

	svc := &WorktreeService{}
	branches := []string{"nrdelegate/concurrent-a", "nrdelegate/concurrent-b"}

	var wg sync.WaitGroup
	errs := make([]error, len(branches))
	for i, branch := range branches {
		wg.Add(1)
		go func(i int, branch string) {
			defer wg.Done()
			worktreePath, baseCommit, err := svc.SetupFromHEAD(repoPath, branch)
			if err != nil {
				errs[i] = err
				return
			}
			fname := branch[len("nrdelegate/"):] + ".txt"
			if err := os.WriteFile(filepath.Join(worktreePath, fname), []byte("payload from "+branch), 0o644); err != nil {
				errs[i] = err
				return
			}
			_, err = svc.CommitAndCollect(repoPath, worktreePath, branch, baseCommit, "deleg-"+branch, "concurrent worker")
			errs[i] = err
		}(i, branch)
	}
	wg.Wait()

	for i, err := range errs {
		if err != nil {
			t.Fatalf("branch %q flow failed: %v", branches[i], err)
		}
	}

	for _, branch := range branches {
		if !branchExists(t, repoPath, branch) {
			t.Errorf("branch %q was not created", branch)
		}
	}

	statusOut := runOutOrFatal(t, repoPath, "status", "--porcelain")
	if strings.TrimSpace(statusOut) != "" {
		t.Errorf("live checkout is dirty after concurrent delegate worktrees: %q", statusOut)
	}

	for _, branch := range branches {
		svc.Cleanup(repoPath, branch, filepath.Join(worktreeBasePath, branch))
	}
}

// TestLiveHead_ReturnsCurrentHEAD_AndTracksNewCommits verifies LiveHead
// resolves the checkout's current HEAD sha and reflects a subsequent commit
// — the read finalizeDelegateWorktree's live-tree-escape check relies on.
func TestLiveHead_ReturnsCurrentHEAD_AndTracksNewCommits(t *testing.T) {
	t.Parallel()
	repoPath := setupWorktreeTestRepo(t)
	defer os.RemoveAll(repoPath)

	svc := &WorktreeService{}
	wantHead := strings.TrimSpace(runOutOrFatal(t, repoPath, "rev-parse", "HEAD"))

	got, err := svc.LiveHead(repoPath)
	if err != nil {
		t.Fatalf("LiveHead: %v", err)
	}
	if got != wantHead {
		t.Errorf("LiveHead() = %q, want %q", got, wantHead)
	}

	createCommit(t, repoPath, "livehead.txt", "moved on", "moved HEAD")
	wantHead2 := strings.TrimSpace(runOutOrFatal(t, repoPath, "rev-parse", "HEAD"))
	if wantHead2 == wantHead {
		t.Fatal("test setup: HEAD did not move after createCommit")
	}

	got2, err := svc.LiveHead(repoPath)
	if err != nil {
		t.Fatalf("LiveHead after new commit: %v", err)
	}
	if got2 != wantHead2 {
		t.Errorf("LiveHead() after new commit = %q, want %q", got2, wantHead2)
	}
}

// TestLiveHead_NonGitDir_ReturnsError verifies the resolveRepoPath guard
// fires before any git plumbing runs.
func TestLiveHead_NonGitDir_ReturnsError(t *testing.T) {
	t.Parallel()
	tmpDir := t.TempDir()

	svc := &WorktreeService{}
	if _, err := svc.LiveHead(tmpDir); err == nil {
		t.Fatal("expected error for non-git directory")
	}
}

func runOrFatal(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git %v: %v (%s)", args, err, out)
	}
}

func runOutOrFatal(t *testing.T, dir string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v: %v (%s)", args, err, out)
	}
	return string(out)
}
