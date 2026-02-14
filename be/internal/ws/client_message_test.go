package ws

import (
	"encoding/json"
	"testing"
)

func TestClientMessageSubscribeBasic(t *testing.T) {
	jsonStr := `{"action":"subscribe","project_id":"proj-1","ticket_id":"ticket-1"}`
	var msg ClientMessage
	if err := json.Unmarshal([]byte(jsonStr), &msg); err != nil {
		t.Fatalf("failed to unmarshal: %v", err)
	}

	if msg.Action != "subscribe" {
		t.Fatalf("expected action=subscribe, got %s", msg.Action)
	}
	if msg.ProjectID != "proj-1" {
		t.Fatalf("expected project_id=proj-1, got %s", msg.ProjectID)
	}
	if msg.TicketID != "ticket-1" {
		t.Fatalf("expected ticket_id=ticket-1, got %s", msg.TicketID)
	}
	if msg.SinceSeq != nil {
		t.Fatalf("expected nil since_seq, got %v", *msg.SinceSeq)
	}
}

func TestClientMessageSubscribeProjectWide(t *testing.T) {
	jsonStr := `{"action":"subscribe","project_id":"proj-1","ticket_id":""}`
	var msg ClientMessage
	if err := json.Unmarshal([]byte(jsonStr), &msg); err != nil {
		t.Fatalf("failed to unmarshal: %v", err)
	}

	if msg.Action != "subscribe" {
		t.Fatalf("expected action=subscribe, got %s", msg.Action)
	}
	if msg.ProjectID != "proj-1" {
		t.Fatalf("expected project_id=proj-1, got %s", msg.ProjectID)
	}
	if msg.TicketID != "" {
		t.Fatalf("expected empty ticket_id, got %s", msg.TicketID)
	}
}

func TestClientMessageSubscribeWithCursor(t *testing.T) {
	jsonStr := `{"action":"subscribe","project_id":"proj-1","ticket_id":"ticket-1","since_seq":42}`
	var msg ClientMessage
	if err := json.Unmarshal([]byte(jsonStr), &msg); err != nil {
		t.Fatalf("failed to unmarshal: %v", err)
	}

	if msg.Action != "subscribe" {
		t.Fatalf("expected action=subscribe, got %s", msg.Action)
	}
	if msg.SinceSeq == nil {
		t.Fatal("expected non-nil since_seq")
	}
	if *msg.SinceSeq != 42 {
		t.Fatalf("expected since_seq=42, got %d", *msg.SinceSeq)
	}
}

func TestClientMessageSubscribeWithCursorZero(t *testing.T) {
	jsonStr := `{"action":"subscribe","project_id":"proj-1","ticket_id":"ticket-1","since_seq":0}`
	var msg ClientMessage
	if err := json.Unmarshal([]byte(jsonStr), &msg); err != nil {
		t.Fatalf("failed to unmarshal: %v", err)
	}

	if msg.SinceSeq == nil {
		t.Fatal("expected non-nil since_seq")
	}
	if *msg.SinceSeq != 0 {
		t.Fatalf("expected since_seq=0, got %d", *msg.SinceSeq)
	}
}

func TestClientMessageSubscribeNoCursor(t *testing.T) {
	jsonStr := `{"action":"subscribe","project_id":"proj-1","ticket_id":"ticket-1"}`
	var msg ClientMessage
	if err := json.Unmarshal([]byte(jsonStr), &msg); err != nil {
		t.Fatalf("failed to unmarshal: %v", err)
	}

	if msg.SinceSeq != nil {
		t.Fatalf("expected nil since_seq when not provided, got %v", *msg.SinceSeq)
	}
}

func TestClientMessageUnsubscribe(t *testing.T) {
	jsonStr := `{"action":"unsubscribe","project_id":"proj-1","ticket_id":"ticket-1"}`
	var msg ClientMessage
	if err := json.Unmarshal([]byte(jsonStr), &msg); err != nil {
		t.Fatalf("failed to unmarshal: %v", err)
	}

	if msg.Action != "unsubscribe" {
		t.Fatalf("expected action=unsubscribe, got %s", msg.Action)
	}
	if msg.ProjectID != "proj-1" {
		t.Fatalf("expected project_id=proj-1, got %s", msg.ProjectID)
	}
	if msg.TicketID != "ticket-1" {
		t.Fatalf("expected ticket_id=ticket-1, got %s", msg.TicketID)
	}
}

func TestClientMessageEmptyAction(t *testing.T) {
	jsonStr := `{"project_id":"proj-1","ticket_id":"ticket-1"}`
	var msg ClientMessage
	if err := json.Unmarshal([]byte(jsonStr), &msg); err != nil {
		t.Fatalf("failed to unmarshal: %v", err)
	}

	if msg.Action != "" {
		t.Fatalf("expected empty action, got %s", msg.Action)
	}
}

func TestClientMessageInvalidJSON(t *testing.T) {
	jsonStr := `{invalid json}`
	var msg ClientMessage
	if err := json.Unmarshal([]byte(jsonStr), &msg); err == nil {
		t.Fatal("expected error for invalid JSON")
	}
}

func TestClientMessageNullSinceSeq(t *testing.T) {
	jsonStr := `{"action":"subscribe","project_id":"proj-1","ticket_id":"ticket-1","since_seq":null}`
	var msg ClientMessage
	if err := json.Unmarshal([]byte(jsonStr), &msg); err != nil {
		t.Fatalf("failed to unmarshal: %v", err)
	}

	if msg.SinceSeq != nil {
		t.Fatalf("expected nil since_seq for null value, got %v", *msg.SinceSeq)
	}
}

func TestClientMessageNegativeCursor(t *testing.T) {
	jsonStr := `{"action":"subscribe","project_id":"proj-1","ticket_id":"ticket-1","since_seq":-5}`
	var msg ClientMessage
	if err := json.Unmarshal([]byte(jsonStr), &msg); err != nil {
		t.Fatalf("failed to unmarshal: %v", err)
	}

	if msg.SinceSeq == nil {
		t.Fatal("expected non-nil since_seq")
	}
	if *msg.SinceSeq != -5 {
		t.Fatalf("expected since_seq=-5, got %d", *msg.SinceSeq)
	}
}

func TestClientMessageLargeCursor(t *testing.T) {
	jsonStr := `{"action":"subscribe","project_id":"proj-1","ticket_id":"ticket-1","since_seq":9999999999}`
	var msg ClientMessage
	if err := json.Unmarshal([]byte(jsonStr), &msg); err != nil {
		t.Fatalf("failed to unmarshal: %v", err)
	}

	if msg.SinceSeq == nil {
		t.Fatal("expected non-nil since_seq")
	}
	if *msg.SinceSeq != 9999999999 {
		t.Fatalf("expected since_seq=9999999999, got %d", *msg.SinceSeq)
	}
}
