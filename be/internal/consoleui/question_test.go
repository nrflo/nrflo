package consoleui

import (
	"strings"
	"testing"

	"charm.land/bubbles/v2/textarea"
)

const questionInput = `{"questions":[
	{"question":"How to fix?","header":"Hook fix","options":[
		{"label":"Patch the hook","description":"resolve cd prefix"},
		{"label":"Manual commit","description":"you run it"}]},
	{"question":"Push too?","header":"Push","multiSelect":true,"options":[
		{"label":"push"},{"label":"bump sitemap"}]}]}`

func questionModel(t *testing.T) *model {
	t.Helper()
	m := &model{detail: ChatDetail{SessionID: "s1"}, deltas: map[string]string{}, input: textarea.New()}
	m.applyStream(streamUpdate{Events: []Event{
		event("console_chat.approval_request", "s1", map[string]any{
			"approval_id": "q1", "kind": "PreToolUse", "tool": "AskUserQuestion", "input": questionInput,
		}),
	}})
	if !m.questionActive() {
		t.Fatal("question card must be active after an AskUserQuestion approval_request")
	}
	return m
}

// A digit picks an option (composer empty), advancing through the questions;
// the final answer combines per-question answers and submits decision=answer.
func TestQuestionCard_DigitSelect_AdvancesAndComposes(t *testing.T) {
	m := questionModel(t)

	if cmd, handled := m.handleQuestionKey("2"); !handled || cmd != nil {
		t.Fatalf("digit on question 1/2 = (handled=%v cmd=%v), want handled, no submit yet", handled, cmd)
	}
	if m.qa.idx != 1 || m.qa.answers[0] != "Manual commit" {
		t.Fatalf("state after first answer = idx %d answers %v", m.qa.idx, m.qa.answers)
	}

	// multiSelect: digits toggle, enter confirms and submits.
	if _, handled := m.handleQuestionKey("1"); !handled {
		t.Fatal("toggle not handled")
	}
	if _, handled := m.handleQuestionKey("2"); !handled {
		t.Fatal("toggle not handled")
	}
	cmd, handled := m.handleQuestionKey("enter")
	if !handled || cmd == nil || !m.qa.sent {
		t.Fatalf("enter confirm = (handled=%v cmd=%v sent=%v), want submit", handled, cmd, m.qa.sent)
	}
	got := composeAnswer(m.qa.questions, m.qa.answers)
	want := "Hook fix: Manual commit; Push: push, bump sitemap"
	if got != want {
		t.Errorf("composed answer = %q, want %q", got, want)
	}
}

// Typed text is a free-form answer; digits typed into a non-empty composer
// are composer input, never option picks.
func TestQuestionCard_FreeTextAnswer(t *testing.T) {
	m := questionModel(t)
	m.input.SetValue("option 3 but with changes")

	if _, handled := m.handleQuestionKey("3"); handled {
		t.Fatal("digit with non-empty composer must fall through to the composer")
	}
	cmd, handled := m.handleQuestionKey("enter")
	if !handled || cmd != nil {
		t.Fatalf("enter with text on question 1/2 = (handled=%v cmd=%v), want handled, no submit yet", handled, cmd)
	}
	if m.qa.answers[0] != "option 3 but with changes" || m.input.Value() != "" {
		t.Fatalf("answers = %v input=%q, want free text recorded + composer reset", m.qa.answers, m.input.Value())
	}
}

// Empty enter under the card is swallowed — it must not send an empty chat
// message or submit an empty answer.
func TestQuestionCard_EmptyEnterSwallowed(t *testing.T) {
	m := questionModel(t)
	cmd, handled := m.handleQuestionKey("enter")
	if !handled || cmd != nil || len(m.qa.answers) != 0 {
		t.Fatalf("empty enter = (handled=%v cmd=%v answers=%v), want swallowed no-op", handled, cmd, m.qa.answers)
	}
}

// The resolved event clears the card; an unparseable payload falls back to
// the generic approval card (questionActive false).
func TestQuestionCard_ResolveAndFallback(t *testing.T) {
	m := questionModel(t)
	m.applyStream(streamUpdate{Events: []Event{
		event("console_chat.approval_resolved", "s1", map[string]any{"approval_id": "q1", "decision": "answer"}),
	}})
	if m.questionActive() || len(m.approvals) != 0 {
		t.Fatalf("card must clear on approval_resolved; approvals=%v", m.approvals)
	}

	m.applyStream(streamUpdate{Events: []Event{
		event("console_chat.approval_request", "s1", map[string]any{
			"approval_id": "q2", "tool": "AskUserQuestion", "input": "not json",
		}),
	}})
	if m.questionActive() {
		t.Fatal("unparseable question payload must fall back to the generic approval card")
	}
	if len(m.approvals) != 1 {
		t.Fatalf("approvals = %v, want the raw approval kept", m.approvals)
	}
}

// console_chat.queued updates the footer's queued counter; the view renders
// the question card in place of the approval box.
func TestQueuedCounterAndQuestionView(t *testing.T) {
	m := questionModel(t)
	m.width, m.height, m.ready = 80, 24, true
	m.applyStream(streamUpdate{Events: []Event{
		event("console_chat.queued", "s1", map[string]any{"count": 2, "prompts": []string{"a", "b"}}),
		event("console_chat.turn", "s1", map[string]any{"state": "running"}),
	}})
	if m.queuedCount != 2 {
		t.Fatalf("queuedCount = %d, want 2", m.queuedCount)
	}
	if !strings.Contains(m.footer(), "queued:2") {
		t.Errorf("running footer = %q, want queued:2 marker", m.footer())
	}
	view := m.questionView()
	for _, want := range []string{"Hook fix", "How to fix?", "[1] Patch the hook", "[2] Manual commit"} {
		if !strings.Contains(view, want) {
			t.Errorf("questionView missing %q:\n%s", want, view)
		}
	}
}
