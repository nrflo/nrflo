package orchestrator

import (
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"be/internal/ws"
)

// setupGitRepoForPush creates a working git repo on "main" with one commit and a
// bare remote at "origin". Both dirs are cleaned up via t.Cleanup.
// Returns (workingPath, remotePath).
func setupGitRepoForPush(t *testing.T) (workingPath, remotePath string) {
	t.Helper()
	baseDir := filepath.Join("/tmp", "orch_push_test_"+t.Name())
	os.RemoveAll(baseDir)
	if err := os.MkdirAll(baseDir, 0o755); err != nil {
		t.Fatalf("create push test base dir: %v", err)
	}
	t.Cleanup(func() { os.RemoveAll(baseDir) })

	remotePath = filepath.Join(baseDir, "remote.git")
	workingPath = filepath.Join(baseDir, "working")

	// Init bare remote.
	if out, err := exec.Command("git", "init", "--bare", remotePath).CombinedOutput(); err != nil {
		t.Fatalf("git init --bare: %v: %s", err, out)
	}

	// Init working repo.
	if err := os.MkdirAll(workingPath, 0o755); err != nil {
		t.Fatalf("mkdir working: %v", err)
	}

	run := func(args ...string) {
		cmd := exec.Command("git", args...)
		cmd.Dir = workingPath
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v: %s", args, err, out)
		}
	}

	run("init")
	run("config", "user.name", "Test User")
	run("config", "user.email", "test@example.com")
	// checkout -b main is best-effort: some git configs already default to main.
	exec.Command("git", "-C", workingPath, "checkout", "-b", "main").Run() //nolint:errcheck

	if err := os.WriteFile(filepath.Join(workingPath, "test.txt"), []byte("hello"), 0o644); err != nil {
		t.Fatalf("write test.txt: %v", err)
	}
	run("add", "test.txt")
	run("commit", "-m", "Initial commit")
	run("remote", "add", "origin", remotePath)

	return workingPath, remotePath
}

// TestPushIfEnabled_DisabledWhenFalse verifies that pushAfterMerge=false causes the
// function to return immediately without executing git or broadcasting any WS event.
func TestPushIfEnabled_DisabledWhenFalse(t *testing.T) {
	env := newTestEnv(t)

	ticketID := "push-disabled-ticket"
	env.createTicket(t, ticketID, "Push disabled ticket")
	ch := env.subscribeWSClient(t, "ws-push-disabled", ticketID)

	// Use a non-existent path — if git were called, it would fail and broadcast
	// EventWorkflowPushFailed. So a clean return proves git was not invoked.
	wt := &worktreeInfo{
		projectRoot:   "/tmp/nonexistent-repo-push-disabled",
		defaultBranch: "main",
	}
	req := RunRequest{
		ProjectID:    env.project,
		TicketID:     ticketID,
		WorkflowName: "test",
	}

	env.orch.pushIfEnabled(context.Background(), false, wt, "wfi-push-disabled", req)

	// No push events should have been broadcast (non-blocking check).
	select {
	case msg := <-ch:
		var ev ws.Event
		if err := json.Unmarshal(msg, &ev); err == nil {
			if ev.Type == ws.EventWorkflowPushed || ev.Type == ws.EventWorkflowPushFailed {
				t.Errorf("unexpected push event %q when pushAfterMerge=false", ev.Type)
			}
		}
	default:
		// good — no events queued
	}
}

