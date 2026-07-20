package spawner

import (
	"context"
	"testing"
	"time"

	"be/internal/clock"
)

// TestTriggerImmediateNudge_UnderCap_IncrementsAndResetsLastMessage verifies
// triggerImmediateNudge sends a nudge (nudgeCount increments, lastMessageTime
// reset) when under the nudge cap.
func TestTriggerImmediateNudge_UnderCap_IncrementsAndResetsLastMessage(t *testing.T) {
	t.Parallel()
	clk := clock.NewTest(time.Now())
	s := New(Config{Clock: clk})

	proc := &processInfo{
		nudgeMax:        5,
		nudgeCount:      1,
		backend:         &cliInteractiveBackend{},
		sessionID:       "sess-imm-nudge",
		agentType:       "implementor",
		lastMessageTime: clk.Now().Add(-10 * time.Minute),
	}

	clk.Advance(1 * time.Minute)
	expectedTime := clk.Now()

	s.triggerImmediateNudge(context.Background(), proc, SpawnRequest{}, "idle")

	if proc.nudgeCount != 2 {
		t.Errorf("nudgeCount = %d, want 2", proc.nudgeCount)
	}
	proc.messagesMutex.Lock()
	lmt := proc.lastMessageTime
	proc.messagesMutex.Unlock()
	if lmt != expectedTime {
		t.Errorf("lastMessageTime = %v, want %v (reset by nudge)", lmt, expectedTime)
	}
}

// TestTriggerImmediateNudge_AtCap_AutoFails verifies triggerImmediateNudge
// dispatches a "fail" terminal signal when the nudge cap is already spent.
func TestTriggerImmediateNudge_AtCap_AutoFails(t *testing.T) {
	t.Parallel()
	clk := clock.NewTest(time.Now())
	s := New(Config{Clock: clk})

	proc := &processInfo{
		nudgeMax:   3,
		nudgeCount: 3, // cap reached
		backend:    &cliInteractiveBackend{},
		sessionID:  "sess-imm-cap",
		agentType:  "implementor",
		projectID:  "proj-1",
	}

	ch := make(chan terminalSignal, 1)
	s.registerTerminalSignal(proc.sessionID, ch)

	s.triggerImmediateNudge(context.Background(), proc, SpawnRequest{}, "permission")

	select {
	case sig := <-ch:
		if sig.SessionID != proc.sessionID {
			t.Errorf("signal.SessionID = %q, want %q", sig.SessionID, proc.sessionID)
		}
		if sig.Result != "fail" {
			t.Errorf("signal.Result = %q, want %q", sig.Result, "fail")
		}
	default:
		t.Error("registered channel empty: auto-fail not dispatched at nudge cap")
	}
}

// TestTriggerImmediateNudge_NonInteractiveBackend_NoOp verifies non-cli_interactive
// backends (e.g. codex) are excluded from the in-band nudge path.
func TestTriggerImmediateNudge_NonInteractiveBackend_NoOp(t *testing.T) {
	t.Parallel()
	clk := clock.NewTest(time.Now())
	s := New(Config{Clock: clk})

	proc := &processInfo{
		nudgeMax:   5,
		nudgeCount: 0,
		backend:    fakeBackend{name: "codex"},
		sessionID:  "sess-imm-codex",
	}

	ch := make(chan terminalSignal, 1)
	s.registerTerminalSignal(proc.sessionID, ch)

	s.triggerImmediateNudge(context.Background(), proc, SpawnRequest{}, "idle")

	if proc.nudgeCount != 0 {
		t.Errorf("nudgeCount = %d, want 0 (codex backend must be a no-op)", proc.nudgeCount)
	}
	select {
	case <-ch:
		t.Error("terminal signal dispatched for non-cli_interactive backend")
	default:
	}
}

// TestTriggerImmediateNudge_NudgeMaxZero_NoOp verifies the nudge-less
// api-via-cli lane (nudgeMax=0) is excluded from the in-band nudge path.
func TestTriggerImmediateNudge_NudgeMaxZero_NoOp(t *testing.T) {
	t.Parallel()
	clk := clock.NewTest(time.Now())
	s := New(Config{Clock: clk})

	proc := &processInfo{
		nudgeMax:   0,
		nudgeCount: 0,
		backend:    &cliInteractiveBackend{},
		sessionID:  "sess-imm-nudgeless",
	}

	ch := make(chan terminalSignal, 1)
	s.registerTerminalSignal(proc.sessionID, ch)

	s.triggerImmediateNudge(context.Background(), proc, SpawnRequest{}, "idle")

	if proc.nudgeCount != 0 {
		t.Errorf("nudgeCount = %d, want 0 (nudgeMax=0 must be a no-op)", proc.nudgeCount)
	}
	select {
	case <-ch:
		t.Error("terminal signal dispatched despite nudgeMax=0")
	default:
	}
}

