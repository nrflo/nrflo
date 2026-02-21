package usagelimits

import (
	"context"
	"os"
	"os/exec"
	"strings"
	"sync"
	"time"

	"github.com/creack/pty"
)

// ptySession manages a process running in a pseudo-terminal.
type ptySession struct {
	cmd  *exec.Cmd
	ptmx *os.File
	mu   sync.Mutex
	buf  strings.Builder
}

// startPTY spawns name in a PTY with the given environment.
func startPTY(name string, env []string) (*ptySession, error) {
	cmd := exec.Command(name)
	cmd.Env = env
	ptmx, err := pty.StartWithSize(cmd, &pty.Winsize{Rows: 50, Cols: 220})
	if err != nil {
		return nil, err
	}

	sess := &ptySession{cmd: cmd, ptmx: ptmx}
	go func() {
		buf := make([]byte, 4096)
		for {
			n, err := ptmx.Read(buf)
			if n > 0 {
				sess.mu.Lock()
				sess.buf.Write(buf[:n])
				sess.mu.Unlock()
			}
			if err != nil {
				return
			}
		}
	}()

	return sess, nil
}

// send writes data to the PTY as keyboard input.
func (s *ptySession) send(data string) {
	s.ptmx.WriteString(data) //nolint:errcheck
}

// waitFor polls until one of the patterns appears in the accumulated output,
// the timeout expires, or ctx is cancelled. Returns false on timeout/cancel.
func (s *ptySession) waitFor(ctx context.Context, patterns []string, timeout time.Duration) bool {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if ctx.Err() != nil {
			return false
		}
		s.mu.Lock()
		text := s.buf.String()
		s.mu.Unlock()
		for _, p := range patterns {
			if strings.Contains(text, p) {
				return true
			}
		}
		select {
		case <-time.After(50 * time.Millisecond):
		case <-ctx.Done():
			return false
		}
	}
	return false
}

// output returns the full accumulated raw output.
func (s *ptySession) output() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.buf.String()
}

// close terminates the process and PTY.
func (s *ptySession) close() {
	if s.cmd.Process != nil {
		s.cmd.Process.Kill() //nolint:errcheck
	}
	s.ptmx.Close()
	s.cmd.Wait() //nolint:errcheck
}
