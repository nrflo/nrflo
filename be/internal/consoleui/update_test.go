package consoleui

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"charm.land/bubbles/v2/spinner"
	"charm.land/bubbles/v2/textarea"
	"charm.land/bubbles/v2/viewport"
	tea "charm.land/bubbletea/v2"
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
// at the transcript tail, and the tick chain starts exactly on the
// idle→running transition.
func TestWorkingIndicator(t *testing.T) {
	m := &model{
		detail: ChatDetail{SessionID: "s1"}, deltas: map[string]string{},
		renderCache: map[string]string{}, viewport: viewport.New(),
		spin: spinner.New(spinner.WithSpinner(spinner.MiniDot)),
	}
	m.width, m.height, m.ready = 80, 24, true
	m.applyStream(streamUpdate{Events: []Event{
		event("console_chat.turn", "s1", map[string]any{"state": "running"}),
	}})
	if !strings.Contains(m.viewport.GetContent(), "working…") {
		t.Fatalf("running transcript = %q, want working indicator", m.viewport.GetContent())
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
	if strings.Contains(m.viewport.GetContent(), "working…") {
		t.Fatal("idle transcript must drop the working indicator")
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

// newScrollTestModel builds a ready model with a small viewport filled with
// many lines, scrolled to the bottom, mirroring TestWorkingIndicator's setup.
func newScrollTestModel() *model {
	m := &model{
		detail: ChatDetail{SessionID: "s1"}, deltas: map[string]string{},
		renderCache: map[string]string{}, viewport: viewport.New(),
		input: textarea.New(), spin: spinner.New(spinner.WithSpinner(spinner.MiniDot)),
	}
	m.width, m.height, m.ready = 80, 24, true
	m.viewport.SetWidth(20)
	m.viewport.SetHeight(5)
	lines := make([]string, 100)
	for i := range lines {
		lines[i] = fmt.Sprintf("line-%d", i)
	}
	m.viewport.SetContent(strings.Join(lines, "\n"))
	m.viewport.GotoBottom()
	return m
}

// TestHandleKey_PgUpScrollsViewportKeepsComposer verifies pgup is routed to
// the transcript viewport (scrolling it up from the bottom) while the
// composer input is left untouched and the key is reported as handled.
func TestHandleKey_PgUpScrollsViewportKeepsComposer(t *testing.T) {
	m := newScrollTestModel()
	m.input.SetValue("draft text")
	before := m.input.Value()
	atBottomBefore := m.viewport.AtBottom()
	if !atBottomBefore {
		t.Fatal("viewport should start at bottom")
	}

	cmd, handled := m.handleKey(tea.KeyPressMsg{Code: tea.KeyPgUp})
	_ = cmd
	if !handled {
		t.Fatal("handleKey(pgup) handled = false, want true")
	}
	if m.viewport.AtBottom() {
		t.Error("handleKey(pgup) did not scroll the viewport away from the bottom")
	}
	if m.input.Value() != before {
		t.Errorf("handleKey(pgup) mutated composer input: got %q, want %q", m.input.Value(), before)
	}
}

// TestHandleKey_PgDownScrollsBack verifies pgdown, following a pgup, scrolls
// the viewport back toward the bottom.
func TestHandleKey_PgDownScrollsBack(t *testing.T) {
	m := newScrollTestModel()
	if _, handled := m.handleKey(tea.KeyPressMsg{Code: tea.KeyPgUp}); !handled {
		t.Fatal("setup pgup not handled")
	}
	if m.viewport.AtBottom() {
		t.Fatal("setup pgup did not scroll away from bottom")
	}
	if _, handled := m.handleKey(tea.KeyPressMsg{Code: tea.KeyPgDown}); !handled {
		t.Fatal("handleKey(pgdown) handled = false, want true")
	}
	if !m.viewport.AtBottom() {
		t.Error("handleKey(pgdown) should scroll back to the bottom")
	}
}

// TestHandleKey_PlainRuneNotConsumedByScrollCase verifies a plain rune key
// (e.g. "a") is not swallowed by the new pgup/pgdown case and falls through
// unhandled to composer routing.
func TestHandleKey_PlainRuneNotConsumedByScrollCase(t *testing.T) {
	m := newScrollTestModel()
	_, handled := m.handleKey(tea.KeyPressMsg{Code: 'a', Text: "a"})
	if handled {
		t.Error("handleKey(\"a\") handled = true, want false (plain rune falls through to composer)")
	}
}

func event(eventType, session string, data map[string]any) Event {
	raw := make(map[string]json.RawMessage, len(data))
	for key, value := range data {
		raw[key], _ = json.Marshal(value)
	}
	return Event{Type: eventType, SessionID: session, Data: raw}
}
