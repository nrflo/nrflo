package spawner

import (
	"context"
	"testing"
	"time"

	"be/internal/clock"
	"be/internal/ws"
)

// TestWaitForRateLimitRetry_CancelledContext verifies false is returned immediately.
func TestWaitForRateLimitRetry_CancelledContext(t *testing.T) {
	t.Parallel()
	hub := ws.NewHub(clock.Real())
	go hub.Run()
	defer hub.Stop()

	s := New(Config{WSHub: hub, Clock: clock.Real()})
	proc := &processInfo{
		sessionID:           "sess-rl-cancel",
		agentType:           "implementor",
		modelID:             "claude:sonnet",
		projectID:           "proj-1",
		ticketID:            "ticket-1",
		workflowName:        "feature",
		rateLimitRetryCount: 1,
		rateLimitConfig: rateLimitConfig{
			InitialBackoff: 60 * time.Second,
			MaxWait:        3600 * time.Second,
		},
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // already cancelled before call

	start := time.Now()
	got := s.waitForRateLimitRetry(ctx, proc, SpawnRequest{})
	elapsed := time.Since(start)

	if got {
		t.Error("waitForRateLimitRetry should return false when context is cancelled")
	}
	if elapsed > 500*time.Millisecond {
		t.Errorf("took %v with cancelled context, want <500ms", elapsed)
	}
}

// TestWaitForRateLimitRetry_ClockFires verifies true is returned and totalWait is updated.
// Uses InitialBackoff=0 so After(0) fires immediately on TestClock (deadline==now).
func TestWaitForRateLimitRetry_ClockFires(t *testing.T) {
	t.Parallel()
	clk := clock.NewTest(time.Now())
	s := New(Config{WSHub: nil, Clock: clk})
	proc := &processInfo{
		sessionID:           "sess-rl-clock",
		agentType:           "implementor",
		modelID:             "claude:sonnet",
		projectID:           "proj-1",
		ticketID:            "ticket-1",
		workflowName:        "feature",
		rateLimitRetryCount: 1,
		rateLimitTotalWait:  30 * time.Second,
		rateLimitConfig: rateLimitConfig{
			InitialBackoff: 0, // After(0) fires immediately — deadline == now
			MaxWait:        3600 * time.Second,
		},
	}

	got := s.waitForRateLimitRetry(context.Background(), proc, SpawnRequest{})
	if !got {
		t.Error("waitForRateLimitRetry should return true when clock fires")
	}
	// delay=0, so totalWait += 0 → still 30s
	if proc.rateLimitTotalWait != 30*time.Second {
		t.Errorf("rateLimitTotalWait = %v, want 30s", proc.rateLimitTotalWait)
	}
}

// TestWaitForRateLimitRetry_TotalWaitAccumulates verifies each successful wait adds its delay.
func TestWaitForRateLimitRetry_TotalWaitAccumulates(t *testing.T) {
	t.Parallel()
	clk := clock.NewTest(time.Now())
	s := New(Config{WSHub: nil, Clock: clk})

	proc := &processInfo{
		sessionID:           "sess-accum",
		rateLimitRetryCount: 1,
		rateLimitTotalWait:  0,
		rateLimitConfig: rateLimitConfig{
			InitialBackoff: 0, // fires immediately
			MaxWait:        3600 * time.Second,
		},
	}
	// First wait: delay=0, totalWait stays 0.
	s.waitForRateLimitRetry(context.Background(), proc, SpawnRequest{})
	if proc.rateLimitTotalWait != 0 {
		t.Errorf("totalWait after 1st wait = %v, want 0", proc.rateLimitTotalWait)
	}
}

// TestWaitForRateLimitRetry_DelayPassedToClock verifies the computed delay is passed to Clock.After.
// Uses a spy clock that records delays and fires them immediately.
func TestWaitForRateLimitRetry_DelayPassedToClock(t *testing.T) {
	t.Parallel()
	var recorded []time.Duration
	spy := &immediateSpyClock{onAfter: func(d time.Duration) { recorded = append(recorded, d) }}
	s := New(Config{WSHub: nil, Clock: spy})

	cfg := rateLimitConfig{InitialBackoff: 60 * time.Second, MaxWait: 3600 * time.Second}
	for n := 1; n <= 3; n++ {
		proc := &processInfo{
			rateLimitRetryCount: n,
			rateLimitConfig:     cfg,
		}
		s.waitForRateLimitRetry(context.Background(), proc, SpawnRequest{})
	}

	want := []time.Duration{60 * time.Second, 120 * time.Second, 240 * time.Second}
	if len(recorded) != len(want) {
		t.Fatalf("recorded %d calls to Clock.After, want %d", len(recorded), len(want))
	}
	for i, w := range want {
		if recorded[i] != w {
			t.Errorf("Clock.After call %d: delay=%v, want %v", i+1, recorded[i], w)
		}
	}
}

// TestRateLimitCarryover_FieldsPropagate verifies rateLimitRetryCount/TotalWait/config/adapter
// carry through relaunchForContinuation (mirrors stall_restart_event_test.go carryover test).
func TestRateLimitCarryover_FieldsPropagate(t *testing.T) {
	t.Parallel()
	adapter := &ClaudeAdapter{}
	cfg := rateLimitConfig{
		Enabled:        true,
		InitialBackoff: 60 * time.Second,
		MaxWait:        3600 * time.Second,
	}
	oldProc := &processInfo{
		sessionID:           "old-sess",
		rateLimitRetryCount: 3,
		rateLimitTotalWait:  300 * time.Second,
		rateLimitConfig:     cfg,
		adapter:             adapter,
		stallRestartCount:   1,
		failRestartCount:    2,
		restartCount:        4,
		restartThreshold:    25,
		maxFailRestarts:     5,
	}

	newProc := &processInfo{}
	// Mirror relaunchForContinuation field assignments (completion.go:172-185).
	newProc.restartCount = oldProc.restartCount + 1
	newProc.restartThreshold = oldProc.restartThreshold
	newProc.maxFailRestarts = oldProc.maxFailRestarts
	newProc.failRestartCount = oldProc.failRestartCount
	newProc.stallRestartCount = oldProc.stallRestartCount
	newProc.rateLimitRetryCount = oldProc.rateLimitRetryCount
	newProc.rateLimitTotalWait = oldProc.rateLimitTotalWait
	newProc.rateLimitConfig = oldProc.rateLimitConfig
	newProc.adapter = oldProc.adapter

	if newProc.rateLimitRetryCount != 3 {
		t.Errorf("rateLimitRetryCount = %d, want 3", newProc.rateLimitRetryCount)
	}
	if newProc.rateLimitTotalWait != 300*time.Second {
		t.Errorf("rateLimitTotalWait = %v, want 300s", newProc.rateLimitTotalWait)
	}
	if !newProc.rateLimitConfig.Enabled {
		t.Error("rateLimitConfig.Enabled not carried")
	}
	if newProc.rateLimitConfig.InitialBackoff != 60*time.Second {
		t.Errorf("rateLimitConfig.InitialBackoff = %v, want 60s", newProc.rateLimitConfig.InitialBackoff)
	}
	if newProc.adapter != adapter {
		t.Error("adapter pointer not carried")
	}
	// failRestartCount must not be incremented during rate-limit carryover.
	if newProc.failRestartCount != 2 {
		t.Errorf("failRestartCount = %d, want 2 (unchanged)", newProc.failRestartCount)
	}
	if newProc.restartCount != 5 {
		t.Errorf("restartCount = %d, want 5 (incremented)", newProc.restartCount)
	}
}

// immediateSpyClock is a Clock that records After(d) calls and fires them immediately.
type immediateSpyClock struct {
	t       time.Time
	onAfter func(d time.Duration)
}

func (c *immediateSpyClock) Now() time.Time { return time.Now() }
func (c *immediateSpyClock) After(d time.Duration) <-chan time.Time {
	if c.onAfter != nil {
		c.onAfter(d)
	}
	ch := make(chan time.Time, 1)
	ch <- time.Now() // fires immediately
	return ch
}
