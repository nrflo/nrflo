package ws

import (
	"encoding/json"
	"testing"
	"time"

	"be/internal/clock"
)

func TestBroadcastGlobal_UnsubscribedClientReceives(t *testing.T) {
	hub := NewHub(clock.Real())
	go hub.Run()
	defer hub.Stop()

	// Register client with NO subscription
	client := newTestClient(hub, "global-1")
	hub.Register(client)
	time.Sleep(50 * time.Millisecond)

	event := NewEvent(EventGlobalRunningAgents, "", "", "", nil)
	hub.BroadcastGlobal(event)

	select {
	case msg := <-client.send:
		var received Event
		if err := json.Unmarshal(msg, &received); err != nil {
			t.Fatalf("failed to unmarshal event: %v", err)
		}
		if received.Type != EventGlobalRunningAgents {
			t.Errorf("received.Type = %q, want %q", received.Type, EventGlobalRunningAgents)
		}
	case <-time.After(time.Second):
		t.Fatal("timeout: unsubscribed client did not receive global event")
	}
}

func TestBroadcastGlobal_AllClientsReceive(t *testing.T) {
	hub := NewHub(clock.Real())
	go hub.Run()
	defer hub.Stop()

	// Register three clients with different subscription scopes
	client1 := newTestClient(hub, "global-client-1")
	client2 := newTestClient(hub, "global-client-2")
	client3 := newTestClient(hub, "global-client-3")

	hub.Register(client1)
	hub.Register(client2)
	hub.Register(client3)
	time.Sleep(50 * time.Millisecond)

	// client1: subscribed to proj-A/ticket-1
	hub.Subscribe(client1, "proj-a", "ticket-1")
	// client2: project-wide sub for proj-B
	hub.Subscribe(client2, "proj-b", "")
	// client3: no subscription at all

	event := NewEvent(EventGlobalRunningAgents, "", "", "", nil)
	hub.BroadcastGlobal(event)

	received := make(chan string, 3)
	for _, c := range []*Client{client1, client2, client3} {
		ch := c.send
		id := c.id
		go func(send <-chan []byte, clientID string) {
			select {
			case <-send:
				received <- clientID
			case <-time.After(time.Second):
				received <- "timeout:" + clientID
			}
		}(ch, id)
	}

	for i := 0; i < 3; i++ {
		result := <-received
		if len(result) > 8 && result[:8] == "timeout:" {
			t.Errorf("client did not receive global event: %s", result[8:])
		}
	}
}

func TestBroadcastGlobal_WrongProjectClientReceives(t *testing.T) {
	hub := NewHub(clock.Real())
	go hub.Run()
	defer hub.Stop()

	// Client subscribed only to proj-X — regular broadcasts for proj-Y would be filtered.
	// BroadcastGlobal must bypass this and still deliver.
	client := newTestClient(hub, "wrong-proj-client")
	hub.Register(client)
	time.Sleep(50 * time.Millisecond)
	hub.Subscribe(client, "proj-x", "ticket-99")

	// First verify regular broadcast for a different project is NOT received
	regularEvent := NewEvent(EventAgentStarted, "proj-y", "ticket-1", "wf", nil)
	hub.Broadcast(regularEvent)

	select {
	case msg := <-client.send:
		t.Fatalf("regular broadcast for wrong project should not be received: %s", string(msg))
	case <-time.After(100 * time.Millisecond):
		// Expected — filtered out
	}

	// Now verify global broadcast IS received
	globalEvent := NewEvent(EventGlobalRunningAgents, "", "", "", nil)
	hub.BroadcastGlobal(globalEvent)

	select {
	case msg := <-client.send:
		var received Event
		if err := json.Unmarshal(msg, &received); err != nil {
			t.Fatalf("failed to unmarshal global event: %v", err)
		}
		if received.Type != EventGlobalRunningAgents {
			t.Errorf("received.Type = %q, want %q", received.Type, EventGlobalRunningAgents)
		}
	case <-time.After(time.Second):
		t.Fatal("timeout: client subscribed to different project did not receive global event")
	}
}

func TestBroadcastGlobal_StampsTimestamp(t *testing.T) {
	fixedTime := time.Date(2025, 6, 15, 10, 0, 0, 0, time.UTC)
	testClock := clock.NewTest(fixedTime)

	hub := NewHub(testClock)
	go hub.Run()
	defer hub.Stop()

	client := newTestClient(hub, "ts-client")
	hub.Register(client)
	time.Sleep(50 * time.Millisecond)

	event := NewEvent(EventGlobalRunningAgents, "", "", "", nil)
	hub.BroadcastGlobal(event)

	select {
	case msg := <-client.send:
		var received Event
		if err := json.Unmarshal(msg, &received); err != nil {
			t.Fatalf("failed to unmarshal event: %v", err)
		}
		if received.Timestamp == "" {
			t.Error("BroadcastGlobal() did not stamp timestamp on event")
		}
		parsed, err := time.Parse(time.RFC3339Nano, received.Timestamp)
		if err != nil {
			t.Fatalf("timestamp not parseable as RFC3339Nano: %q", received.Timestamp)
		}
		if !parsed.Equal(fixedTime) {
			t.Errorf("timestamp = %v, want %v", parsed, fixedTime)
		}
	case <-time.After(time.Second):
		t.Fatal("timeout waiting for global event")
	}
}

func TestBroadcastGlobal_NoClientsNoPanic(t *testing.T) {
	hub := NewHub(clock.Real())
	go hub.Run()
	defer hub.Stop()

	time.Sleep(50 * time.Millisecond)

	// Broadcast with no clients — must not panic
	event := NewEvent(EventGlobalRunningAgents, "", "", "", nil)
	hub.BroadcastGlobal(event)

	time.Sleep(100 * time.Millisecond) // allow hub to process
}
