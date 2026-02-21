package ws

import (
	"encoding/json"
	"testing"
	"time"

	"be/internal/clock"
)

func newTestClient(hub *Hub, id string) *Client {
	return &Client{
		hub:           hub,
		conn:          nil, // nil conn is fine - we only read from send channel
		send:          make(chan []byte, 256),
		id:            id,
		subscriptions: make(map[string]bool),
	}
}

func TestHubBroadcastToSubscriber(t *testing.T) {
	hub := NewHub(clock.Real())
	go hub.Run()
	defer hub.Stop()

	client := newTestClient(hub, "test-1")
	hub.Register(client)

	hub.Subscribe(client, "proj-1", "ticket-1")

	event := NewEvent(EventMessagesUpdated, "proj-1", "ticket-1", "wf", map[string]interface{}{
		"session_id": "s1",
	})
	hub.Broadcast(event)

	select {
	case msg := <-client.send:
		var received Event
		if err := json.Unmarshal(msg, &received); err != nil {
			t.Fatalf("failed to unmarshal event: %v", err)
		}
		if received.Type != EventMessagesUpdated {
			t.Fatalf("expected event type %s, got %s", EventMessagesUpdated, received.Type)
		}
		if received.ProjectID != "proj-1" {
			t.Fatalf("expected project_id proj-1, got %s", received.ProjectID)
		}
		if received.TicketID != "ticket-1" {
			t.Fatalf("expected ticket_id ticket-1, got %s", received.TicketID)
		}
	case <-time.After(time.Second):
		t.Fatal("timeout waiting for broadcast")
	}
}

func TestHubBroadcastNoSubscribers(t *testing.T) {
	hub := NewHub(clock.Real())
	go hub.Run()
	defer hub.Stop()

	// Broadcast with no clients at all - should not panic
	event := NewEvent(EventAgentStarted, "proj-1", "ticket-1", "wf", nil)
	hub.Broadcast(event)

	// Also test with a registered client but no subscriptions
	client := newTestClient(hub, "test-1")
	hub.Register(client)

	event = NewEvent(EventAgentStarted, "proj-1", "ticket-1", "wf", nil)
	hub.Broadcast(event)

	// Client should NOT receive the event (not subscribed)
	select {
	case msg := <-client.send:
		t.Fatalf("should not have received event, got: %s", string(msg))
	case <-time.After(100 * time.Millisecond):
		// Expected - no event received
	}
}

func TestHubBroadcastProjectWide(t *testing.T) {
	hub := NewHub(clock.Real())
	go hub.Run()
	defer hub.Stop()

	client := newTestClient(hub, "test-1")
	hub.Register(client)

	// Subscribe to project-wide (empty ticketID)
	hub.Subscribe(client, "proj-1", "")

	// Broadcast a ticket-specific event
	event := NewEvent(EventAgentCompleted, "proj-1", "ticket-42", "wf", nil)
	hub.Broadcast(event)

	select {
	case msg := <-client.send:
		var received Event
		if err := json.Unmarshal(msg, &received); err != nil {
			t.Fatalf("failed to unmarshal event: %v", err)
		}
		if received.Type != EventAgentCompleted {
			t.Fatalf("expected event type %s, got %s", EventAgentCompleted, received.Type)
		}
		if received.TicketID != "ticket-42" {
			t.Fatalf("expected ticket_id ticket-42, got %s", received.TicketID)
		}
	case <-time.After(time.Second):
		t.Fatal("timeout waiting for project-wide broadcast")
	}
}

func TestHubBroadcastWrongProject(t *testing.T) {
	hub := NewHub(clock.Real())
	go hub.Run()
	defer hub.Stop()

	client := newTestClient(hub, "test-1")
	hub.Register(client)

	// Subscribe to proj-1
	hub.Subscribe(client, "proj-1", "ticket-1")

	// Broadcast event for proj-2
	event := NewEvent(EventPhaseStarted, "proj-2", "ticket-1", "wf", nil)
	hub.Broadcast(event)

	select {
	case msg := <-client.send:
		t.Fatalf("should not have received event for wrong project, got: %s", string(msg))
	case <-time.After(100 * time.Millisecond):
		// Expected - no event received
	}
}

func TestHubUnsubscribe(t *testing.T) {
	hub := NewHub(clock.Real())
	go hub.Run()
	defer hub.Stop()

	client := newTestClient(hub, "test-1")
	hub.Register(client)

	hub.Subscribe(client, "proj-1", "ticket-1")

	// First broadcast should arrive
	event := NewEvent(EventMessagesUpdated, "proj-1", "ticket-1", "wf", nil)
	hub.Broadcast(event)

	select {
	case <-client.send:
		// Good, received
	case <-time.After(time.Second):
		t.Fatal("timeout waiting for first broadcast")
	}

	// Unsubscribe
	hub.Unsubscribe(client, "proj-1", "ticket-1")

	// Second broadcast should NOT arrive
	event = NewEvent(EventMessagesUpdated, "proj-1", "ticket-1", "wf", nil)
	hub.Broadcast(event)

	select {
	case msg := <-client.send:
		t.Fatalf("should not have received event after unsubscribe, got: %s", string(msg))
	case <-time.After(100 * time.Millisecond):
		// Expected - no event received
	}
}
