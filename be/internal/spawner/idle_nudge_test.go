package spawner

import (
	"context"
	"testing"
	"time"

	"be/internal/clock"
)

// TestIdleNudge_Constants verifies the idle/nudge detection constants match the spec.
func TestIdleNudge_Constants(t *testing.T) {
	t.Parallel()
	if defaultIdleAfterMessageTimeout != 4*time.Minute {
		t.Errorf("defaultIdleAfterMessageTimeout = %v, want 4m", defaultIdleAfterMessageTimeout)
	}
	if defaultIdleStartTimeout != 2*time.Minute {
		t.Errorf("defaultIdleStartTimeout = %v, want 2m", defaultIdleStartTimeout)
	}
	if defaultNudgeMax != 5 {
		t.Errorf("defaultNudgeMax = %d, want 5", defaultNudgeMax)
	}
}

// TestIdleNudge_Fields_ProcessInfo verifies processInfo has the expected nudge fields.
func TestIdleNudge_Fields_ProcessInfo(t *testing.T) {
	t.Parallel()
	proc := &processInfo{
		nudgeCount:              2,
		nudgeMax:                5,
		idleStartTimeout:        2 * time.Minute,
		idleAfterMessageTimeout: 3 * time.Minute,
	}

	if proc.nudgeCount != 2 {
		t.Errorf("nudgeCount = %d, want 2", proc.nudgeCount)
	}
	if proc.nudgeMax != 5 {
		t.Errorf("nudgeMax = %d, want 5", proc.nudgeMax)
	}
	if proc.idleStartTimeout != 2*time.Minute {
		t.Errorf("idleStartTimeout = %v, want 2m", proc.idleStartTimeout)
	}
	if proc.idleAfterMessageTimeout != 3*time.Minute {
		t.Errorf("idleAfterMessageTimeout = %v, want 3m", proc.idleAfterMessageTimeout)
	}
	if !proc.lastNudgeAt.IsZero() {
		t.Error("default lastNudgeAt should be zero")
	}
}

// TestCheckIdleNudge_DisabledWhenNudgeMaxZero verifies nudgeMax=0 skips all logic.
func TestCheckIdleNudge_DisabledWhenNudgeMaxZero(t *testing.T) {
	t.Parallel()
	clk := clock.NewTest(time.Now())
	s := New(Config{Clock: clk})

	proc := &processInfo{
		nudgeMax:         0, // disabled
		backend:          &cliInteractiveBackend{},
		lastMessageTime:  clk.Now().Add(-10 * time.Minute),
		idleStartTimeout: 2 * time.Minute,
	}

	s.checkIdleNudge(context.Background(), proc, SpawnRequest{})

	if proc.nudgeCount != 0 {
		t.Errorf("nudgeCount = %d, want 0 (nudgeMax=0 disables nudge)", proc.nudgeCount)
	}
}

// TestCheckIdleNudge_SkipsNilBackend verifies nil backend skips all logic.
func TestCheckIdleNudge_SkipsNilBackend(t *testing.T) {
	t.Parallel()
	clk := clock.NewTest(time.Now())
	s := New(Config{Clock: clk})

	proc := &processInfo{
		nudgeMax:         5,
		backend:          nil,
		lastMessageTime:  clk.Now().Add(-10 * time.Minute),
		idleStartTimeout: 2 * time.Minute,
	}

	s.checkIdleNudge(context.Background(), proc, SpawnRequest{})

	if proc.nudgeCount != 0 {
		t.Errorf("nudgeCount = %d, want 0 (nil backend skips)", proc.nudgeCount)
	}
}

// TestCheckIdleNudge_SkipsNonInteractiveBackend verifies any backend whose Name()
// is not "cli_interactive" is skipped by idle-nudge (api, script, etc.).
func TestCheckIdleNudge_SkipsNonInteractiveBackend(t *testing.T) {
	t.Parallel()
	clk := clock.NewTest(time.Now())
	s := New(Config{Clock: clk})

	proc := &processInfo{
		nudgeMax:         5,
		backend:          fakeBackend{name: "script"},
		lastMessageTime:  clk.Now().Add(-10 * time.Minute),
		idleStartTimeout: 2 * time.Minute,
	}

	s.checkIdleNudge(context.Background(), proc, SpawnRequest{})

	if proc.nudgeCount != 0 {
		t.Errorf("nudgeCount = %d, want 0 (non-interactive backend is skipped)", proc.nudgeCount)
	}
}

