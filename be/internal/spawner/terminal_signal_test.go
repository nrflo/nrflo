package spawner

import (
	"testing"

	"be/internal/clock"
)

// TestRequestTerminalSignal_SendsToChannel verifies that RequestTerminalSignal
// puts a terminalSignal onto terminalSignalCh with the correct SessionID and Result.
func TestRequestTerminalSignal_SendsToChannel(t *testing.T) {
	sp := New(Config{Clock: clock.Real()})

	sp.RequestTerminalSignal("sess-abc", "fail")

	select {
	case sig := <-sp.terminalSignalCh:
		if sig.SessionID != "sess-abc" {
			t.Errorf("expected SessionID='sess-abc', got %q", sig.SessionID)
		}
		if sig.Result != "fail" {
			t.Errorf("expected Result='fail', got %q", sig.Result)
		}
	default:
		t.Fatal("expected terminalSignal on terminalSignalCh, channel was empty")
	}
}

// TestRequestTerminalSignal_NonBlockingWhenFull verifies that calling
// RequestTerminalSignal a second time when the channel is full is a no-op
// (no panic, no block). The first signal is preserved.
func TestRequestTerminalSignal_NonBlockingWhenFull(t *testing.T) {
	sp := New(Config{Clock: clock.Real()})

	sp.RequestTerminalSignal("first", "fail")
	// Channel is now full (capacity 1). Second call must not block or panic.
	sp.RequestTerminalSignal("second", "continue")

	select {
	case sig := <-sp.terminalSignalCh:
		if sig.SessionID != "first" {
			t.Errorf("expected SessionID='first', got %q", sig.SessionID)
		}
		if sig.Result != "fail" {
			t.Errorf("expected Result='fail', got %q", sig.Result)
		}
	default:
		t.Fatal("expected 'first' signal on terminalSignalCh")
	}

	// Channel should now be empty — 'second' was dropped.
	select {
	case sig := <-sp.terminalSignalCh:
		t.Errorf("expected channel empty after drop, got {%q, %q}", sig.SessionID, sig.Result)
	default:
	}
}

// TestRequestTerminalSignal_ResultValues verifies that each result string
// (fail, continue, callback) is correctly carried through the channel.
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
			sp.RequestTerminalSignal(tc.sessionID, tc.result)

			select {
			case sig := <-sp.terminalSignalCh:
				if sig.SessionID != tc.sessionID {
					t.Errorf("expected SessionID=%q, got %q", tc.sessionID, sig.SessionID)
				}
				if sig.Result != tc.result {
					t.Errorf("expected Result=%q, got %q", tc.result, sig.Result)
				}
			default:
				t.Fatalf("expected terminalSignal for result=%q, channel was empty", tc.result)
			}
		})
	}
}

// TestRequestTerminalSignal_ChannelCapacityOne verifies that terminalSignalCh
// has exactly capacity 1, so at most one pending signal is buffered.
func TestRequestTerminalSignal_ChannelCapacityOne(t *testing.T) {
	sp := New(Config{Clock: clock.Real()})
	if got := cap(sp.terminalSignalCh); got != 1 {
		t.Errorf("expected channel capacity 1, got %d", got)
	}
}