// TestTriggerIdleNudge_EnqueuesNudgeRequest verifies Spawner.TriggerIdleNudge
// (the socket-facing entry point) enqueues a nudgeRequest onto nudgeRequestCh
// for monitorAll to drain, without touching proc state directly.
func TestTriggerIdleNudge_EnqueuesNudgeRequest(t *testing.T) {
	t.Parallel()
	clk := clock.NewTest(time.Now())
	s := New(Config{Clock: clk})

	s.TriggerIdleNudge("sess-enqueue-1", "permission")

	select {
	case nr := <-s.nudgeRequestCh:
		if nr.sessionID != "sess-enqueue-1" {
			t.Errorf("sessionID = %q, want %q", nr.sessionID, "sess-enqueue-1")
		}
		if nr.reason != "permission" {
			t.Errorf("reason = %q, want %q", nr.reason, "permission")
		}
	default:
		t.Fatal("nudgeRequestCh empty: TriggerIdleNudge did not enqueue a request")
	}
}

// TestDispatchNudgeRequest_MatchesRunningProc verifies dispatchNudgeRequest
// finds the proc matching the request's sessionID among running procs and
// fires triggerImmediateNudge for it (nudgeCount increments); a request for
// an unmatched (already-finished) session is a no-op.
func TestDispatchNudgeRequest_MatchesRunningProc(t *testing.T) {
	t.Parallel()
	clk := clock.NewTest(time.Now())
	s := New(Config{Clock: clk})

	proc := &processInfo{
		nudgeMax:   5,
		nudgeCount: 0,
		backend:    &cliInteractiveBackend{},
		sessionID:  "sess-dispatch-match",
	}
	running := []*processInfo{proc}

	s.dispatchNudgeRequest(context.Background(), running, SpawnRequest{}, nudgeRequest{sessionID: "sess-not-running", reason: "idle"})
	if proc.nudgeCount != 0 {
		t.Errorf("nudgeCount = %d, want 0 (unmatched session must be a no-op)", proc.nudgeCount)
	}

	s.dispatchNudgeRequest(context.Background(), running, SpawnRequest{}, nudgeRequest{sessionID: "sess-dispatch-match", reason: "idle"})
	if proc.nudgeCount != 1 {
		t.Errorf("nudgeCount = %d, want 1 (matched session must nudge)", proc.nudgeCount)
	}
}

// TestNoDoubleNudge_ImmediateThenWallClock_NoSecondNudge verifies the
// no-double-nudge invariant: an in-band nudge resets lastMessageTime, so a
// subsequent wall-clock checkIdleNudge pass with the clock un-advanced
// returns early and does not send a second nudge for the same idle episode.
func TestNoDoubleNudge_ImmediateThenWallClock_NoSecondNudge(t *testing.T) {
	t.Parallel()
	clk := clock.NewTest(time.Now())
	s := New(Config{Clock: clk})

	proc := &processInfo{
		nudgeMax:                5,
		nudgeCount:              0,
		backend:                 &cliInteractiveBackend{},
		sessionID:               "sess-no-double",
		hasReceivedMessage:      true,
		lastMessageTime:         clk.Now().Add(-10 * time.Minute), // already past idle window
		idleAfterMessageTimeout: 3 * time.Minute,
	}

	// In-band Notification-triggered nudge fires first.
	s.triggerImmediateNudge(context.Background(), proc, SpawnRequest{}, "idle")
	if proc.nudgeCount != 1 {
		t.Fatalf("nudgeCount = %d, want 1 after immediate nudge", proc.nudgeCount)
	}

	// Wall-clock fallback runs next, clock un-advanced: sendNudge already
	// reset lastMessageTime to now, so checkIdleNudge's sinceLastMsg check
	// must return early without sending a second nudge.
	s.checkIdleNudge(context.Background(), proc, SpawnRequest{})
	if proc.nudgeCount != 1 {
		t.Errorf("nudgeCount = %d, want 1 (wall-clock pass must not double-nudge same idle episode)", proc.nudgeCount)
	}
}
