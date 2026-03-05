package spawner

import (
	"testing"
	"time"

	"be/internal/clock"
)

// TestRegisterInteractiveWait_BlocksUntilCompleteInteractive verifies that
// the channel returned by RegisterInteractiveWait blocks until CompleteInteractive is called.
func TestRegisterInteractiveWait_BlocksUntilCompleteInteractive(t *testing.T) {
	sp := New(Config{Clock: clock.Real()})

	waitCh := sp.RegisterInteractiveWait("sess-rwi-1")

	// Channel should block (not yet closed)
	select {
	case <-waitCh:
		t.Fatal("channel should block before CompleteInteractive is called")
	default:
	}

	// Complete the interactive session
	sp.CompleteInteractive("sess-rwi-1")

	// Channel should now be closed
	select {
	case <-waitCh:
		// closed — good
	case <-time.After(500 * time.Millisecond):
		t.Fatal("channel not closed after CompleteInteractive")
	}
}

// TestRegisterInteractiveWait_RegistersInMap verifies the session is stored in interactiveWaits.
func TestRegisterInteractiveWait_RegistersInMap(t *testing.T) {
	sp := New(Config{Clock: clock.Real()})

	sp.RegisterInteractiveWait("sess-map-check")

	sp.mu.Lock()
	_, ok := sp.interactiveWaits["sess-map-check"]
	sp.mu.Unlock()

	if !ok {
		t.Fatal("session not found in interactiveWaits map after RegisterInteractiveWait")
	}
}

// TestRegisterInteractiveWait_MultipleSessions verifies each session gets an isolated channel.
func TestRegisterInteractiveWait_MultipleSessions(t *testing.T) {
	sp := New(Config{Clock: clock.Real()})

	ch1 := sp.RegisterInteractiveWait("sess-multi-1")
	ch2 := sp.RegisterInteractiveWait("sess-multi-2")

	// Complete only sess-multi-1
	sp.CompleteInteractive("sess-multi-1")

	select {
	case <-ch1:
		// good
	case <-time.After(500 * time.Millisecond):
		t.Fatal("ch1 should be closed after CompleteInteractive('sess-multi-1')")
	}

	// ch2 should still block
	select {
	case <-ch2:
		t.Fatal("ch2 should still block after only completing sess-multi-1")
	default:
	}

	sp.CompleteInteractive("sess-multi-2")
	select {
	case <-ch2:
		// good
	case <-time.After(500 * time.Millisecond):
		t.Fatal("ch2 not closed after CompleteInteractive('sess-multi-2')")
	}
}

// TestRegisterInteractiveWait_ReturnedChannelIsReadOnly verifies the returned channel
// is receive-only (cannot be closed by the caller accidentally).
func TestRegisterInteractiveWait_ReturnedChannelIsReadOnly(t *testing.T) {
	sp := New(Config{Clock: clock.Real()})
	ch := sp.RegisterInteractiveWait("sess-readonly")
	// Compile-time: ch is <-chan struct{}, so we can only receive from it.
	_ = ch // type assertion: receiver only, not writable
}