// TestCheckIdleNudge_SkipsAPIBackend verifies apiBackend (Name()="api") is skipped.
func TestCheckIdleNudge_SkipsAPIBackend(t *testing.T) {
	t.Parallel()
	clk := clock.NewTest(time.Now())
	s := New(Config{Clock: clk})

	proc := &processInfo{
		nudgeMax:         5,
		backend:          &apiBackend{},
		lastMessageTime:  clk.Now().Add(-10 * time.Minute),
		idleStartTimeout: 2 * time.Minute,
	}

	s.checkIdleNudge(context.Background(), proc, SpawnRequest{})

	if proc.nudgeCount != 0 {
		t.Errorf("nudgeCount = %d, want 0 (apiBackend Name()=api is skipped)", proc.nudgeCount)
	}
}

// TestCheckIdleNudge_WithinIdleStartTimeout_NoNudge verifies no nudge while within the start window.
func TestCheckIdleNudge_WithinIdleStartTimeout_NoNudge(t *testing.T) {
	t.Parallel()
	clk := clock.NewTest(time.Now())
	s := New(Config{Clock: clk})

	proc := &processInfo{
		nudgeMax:           5,
		backend:            &cliInteractiveBackend{},
		hasReceivedMessage: false,
		lastMessageTime:    clk.Now(),
		idleStartTimeout:   2 * time.Minute,
	}

	clk.Advance(1 * time.Minute) // within the 2m window

	s.checkIdleNudge(context.Background(), proc, SpawnRequest{})

	if proc.nudgeCount != 0 {
		t.Errorf("nudgeCount = %d, want 0 (within idle window)", proc.nudgeCount)
	}
}

// TestCheckIdleNudge_IdleStartTimeoutExceeded_FirstNudge verifies first nudge when start timeout exceeded.
func TestCheckIdleNudge_IdleStartTimeoutExceeded_FirstNudge(t *testing.T) {
	t.Parallel()
	clk := clock.NewTest(time.Now())
	s := New(Config{Clock: clk})

	proc := &processInfo{
		nudgeMax:           5,
		nudgeCount:         0,
		backend:            &cliInteractiveBackend{},
		hasReceivedMessage: false,
		lastMessageTime:    clk.Now(),
		idleStartTimeout:   2 * time.Minute,
	}

	clk.Advance(3 * time.Minute) // past the 2m window

	s.checkIdleNudge(context.Background(), proc, SpawnRequest{})

	if proc.nudgeCount != 1 {
		t.Errorf("nudgeCount = %d, want 1 (first nudge sent)", proc.nudgeCount)
	}
}

// TestCheckIdleNudge_HasMessageUsesAfterMessageTimeout verifies the post-message idle window.
func TestCheckIdleNudge_HasMessageUsesAfterMessageTimeout(t *testing.T) {
	t.Parallel()
	clk := clock.NewTest(time.Now())
	s := New(Config{Clock: clk})

	proc := &processInfo{
		nudgeMax:                5,
		nudgeCount:              0,
		backend:                 &cliInteractiveBackend{},
		hasReceivedMessage:      true,
		lastMessageTime:         clk.Now(),
		idleStartTimeout:        2 * time.Minute,
		idleAfterMessageTimeout: 3 * time.Minute,
	}

	// Advance past idleStartTimeout but not idleAfterMessageTimeout.
	clk.Advance(2*time.Minute + 30*time.Second)
	s.checkIdleNudge(context.Background(), proc, SpawnRequest{})
	if proc.nudgeCount != 0 {
		t.Errorf("nudgeCount = %d, want 0 (within idleAfterMessageTimeout=3m)", proc.nudgeCount)
	}

	// Now advance past idleAfterMessageTimeout.
	clk.Advance(1 * time.Minute)
	s.checkIdleNudge(context.Background(), proc, SpawnRequest{})
	if proc.nudgeCount != 1 {
		t.Errorf("nudgeCount = %d, want 1 (idleAfterMessageTimeout exceeded)", proc.nudgeCount)
	}
}

// TestCheckIdleNudge_ZeroIdleWindow_NoNudge verifies that a zero idle window disables nudging.
func TestCheckIdleNudge_ZeroIdleWindow_NoNudge(t *testing.T) {
	t.Parallel()
	clk := clock.NewTest(time.Now())
	s := New(Config{Clock: clk})

	proc := &processInfo{
		nudgeMax:           5,
		backend:            &cliInteractiveBackend{},
		hasReceivedMessage: false,
		lastMessageTime:    clk.Now().Add(-10 * time.Minute),
		idleStartTimeout:   0, // disabled
	}

	s.checkIdleNudge(context.Background(), proc, SpawnRequest{})

	if proc.nudgeCount != 0 {
		t.Errorf("nudgeCount = %d, want 0 (idleStartTimeout=0 disables nudge)", proc.nudgeCount)
	}
}

