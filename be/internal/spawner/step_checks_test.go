package spawner

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
)

// stepCheckProc registers a fresh processInfo under a new sessionID with env
// so RunStepChecks (session-keyed) can find it.
func stepCheckProc(env *testEnv, workDir string) (*processInfo, string) {
	sessionID := uuid.New().String()
	proc := &processInfo{
		sessionID:          sessionID,
		agentID:            "test-agent-id",
		modelID:            "claude:sonnet-5",
		agentType:          "test-agent",
		workflowInstanceID: env.wfiID,
		projectID:          env.projectID,
		ticketID:           env.ticketID,
		workflowName:       env.workflowID,
		startTime:          time.Now(),
		workDir:            workDir,
		env:                []string{"NRFLO_PROJECT=" + env.projectID, "NRFLO_AGENT_TOKEN=secret-token", "NRF_SESSION_ID=" + sessionID},
	}
	env.spawner.registerSessionProc(sessionID, proc)
	return proc, sessionID
}

// TestRunStepChecks_AllPass verifies (-1, 0, "", nil) when every command exits 0.
func TestRunStepChecks_AllPass(t *testing.T) {
	t.Parallel()
	env := setupTestEnv(t)
	defer env.cleanup()

	_, sessionID := stepCheckProc(env, "")

	idx, code, tail, err := env.spawner.RunStepChecks(context.Background(), sessionID, []string{"true", "true"})
	if idx != -1 || code != 0 || tail != "" || err != nil {
		t.Errorf("RunStepChecks(all pass) = (%d,%d,%q,%v), want (-1,0,\"\",nil)", idx, code, tail, err)
	}
}

// TestRunStepChecks_MiddleFailStopsShortCircuits verifies the failing index is
// reported and the command after it never runs.
func TestRunStepChecks_MiddleFailStopsShortCircuits(t *testing.T) {
	t.Parallel()
	env := setupTestEnv(t)
	defer env.cleanup()

	dir := t.TempDir()
	marker := filepath.Join(dir, "should-not-exist")

	_, sessionID := stepCheckProc(env, dir)

	idx, code, _, err := env.spawner.RunStepChecks(context.Background(), sessionID,
		[]string{"true", "false", "touch " + marker})
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if idx != 1 {
		t.Errorf("failedIdx = %d, want 1", idx)
	}
	if code != 1 {
		t.Errorf("exitCode = %d, want 1", code)
	}
	if _, statErr := os.Stat(marker); statErr == nil {
		t.Error("third command ran despite the second one's failure")
	}
}

// TestRunStepChecks_WorkDirHonoured verifies commands execute with cwd == proc.workDir.
func TestRunStepChecks_WorkDirHonoured(t *testing.T) {
	t.Parallel()
	env := setupTestEnv(t)
	defer env.cleanup()

	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "marker"), []byte("x"), 0o644); err != nil {
		t.Fatalf("seed marker: %v", err)
	}

	_, sessionID := stepCheckProc(env, dir)

	idx, code, _, err := env.spawner.RunStepChecks(context.Background(), sessionID, []string{"test -f marker"})
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if idx != -1 || code != 0 {
		t.Errorf("RunStepChecks(cwd check) = (%d,%d), want (-1,0) — workDir %q not honoured", idx, code, dir)
	}
}

// TestRunStepChecks_UnknownSessionOrEmptyCmds verifies both degrade to a no-op pass.
func TestRunStepChecks_UnknownSessionOrEmptyCmds(t *testing.T) {
	t.Parallel()
	env := setupTestEnv(t)
	defer env.cleanup()

	idx, code, tail, err := env.spawner.RunStepChecks(context.Background(), "no-such-session", []string{"false"})
	if idx != -1 || code != 0 || tail != "" || err != nil {
		t.Errorf("unknown session: got (%d,%d,%q,%v), want (-1,0,\"\",nil)", idx, code, tail, err)
	}

	_, sessionID := stepCheckProc(env, "")
	idx2, code2, tail2, err2 := env.spawner.RunStepChecks(context.Background(), sessionID, nil)
	if idx2 != -1 || code2 != 0 || tail2 != "" || err2 != nil {
		t.Errorf("empty cmds: got (%d,%d,%q,%v), want (-1,0,\"\",nil)", idx2, code2, tail2, err2)
	}
}

// TestRunStepChecks_TailCapped verifies output beyond stepCheckTailSize is trimmed.
func TestRunStepChecks_TailCapped(t *testing.T) {
	t.Parallel()
	env := setupTestEnv(t)
	defer env.cleanup()

	_, sessionID := stepCheckProc(env, "")

	shellCmd := `python3 -c "import sys; sys.stdout.write('A'*20000); sys.exit(1)"`
	idx, _, tail, err := env.spawner.RunStepChecks(context.Background(), sessionID, []string{shellCmd})
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if idx != 0 {
		t.Fatalf("failedIdx = %d, want 0", idx)
	}
	if len(tail) > stepCheckTailSize {
		t.Errorf("tail length %d exceeds cap %d", len(tail), stepCheckTailSize)
	}
}

// TestRunStepChecks_EnvScrubbed verifies session credentials never reach the
// child process env.
func TestRunStepChecks_EnvScrubbed(t *testing.T) {
	t.Parallel()
	env := setupTestEnv(t)
	defer env.cleanup()

	dir := t.TempDir()
	outFile := filepath.Join(dir, "out.txt")
	_, sessionID := stepCheckProc(env, dir)

	idx, code, _, err := env.spawner.RunStepChecks(context.Background(), sessionID,
		[]string{"echo TOKEN=$NRFLO_AGENT_TOKEN SESSION=$NRF_SESSION_ID > " + outFile})
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if idx != -1 || code != 0 {
		t.Fatalf("setup command failed: idx=%d code=%d", idx, code)
	}
	out, readErr := os.ReadFile(outFile)
	if readErr != nil {
		t.Fatalf("read out file: %v", readErr)
	}
	got := string(out)
	if strings.Contains(got, "secret-token") {
		t.Errorf("NRFLO_AGENT_TOKEN leaked into check env: %q", got)
	}
	if strings.Contains(got, sessionID) {
		t.Errorf("NRF_SESSION_ID leaked into check env: %q", got)
	}
}

// TestRunStepChecks_TotalBudgetExceededReturnsCheckFailure verifies a budget
// that expires mid-run converts into a normal check failure, not a Go error.
func TestRunStepChecks_TotalBudgetExceededReturnsCheckFailure(t *testing.T) {
	origBudget := stepChecksTotalBudget
	origTimeout := stepCheckCommandTimeout
	stepChecksTotalBudget = 30 * time.Millisecond
	stepCheckCommandTimeout = 5 * time.Second
	t.Cleanup(func() {
		stepChecksTotalBudget = origBudget
		stepCheckCommandTimeout = origTimeout
	})

	env := setupTestEnv(t)
	defer env.cleanup()

	_, sessionID := stepCheckProc(env, "")

	idx, _, _, err := env.spawner.RunStepChecks(context.Background(), sessionID, []string{"sleep 5"})
	if err != nil {
		t.Fatalf("budget expiry surfaced as a Go error: %v, want a check failure", err)
	}
	if idx < 0 {
		t.Errorf("failedIdx = %d, want >= 0 (a check failure) when the total budget expires", idx)
	}
}
