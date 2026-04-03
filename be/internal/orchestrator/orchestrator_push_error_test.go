package orchestrator

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"
)

// mockErrorRecorder captures RecordError calls for assertion in orchestrator tests.
type mockErrorRecorder struct {
	mu    sync.Mutex
	calls []errorCall
}

type errorCall struct {
	projectID  string
	errorType  string
	instanceID string
	message    string
}

func (m *mockErrorRecorder) RecordError(projectID, errorType, instanceID, message string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.calls = append(m.calls, errorCall{
		projectID:  projectID,
		errorType:  errorType,
		instanceID: instanceID,
		message:    message,
	})
	return nil
}

func (m *mockErrorRecorder) callCount() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return len(m.calls)
}

func (m *mockErrorRecorder) getCall(i int) errorCall {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.calls[i]
}

// setupBadGitRepo creates a git repo with a nonexistent remote so that git push
// will fail with non-empty error output. The remote path suffix (used for uniqueness)
// is appended to "/tmp/nonexistent-remote-". Returns the working directory.
func setupBadGitRepo(t *testing.T, suffix string) string {
	t.Helper()
	dir := t.TempDir()
	run := func(args ...string) {
		cmd := exec.Command("git", args...)
		cmd.Dir = dir
		cmd.Run() //nolint:errcheck — best-effort git setup
	}
	run("init")
	run("config", "user.name", "Test User")
	run("config", "user.email", "test@example.com")
	exec.Command("git", "-C", dir, "checkout", "-b", "main").Run() //nolint:errcheck
	if err := os.WriteFile(filepath.Join(dir, "x.txt"), []byte("x"), 0o644); err != nil {
		t.Fatalf("write x.txt: %v", err)
	}
	run("add", "x.txt")
	run("commit", "-m", "init")
	run("remote", "add", "origin", "/tmp/nonexistent-remote-"+suffix)
	return dir
}

// TestPushIfEnabled_FailureRecordsError verifies that when push fails, RecordError is
// called exactly once with type=system, the correct projectID and instanceID, and a
// message containing both the branch name and "git push failed".
func TestPushIfEnabled_FailureRecordsError(t *testing.T) {
	env := newTestEnv(t)
	mock := &mockErrorRecorder{}
	env.orch.errorSvc = mock

	const defaultBranch = "main"
	const wfiID = "wfi-push-err-record"

	badDir := setupBadGitRepo(t, "err-record")
	wt := &worktreeInfo{
		projectRoot:   badDir,
		defaultBranch: defaultBranch,
	}
	req := RunRequest{
		ProjectID:    env.project,
		WorkflowName: "test",
	}

	env.orch.pushIfEnabled(context.Background(), true, wt, wfiID, req)

	if got := mock.callCount(); got != 1 {
		t.Fatalf("RecordError calls = %d, want 1", got)
	}

	call := mock.getCall(0)
	if call.projectID != env.project {
		t.Errorf("RecordError projectID = %q, want %q", call.projectID, env.project)
	}
	if call.errorType != "system" {
		t.Errorf("RecordError errorType = %q, want %q", call.errorType, "system")
	}
	if call.instanceID != wfiID {
		t.Errorf("RecordError instanceID = %q, want %q", call.instanceID, wfiID)
	}
	if !strings.Contains(call.message, defaultBranch) {
		t.Errorf("RecordError message = %q, want it to contain branch name %q", call.message, defaultBranch)
	}
	if !strings.Contains(call.message, "git push failed") {
		t.Errorf("RecordError message = %q, want it to contain 'git push failed'", call.message)
	}
}

// TestPushIfEnabled_SuccessDoesNotRecordError verifies that RecordError is NOT called
// when push succeeds.
func TestPushIfEnabled_SuccessDoesNotRecordError(t *testing.T) {
	env := newTestEnv(t)
	workingPath, _ := setupGitRepoForPush(t)

	mock := &mockErrorRecorder{}
	env.orch.errorSvc = mock

	wt := &worktreeInfo{
		projectRoot:   workingPath,
		defaultBranch: "main",
	}
	req := RunRequest{
		ProjectID:    env.project,
		WorkflowName: "test",
	}

	env.orch.pushIfEnabled(context.Background(), true, wt, "wfi-success-no-err", req)

	if got := mock.callCount(); got != 0 {
		t.Errorf("RecordError calls = %d on successful push, want 0", got)
	}
}

// TestPushIfEnabled_NilErrorSvcNoPanic verifies that a nil errorSvc does not panic
// on push failure (the nil-guard in pushIfEnabled preserves existing behavior).
func TestPushIfEnabled_NilErrorSvcNoPanic(t *testing.T) {
	env := newTestEnv(t)
	// errorSvc is nil by default from newTestEnv — no injection needed

	badDir := setupBadGitRepo(t, "nil-errsvc")
	wt := &worktreeInfo{
		projectRoot:   badDir,
		defaultBranch: "main",
	}
	req := RunRequest{
		ProjectID:    env.project,
		WorkflowName: "test",
	}

	// Must not panic when errorSvc is nil
	env.orch.pushIfEnabled(context.Background(), true, wt, "wfi-nil-errsvc", req)
}

// TestPushIfEnabled_DisabledDoesNotRecordError verifies that RecordError is NOT called
// when pushAfterMerge=false (pushIfEnabled returns early before git or error recording).
func TestPushIfEnabled_DisabledDoesNotRecordError(t *testing.T) {
	env := newTestEnv(t)
	mock := &mockErrorRecorder{}
	env.orch.errorSvc = mock

	wt := &worktreeInfo{
		projectRoot:   "/tmp/nonexistent-push-disabled-err",
		defaultBranch: "main",
	}
	req := RunRequest{
		ProjectID:    env.project,
		WorkflowName: "test",
	}

	env.orch.pushIfEnabled(context.Background(), false, wt, "wfi-disabled-err", req)

	if got := mock.callCount(); got != 0 {
		t.Errorf("RecordError calls = %d when push disabled, want 0", got)
	}
}