// TestCheckIdleNudge_NudgeCapReached_RecentLastNudge_NoAutoFail verifies no auto-fail when
// cap is reached but the last nudge was recent (within one idle window).
func TestCheckIdleNudge_NudgeCapReached_RecentLastNudge_NoAutoFail(t *testing.T) {
	t.Parallel()
	clk := clock.NewTest(time.Now())
	s := New(Config{Clock: clk})

	base := clk.Now()
	proc := &processInfo{
		nudgeMax:                5,
		nudgeCount:              5, // cap reached
		backend:                 &cliInteractiveBackend{},
		hasReceivedMessage:      true,
		lastMessageTime:         base.Add(-5 * time.Minute),
		idleAfterMessageTimeout: 3 * time.Minute,
		lastNudgeAt:             base.Add(-1 * time.Minute), // recent (< idle window)
		sessionID:               "sess-recent-nudge",
	}

	ch := make(chan terminalSignal, 1)
	s.registerTerminalSignal(proc.sessionID, ch)

	s.checkIdleNudge(context.Background(), proc, SpawnRequest{})

	select {
	case <-ch:
		t.Error("auto-fail triggered when lastNudgeAt is recent (< idle window)")
	default:
	}
}

// TestCheckIdleNudge_NudgeCapReached_OldLastNudge_AutoFail verifies auto-fail when cap is
// reached and a full idle window has elapsed since the last nudge.
func TestCheckIdleNudge_NudgeCapReached_OldLastNudge_AutoFail(t *testing.T) {
	t.Parallel()
	clk := clock.NewTest(time.Now())
	s := New(Config{Clock: clk})

	base := clk.Now()
	proc := &processInfo{
		nudgeMax:                5,
		nudgeCount:              5, // cap reached
		backend:                 &cliInteractiveBackend{},
		hasReceivedMessage:      true,
		lastMessageTime:         base.Add(-10 * time.Minute),
		idleAfterMessageTimeout: 3 * time.Minute,
		lastNudgeAt:             base.Add(-4 * time.Minute), // older than idle window
		sessionID:               "sess-auto-fail",
	}

	ch := make(chan terminalSignal, 1)
	s.registerTerminalSignal(proc.sessionID, ch)

	s.checkIdleNudge(context.Background(), proc, SpawnRequest{})

	select {
	case sig := <-ch:
		if sig.SessionID != "sess-auto-fail" {
			t.Errorf("signal.SessionID = %q, want 'sess-auto-fail'", sig.SessionID)
		}
		if sig.Result != "fail" {
			t.Errorf("signal.Result = %q, want 'fail'", sig.Result)
		}
	default:
		t.Error("auto-fail not triggered: registered channel empty after cap + elapsed idle window")
	}
}

// TestCheckIdleNudge_IncrementCounter_MultipleNudges verifies nudgeCount increments each
// cycle and lastNudgeAt is updated after each nudge.
func TestCheckIdleNudge_IncrementCounter_MultipleNudges(t *testing.T) {
	t.Parallel()
	clk := clock.NewTest(time.Now())
	s := New(Config{Clock: clk})

	proc := &processInfo{
		nudgeMax:                5,
		nudgeCount:              0,
		backend:                 &cliInteractiveBackend{},
		hasReceivedMessage:      false,
		lastMessageTime:         clk.Now(),
		idleStartTimeout:        2 * time.Minute,
		idleAfterMessageTimeout: 2 * time.Minute,
	}

	// First nudge.
	clk.Advance(3 * time.Minute)
	s.checkIdleNudge(context.Background(), proc, SpawnRequest{})
	if proc.nudgeCount != 1 {
		t.Fatalf("nudgeCount = %d, want 1 after first nudge", proc.nudgeCount)
	}
	firstNudgeAt := proc.lastNudgeAt
	if firstNudgeAt.IsZero() {
		t.Error("lastNudgeAt not set after first nudge")
	}

	// sendNudge resets lastMessageTime to Now(); advance past the window again.
	clk.Advance(3 * time.Minute)
	s.checkIdleNudge(context.Background(), proc, SpawnRequest{})
	if proc.nudgeCount != 2 {
		t.Errorf("nudgeCount = %d, want 2 after second nudge", proc.nudgeCount)
	}
	if !proc.lastNudgeAt.After(firstNudgeAt) {
		t.Error("lastNudgeAt not advanced after second nudge")
	}
}
