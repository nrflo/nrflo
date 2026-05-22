package spawner

import (
	"context"
	"testing"
	"time"

	"be/internal/clock"
)

// TestCheckStall_LowContextSavingSkips verifies stall check is bypassed during low-context save.
func TestCheckStall_LowContextSavingSkips(t *testing.T) {
	t.Parallel()
	clk := clock.NewTest(time.Now())
	s := New(Config{WSHub: nil, Clock: clk})

	proc := &processInfo{
		lowContextSaving:   true,
		hasReceivedMessage: false,
		// last message far in the past — would stall if not for lowContextSaving
		lastMessageTime:   clk.Now().Add(-10 * time.Minute),
		stallStartTimeout: 2 * time.Minute,
		stallRestartCount: 0,
	}

	got := s.checkStall(context.Background(), proc, SpawnRequest{})
	if got {
		t.Error("checkStall should return false when lowContextSaving=true")
	}
}

// TestCheckStall_MaxRestartsReached verifies stall check is blocked at maxStallRestarts.
func TestCheckStall_MaxRestartsReached(t *testing.T) {
	t.Parallel()
	clk := clock.NewTest(time.Now())
	s := New(Config{WSHub: nil, Clock: clk})

	proc := &processInfo{
		lowContextSaving:   false,
		stallRestartCount:  maxStallRestarts, // exhausted
		hasReceivedMessage: false,
		lastMessageTime:    clk.Now().Add(-10 * time.Minute),
		stallStartTimeout:  2 * time.Minute,
	}

	got := s.checkStall(context.Background(), proc, SpawnRequest{})
	if got {
		t.Error("checkStall should return false when stallRestartCount >= maxStallRestarts")
	}
}

// TestCheckStall_StartStallNotYet verifies no stall when within start timeout.
func TestCheckStall_StartStallNotYet(t *testing.T) {
	t.Parallel()
	clk := clock.NewTest(time.Now())
	s := New(Config{WSHub: nil, Clock: clk})

	proc := &processInfo{
		hasReceivedMessage: false,
		lastMessageTime:    clk.Now(),
		stallStartTimeout:  2 * time.Minute,
		stallRestartCount:  0,
	}

	// Advance less than timeout
	clk.Advance(1 * time.Minute)

	got := s.checkStall(context.Background(), proc, SpawnRequest{})
	if got {
		t.Error("checkStall should return false when elapsed < stallStartTimeout")
	}
}

// TestCheckStall_RunningStallNotYet verifies no stall when within running timeout.
func TestCheckStall_RunningStallNotYet(t *testing.T) {
	t.Parallel()
	clk := clock.NewTest(time.Now())
	s := New(Config{WSHub: nil, Clock: clk})

	proc := &processInfo{
		hasReceivedMessage:  true,
		lastMessageTime:     clk.Now(),
		stallRunningTimeout: 8 * time.Minute,
		stallRestartCount:   0,
	}

	// Advance less than running timeout
	clk.Advance(5 * time.Minute)

	got := s.checkStall(context.Background(), proc, SpawnRequest{})
	if got {
		t.Error("checkStall should return false when elapsed < stallRunningTimeout")
	}
}

// TestCheckStall_StartStallDisabled verifies no stall when stallStartTimeout=0.
func TestCheckStall_StartStallDisabled(t *testing.T) {
	t.Parallel()
	clk := clock.NewTest(time.Now())
	s := New(Config{WSHub: nil, Clock: clk})

	proc := &processInfo{
		hasReceivedMessage: false,
		lastMessageTime:    clk.Now().Add(-10 * time.Minute), // way overdue
		stallStartTimeout:  0,                                // disabled
		stallRestartCount:  0,
	}

	got := s.checkStall(context.Background(), proc, SpawnRequest{})
	if got {
		t.Error("checkStall should return false when stallStartTimeout=0 (disabled)")
	}
}

// TestCheckStall_RunningStallDisabled verifies no stall when stallRunningTimeout=0.
func TestCheckStall_RunningStallDisabled(t *testing.T) {
	t.Parallel()
	clk := clock.NewTest(time.Now())
	s := New(Config{WSHub: nil, Clock: clk})

	proc := &processInfo{
		hasReceivedMessage:  true,
		lastMessageTime:     clk.Now().Add(-10 * time.Minute), // way overdue
		stallRunningTimeout: 0,                                // disabled
		stallRestartCount:   0,
	}

	got := s.checkStall(context.Background(), proc, SpawnRequest{})
	if got {
		t.Error("checkStall should return false when stallRunningTimeout=0 (disabled)")
	}
}
