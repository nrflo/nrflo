package consoleui

import (
	"encoding/json"
	"fmt"
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
		event("console_chat.session_approvals", "s1", map[string]any{"tools": []string{"bash"}}),
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
	if len(m.detail.SessionApprovals) != 1 || m.detail.SessionApprovals[0] != "bash" {
		t.Fatalf("session approvals = %+v", m.detail.SessionApprovals)
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

func TestApplyDetail_ReconcilesMissedLiveState(t *testing.T) {
	m := &model{
		detail: ChatDetail{SessionID: "s1"}, deltas: map[string]string{"stale": "old"},
		approvals: []Approval{{ID: "old"}}, status: "idle",
	}
	m.applyDetail(ChatDetail{
		SessionID: "s1", Turn: "running",
		PendingApprovals: []Approval{{ID: "current"}},
		LiveItems:        []LiveItem{{ID: "answer", Text: "recovered"}},
		Thinking:         &LiveItem{ID: "thought", Text: "still thinking"},
	})
	if m.status != "running" || m.deltas["answer"] != "recovered" || len(m.deltas) != 1 {
		t.Fatalf("status=%q deltas=%+v", m.status, m.deltas)
	}
	if len(m.approvals) != 1 || m.approvals[0].ID != "current" || m.thinking != "still thinking" {
		t.Fatalf("approvals=%+v thinking=%q", m.approvals, m.thinking)
	}
}

func TestApplySync_PreservesRecoveredLiveStateAfterHistoryReplacement(t *testing.T) {
	m := &model{
		deltas: map[string]string{"stale": "old"}, renderCache: map[string]string{},
	}
	m.applySync(
		ChatDetail{SessionID: "s1", Turn: "running", LiveItems: []LiveItem{{ID: "answer", Text: "partial"}}},
		MessagePage{Messages: []Message{{Category: "user_input", Content: "hello"}}, Total: 1},
	)
	if len(m.messages) != 1 || m.deltas["answer"] != "partial" || len(m.deltas) != 1 {
		t.Fatalf("messages=%+v deltas=%+v", m.messages, m.deltas)
	}
}

func TestApplyHistory_BoundsTranscriptWindow(t *testing.T) {
	messages := make([]Message, historyWindowSize+50)
	for i := range messages {
		messages[i] = Message{Category: "text", Content: fmt.Sprintf("message-%d", i)}
	}
	m := &model{renderCache: map[string]string{}}
	m.applyHistory(MessagePage{Messages: messages, Total: len(messages)}, true, 0)
	if len(m.messages) != historyWindowSize {
		t.Fatalf("message window = %d, want %d", len(m.messages), historyWindowSize)
	}
}

func event(eventType, session string, data map[string]any) Event {
	raw := make(map[string]json.RawMessage, len(data))
	for key, value := range data {
		raw[key], _ = json.Marshal(value)
	}
	return Event{Type: eventType, SessionID: session, Data: raw}
}
