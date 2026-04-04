package spawner

import (
	"context"
	"testing"
	"time"

	"be/internal/clock"
)

// TestStall_Constants verifies the stall detection constants match the spec.
func TestStall_Constants(t *testing.T) {
	if defaultStallStartTimeout != 2*time.Minute {
		t.Errorf("defaultStallStartTimeout = %v, want 2m", defaultStallStartTimeout)
	}
	if defaultStallRunningTimeout != 8*time.Minute {
		t.Errorf("defaultStallRunningTimeout = %v, want 8m", defaultStallRunningTimeout)
	}
	if maxStallRestarts != 15 {
		t.Errorf("maxStallRestarts = %d, want 15", maxStallRestarts)
	}
}

// TestStall_Fields_ProcessInfo verifies processInfo has the expected stall fields.
func TestStall_Fields_ProcessInfo(t *testing.T) {
	proc := &processInfo{
		stallStartTimeout:   30 * time.Second,
		stallRunningTimeout: 5 * time.Minute,
		stallRestartCount:   2,
		hasReceivedMessage:  true,
	}

	if proc.stallStartTimeout != 30*time.Second {
		t.Errorf("stallStartTimeout = %v, want 30s", proc.stallStartTimeout)
	}
	if proc.stallRunningTimeout != 5*time.Minute {
		t.Errorf("stallRunningTimeout = %v, want 5m", proc.stallRunningTimeout)
	}
	if proc.stallRestartCount != 2 {
		t.Errorf("stallRestartCount = %d, want 2", proc.stallRestartCount)
	}
	if !proc.hasReceivedMessage {
		t.Error("hasReceivedMessage = false, want true")
	}

	// Zero values
	proc2 := &processInfo{}
	if proc2.stallRestartCount != 0 {
		t.Errorf("default stallRestartCount = %d, want 0", proc2.stallRestartCount)
	}
	if proc2.hasReceivedMessage {
		t.Error("default hasReceivedMessage = true, want false")
	}
}

// TestCheckStall_LowContextSavingSkips verifies stall check is bypassed during low-context save.
func TestCheckStall_LowContextSavingSkips(t *testing.T) {
	clk := clock.NewTest(time.Now())
	s := New(Config{WSHub: nil, Clock: clk})

	proc := &processInfo{
		lowContextSaving:   true,
		hasReceivedMessage: false,
		// last message far in the past — would stall if not for lowContextSaving
		lastMessageTime:    clk.Now().Add(-10 * time.Minute),
		stallStartTimeout:  2 * time.Minute,
		stallRestartCount:  0,
	}

	got := s.checkStall(context.Background(), proc, SpawnRequest{})
	if got {
		t.Error("checkStall should return false when lowContextSaving=true")
	}
}

// TestCheckStall_MaxRestartsReached verifies stall check is blocked at maxStallRestarts.
func TestCheckStall_MaxRestartsReached(t *testing.T) {
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
	clk := clock.NewTest(time.Now())
	s := New(Config{WSHub: nil, Clock: clk})

	proc := &processInfo{
		hasReceivedMessage:  true,
		lastMessageTime:     clk.Now().Add(-10 * time.Minute), // way overdue
		stallRunningTimeout: 0,                                 // disabled
		stallRestartCount:   0,
	}

	got := s.checkStall(context.Background(), proc, SpawnRequest{})
	if got {
		t.Error("checkStall should return false when stallRunningTimeout=0 (disabled)")
	}
}

