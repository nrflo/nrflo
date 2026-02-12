package spawner

import (
	"testing"
)

func TestRequestRestart_SendsSessionID(t *testing.T) {
	sp := New(Config{})

	sp.RequestRestart("session-123")

	select {
	case got := <-sp.restartCh:
		if got != "session-123" {
			t.Fatalf("expected session-123, got %q", got)
		}
	default:
		t.Fatal("restartCh should have a pending value")
	}
}

func TestRequestRestart_NonBlocking(t *testing.T) {
	sp := New(Config{})

	// Fill the channel (capacity 1)
	sp.RequestRestart("first")

	// Second call should not block
	sp.RequestRestart("second")

	// Channel should still hold "first" since send was non-blocking
	got := <-sp.restartCh
	if got != "first" {
		t.Fatalf("expected first value 'first', got %q", got)
	}

	// Channel should be empty now
	select {
	case v := <-sp.restartCh:
		t.Fatalf("expected empty channel, got %q", v)
	default:
		// ok
	}
}

func TestRequestRestart_Idempotent(t *testing.T) {
	sp := New(Config{})

	// Multiple rapid calls should not panic or block
	for i := 0; i < 100; i++ {
		sp.RequestRestart("session-abc")
	}

	// Drain whatever is in the channel
	count := 0
	for {
		select {
		case <-sp.restartCh:
			count++
		default:
			goto done
		}
	}
done:
	if count != 1 {
		t.Fatalf("expected exactly 1 pending restart, got %d", count)
	}
}

func TestNewSpawner_RestartChBuffered(t *testing.T) {
	sp := New(Config{})

	if cap(sp.restartCh) != 1 {
		t.Fatalf("expected restartCh capacity 1, got %d", cap(sp.restartCh))
	}
}

func TestRequestRestart_DifferentSessionIDs(t *testing.T) {
	sp := New(Config{})

	// Send first
	sp.RequestRestart("session-A")

	// Consume it
	got := <-sp.restartCh
	if got != "session-A" {
		t.Fatalf("expected session-A, got %q", got)
	}

	// Send second
	sp.RequestRestart("session-B")

	got = <-sp.restartCh
	if got != "session-B" {
		t.Fatalf("expected session-B, got %q", got)
	}
}

func TestSpawnerClose_NoOp(t *testing.T) {
	sp := New(Config{})
	// Close should not panic
	sp.Close()
	sp.Close() // double close is safe
}
