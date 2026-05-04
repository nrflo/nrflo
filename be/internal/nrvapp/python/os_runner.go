package python

import (
	"bytes"
	"context"
	"errors"
	"os/exec"
	"syscall"
	"time"
)

const stderrCap = 4 * 1024 // 4 KB stderr capture

// OSRunner is a Runner that executes scripts via python3 from PATH.
// The cmdFactory field is injectable for testing.
type OSRunner struct {
	cmdFactory func(ctx context.Context, name string, args ...string) *exec.Cmd
}

// NewOSRunner creates an OSRunner using exec.CommandContext as the factory.
func NewOSRunner() *OSRunner {
	return &OSRunner{cmdFactory: exec.CommandContext}
}

// newOSRunnerWithFactory creates an OSRunner with a custom command factory (for tests).
func newOSRunnerWithFactory(factory func(ctx context.Context, name string, args ...string) *exec.Cmd) *OSRunner {
	return &OSRunner{cmdFactory: factory}
}

// Invoke runs scriptPath with the given input via python3.
// env is the complete environment slice (use MatchEnv to scope).
// timeout is applied as a context deadline; on expiry the process group is killed.
func (r *OSRunner) Invoke(ctx context.Context, scriptPath string, input []byte, env []string, timeout time.Duration) ([]byte, error) {
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	cmd := r.cmdFactory(ctx, "python3", scriptPath)
	cmd.Env = env
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	cmd.Stdin = bytes.NewReader(input)

	var stdout bytes.Buffer
	stderrBuf := newRingBuffer(stderrCap)
	cmd.Stdout = &stdout
	cmd.Stderr = stderrBuf

	err := cmd.Run()
	if err == nil {
		return stdout.Bytes(), nil
	}

	// On context deadline, kill the entire process group to avoid zombies.
	if ctx.Err() == context.DeadlineExceeded && cmd.Process != nil {
		//nolint:errcheck
		syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
	}

	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) {
		return nil, &ScriptError{
			ExitCode: exitErr.ExitCode(),
			Stderr:   stderrBuf.String(),
			Cause:    err,
		}
	}
	return nil, err
}
