package spawner

import (
	"strings"
	"sync"
	"testing"
	"time"
)

// countSubmits returns how many body+CR submissions the mock received.
// Each attempt writes the body once and a bare "\r" once, so counting the
// trailing CRs counts attempts.
func countSubmits(m *mockPtySession) int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return strings.Count(string(m.writtenBytes), "\r")
}

func ackProc() *processInfo {
	return &processInfo{sessionID: "sess-ack", agentType: "web-researcher-cheap", doneCh: make(chan struct{})}
}

// TestSubmitPromptWithRetry_ResubmitsWhenPromptWasNeverAccepted is the
// regression guard for the ~21% silent prompt loss seen on 7-wide layer
// fan-outs. Readiness before the first write is inferred, never confirmed: a
// quiet PTY during a starved bootstrap looks exactly like a quiet PTY on a
// ready input loop, so the paste + CR lands mid-bootstrap and the TUI redraws
// over it. The agent then sits idle at 100% context with zero recorded
// messages until the start-stall detector kills it two minutes later.
func TestSubmitPromptWithRetry_ResubmitsWhenPromptWasNeverAccepted(t *testing.T) {
	t.Parallel()

	s := New(Config{})
	proc := ackProc() // hasReceivedMessage stays false: nothing ever recorded
	sess := newMockSession()

	submitPromptWithRetryFast(s, proc, sess, "PROMPT")

	if got := countSubmits(sess); got != maxPromptSubmits {
		t.Errorf("submissions = %d, want %d — an unacknowledged prompt must be re-sent, not left for the 2-minute stall detector", got, maxPromptSubmits)
	}
}

// TestSubmitPromptWithRetry_StopsOnceAccepted pins the safety property that
// makes the retry sound: a prompt that WAS accepted must never be sent twice,
// or the agent receives its instructions duplicated. hasReceivedMessage is the
// ack because, for adapters that do not bump on raw PTY bytes, it flips only
// on a real recorded event — and UserPromptSubmit (seq 0) is always the first.
func TestSubmitPromptWithRetry_StopsOnceAccepted(t *testing.T) {
	t.Parallel()

	s := New(Config{})
	proc := ackProc()
	sess := newMockSession()

	// Simulate the UserPromptSubmit echo landing shortly after the first CR.
	var once sync.Once
	go func() {
		once.Do(func() {
			time.Sleep(20 * time.Millisecond)
			proc.messagesMutex.Lock()
			proc.hasReceivedMessage = true
			proc.messagesMutex.Unlock()
		})
	}()

	submitPromptWithRetryFast(s, proc, sess, "PROMPT")

	if got := countSubmits(sess); got != 1 {
		t.Errorf("submissions = %d, want 1 — an accepted prompt must never be re-sent (duplicate instructions)", got)
	}
}

// TestSubmitPromptWithRetry_NoAckSignalWritesExactlyOnce covers adapters whose
// stall state bumps on raw PTY bytes (codex): paint traffic alone would
// satisfy the ack, so there is no usable signal and the retry must be off
// rather than firing on a false negative.
func TestSubmitPromptWithRetry_NoAckSignalWritesExactlyOnce(t *testing.T) {
	t.Parallel()

	s := New(Config{})
	proc := ackProc()
	sess := newMockSession()

	submitPromptWithRetry(s, proc, sess, "PROMPT", "codex", time.Now(), false, 60*time.Millisecond, 10*time.Millisecond)

	if got := countSubmits(sess); got != 1 {
		t.Errorf("submissions = %d, want 1 when the adapter has no delivery ack", got)
	}
}

// TestWaitPromptAck_ProcessExitTreatedAsAcked stops a dead process from
// burning the full attempt budget: once doneCh is closed there is nothing left
// to accept the prompt, so re-writing into its PTY is pure noise.
func TestWaitPromptAck_ProcessExitTreatedAsAcked(t *testing.T) {
	t.Parallel()

	proc := ackProc()
	close(proc.doneCh)

	if !waitPromptAck(proc, time.Second, 10*time.Millisecond) {
		t.Error("waitPromptAck = false for an exited process, want true (stop retrying a dead PTY)")
	}
}

// submitPromptWithRetryFast runs the real retry loop at test scale so the
// suite stays inside the 60s cap (Rule 4).
func submitPromptWithRetryFast(s *Spawner, proc *processInfo, sess ptySessionIface, body string) {
	submitPromptWithRetry(s, proc, sess, body, "claude", time.Now(), true, 60*time.Millisecond, 10*time.Millisecond)
}
