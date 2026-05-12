// Tests for the bumpOnPTYBytes path in ferryPTYOutput and the BumpsOnPTYBytes
// adapter method on all three CLIAdapter implementations.
package spawner

import (
	"strings"
	"testing"
	"time"

	"be/internal/clock"
)

// ferryProc returns a processInfo ready for ferryPTYOutput tests.
// It extends minProc with an initialised firstByteCh and a set lastMessageTime.
func ferryProc(sessionID string, initialTime time.Time) *processInfo {
	p := minProc(sessionID)
	p.firstByteCh = make(chan struct{})
	p.lastMessageTime = initialTime
	return p
}

// isClosed reports whether ch has been closed (non-blocking receive).
func isClosed(ch <-chan struct{}) bool {
	select {
	case <-ch:
		return true
	default:
		return false
	}
}

// TestFerryPTYOutput_BumpOnBytesEnabled verifies that when bumpOnPTYBytes=true,
// ferryPTYOutput sets lastMessageTime to Clock.Now() and hasReceivedMessage=true
// on the first PTY read. firstByteCh is closed unconditionally.
func TestFerryPTYOutput_BumpOnBytesEnabled(t *testing.T) {
	t.Parallel()

	clk := clock.NewTest(time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC))
	s := New(Config{Clock: clk})

	initialTime := clk.Now()
	proc := ferryProc("bump-enabled-1", initialTime)

	// Advance before the ferry call so Clock.Now() returns a strictly later value.
	clk.Advance(1 * time.Second)
	expectedTime := clk.Now()

	// One 100-byte chunk followed by immediate EOF (done channel pre-closed).
	sess := newMockSession()
	sess.readChunks = []string{strings.Repeat("x", 100)}
	sess.Kill() //nolint — always nil; pre-closes the done channel

	ferryPTYOutput(s, proc, sess, false, true)

	proc.messagesMutex.Lock()
	gotTime := proc.lastMessageTime
	gotHasMsg := proc.hasReceivedMessage
	proc.messagesMutex.Unlock()

	if !gotTime.Equal(expectedTime) {
		t.Errorf("lastMessageTime = %v, want %v (Clock.Now() at read time)", gotTime, expectedTime)
	}
	if !gotHasMsg {
		t.Error("hasReceivedMessage = false, want true after PTY bytes received")
	}
	if !isClosed(proc.firstByteCh) {
		t.Error("firstByteCh not closed; must close unconditionally on first PTY chunk")
	}
}

// TestFerryPTYOutput_BumpOnBytesDisabled verifies that when bumpOnPTYBytes=false,
// ferryPTYOutput does NOT update lastMessageTime or hasReceivedMessage, but still
// closes firstByteCh (the close is unconditional).
func TestFerryPTYOutput_BumpOnBytesDisabled(t *testing.T) {
	t.Parallel()

	clk := clock.NewTest(time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC))
	s := New(Config{Clock: clk})

	initialTime := clk.Now()
	proc := ferryProc("bump-disabled-1", initialTime)

	// Advance the clock — ferryPTYOutput must NOT use the new time.
	clk.Advance(1 * time.Second)

	sess := newMockSession()
	sess.readChunks = []string{strings.Repeat("x", 100)}
	sess.Kill() //nolint — always nil; pre-closes the done channel

	ferryPTYOutput(s, proc, sess, false, false)

	proc.messagesMutex.Lock()
	gotTime := proc.lastMessageTime
	gotHasMsg := proc.hasReceivedMessage
	proc.messagesMutex.Unlock()

	if !gotTime.Equal(initialTime) {
		t.Errorf("lastMessageTime = %v, want %v (must not change when bumpOnPTYBytes=false)",
			gotTime, initialTime)
	}
	if gotHasMsg {
		t.Error("hasReceivedMessage = true, want false when bumpOnPTYBytes=false")
	}
	// firstByteCh close is unconditional — it must still fire.
	if !isClosed(proc.firstByteCh) {
		t.Error("firstByteCh not closed; must close unconditionally even when bumpOnPTYBytes=false")
	}
}

// TestFerryPTYOutput_NilFirstByteCh verifies that ferryPTYOutput does not panic
// when firstByteCh is nil (the nil guard in production code must hold).
func TestFerryPTYOutput_NilFirstByteCh(t *testing.T) {
	t.Parallel()

	clk := clock.NewTest(time.Now())
	s := New(Config{Clock: clk})

	proc := minProc("nil-fbch-1")
	// firstByteCh intentionally left nil (zero value from minProc)

	sess := newMockSession()
	sess.readChunks = []string{"hello"}
	sess.Kill()

	// Must not panic regardless of bumpOnPTYBytes value.
	ferryPTYOutput(s, proc, sess, false, true)
}

// === BumpsOnPTYBytes adapter contract ===

// TestBumpsOnPTYBytes_Codex verifies CodexAdapter returns false — the rollout
// JSONL tailer (cli_adapter_codex_jsonl_tail.go, started by PostInteractiveStart)
// calls Sink.BumpLastMessage on every real agent event (agent_message,
// function_call, function_call_output, token_count), so PTY bytes must not
// reset the stall timer or stall detection becomes unreachable during redraws.
func TestBumpsOnPTYBytes_Codex(t *testing.T) {
	t.Parallel()
	if (&CodexAdapter{}).BumpsOnPTYBytes() {
		t.Error("CodexAdapter.BumpsOnPTYBytes() = true, want false (JSONL tailer drives heartbeat)")
	}
}

// TestBumpsOnPTYBytes_Claude verifies ClaudeAdapter returns false — hooks
// (PreToolUse/PostToolUse/Stop) already drive BumpLastMessage.
func TestBumpsOnPTYBytes_Claude(t *testing.T) {
	t.Parallel()
	if (&ClaudeAdapter{}).BumpsOnPTYBytes() {
		t.Error("ClaudeAdapter.BumpsOnPTYBytes() = true, want false (hooks drive heartbeat)")
	}
}

// TestBumpsOnPTYBytes_Opencode verifies OpencodeAdapter returns false — SSE
// events (message.part.updated / session.idle) drive BumpLastMessage.
func TestBumpsOnPTYBytes_Opencode(t *testing.T) {
	t.Parallel()
	if (&OpencodeAdapter{}).BumpsOnPTYBytes() {
		t.Error("OpencodeAdapter.BumpsOnPTYBytes() = true, want false (SSE events drive heartbeat)")
	}
}
