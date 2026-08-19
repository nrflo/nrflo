package spawner

import (
	"context"
	"testing"
	"time"

	"be/internal/clock"
)

// TestHandleNudgeAutoFail_DispatchesTerminalSignal verifies RequestTerminalSignal is called
// with the correct sessionID and "fail" result once the stall-restart budget is also exhausted.
func TestHandleNudgeAutoFail_DispatchesTerminalSignal(t *testing.T) {
	t.Parallel()
	clk := clock.NewTest(time.Now())
	s := New(Config{Clock: clk})

	proc := &processInfo{
		sessionID:         "sess-auto-fail",
		agentType:         "implementor",
		projectID:         "proj-1",
		nudgeMax:          5,
		nudgeCount:        5,
		stallRestartCount: maxStallRestarts, // budget exhausted — must terminally fail, not restart
	}

	ch := make(chan terminalSignal, 1)
	s.registerTerminalSignal(proc.sessionID, ch)

	s.handleNudgeAutoFail(context.Background(), proc, SpawnRequest{})

	select {
	case sig := <-ch:
		if sig.SessionID != "sess-auto-fail" {
			t.Errorf("signal.SessionID = %q, want 'sess-auto-fail'", sig.SessionID)
		}
		if sig.Result != "fail" {
			t.Errorf("signal.Result = %q, want 'fail'", sig.Result)
		}
	default:
		t.Error("registered channel empty: RequestTerminalSignal not dispatched by handleNudgeAutoFail")
	}
}

// TestHandleNudgeAutoFail_NilServices_NoPanic verifies no panic when AgentSvcReal and
// ErrorSvc are nil (the common test configuration).
func TestHandleNudgeAutoFail_NilServices_NoPanic(t *testing.T) {
	t.Parallel()
	clk := clock.NewTest(time.Now())
	s := New(Config{Clock: clk}) // AgentSvcReal=nil, ErrorSvc=nil

	proc := &processInfo{
		sessionID:         "sess-nil-svc",
		agentType:         "test-agent",
		projectID:         "proj-1",
		nudgeMax:          5,
		stallRestartCount: maxStallRestarts, // budget exhausted — exercises the Fail()/ErrorSvc nil-guard path
	}

	// Must not panic regardless of nil services.
	s.handleNudgeAutoFail(context.Background(), proc, SpawnRequest{})
}

// TestHandleNudgeAutoFail_RestartsBeforeFailing verifies that when the
// stall-restart budget is not yet exhausted, an unresponsive nudge cap
// triggers a kill+relaunch (handleStallRestart) instead of an immediate
// terminal fail — a stuck interactive dialog needs a fresh process, not more
// nudge text, and a terminal signal must NOT be dispatched in this case.
func TestHandleNudgeAutoFail_RestartsBeforeFailing(t *testing.T) {
	t.Parallel()
	clk := clock.NewTest(time.Now())
	s := New(Config{Clock: clk})

	proc := stallPositiveProc(clk, true, 0, 0) // stallRestartCount=0, doneCh pre-closed
	proc.sessionID = "sess-nudge-restart"
	proc.agentType = "implementor"
	proc.nudgeMax = 5
	proc.nudgeCount = 5

	ch := make(chan terminalSignal, 1)
	s.registerTerminalSignal(proc.sessionID, ch)

	s.handleNudgeAutoFail(context.Background(), proc, SpawnRequest{})

	if proc.finalStatus != "CONTINUE" {
		t.Errorf("finalStatus = %q, want CONTINUE (restart, not fail)", proc.finalStatus)
	}
	if proc.stallRestartCount != 1 {
		t.Errorf("stallRestartCount = %d, want 1 after restart", proc.stallRestartCount)
	}
	select {
	case sig := <-ch:
		t.Errorf("terminal signal unexpectedly dispatched: %+v", sig)
	default:
	}
}
