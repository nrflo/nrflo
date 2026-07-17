package console

import (
	"testing"

	"be/internal/ws"
)

func TestWaitBroker_WakesMatchingProject(t *testing.T) {
	b := NewWaitBroker()
	wake, cancel := b.Subscribe("Proj-A")
	defer cancel()

	b.OnEvent(&ws.Event{ProjectID: "proj-a", Type: ws.EventWorkflowUpdated})

	select {
	case <-wake:
	default:
		t.Fatal("waiter not woken by matching-project event (case-insensitive)")
	}
}

func TestWaitBroker_IgnoresOtherProjectAndNilEvents(t *testing.T) {
	b := NewWaitBroker()
	wake, cancel := b.Subscribe("proj-a")
	defer cancel()

	b.OnEvent(&ws.Event{ProjectID: "proj-b", Type: ws.EventWorkflowUpdated})
	b.OnEvent(&ws.Event{Type: ws.EventWorkflowUpdated}) // no project id
	b.OnEvent(nil)

	select {
	case <-wake:
		t.Fatal("waiter woken by unrelated event")
	default:
	}
}

func TestWaitBroker_NonBlockingWhenWakePending(t *testing.T) {
	b := NewWaitBroker()
	wake, cancel := b.Subscribe("proj-a")
	defer cancel()

	// Two events with no consumer must not block OnEvent.
	b.OnEvent(&ws.Event{ProjectID: "proj-a"})
	b.OnEvent(&ws.Event{ProjectID: "proj-a"})

	select {
	case <-wake:
	default:
		t.Fatal("expected one buffered wake")
	}
}

func TestWaitBroker_CancelRemovesWaiter(t *testing.T) {
	b := NewWaitBroker()
	_, cancel := b.Subscribe("proj-a")
	if got := b.WaiterCount("proj-a"); got != 1 {
		t.Fatalf("WaiterCount = %d, want 1", got)
	}
	cancel()
	if got := b.WaiterCount("proj-a"); got != 0 {
		t.Fatalf("WaiterCount after cancel = %d, want 0", got)
	}
	// Waking after cancel must be a no-op, not a panic.
	b.OnEvent(&ws.Event{ProjectID: "proj-a"})
}
