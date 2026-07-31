package orchestrator

import (
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"

	"be/internal/spawner"
	"be/internal/ws"
)

// runGitCmd runs a git command in repoPath, failing the test on error.
func runGitCmd(t *testing.T, repoPath string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = repoPath
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git %v failed: %v\n%s", args, err, out)
	}
}

// branchExistsInRepo checks if a branch exists in a git repo.
func branchExistsInRepo(t *testing.T, repoPath, branchName string) bool {
	t.Helper()
	cmd := exec.Command("git", "branch", "--list", branchName)
	cmd.Dir = repoPath
	out, err := cmd.Output()
	if err != nil {
		return false
	}
	return strings.Contains(string(out), branchName)
}

// waitForMergeEvent drains ch until it sees one of the two terminal merge
// events (or times out), returning which one arrived first.
func waitForMergeEvent(t *testing.T, ch chan []byte, timeout time.Duration) ws.Event {
	t.Helper()
	deadline := time.After(timeout)
	for {
		select {
		case msg := <-ch:
			var ev ws.Event
			if jsonErr := json.Unmarshal(msg, &ev); jsonErr != nil {
				continue
			}
			if ev.Type == ws.EventMergeConflictResolved || ev.Type == ws.EventMergeConflictFailed {
				return ev
			}
		case <-deadline:
			t.Fatal("timeout waiting for a terminal merge event")
		}
	}
}

// TestAttemptConflictResolution_AlreadyMerged_SpawnFailure verifies the ancestry
// escape hatch: when the resolver's spawn fails (here, forced by a
// zero-value spawner.Config with no DB pool wired, driven by an
// already-cancelled context) but the branch is already an ancestor of the
// default branch, attemptConflictResolution treats it as success — returns
// nil and broadcasts merge.conflict_resolved, not merge.conflict_failed.
func TestAttemptConflictResolution_AlreadyMerged_SpawnFailure(t *testing.T) {
	repoPath := setupGitRepo(t)
	defer os.RemoveAll(repoPath)

	branchName := "feature-already-merged"
	runGitCmd(t, repoPath, "checkout", "-b", branchName)
	if err := os.WriteFile(repoPath+"/merged.txt", []byte("done"), 0o644); err != nil {
		t.Fatalf("write file: %v", err)
	}
	runGitCmd(t, repoPath, "add", "merged.txt")
	runGitCmd(t, repoPath, "commit", "-m", "feature work")
	runGitCmd(t, repoPath, "checkout", "main")
	runGitCmd(t, repoPath, "merge", branchName, "--no-edit")

	env := newTestEnv(t)
	seedConflictResolver(t, env)

	ticketID := "ticket-already-merged"
	env.createTicket(t, ticketID, "Already merged ticket")
	wfiID := env.initWorkflow(t, ticketID)

	ch := env.subscribeWSClient(t, "ws-already-merged", ticketID)

	wt := &worktreeInfo{
		projectRoot:   repoPath,
		worktreePath:  repoPath,
		branchName:    branchName,
		defaultBranch: "main",
	}
	req := RunRequest{
		ProjectID:    env.project,
		TicketID:     ticketID,
		WorkflowName: "test",
	}

	// Force spawn failure fast: no time.Sleep, no multi-second timeout.
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	err := env.orch.attemptConflictResolution(ctx, wfiID, req, wt, env.pool, "auto-merge failed", spawner.Config{})

	if err != nil {
		t.Errorf("attemptConflictResolution returned %v, want nil (branch already merged should mask spawn failure)", err)
	}

	event := waitForMergeEvent(t, ch, 3*time.Second)
	if event.Type != ws.EventMergeConflictResolved {
		t.Errorf("broadcast event = %q, want %q (already-merged branch must not report conflict_failed)", event.Type, ws.EventMergeConflictResolved)
	}
	if event.Data["branch"] != branchName {
		t.Errorf("resolved event branch = %v, want %v", event.Data["branch"], branchName)
	}

	// Branch should have been deleted as part of the success path.
	if branchExistsInRepo(t, repoPath, branchName) {
		t.Error("already-merged branch should have been deleted by deleteResolvedBranch")
	}
}

// TestAttemptConflictResolution_UnmergedBranch_SpawnFailure verifies the
// unchanged failure path: when the branch genuinely has commits not yet in
// the default branch, a resolver spawn failure still reports failure.
func TestAttemptConflictResolution_UnmergedBranch_SpawnFailure(t *testing.T) {
	repoPath := setupGitRepo(t)
	defer os.RemoveAll(repoPath)

	branchName := "feature-unmerged"
	runGitCmd(t, repoPath, "checkout", "-b", branchName)
	if err := os.WriteFile(repoPath+"/unmerged.txt", []byte("wip"), 0o644); err != nil {
		t.Fatalf("write file: %v", err)
	}
	runGitCmd(t, repoPath, "add", "unmerged.txt")
	runGitCmd(t, repoPath, "commit", "-m", "unmerged feature work")
	runGitCmd(t, repoPath, "checkout", "main")

	env := newTestEnv(t)
	seedConflictResolver(t, env)

	ticketID := "ticket-unmerged"
	env.createTicket(t, ticketID, "Unmerged ticket")
	wfiID := env.initWorkflow(t, ticketID)

	ch := env.subscribeWSClient(t, "ws-unmerged", ticketID)

	wt := &worktreeInfo{
		projectRoot:   repoPath,
		worktreePath:  repoPath,
		branchName:    branchName,
		defaultBranch: "main",
	}
	req := RunRequest{
		ProjectID:    env.project,
		TicketID:     ticketID,
		WorkflowName: "test",
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	err := env.orch.attemptConflictResolution(ctx, wfiID, req, wt, env.pool, "auto-merge failed", spawner.Config{})

	if err == nil {
		t.Fatal("expected error for unmerged branch with failed resolver spawn")
	}
	if !strings.Contains(err.Error(), "conflict resolution failed") {
		t.Errorf("error = %v, want it to contain 'conflict resolution failed'", err)
	}

	event := waitForMergeEvent(t, ch, 3*time.Second)
	if event.Type != ws.EventMergeConflictFailed {
		t.Errorf("broadcast event = %q, want %q", event.Type, ws.EventMergeConflictFailed)
	}

	// Branch must survive for manual resolution.
	if !branchExistsInRepo(t, repoPath, branchName) {
		t.Error("unmerged branch should NOT have been deleted on the failure path")
	}
}
