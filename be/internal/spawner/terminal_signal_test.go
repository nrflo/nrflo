package spawner

import (
	"testing"

	"be/internal/clock"
)

// TestRequestTerminalSignal_RoutesToRegisteredChannel verifies that
// RequestTerminalSignal sends a terminalSignal to the channel registered
// for that session ID via registerTerminalSignal.
func TestRequestTerminalSignal_RoutesToRegisteredChannel(t *testing.T) {
	sp := New(Config{Clock: clock.Real()})
	ch := make(chan terminalSignal, 1)
	sp.registerTerminalSignal("sess-abc", ch)

	sp.RequestTerminalSignal("sess-abc", "fail")

	select {
	case sig := <-ch:
		if sig.SessionID != "sess-abc" {
			t.Errorf("expected SessionID='sess-abc', got %q", sig.SessionID)
		}
		if sig.Result != "fail" {
			t.Errorf("expected Result='fail', got %q", sig.Result)
		}
	default:
		t.Fatal("expected terminalSignal on registered channel, channel was empty")
	}
}

// TestRequestTerminalSignal_UnregisteredSessionIsNoOp verifies that calling
// RequestTerminalSignal for a session not in the registry is a silent no-op.
func TestRequestTerminalSignal_UnregisteredSessionIsNoOp(t *testing.T) {
	sp := New(Config{Clock: clock.Real()})
	// No registerTerminalSignal call — session is unknown.
	sp.RequestTerminalSignal("unknown-session", "fail") // must not panic / block
}

// TestRequestTerminalSignal_UnregisterStopsRouting verifies that after
// unregisterTerminalSignal, subsequent sends for that session are dropped.
func TestRequestTerminalSignal_UnregisterStopsRouting(t *testing.T) {
	sp := New(Config{Clock: clock.Real()})
	ch := make(chan terminalSignal, 1)
	sp.registerTerminalSignal("sess-x", ch)
	sp.unregisterTerminalSignal("sess-x")

	sp.RequestTerminalSignal("sess-x", "fail")

	select {
	case sig := <-ch:
		t.Errorf("expected no signal after unregister, got {%q, %q}", sig.SessionID, sig.Result)
	default:
	}
}

// TestRequestTerminalSignal_RoutesPerSession verifies that signals for
// different session IDs land on their own channels. This is the property
// that prevents one monitorAll from stealing another's signal.
func TestRequestTerminalSignal_RoutesPerSession(t *testing.T) {
	sp := New(Config{Clock: clock.Real()})
	chA := make(chan terminalSignal, 1)
	chB := make(chan terminalSignal, 1)
	sp.registerTerminalSignal("sess-A", chA)
	sp.registerTerminalSignal("sess-B", chB)

	sp.RequestTerminalSignal("sess-A", "fail")
	sp.RequestTerminalSignal("sess-B", "continue")

	select {
	case sig := <-chA:
		if sig.SessionID != "sess-A" || sig.Result != "fail" {
			t.Errorf("chA: expected {sess-A, fail}, got {%q, %q}", sig.SessionID, sig.Result)
		}
	default:
		t.Fatal("chA empty: signal for sess-A not routed")
	}
	select {
	case sig := <-chB:
		if sig.SessionID != "sess-B" || sig.Result != "continue" {
			t.Errorf("chB: expected {sess-B, continue}, got {%q, %q}", sig.SessionID, sig.Result)
		}
	default:
		t.Fatal("chB empty: signal for sess-B not routed")
	}
}

// TestRequestTerminalSignal_NonBlockingWhenFull verifies that when the
// destination channel is full, a second send for the same session is
// silently dropped without blocking.
func TestRequestTerminalSignal_NonBlockingWhenFull(t *testing.T) {
	sp := New(Config{Clock: clock.Real()})
	ch := make(chan terminalSignal, 1)
	sp.registerTerminalSignal("sess-full", ch)

	sp.RequestTerminalSignal("sess-full", "fail")
	// Channel is now full (capacity 1). Second call must not block or panic.
	sp.RequestTerminalSignal("sess-full", "continue")

	sig := <-ch
	if sig.SessionID != "sess-full" || sig.Result != "fail" {
		t.Errorf("expected first signal preserved {sess-full, fail}, got {%q, %q}", sig.SessionID, sig.Result)
	}
	select {
	case extra := <-ch:
		t.Errorf("expected channel empty after drop, got {%q, %q}", extra.SessionID, extra.Result)
	default:
	}
}

// TestRequestTerminalSignal_ResultValues verifies that each result string
// (fail, continue, callback) is correctly carried through the registry path.
func TestRequestTerminalSignal_ResultValues(t *testing.T) {
	cases := []struct {
		sessionID string
		result    string
	}{
		{"sess-fail", "fail"},
		{"sess-continue", "continue"},
		{"sess-callback", "callback"},
	}
	for _, tc := range cases {
		t.Run(tc.result, func(t *testing.T) {
			sp := New(Config{Clock: clock.Real()})
			ch := make(chan terminalSignal, 1)
			sp.registerTerminalSignal(tc.sessionID, ch)

			sp.RequestTerminalSignal(tc.sessionID, tc.result)

			select {
			case sig := <-ch:
				if sig.SessionID != tc.sessionID {
					t.Errorf("expected SessionID=%q, got %q", tc.sessionID, sig.SessionID)
				}
				if sig.Result != tc.result {
					t.Errorf("expected Result=%q, got %q", tc.result, sig.Result)
				}
			default:
				t.Fatalf("expected terminalSignal for result=%q, registered channel was empty", tc.result)
			}
		})
	}
}
