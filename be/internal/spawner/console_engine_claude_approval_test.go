package spawner

import (
	"context"
	"testing"
	"time"
)

type approvalResult struct {
	decision, reason string
	handled          bool
}

// requestApprovalViaHub drives hub.ApproveConsoleTool from a goroutine (the
// actual call shape socket's consolePreToolApproval uses) and returns a
// channel delivering its result.
func requestApprovalViaHub(hub *ConsoleHub, sessionID, toolName, toolUseID string, toolInput map[string]any) <-chan approvalResult {
	resCh := make(chan approvalResult, 1)
	go func() {
		d, r, h := hub.ApproveConsoleTool(context.Background(), sessionID, toolName, toolInput, toolUseID)
		resCh <- approvalResult{d, r, h}
	}()
	return resCh
}

func TestClaudeEngine_Approval_Allow(t *testing.T) {
	sink := &testSink{}
	hub := NewConsoleHub()
	e, _ := startTestClaudeEngine(t, sink, hub, EngineSpec{})

	resCh := requestApprovalViaHub(hub, e.spec.SessionID, "Bash", "tu-allow-1", map[string]any{"command": "ls"})

	invoke := waitForEventType(t, e.Events(), EventToolInvoke, time.Second)
	if invoke.ToolName != "Bash" {
		t.Errorf("tool_invoke ToolName = %q, want Bash", invoke.ToolName)
	}
	ev := waitForEventType(t, e.Events(), EventApprovalRequest, time.Second)
	if ev.Approval == nil || ev.Approval.ID != "tu-allow-1" {
		t.Fatalf("approval request = %+v, want id=tu-allow-1", ev.Approval)
	}
	if n := len(sink.recordedMsgs); n != 0 {
		t.Errorf("an approval request must not persist to Sink, got %d rows", n)
	}

	if err := e.ReplyApproval(ev.Approval.ID, ApprovalApprove); err != nil {
		t.Fatalf("ReplyApproval: %v", err)
	}

	select {
	case res := <-resCh:
		if !res.handled || res.decision != "allow" {
			t.Errorf("result = %+v, want handled=true decision=allow", res)
		}
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for ApproveConsoleTool to return")
	}
}

func TestClaudeEngine_Approval_Deny(t *testing.T) {
	sink := &testSink{}
	hub := NewConsoleHub()
	e, _ := startTestClaudeEngine(t, sink, hub, EngineSpec{})

	resCh := requestApprovalViaHub(hub, e.spec.SessionID, "Bash", "tu-deny-1", map[string]any{"command": "rm -rf /"})
	ev := waitForEventType(t, e.Events(), EventApprovalRequest, time.Second)

	if err := e.ReplyApproval(ev.Approval.ID, ApprovalDeny); err != nil {
		t.Fatalf("ReplyApproval: %v", err)
	}

	select {
	case res := <-resCh:
		if !res.handled || res.decision != "deny" {
			t.Errorf("result = %+v, want handled=true decision=deny", res)
		}
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for ApproveConsoleTool to return")
	}
}

func TestClaudeEngine_Approval_Timeout(t *testing.T) {
	sink := &testSink{}
	hub := NewConsoleHub()
	e, _ := startTestClaudeEngine(t, sink, hub, EngineSpec{})
	e.approvalTimeout = 5 * time.Millisecond

	resCh := requestApprovalViaHub(hub, e.spec.SessionID, "Bash", "tu-timeout-1", map[string]any{"command": "rm -rf /"})
	_ = waitForEventType(t, e.Events(), EventApprovalRequest, time.Second)

	select {
	case res := <-resCh:
		if !res.handled || res.decision != "deny" || res.reason != "nrflo: approval timed out" {
			t.Errorf("result = %+v, want handled=true decision=deny reason=%q", res, "nrflo: approval timed out")
		}
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for the approval itself to time out")
	}

	// An unanswered approval must still emit EventApprovalResolved — the
	// single event pumpChatEvents relies on to resolve the pending approval
	// and push console_chat.approval_resolved for a client that never sees
	// the RequestApproval return value directly.
	resolved := waitForEventType(t, e.Events(), EventApprovalResolved, time.Second)
	if resolved.ApprovalID != "tu-timeout-1" {
		t.Errorf("resolved.ApprovalID = %q, want %q", resolved.ApprovalID, "tu-timeout-1")
	}
	if resolved.Decision != ApprovalDeny {
		t.Errorf("resolved.Decision = %q, want %q", resolved.Decision, ApprovalDeny)
	}
	if resolved.Text != "nrflo: approval timed out" {
		t.Errorf("resolved.Text = %q, want %q", resolved.Text, "nrflo: approval timed out")
	}

	// The pending entry must be cleared on timeout.
	if err := e.ReplyApproval("tu-timeout-1", ApprovalApprove); err == nil {
		t.Error("expected ReplyApproval to error after the approval already timed out")
	}
}

