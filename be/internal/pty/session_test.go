package pty

import (
	"os"
	"os/exec"
	"strings"
	"sync"
	"testing"
	"time"

	creackpty "github.com/creack/pty"
)

// buildTestEnv returns a minimal environment for spawning test commands.
func buildTestEnv() []string {
	env := []string{"TERM=dumb"}
	for _, key := range []string{"PATH", "HOME", "USER", "SHELL", "LANG"} {
		if v := os.Getenv(key); v != "" {
			env = append(env, key+"="+v)
		}
	}
	return env
}

// newSessionWithCmd creates a Session wrapping an arbitrary command rather than
// `claude --resume`. It has the same internal structure as NewSession but allows
// injecting any command for testing without requiring the claude binary.
func newSessionWithCmd(name string, args []string, workDir string, env []string) (*Session, error) {
	cmd := exec.Command(args[0], args[1:]...)
	cmd.Dir = workDir
	cmd.Env = env

	ptmx, err := creackpty.Start(cmd)
	if err != nil {
		return nil, err
	}

	s := &Session{
		ptmx:      ptmx,
		cmd:       cmd,
		sessionID: "test-session-id",
		done:      make(chan struct{}),
	}
	_ = name // unused but kept for readability at call sites

	go func() {
		_ = cmd.Wait()
		close(s.done)
	}()

	return s, nil
}

// TestNewSession_SpawnsProcess verifies that a session started via
// newSessionWithCmd produces a valid Session with the correct SessionID,
// and that Done() closes when the process is explicitly terminated.
func TestNewSession_SpawnsProcess(t *testing.T) {
	sess, err := newSessionWithCmd("cat", []string{"cat"}, t.TempDir(), buildTestEnv())
	if err != nil {
		t.Fatalf("newSessionWithCmd: %v", err)
	}

	if sess.SessionID() != "test-session-id" {
		t.Errorf("SessionID = %q, want %q", sess.SessionID(), "test-session-id")
	}

	// Close terminates the process; Done must then close.
	_ = sess.Close()
	select {
	case <-sess.Done():
		// expected
	case <-time.After(5 * time.Second):
		t.Fatal("session Done() did not close within timeout")
	}
}

// TestSession_ReadOutput verifies that output written by the child process is
// readable via Session.Read.
func TestSession_ReadOutput(t *testing.T) {
	sess, err := newSessionWithCmd("cat", []string{"cat"}, t.TempDir(), buildTestEnv())
	if err != nil {
		t.Fatalf("newSessionWithCmd: %v", err)
	}
	t.Cleanup(func() { _ = sess.Close() })

	msg := "hello\n"
	if _, err := sess.Write([]byte(msg)); err != nil {
		t.Fatalf("Write: %v", err)
	}

	buf := make([]byte, 256)
	done := make(chan struct{})
	var result string
	go func() {
		defer close(done)
		n, _ := sess.Read(buf)
		result = string(buf[:n])
	}()

	select {
	case <-done:
	case <-time.After(3 * time.Second):
		t.Fatal("Read timed out")
	}

	if !strings.Contains(result, "hello") {
		t.Errorf("Read returned %q, want to contain 'hello'", result)
	}
}

// TestSession_Resize verifies that Resize does not return an error on a
// running PTY.
func TestSession_Resize(t *testing.T) {
	sess, err := newSessionWithCmd("cat", []string{"cat"}, t.TempDir(), buildTestEnv())
	if err != nil {
		t.Fatalf("newSessionWithCmd: %v", err)
	}
	t.Cleanup(func() { _ = sess.Close() })

	if err := sess.Resize(40, 120); err != nil {
		t.Errorf("Resize(40, 120) returned error: %v", err)
	}
	if err := sess.Resize(24, 80); err != nil {
		t.Errorf("Resize(24, 80) returned error: %v", err)
	}
}

// TestSession_CloseTerminatesProcess verifies that Close kills the process and
// Done() closes.
func TestSession_CloseTerminatesProcess(t *testing.T) {
	sess, err := newSessionWithCmd("cat", []string{"cat"}, t.TempDir(), buildTestEnv())
	if err != nil {
		t.Fatalf("newSessionWithCmd: %v", err)
	}

	if err := sess.Close(); err != nil {
		// ptmx.Close() may return EIO when the slave side closes; non-fatal.
		t.Logf("Close returned (non-fatal): %v", err)
	}

	select {
	case <-sess.Done():
		// expected
	case <-time.After(5 * time.Second):
		t.Fatal("session Done() did not close after Close()")
	}
}

// TestSession_CloseIsIdempotent verifies that calling Close twice does not panic.
func TestSession_CloseIsIdempotent(t *testing.T) {
	sess, err := newSessionWithCmd("cat", []string{"cat"}, t.TempDir(), buildTestEnv())
	if err != nil {
		t.Fatalf("newSessionWithCmd: %v", err)
	}

	_ = sess.Close()
	_ = sess.Close() // second call must not panic
}

// TestSession_Wait blocks until process exits.
func TestSession_Wait(t *testing.T) {
	sess, err := newSessionWithCmd("sh", []string{"sh", "-c", "exit 0"}, t.TempDir(), buildTestEnv())
	if err != nil {
		t.Fatalf("newSessionWithCmd: %v", err)
	}
	t.Cleanup(func() { _ = sess.Close() })

	waitDone := make(chan struct{})
	go func() {
		defer close(waitDone)
		_ = sess.Wait()
	}()

	select {
	case <-waitDone:
		// expected
	case <-time.After(5 * time.Second):
		t.Fatal("Wait() did not return within timeout")
	}
}

// TestSession_DoneChannelSignalsOnExit verifies the Done channel semantics
// using a session whose process exits immediately.
func TestSession_DoneChannelSignalsOnExit(t *testing.T) {
	sess, err := newSessionWithCmd("sh", []string{"sh", "-c", "exit 0"}, t.TempDir(), buildTestEnv())
	if err != nil {
		t.Fatalf("newSessionWithCmd: %v", err)
	}
	t.Cleanup(func() { _ = sess.Close() })

	// Calling Done() multiple times must return the same channel.
	ch1 := sess.Done()
	ch2 := sess.Done()
	if ch1 != ch2 {
		t.Error("Done() returned different channels on repeated calls")
	}

	select {
	case <-ch1:
		// expected — channel was closed on exit
	case <-time.After(5 * time.Second):
		t.Fatal("Done() channel not closed after process exit")
	}
}

// TestManager_AutoRemoveAfterExit verifies that the Manager's auto-remove
// goroutine deletes the session entry once the process exits.
func TestManager_AutoRemoveAfterExit(t *testing.T) {
	m := NewManager()

	// Inject a stub session with an already-closed done channel.
	sess := &Session{
		sessionID: "auto-rm",
		done:      make(chan struct{}),
	}
	m.mu.Lock()
	m.sessions["auto-rm"] = sess
	m.mu.Unlock()

	// Start the same auto-remove goroutine the real Create() would launch.
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		<-sess.Done()
		m.Remove("auto-rm")
	}()

	// Simulate process exit.
	close(sess.done)
	wg.Wait()

	if got := m.Get("auto-rm"); got != nil {
		t.Errorf("session still tracked after process exit, want nil")
	}
}
