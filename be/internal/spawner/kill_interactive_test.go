package spawner

import (
	"testing"
	"time"

	"be/internal/clock"
)

// TestKillInteractive_ClosesWaitAndFlagsKill verifies that calling KillInteractive
// on a registered session closes the wait channel and marks killedInteractive.
func TestKillInteractive_ClosesWaitAndFlagsKill(t *testing.T) {
	t.Parallel()
	sp := New(Config{Clock: clock.Real()})
	sid := "sess-kill-1"

	waitCh := sp.RegisterInteractiveWait(sid)

	// Channel must block before kill
	select {
	case <-waitCh:
		t.Fatal("channel should block before KillInteractive is called")
	default:
	}

	sp.KillInteractive(sid)

	// Channel must be closed after kill
	select {
	case <-waitCh:
		// good
	case <-time.After(500 * time.Millisecond):
		t.Fatal("channel not closed after KillInteractive")
	}

	// killedInteractive flag must be set
	sp.mu.Lock()
	_, killed := sp.killedInteractive[sid]
	sp.mu.Unlock()
	if !killed {
		t.Error("killedInteractive[sid] should be set after KillInteractive")
	}
}

// TestKillInteractive_Multiple verifies each session is independent — killing one
// does not affect others.
func TestKillInteractive_Multiple(t *testing.T) {
	t.Parallel()
	sp := New(Config{Clock: clock.Real()})

	ch1 := sp.RegisterInteractiveWait("sess-kill-m1")
	ch2 := sp.RegisterInteractiveWait("sess-kill-m2")

	sp.KillInteractive("sess-kill-m1")

	// ch1 must be closed
	select {
	case <-ch1:
		// good
	case <-time.After(500 * time.Millisecond):
		t.Fatal("ch1 not closed after KillInteractive(sess-kill-m1)")
	}

	// ch2 must still block
	select {
	case <-ch2:
		t.Fatal("ch2 should still block")
	default:
	}

	// Only sess-kill-m1 should be flagged
	sp.mu.Lock()
	_, k1 := sp.killedInteractive["sess-kill-m1"]
	_, k2 := sp.killedInteractive["sess-kill-m2"]
	sp.mu.Unlock()
	if !k1 {
		t.Error("killedInteractive[sess-kill-m1] should be set")
	}
	if k2 {
		t.Error("killedInteractive[sess-kill-m2] should NOT be set")
	}
}

// TestKillInteractive_NoRegisteredWait verifies calling KillInteractive without a
// prior RegisterInteractiveWait still sets the kill flag and doesn't panic.
func TestKillInteractive_NoRegisteredWait(t *testing.T) {
	t.Parallel()
	sp := New(Config{Clock: clock.Real()})
	sid := "sess-kill-no-wait"

	// Must not panic
	sp.KillInteractive(sid)

	sp.mu.Lock()
	_, killed := sp.killedInteractive[sid]
	sp.mu.Unlock()
	if !killed {
		t.Error("killedInteractive[sid] should be set even without a registered wait")
	}
}

// TestKillInteractive_IdempotentClose verifies that calling KillInteractive twice
// on the same session does not panic (channel already closed).
func TestKillInteractive_IdempotentClose(t *testing.T) {
	t.Parallel()
	sp := New(Config{Clock: clock.Real()})
	sid := "sess-kill-idem"

	sp.RegisterInteractiveWait(sid)
	sp.KillInteractive(sid)
	// Second call must not panic
	sp.KillInteractive(sid)
}

// TestKillInteractive_SetsProFinalStatusFail verifies the monitorAll take-control path
// sets proc.finalStatus = "FAIL" when killedInteractive is set on the session.
// This is done by confirming the killedInteractive flag survives until the wait channel
// unblocks, which is the condition checked by monitorAll.
func TestKillInteractive_SetsProFinalStatusFail(t *testing.T) {
	t.Parallel()
	sp := New(Config{Clock: clock.Real()})
	sid := "sess-kill-final"

	// Simulate what monitorAll does: register wait, then kill
	sp.mu.Lock()
	ch := make(chan struct{})
	sp.interactiveWaits[sid] = ch
	sp.mu.Unlock()

	sp.KillInteractive(sid)

	// After kill: the wait channel must be closed and the kill flag must be present
	select {
	case <-ch:
		// closed — correct
	case <-time.After(500 * time.Millisecond):
		t.Fatal("wait channel not closed after KillInteractive")
	}

	// The flag is still present (monitorAll reads and deletes it after unblocking)
	sp.mu.Lock()
	_, wasKilled := sp.killedInteractive[sid]
	sp.mu.Unlock()
	if !wasKilled {
		t.Error("killedInteractive flag should be present for monitorAll to pick up")
	}
}