// TestPushIfEnabled_SuccessEventBroadcast verifies that a successful push broadcasts
// workflow.pushed with the correct branch name and instance_id.
func TestPushIfEnabled_SuccessEventBroadcast(t *testing.T) {
	env := newTestEnv(t)
	workingPath, _ := setupGitRepoForPush(t)

	ticketID := "push-success-ticket"
	env.createTicket(t, ticketID, "Push success ticket")
	ch := env.subscribeWSClient(t, "ws-push-success", ticketID)

	const defaultBranch = "main"
	const wfiID = "wfi-push-success"

	wt := &worktreeInfo{
		projectRoot:   workingPath,
		defaultBranch: defaultBranch,
	}
	req := RunRequest{
		ProjectID:    env.project,
		TicketID:     ticketID,
		WorkflowName: "test",
	}

	env.orch.pushIfEnabled(context.Background(), true, wt, wfiID, req)

	event := expectEvent(t, ch, ws.EventWorkflowPushed, 3*time.Second)

	if event.Data["branch"] != defaultBranch {
		t.Errorf("workflow.pushed branch = %v, want %q", event.Data["branch"], defaultBranch)
	}
	if event.Data["instance_id"] != wfiID {
		t.Errorf("workflow.pushed instance_id = %v, want %q", event.Data["instance_id"], wfiID)
	}
}

// TestPushIfEnabled_FailureEventBroadcast verifies that a failed push broadcasts
// workflow.push_failed with branch, instance_id, and a non-empty error field,
// and the function returns without panicking (push is best-effort, not fatal).
func TestPushIfEnabled_FailureEventBroadcast(t *testing.T) {
	env := newTestEnv(t)

	ticketID := "push-fail-ticket"
	env.createTicket(t, ticketID, "Push fail ticket")
	ch := env.subscribeWSClient(t, "ws-push-fail", ticketID)

	const defaultBranch = "main"
	const wfiID = "wfi-push-fail"

	// Set up a git repo with origin pointing to a nonexistent path so push fails.
	badDir := t.TempDir()
	setupRun := func(args ...string) {
		cmd := exec.Command("git", args...)
		cmd.Dir = badDir
		cmd.Run() //nolint:errcheck — best-effort for test setup
	}
	setupRun("init")
	setupRun("config", "user.name", "Test User")
	setupRun("config", "user.email", "test@example.com")
	exec.Command("git", "-C", badDir, "checkout", "-b", "main").Run() //nolint:errcheck
	_ = os.WriteFile(filepath.Join(badDir, "x.txt"), []byte("x"), 0o644)
	setupRun("add", "x.txt")
	setupRun("commit", "-m", "init")
	setupRun("remote", "add", "origin", "/tmp/nonexistent-remote-"+t.Name())

	wt := &worktreeInfo{
		projectRoot:   badDir,
		defaultBranch: defaultBranch,
	}
	req := RunRequest{
		ProjectID:    env.project,
		TicketID:     ticketID,
		WorkflowName: "test",
	}

	env.orch.pushIfEnabled(context.Background(), true, wt, wfiID, req)

	event := expectEvent(t, ch, ws.EventWorkflowPushFailed, 3*time.Second)

	if event.Data["branch"] != defaultBranch {
		t.Errorf("workflow.push_failed branch = %v, want %q", event.Data["branch"], defaultBranch)
	}
	if event.Data["instance_id"] != wfiID {
		t.Errorf("workflow.push_failed instance_id = %v, want %q", event.Data["instance_id"], wfiID)
	}
	if event.Data["error"] == "" {
		t.Error("workflow.push_failed event.error should be non-empty")
	}
}

// TestPushAfterMerge_ConfigReadFromProjectConfig verifies that the push_after_merge
// project config key is correctly interpreted: "true" → true, "" → false.
func TestPushAfterMerge_ConfigReadFromProjectConfig(t *testing.T) {
	env := newTestEnv(t)

	cases := []struct {
		stored string
		want   bool
	}{
		{"true", true},
		{"", false},
		{"false", false},
	}

	for _, tc := range cases {
		t.Run(tc.stored, func(t *testing.T) {
			if err := env.pool.SetProjectConfig(env.project, "push_after_merge", tc.stored); err != nil {
				t.Fatalf("SetProjectConfig: %v", err)
			}
			val, _ := env.pool.GetProjectConfig(env.project, "push_after_merge")
			got := val == "true"
			if got != tc.want {
				t.Errorf("push_after_merge config %q → %v, want %v", tc.stored, got, tc.want)
			}
		})
	}
}
