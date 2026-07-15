package consoleui

import (
	"encoding/json"
	"testing"
)

func TestApplyStream_AccumulatesProviderAgnosticState(t *testing.T) {
	m := &model{detail: ChatDetail{SessionID: "s1"}, deltas: map[string]string{}}
	m.applyStream(streamUpdate{Events: []Event{
		event("console_chat.delta", "s1", map[string]any{"item_id": "answer", "text": "hel"}),
		event("console_chat.delta", "s1", map[string]any{"item_id": "answer", "text": "lo"}),
		event("console_chat.approval_request", "s1", map[string]any{"approval_id": "a1", "command": "make test"}),
		event("agent.context_updated", "s1", map[string]any{"context_left": 73}),
		event("console_chat.turn", "s1", map[string]any{"state": "running"}),
	}})
	if m.deltas["answer"] != "hello" || len(m.deltaOrder) != 1 {
		t.Fatalf("deltas = %+v order=%+v", m.deltas, m.deltaOrder)
	}
	if len(m.approvals) != 1 || m.approvals[0].ID != "a1" {
		t.Fatalf("approvals = %+v", m.approvals)
	}
	if m.detail.ContextLeft == nil || *m.detail.ContextLeft != 73 || m.status != "running" {
		t.Fatalf("context=%v status=%q", m.detail.ContextLeft, m.status)
	}
}

func TestApplyStream_IgnoresOtherSession(t *testing.T) {
	m := &model{detail: ChatDetail{SessionID: "mine"}, deltas: map[string]string{}}
	m.applyStream(streamUpdate{Events: []Event{
		event("console_chat.delta", "other", map[string]any{"item_id": "answer", "text": "wrong"}),
	}})
	if len(m.deltas) != 0 {
		t.Fatalf("accepted foreign-session event: %+v", m.deltas)
	}
}

func event(eventType, session string, data map[string]any) Event {
	raw := make(map[string]json.RawMessage, len(data))
	for key, value := range data {
		raw[key], _ = json.Marshal(value)
	}
	return Event{Type: eventType, SessionID: session, Data: raw}
}
