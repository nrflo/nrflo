package spawner

import (
	"strings"
	"testing"
	"time"
)

// A question must reach the human as a card even under yolo — an allow would
// open claude's TUI picker inside the hidden PTY, which no console client can
// answer, pinning the turn forever.
func TestClaudeEngine_Question_YoloStillEmitsApprovalRequest(t *testing.T) {
	sink := &testSink{}
	hub := NewConsoleHub()
	e, _ := startTestClaudeEngine(t, sink, hub, EngineSpec{Yolo: true})

	input := map[string]any{"questions": []any{map[string]any{"question": "Pick one?", "header": "Fix", "options": []any{map[string]any{"label": "A"}, map[string]any{"label": "B"}}}}}
	resCh := requestApprovalViaHub(hub, e.spec.SessionID, AskUserQuestionTool, "tu-q-1", input)

	ev := waitForEventType(t, e.Events(), EventApprovalRequest, time.Second)
	if ev.Approval == nil || ev.Approval.Tool != AskUserQuestionTool {
		t.Fatalf("approval request = %+v, want Tool=AskUserQuestion", ev.Approval)
	}
	if !strings.Contains(string(ev.Approval.Raw), "Pick one?") {
		t.Errorf("Approval.Raw must carry the verbatim questions JSON, got %s", ev.Approval.Raw)
	}

	if err := e.AnswerQuestion(ev.Approval.ID, "option B, because reasons"); err != nil {
		t.Fatalf("AnswerQuestion: %v", err)
	}

	select {
	case res := <-resCh:
		if !res.handled || res.decision != "deny" {
			t.Errorf("result = %+v, want handled=true decision=deny (answer rides the deny reason)", res)
		}
		if !strings.Contains(res.reason, "option B, because reasons") {
			t.Errorf("reason %q must carry the user's answer", res.reason)
		}
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for ApproveConsoleTool to return")
	}

	resolved := waitForEventType(t, e.Events(), EventApprovalResolved, time.Second)
	if resolved.Decision != ApprovalAnswer || resolved.Text != "option B, because reasons" {
		t.Errorf("resolved = %+v, want Decision=answer Text=answer", resolved)
	}
}

// An allow-shaped decision on a question (a consumer without a question card)
// must be rewritten to the plain-text redirect deny, never wired as allow.
func TestClaudeEngine_Question_ApproveRewrittenToRedirectDeny(t *testing.T) {
	sink := &testSink{}
	hub := NewConsoleHub()
	e, _ := startTestClaudeEngine(t, sink, hub, EngineSpec{})

	resCh := requestApprovalViaHub(hub, e.spec.SessionID, AskUserQuestionTool, "tu-q-2", map[string]any{"questions": []any{}})
	ev := waitForEventType(t, e.Events(), EventApprovalRequest, time.Second)

	if err := e.ReplyApproval(ev.Approval.ID, ApprovalApproveForSession); err != nil {
		t.Fatalf("ReplyApproval: %v", err)
	}

	select {
	case res := <-resCh:
		if res.decision != "deny" || !strings.Contains(res.reason, "plain text") {
			t.Errorf("result = %+v, want deny with plain-text redirect reason", res)
		}
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for ApproveConsoleTool to return")
	}
	if got := e.SessionApprovals(); len(got) != 0 {
		t.Errorf("approve_for_session on a question must not enter the session allowlist, got %v", got)
	}
}

func TestClaudeEngine_AnswerQuestion_NonQuestionApproval_Errors(t *testing.T) {
	sink := &testSink{}
	hub := NewConsoleHub()
	e, _ := startTestClaudeEngine(t, sink, hub, EngineSpec{})

	resCh := requestApprovalViaHub(hub, e.spec.SessionID, "Bash", "tu-q-3", map[string]any{"command": "ls"})
	ev := waitForEventType(t, e.Events(), EventApprovalRequest, time.Second)

	if err := e.AnswerQuestion(ev.Approval.ID, "not a question"); err == nil {
		t.Error("AnswerQuestion on a Bash approval must error")
	}
	if err := e.AnswerQuestion("no-such-id", "x"); err == nil {
		t.Error("AnswerQuestion on an unknown id must error")
	}

	// The pending approval must still be answerable after the failed calls.
	if err := e.ReplyApproval(ev.Approval.ID, ApprovalDeny); err != nil {
		t.Fatalf("ReplyApproval after failed AnswerQuestion: %v", err)
	}
	select {
	case res := <-resCh:
		if res.decision != "deny" {
			t.Errorf("decision = %q, want deny", res.decision)
		}
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for ApproveConsoleTool to return")
	}
}
