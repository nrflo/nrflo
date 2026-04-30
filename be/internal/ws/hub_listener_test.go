package ws

import (
	"testing"
	"time"

	"be/internal/clock"
)

// blockingListener is a Listener whose OnEvent blocks until unblocked.
type blockingListener struct {
	unblock chan struct{}
	count   chan struct{}
}

func (l *blockingListener) OnEvent(event *Event) {
	// Signal that OnEvent was called
	select {
	case l.count <- struct{}{}:
	default:
	}
	// Block until test releases
	<-l.unblock
}

// fastListener counts OnEvent calls without blocking.
type fastListener struct {
	calls chan struct{}
}

func (l *fastListener) OnEvent(_ *Event) {
	select {
	case l.calls <- struct{}{}:
	default:
	}
}

func TestHubListener_BlockingListenerDoesNotBlockBroadcast(t *testing.T) {
	hub := NewHub(clock.Real())

	unblock := make(chan struct{})
	bl := &blockingListener{unblock: unblock, count: make(chan struct{}, 8)}
	hub.RegisterListener(bl)

	go hub.Run()
	defer func() {
		close(unblock) // release any goroutines blocking on the listener
		hub.Stop()
	}()

	// Subscribe a client to receive events
	client, recvCh := NewTestClient(hub, "c-listener-test")
	hub.Subscribe(client, "proj-listener", "")

	// Broadcast event 1 — spawns a goroutine that blocks on bl.OnEvent
	hub.Broadcast(NewEvent(EventAgentStarted, "proj-listener", "", "", nil))

	// Wait briefly for the listener goroutine to enter OnEvent
	select {
	case <-bl.count:
	case <-time.After(2 * time.Second):
		t.Fatal("listener never called for event 1")
	}

	// Broadcast event 2 — hub must not block even though goroutine A is blocked in listener
	hub.Broadcast(NewEvent(EventAgentCompleted, "proj-listener", "", "", map[string]interface{}{"result": "pass"}))

	// Client must receive both events within 100ms
	received := 0
	timeout := time.After(100 * time.Millisecond)
	for received < 2 {
		select {
		case <-recvCh:
			received++
		case <-timeout:
			t.Fatalf("only received %d events within 100ms, want 2 (blocking listener stalled broadcast)", received)
		}
	}
}

func TestHubListener_OnEventCalledForEachBroadcast(t *testing.T) {
	hub := NewHub(clock.Real())

	fl := &fastListener{calls: make(chan struct{}, 16)}
	hub.RegisterListener(fl)

	go hub.Run()
	defer hub.Stop()

	client, _ := NewTestClient(hub, "c-fast-listener")
	hub.Subscribe(client, "proj-fl", "")

	const count = 5
	for i := 0; i < count; i++ {
		hub.Broadcast(NewEvent(EventAgentStarted, "proj-fl", "", "", nil))
	}

	received := 0
	timeout := time.After(500 * time.Millisecond)
	for received < count {
		select {
		case <-fl.calls:
			received++
		case <-timeout:
			t.Fatalf("listener only received %d of %d calls", received, count)
		}
	}
}

func TestHubListener_NoListeners_NoGoroutineSpawned(t *testing.T) {
	// Hub with no listeners — broadcastEvent must not panic
	hub := NewHub(clock.Real())
	go hub.Run()
	defer hub.Stop()

	client, recvCh := NewTestClient(hub, "c-no-listener")
	hub.Subscribe(client, "proj-nl", "")

	hub.Broadcast(NewEvent(EventAgentStarted, "proj-nl", "", "", nil))

	select {
	case <-recvCh:
		// event delivered fine
	case <-time.After(time.Second):
		t.Fatal("timeout waiting for event with no listeners")
	}
}

func TestHubListener_MultipleListeners_AllReceiveEvent(t *testing.T) {
	hub := NewHub(clock.Real())

	fl1 := &fastListener{calls: make(chan struct{}, 4)}
	fl2 := &fastListener{calls: make(chan struct{}, 4)}
	hub.RegisterListener(fl1)
	hub.RegisterListener(fl2)

	go hub.Run()
	defer hub.Stop()

	client, _ := NewTestClient(hub, "c-multi-listener")
	hub.Subscribe(client, "proj-ml", "")

	hub.Broadcast(NewEvent(EventAgentStarted, "proj-ml", "", "", nil))

	for _, fl := range []*fastListener{fl1, fl2} {
		select {
		case <-fl.calls:
		case <-time.After(500 * time.Millisecond):
			t.Errorf("listener did not receive event within 500ms")
		}
	}
}
