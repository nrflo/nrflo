package pty

import (
	"fmt"
	"os"
	"os/exec"
	"sync"
	"syscall"

	creackpty "github.com/creack/pty"
)

// Session wraps a PTY master fd and the claude process running inside it.
type Session struct {
	ptmx      *os.File
	cmd       *exec.Cmd
	sessionID string
	done      chan struct{}
	closeOnce sync.Once
}

// NewSession spawns `claude --resume <sessionID>` in a new PTY.
// workDir sets the process working directory; env is the full environment.
func NewSession(sessionID, workDir string, env []string) (*Session, error) {
	cmd := exec.Command("claude", "--resume", sessionID)
	cmd.Dir = workDir
	cmd.Env = env

	ptmx, err := creackpty.Start(cmd)
	if err != nil {
		return nil, fmt.Errorf("pty start: %w", err)
	}

	s := &Session{
		ptmx:      ptmx,
		cmd:       cmd,
		sessionID: sessionID,
		done:      make(chan struct{}),
	}

	// Monitor process exit in background.
	go func() {
		_ = cmd.Wait()
		close(s.done)
	}()

	return s, nil
}

// Read reads from the PTY master (agent output).
func (s *Session) Read(p []byte) (int, error) {
	return s.ptmx.Read(p)
}

// Write writes to the PTY master (user input).
func (s *Session) Write(p []byte) (int, error) {
	return s.ptmx.Write(p)
}

// Resize sends a window-size change to the PTY.
func (s *Session) Resize(rows, cols uint16) error {
	return creackpty.Setsize(s.ptmx, &creackpty.Winsize{Rows: rows, Cols: cols})
}

// Close kills the process and closes the PTY master fd.
func (s *Session) Close() error {
	var err error
	s.closeOnce.Do(func() {
		if s.cmd.Process != nil {
			_ = s.cmd.Process.Signal(syscall.SIGTERM)
		}
		err = s.ptmx.Close()
	})
	return err
}

// Wait blocks until the process exits.
func (s *Session) Wait() error {
	<-s.done
	return nil
}

// Done returns a channel that is closed when the process exits.
func (s *Session) Done() <-chan struct{} {
	return s.done
}

// SessionID returns the nrworkflow session ID associated with this PTY.
func (s *Session) SessionID() string {
	return s.sessionID
}
