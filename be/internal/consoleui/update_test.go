package consoleui

import (
	"encoding/json"
	"strings"
	"testing"

	"charm.land/bubbles/v2/spinner"
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

// TestApplyStream_SessionCostUpdated verifies a session.cost_updated event
// sets m.detail.CostEstimate from the event's cost_estimate field.
func TestApplyStream_SessionCostUpdated(t *testing.T) {
	m := &model{detail: ChatDetail{SessionID: "s1"}, deltas: map[string]string{}}
	m.applyStream(streamUpdate{Events: []Event{
		event("session.cost_updated", "s1", map[string]any{"cost_estimate": 1.5, "pricing_known": true}),
	}})
	if m.detail.CostEstimate == nil || *m.detail.CostEstimate != 1.5 {
		t.Fatalf("CostEstimate = %v, want 1.5", m.detail.CostEstimate)
	}
}

// TestWorkingIndicator: a running turn renders an animated "working…" line
// in the live region, and the tick chain starts exactly on the
// idle→running transition.
func TestWorkingIndicator(t *testing.T) {
	m := &model{
		detail: ChatDetail{SessionID: "s1"}, deltas: map[string]string{},
		spin: spinner.New(spinner.WithSpinner(spinner.MiniDot)),
	}
	m.width, m.height, m.ready = 80, 24, true
	m.applyStream(streamUpdate{Events: []Event{
		event("console_chat.turn", "s1", map[string]any{"state": "running"}),
	}})
	if !strings.Contains(m.liveRegionView(), "working…") {
		t.Fatalf("running live region = %q, want working indicator", m.liveRegionView())
	}
	if m.tickOnRunning(false) == nil {
		t.Fatal("idle→running must start the spinner tick chain")
	}
	if m.tickOnRunning(true) != nil {
		t.Fatal("already-running must not fork a second tick chain")
	}
	m.applyStream(streamUpdate{Events: []Event{
		event("console_chat.turn", "s1", map[string]any{"state": "idle"}),
	}})
	if strings.Contains(m.liveRegionView(), "working…") {
		t.Fatal("idle live region must drop the working indicator")
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

// TestApplySync_PreservesRecoveredLiveStateAfterHistoryReplacement verifies
// applySync re-seeds live deltas/thinking from the reconnect detail (dropping
// stale entries), and that printNewMessages (called by the syncMsg branch
// before applySync, per update.go) independently advances printedTotal from
// the accompanying page.
func TestApplySync_PreservesRecoveredLiveStateAfterHistoryReplacement(t *testing.T) {
	m := &model{deltas: map[string]string{"stale": "old"}}
	m.width, m.height, m.ready = 80, 24, true

	cmds := m.printNewMessages(MessagePage{Messages: []Message{{Category: "user_input", Content: "hello"}}, Total: 1})
	if len(cmds) != 1 {
		t.Fatalf("printNewMessages returned %d cmds, want 1", len(cmds))
	}
	if m.printedTotal != 1 {
		t.Fatalf("printedTotal = %d, want 1", m.printedTotal)
	}

	m.applySync(ChatDetail{SessionID: "s1", Turn: "running", LiveItems: []LiveItem{{ID: "answer", Text: "partial"}}})
	if m.deltas["answer"] != "partial" || len(m.deltas) != 1 {
		t.Fatalf("deltas=%+v, want only re-seeded answer=partial", m.deltas)
	}
}

func event(eventType, session string, data map[string]any) Event {
	raw := make(map[string]json.RawMessage, len(data))
	for key, value := range data {
		raw[key], _ = json.Marshal(value)
	}
	return Event{Type: eventType, SessionID: session, Data: raw}
}
