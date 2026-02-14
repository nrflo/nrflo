package ws

import (
	"testing"
)

func TestCheckBackpressureUnderThreshold(t *testing.T) {
	hub := NewHub()
	client := newTestClient(hub, "test-1")

	// Empty buffer
	if checkBackpressure(client) {
		t.Fatal("expected false for empty buffer")
	}

	// Fill to 50% (below 75% threshold)
	for i := 0; i < clientBufferSize/2; i++ {
		select {
		case client.send <- []byte("test"):
		default:
			t.Fatalf("failed to fill buffer at %d", i)
		}
	}

	if checkBackpressure(client) {
		t.Fatal("expected false for 50% full buffer")
	}
}

func TestCheckBackpressureAboveThreshold(t *testing.T) {
	hub := NewHub()
	client := newTestClient(hub, "test-1")

	// Fill to 75% (at threshold)
	threshold := clientBufferSize * clientBufferWarningPct / 100
	for i := 0; i < threshold; i++ {
		select {
		case client.send <- []byte("test"):
		default:
			t.Fatalf("failed to fill buffer at %d", i)
		}
	}

	if !checkBackpressure(client) {
		t.Fatal("expected true for 75% full buffer")
	}
}

func TestCheckBackpressureNearFull(t *testing.T) {
	hub := NewHub()
	client := newTestClient(hub, "test-1")

	// Fill to 90%
	fillSize := (clientBufferSize * 90) / 100
	for i := 0; i < fillSize; i++ {
		select {
		case client.send <- []byte("test"):
		default:
			t.Fatalf("failed to fill buffer at %d", i)
		}
	}

	if !checkBackpressure(client) {
		t.Fatal("expected true for 90% full buffer")
	}
}

func TestCheckBackpressureFull(t *testing.T) {
	hub := NewHub()
	client := newTestClient(hub, "test-1")

	// Fill completely
	for i := 0; i < clientBufferSize; i++ {
		select {
		case client.send <- []byte("test"):
		default:
			t.Fatalf("failed to fill buffer at %d", i)
		}
	}

	if !checkBackpressure(client) {
		t.Fatal("expected true for full buffer")
	}
}

func TestCheckBackpressureBoundary(t *testing.T) {
	hub := NewHub()
	client := newTestClient(hub, "test-1")

	// Fill to exactly threshold - 1 (should be false)
	threshold := clientBufferSize * clientBufferWarningPct / 100
	for i := 0; i < threshold-1; i++ {
		select {
		case client.send <- []byte("test"):
		default:
			t.Fatalf("failed to fill buffer at %d", i)
		}
	}

	if checkBackpressure(client) {
		t.Fatalf("expected false for %d items (below threshold)", threshold-1)
	}

	// Add one more to reach threshold
	client.send <- []byte("test")

	if !checkBackpressure(client) {
		t.Fatalf("expected true for %d items (at threshold)", threshold)
	}
}
