package integration

import (
	"encoding/json"
	"testing"
	"time"

	"be/internal/ws"
)

// expectEvent waits for a WS event of the given type on the channel.
func expectEvent(t *testing.T, ch chan []byte, eventType string, timeout time.Duration) ws.Event {
	t.Helper()
	for {
		select {
		case msg := <-ch:
			var event ws.Event
			if err := json.Unmarshal(msg, &event); err != nil {
				t.Fatalf("failed to unmarshal event: %v", err)
			}
			if event.Type == eventType {
				return event
			}
			// Skip non-matching events (e.g. ack messages)
		case <-time.After(timeout):
			t.Fatalf("timeout waiting for event type '%s'", eventType)
			return ws.Event{}
		}
	}
}

// expectNoEvent asserts no event arrives within the timeout.
func expectNoEvent(t *testing.T, ch chan []byte, timeout time.Duration) {
	t.Helper()
	select {
	case msg := <-ch:
		t.Fatalf("expected no event, got: %s", string(msg))
	case <-time.After(timeout):
		// Good, no event
	}
}

// drainChannel drains any pending messages from the channel.
func drainChannel(ch chan []byte) {
	for {
		select {
		case <-ch:
		default:
			return
		}
	}
}

func TestWSFindingsEvents(t *testing.T) {
	env := NewTestEnv(t)

	env.CreateTicket(t, "WS-F1", "WS findings")
	env.InitWorkflow(t, "WS-F1")

	wfiID := env.GetWorkflowInstanceID(t, "WS-F1", "test")
	env.InsertAgentSession(t, "sess-findings", "WS-F1", wfiID, "analyzer", "analyzer", "")

	_, ch := env.NewWSClient(t, "ws-findings", "WS-F1")

	// Add finding
	env.MustExecute(t, "findings.add", map[string]interface{}{
		"session_id":  "sess-findings",
		"instance_id": wfiID,
		"key":         "data",
		"value":       `"hello"`,
	}, nil)
	event := expectEvent(t, ch, ws.EventFindingsUpdated, 2*time.Second)
	if event.Data["action"] != "add" {
		t.Fatalf("expected action 'add', got %v", event.Data["action"])
	}

	// Append
	env.MustExecute(t, "findings.append", map[string]interface{}{
		"session_id":  "sess-findings",
		"instance_id": wfiID,
		"key":         "data",
		"value":       `"world"`,
	}, nil)
	event = expectEvent(t, ch, ws.EventFindingsUpdated, 2*time.Second)
	if event.Data["action"] != "append" {
		t.Fatalf("expected action 'append', got %v", event.Data["action"])
	}

	// Delete
	env.MustExecute(t, "findings.delete", map[string]interface{}{
		"session_id":  "sess-findings",
		"instance_id": wfiID,
		"keys":        []string{"data"},
	}, nil)
	event = expectEvent(t, ch, ws.EventFindingsUpdated, 2*time.Second)
	if event.Data["action"] != "delete" {
		t.Fatalf("expected action 'delete', got %v", event.Data["action"])
	}
}

func TestWSAgentEvents(t *testing.T) {
	env := NewTestEnv(t)

	env.CreateTicket(t, "WS-A1", "WS agent events")
	env.InitWorkflow(t, "WS-A1")

	wfiID := env.GetWorkflowInstanceID(t, "WS-A1", "test")
	env.InsertAgentSession(t, "sess-ws-agent", "WS-A1", wfiID, "analyzer", "analyzer", "")

	_, ch := env.NewWSClient(t, "ws-agent", "WS-A1")

	// Fail agent -> should broadcast agent.completed
	env.MustExecute(t, "agent.fail", map[string]interface{}{
		"ticket_id":   "WS-A1",
		"workflow":    "test",
		"agent_type":  "analyzer",
		"session_id":  "sess-ws-agent",
		"instance_id": wfiID,
	}, nil)
	event := expectEvent(t, ch, ws.EventAgentCompleted, 2*time.Second)
	if event.Data["agent_type"] != "analyzer" {
		t.Fatalf("expected agent_type 'analyzer', got %v", event.Data["agent_type"])
	}
}

func TestWSSubscriptionFiltering(t *testing.T) {
	env := NewTestEnv(t)

	// Create 2 tickets
	env.CreateTicket(t, "WS-S1", "Ticket 1")
	env.CreateTicket(t, "WS-S2", "Ticket 2")
	env.InitWorkflow(t, "WS-S1")
	env.InitWorkflow(t, "WS-S2")

	wfi1 := env.GetWorkflowInstanceID(t, "WS-S1", "test")
	wfi2 := env.GetWorkflowInstanceID(t, "WS-S2", "test")
	env.InsertAgentSession(t, "sess-filt-1", "WS-S1", wfi1, "analyzer", "analyzer", "")
	env.InsertAgentSession(t, "sess-filt-2", "WS-S2", wfi2, "analyzer", "analyzer", "")

	// Subscribe client1 to ticket1, client2 to ticket2
	_, ch1 := env.NewWSClient(t, "ws-filter-1", "WS-S1")
	_, ch2 := env.NewWSClient(t, "ws-filter-2", "WS-S2")

	// Add findings on ticket1 (session from ticket1 → broadcasts to ticket1 subscribers)
	env.MustExecute(t, "findings.add", map[string]interface{}{
		"session_id":  "sess-filt-1",
		"instance_id": wfi1,
		"key":         "x",
		"value":       `"1"`,
	}, nil)

	// Client1 should get event
	expectEvent(t, ch1, ws.EventFindingsUpdated, 2*time.Second)

	// Client2 should NOT get event
	expectNoEvent(t, ch2, 200*time.Millisecond)

	// Add findings on ticket2 (session from ticket2 → broadcasts to ticket2 subscribers)
	env.MustExecute(t, "findings.add", map[string]interface{}{
		"session_id":  "sess-filt-2",
		"instance_id": wfi2,
		"key":         "x",
		"value":       `"2"`,
	}, nil)

	// Client2 should get event
	expectEvent(t, ch2, ws.EventFindingsUpdated, 2*time.Second)

	// Client1 should NOT get this event
	expectNoEvent(t, ch1, 200*time.Millisecond)
}

func TestWSBroadcastViaSocketMethod(t *testing.T) {
	env := NewTestEnv(t)

	env.CreateTicket(t, "WS-B1", "WS broadcast")
	_, ch := env.NewWSClient(t, "ws-broadcast", "WS-B1")

	// Call ws.broadcast directly (this is what the spawner uses)
	env.MustExecute(t, "ws.broadcast", map[string]interface{}{
		"type":       ws.EventAgentStarted,
		"project_id": env.ProjectID,
		"ticket_id":  "WS-B1",
		"workflow":   "test",
		"data": map[string]interface{}{
			"agent_type": "analyzer",
			"session_id": "test-sess",
		},
	}, nil)

	event := expectEvent(t, ch, ws.EventAgentStarted, 2*time.Second)
	if event.Data["agent_type"] != "analyzer" {
		t.Fatalf("expected agent_type 'analyzer', got %v", event.Data["agent_type"])
	}
	if event.Data["session_id"] != "test-sess" {
		t.Fatalf("expected session_id 'test-sess', got %v", event.Data["session_id"])
	}
}