func TestClaudeEngine_ReplyApproval_UnknownID_Errors(t *testing.T) {
	e, _ := startTestClaudeEngine(t, &testSink{}, nil, EngineSpec{})
	if err := e.ReplyApproval("does-not-exist", ApprovalApprove); err == nil {
		t.Error("expected error for an unknown approval id")
	}
}

// TestClaudeEngine_ReplyApproval_ApproveForSession_AllowsAndRemembersTool
// covers the session-scoped allowlist: approve_for_session resolves the
// pending approval as allow AND auto-allows later requests for the same tool
// without emitting a new EventApprovalRequest, while other tools still ask.
func TestClaudeEngine_ReplyApproval_ApproveForSession_AllowsAndRemembersTool(t *testing.T) {
	sink := &testSink{}
	hub := NewConsoleHub()
	e, _ := startTestClaudeEngine(t, sink, hub, EngineSpec{})

	resCh := requestApprovalViaHub(hub, e.spec.SessionID, "Write", "tu-afs-1", map[string]any{})
	_ = waitForEventType(t, e.Events(), EventApprovalRequest, time.Second)

	if err := e.ReplyApproval("tu-afs-1", ApprovalApproveForSession); err != nil {
		t.Fatalf("ReplyApproval(approve_for_session): %v", err)
	}
	resolved := waitForEventType(t, e.Events(), EventApprovalResolved, time.Second)
	if resolved.Decision != ApprovalApproveForSession {
		t.Errorf("resolved.Decision = %q, want approve_for_session", resolved.Decision)
	}
	select {
	case res := <-resCh:
		if res.decision != "allow" {
			t.Errorf("decision = %q, want allow", res.decision)
		}
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for the approval to resolve")
	}

	// Same tool again: auto-allowed, no human round-trip, no approval request.
	res2 := requestApprovalViaHub(hub, e.spec.SessionID, "Write", "tu-afs-2", map[string]any{})
	select {
	case res := <-res2:
		if res.decision != "allow" {
			t.Errorf("second decision = %q, want allow", res.decision)
		}
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for the auto-allowed approval")
	}
	if err := e.ReplyApproval("tu-afs-2", ApprovalApprove); err == nil {
		t.Error("expected no pending approval for an auto-allowed tool")
	}

	// A different tool still asks.
	res3 := requestApprovalViaHub(hub, e.spec.SessionID, "Bash", "tu-afs-3", map[string]any{"command": "ls"})
	_ = waitForEventType(t, e.Events(), EventApprovalRequest, time.Second)
	if err := e.ReplyApproval("tu-afs-3", ApprovalDeny); err != nil {
		t.Fatalf("ReplyApproval(deny) for the other tool: %v", err)
	}
	select {
	case res := <-res3:
		if res.decision != "deny" {
			t.Errorf("other-tool decision = %q, want deny", res.decision)
		}
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for the other tool's approval")
	}
}

func TestClaudeEngine_Stop_UnblocksPendingApprovalWithDeny(t *testing.T) {
	sink := &testSink{}
	hub := NewConsoleHub()
	e, _ := startTestClaudeEngine(t, sink, hub, EngineSpec{})

	resCh := requestApprovalViaHub(hub, e.spec.SessionID, "Bash", "tu-stop-1", map[string]any{"command": "ls"})
	_ = waitForEventType(t, e.Events(), EventApprovalRequest, time.Second)

	e.Stop()

	select {
	case res := <-resCh:
		if !res.handled || res.decision != "deny" || res.reason != "nrflo: console session stopped" {
			t.Errorf("result = %+v, want handled=true decision=deny reason=%q", res, "nrflo: console session stopped")
		}
	case <-time.After(time.Second):
		t.Fatal("Stop did not unblock the pending approval")
	}
}
