package spawner

import (
	"testing"
	"time"

	"be/internal/clock"
)

// newIdleTestBackend builds a codex app-server backend wired to a test clock.
func newIdleTestBackend(clk *clock.TestClock) *codexAppServerBackend {
	return newCodexAppServerBackend(New(Config{Clock: clk}))
}

// TestBetweenTurnsDelay_CapsAtGrace is the regression guard for the fix: a codex
// agent that completed a turn without finishing must be nudged after the short
// between-turns grace, NOT the full 4-minute idle window.
func TestBetweenTurnsDelay_CapsAtGrace(t *testing.T) {
	t.Parallel()
	b := newIdleTestBackend(clock.NewTest(time.Now()))

	proc := &processInfo{
		nudgeMax:                5,
		hasReceivedMessage:      true,
		idleAfterMessageTimeout: 4 * time.Minute,
	}

	got := b.betweenTurnsDelay(proc)
	if got != codexBetweenTurnsNudgeDelay {
		t.Fatalf("betweenTurnsDelay = %v, want %v (must not be the full idle window %v)",
			got, codexBetweenTurnsNudgeDelay, proc.idleAfterMessageTimeout)
	}
}

// TestBetweenTurnsDelay_ShorterWindowWins verifies the configured idle window
// still wins when it is shorter than the grace.
func TestBetweenTurnsDelay_ShorterWindowWins(t *testing.T) {
	t.Parallel()
	b := newIdleTestBackend(clock.NewTest(time.Now()))

	proc := &processInfo{
		nudgeMax:                5,
		hasReceivedMessage:      true,
		idleAfterMessageTimeout: 5 * time.Second,
	}

	if got := b.betweenTurnsDelay(proc); got != 5*time.Second {
		t.Fatalf("betweenTurnsDelay = %v, want 5s (shorter configured window wins)", got)
	}
}

// TestBetweenTurnsDelay_DisabledPropagates verifies a 0/disabled idle window
// propagates through so the nudge is suppressed entirely.
func TestBetweenTurnsDelay_DisabledPropagates(t *testing.T) {
	t.Parallel()
	b := newIdleTestBackend(clock.NewTest(time.Now()))

	proc := &processInfo{
		nudgeMax:                5,
		hasReceivedMessage:      true,
		idleAfterMessageTimeout: 0,
	}

	if got := b.betweenTurnsDelay(proc); got != 0 {
		t.Fatalf("betweenTurnsDelay = %v, want 0 (disabled window propagates)", got)
	}
}

// TestBetweenTurnsDelay_NoMessageUsesStartTimeout verifies the start-timeout
// branch is also capped at the grace before the first message lands.
func TestBetweenTurnsDelay_NoMessageUsesStartTimeout(t *testing.T) {
	t.Parallel()
	b := newIdleTestBackend(clock.NewTest(time.Now()))

	proc := &processInfo{
		nudgeMax:         5,
		idleStartTimeout: 2 * time.Minute,
	}

	if got := b.betweenTurnsDelay(proc); got != codexBetweenTurnsNudgeDelay {
		t.Fatalf("betweenTurnsDelay = %v, want %v (start timeout capped at grace)", got, codexBetweenTurnsNudgeDelay)
	}
}

// TestArmIdleTimer_NilWhenTurnActive verifies no idle timer is armed while a turn
// is active — mid-turn silence is monitorAll's stall detector's job.
func TestArmIdleTimer_NilWhenTurnActive(t *testing.T) {
	t.Parallel()
	b := newIdleTestBackend(clock.NewTest(time.Now()))

	proc := &processInfo{nudgeMax: 5, hasReceivedMessage: true, idleAfterMessageTimeout: 4 * time.Minute}

	if ch := b.armIdleTimer(proc, true); ch != nil {
		t.Fatal("armIdleTimer returned non-nil channel during an active turn")
	}
}

// TestArmIdleTimer_NilWhenNudgeDisabled verifies nudgeMax=0 (api/script lanes)
// arms no timer.
func TestArmIdleTimer_NilWhenNudgeDisabled(t *testing.T) {
	t.Parallel()
	b := newIdleTestBackend(clock.NewTest(time.Now()))

	proc := &processInfo{nudgeMax: 0, hasReceivedMessage: true, idleAfterMessageTimeout: 4 * time.Minute}

	if ch := b.armIdleTimer(proc, false); ch != nil {
		t.Fatal("armIdleTimer returned non-nil channel when nudgeMax=0")
	}
}

// TestArmIdleTimer_FiresAfterGrace verifies that between turns the timer fires at
// the grace boundary (~10s), not the full 4-minute idle window.
func TestArmIdleTimer_FiresAfterGrace(t *testing.T) {
	t.Parallel()
	clk := clock.NewTest(time.Now())
	b := newIdleTestBackend(clk)

	proc := &processInfo{
		nudgeMax:                5,
		hasReceivedMessage:      true,
		idleAfterMessageTimeout: 4 * time.Minute,
		lastMessageTime:         clk.Now(),
	}

	ch := b.armIdleTimer(proc, false)
	if ch == nil {
		t.Fatal("armIdleTimer returned nil between turns")
	}

	// Just shy of the grace: must not fire yet.
	clk.Advance(codexBetweenTurnsNudgeDelay - time.Second)
	select {
	case <-ch:
		t.Fatal("idle timer fired before the between-turns grace elapsed")
	default:
	}

	// Cross the grace boundary: must fire (proving it is not the 4m window).
	clk.Advance(2 * time.Second)
	select {
	case <-ch:
	default:
		t.Fatal("idle timer did not fire after the between-turns grace")
	}
}
