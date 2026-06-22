package ws

import (
	"testing"

	"be/internal/clock"
)

// TestClient_TrySendAfterClose verifies the close-safe send primitive: once a
// client is closed, trySend drops the message (returns false) instead of
// panicking on a closed channel, and closeSend is idempotent.
func TestClient_TrySendAfterClose(t *testing.T) {
	hub := NewHub(clock.Real())
	c, _ := NewTestClient(hub, "t1")

	if !c.trySend([]byte("before")) {
		t.Fatal("trySend before close = false, want true")
	}
	c.closeSend()
	if c.trySend([]byte("after")) {
		t.Error("trySend after close = true, want false (must drop, not panic)")
	}
	c.closeSend() // idempotent — must not double-close panic
	if c.trySend([]byte("after2")) {
		t.Error("trySend after double close = true, want false")
	}
}

// TestStreamSnapshot_AfterClose_NoPanic reproduces the server crash surfaced by
// the pre-release run: a detached replay/snapshot goroutine streaming to a
// client whose send channel was already closed (client unregistered mid-stream)
// used to panic with "send on closed channel" and take down the whole process
// (ws/snapshot.go -> ws/replay.go:sendControlEvent). The send sites now route
// through trySend, so the goroutine drops its writes instead of crashing.
func TestStreamSnapshot_AfterClose_NoPanic(t *testing.T) {
	hub := NewHub(clock.Real())
	hub.SetSnapshotProvider(&mockSnapshotProvider{
		chunks: []SnapshotChunk{
			{Entity: EntityWorkflowState, Data: map[string]interface{}{"status": "active"}},
		},
	})
	c, _ := NewTestClient(hub, "t1")
	c.closeSend() // client unregistered before the detached goroutine runs

	// Neither of these may panic on the closed send channel.
	streamSnapshot(c, "proj", "ticket", hub)
	sendControlEvent(c, EventResyncRequired, "proj", "ticket", nil)
}
