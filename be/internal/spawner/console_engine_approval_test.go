package spawner

import (
	"encoding/json"
	"fmt"
	"testing"
	"time"
)

// drainReplyFor scans f.outbound for the first response (id set, method
// empty — a reply, not a call) matching wantID.
func drainReplyFor(t *testing.T, f *fakeAppServer, wantID string, timeout time.Duration) rpcEnvelope {
	t.Helper()
	deadline := time.After(timeout)
	for {
		select {
		case env := <-f.outbound:
			if env.Method == "" && env.ID != nil && string(*env.ID) == wantID {
				return env
			}
		case <-deadline:
			t.Fatalf("timed out waiting for a reply to id %s", wantID)
		}
	}
}

// TestCodexEngine_Approval_V2Request asserts a v2 requestApproval server
// request yields exactly one EventApprovalRequest carrying the command/cwd
// and id, and persists nothing to the Sink.
func TestCodexEngine_Approval_V2Request(t *testing.T) {
	sink := &testSink{}
	eng, f := startTestCodexEngine(t, sink, EngineSpec{})

	f.feed(`{"id":7,"method":"item/commandExecution/requestApproval","params":{"itemId":"c1","command":"rm -rf /tmp/x","cwd":"/work"}}`)
	ev := waitForEventType(t, eng.Events(), EventApprovalRequest, 2*time.Second)
	if ev.Approval == nil {
		t.Fatal("Approval field is nil")
	}
	if ev.Approval.ID != "7" || ev.Approval.Command != "rm -rf /tmp/x" || ev.Approval.Cwd != "/work" {
		t.Errorf("approval request = %+v, want id=7 command=%q cwd=%q", ev.Approval, "rm -rf /tmp/x", "/work")
	}
	if n := len(sink.recordedMsgs); n != 0 {
		t.Errorf("approval request must not persist to Sink, got %d rows", n)
	}
}

// TestCodexEngine_Approval_DecisionMapping asserts ReplyApproval maps each
// ApprovalDecision to the correct wire vocabulary for the v2 protocol
// (accept/acceptForSession/decline/cancel).
func TestCodexEngine_Approval_DecisionMapping(t *testing.T) {
	sink := &testSink{}
	eng, f := startTestCodexEngine(t, sink, EngineSpec{})

	cases := []struct {
		name     string
		method   string
		decision ApprovalDecision
		wantWire string
	}{
		{"v2_command_approve", "item/commandExecution/requestApproval", ApprovalApprove, "accept"},
		{"v2_command_approve_for_session", "item/commandExecution/requestApproval", ApprovalApproveForSession, "acceptForSession"},
		{"v2_command_deny", "item/commandExecution/requestApproval", ApprovalDeny, "decline"},
		{"v2_command_abort", "item/commandExecution/requestApproval", ApprovalAbort, "cancel"},
		{"v2_filechange_deny", "item/fileChange/requestApproval", ApprovalDeny, "decline"},
	}

	for i, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			id := fmt.Sprintf("%d", 100+i)
			f.feed(fmt.Sprintf(`{"id":%s,"method":%q,"params":{"itemId":"x","command":"ls","cwd":"/w"}}`, id, tc.method))
			_ = waitForEventType(t, eng.Events(), EventApprovalRequest, 2*time.Second)

			if err := eng.ReplyApproval(id, tc.decision); err != nil {
				t.Fatalf("ReplyApproval: %v", err)
			}
			reply := drainReplyFor(t, f, id, 2*time.Second)
			var res struct {
				Decision string `json:"decision"`
			}
			_ = json.Unmarshal(reply.Result, &res)
			if res.Decision != tc.wantWire {
				t.Errorf("wire decision = %q, want %q", res.Decision, tc.wantWire)
			}
		})
	}
}

// TestCodexEngine_Approval_UnknownMethodRejected asserts a server request
// that isn't one of the two approval-shaped methods gets a JSON-RPC error
// reply — never a fabricated decision — and emits no event.
func TestCodexEngine_Approval_UnknownMethodRejected(t *testing.T) {
	sink := &testSink{}
	eng, f := startTestCodexEngine(t, sink, EngineSpec{})

	cases := []struct{ id, method string }{
		{"201", "item/permissions/requestApproval"},
		{"202", "item/tool/requestUserInput"},
		{"203", "execCommandApproval"},
		{"204", "applyPatchApproval"},
	}
	for _, tc := range cases {
		t.Run(tc.method, func(t *testing.T) {
			f.feed(fmt.Sprintf(`{"id":%s,"method":%q,"params":{}}`, tc.id, tc.method))
			reply := drainReplyFor(t, f, tc.id, 2*time.Second)
			if reply.Error == nil || reply.Error.Code != -32601 {
				t.Errorf("reply = %+v, want a -32601 error", reply)
			}
		})
	}

	select {
	case ev := <-eng.Events():
		t.Errorf("unexpected event for an unhandled server request: %+v", ev)
	case <-time.After(200 * time.Millisecond):
	}
}

// TestCodexEngine_Approval_ResolvedElsewhereDropsPending asserts
// serverRequest/resolved drops the pending entry so a later ReplyApproval for
// that id errors instead of replying twice.
func TestCodexEngine_Approval_ResolvedElsewhereDropsPending(t *testing.T) {
	sink := &testSink{}
	eng, f := startTestCodexEngine(t, sink, EngineSpec{})

	f.feed(`{"id":300,"method":"item/commandExecution/requestApproval","params":{"itemId":"c1","command":"ls","cwd":"/w"}}`)
	_ = waitForEventType(t, eng.Events(), EventApprovalRequest, 2*time.Second)

	f.feed(`{"method":"serverRequest/resolved","params":{"id":300}}`)

	// onServerRequestResolved must emit EventApprovalResolved (deny, with the
	// "resolved by app-server (timed out)" reason) before dropping the pending
	// entry — this is the single event pumpChatEvents relies on to resolve the
	// pending approval and push console_chat.approval_resolved, mirroring the
	// claude engine's timeout/stop paths.
	resolved := waitForEventType(t, eng.Events(), EventApprovalResolved, 2*time.Second)
	if resolved.ApprovalID != "300" {
		t.Errorf("resolved.ApprovalID = %q, want %q", resolved.ApprovalID, "300")
	}
	if resolved.Decision != ApprovalDeny {
		t.Errorf("resolved.Decision = %q, want %q", resolved.Decision, ApprovalDeny)
	}
	if resolved.Text != "resolved by app-server (timed out)" {
		t.Errorf("resolved.Text = %q, want %q", resolved.Text, "resolved by app-server (timed out)")
	}

	if err := eng.ReplyApproval("300", ApprovalApprove); err == nil {
		t.Error("ReplyApproval after serverRequest/resolved should error, got nil")
	}
}

// TestCodexEngine_Approval_ReplyUnknownID asserts ReplyApproval with an id
// that was never registered errors.
func TestCodexEngine_Approval_ReplyUnknownID(t *testing.T) {
	sink := &testSink{}
	eng, _ := startTestCodexEngine(t, sink, EngineSpec{})

	if err := eng.ReplyApproval("does-not-exist", ApprovalApprove); err == nil {
		t.Error("expected error for an unknown approval id, got nil")
	}
}

// TestCodexEngine_ReplyApproval_EmitsApprovalResolved asserts a successful
// human-driven ReplyApproval also emits EventApprovalResolved carrying the
// decision — the same event pumpChatEvents relies on for the
// onServerRequestResolved (timeout) path, so both settle the pending
// approval through exactly one event type.
func TestCodexEngine_ReplyApproval_EmitsApprovalResolved(t *testing.T) {
	sink := &testSink{}
	eng, f := startTestCodexEngine(t, sink, EngineSpec{})

	f.feed(`{"id":400,"method":"item/commandExecution/requestApproval","params":{"itemId":"c1","command":"ls","cwd":"/w"}}`)
	_ = waitForEventType(t, eng.Events(), EventApprovalRequest, 2*time.Second)

	if err := eng.ReplyApproval("400", ApprovalDeny); err != nil {
		t.Fatalf("ReplyApproval: %v", err)
	}

	resolved := waitForEventType(t, eng.Events(), EventApprovalResolved, 2*time.Second)
	if resolved.ApprovalID != "400" {
		t.Errorf("resolved.ApprovalID = %q, want %q", resolved.ApprovalID, "400")
	}
	if resolved.Decision != ApprovalDeny {
		t.Errorf("resolved.Decision = %q, want %q", resolved.Decision, ApprovalDeny)
	}
}

// TestCodexEngine_ResolvedAfterReply_EmitsNothing asserts an approval already
// settled by ReplyApproval is not re-resolved when the app-server then sends
// serverRequest/resolved for the same id (which is exactly what it does after a
// client answers). A second, unconditional deny push would flip an allowed
// tool's card to "denied — timed out" and audit a resolution that never
// happened. Same guard covers a resolved id that was never an approval.
func TestCodexEngine_ResolvedAfterReply_EmitsNothing(t *testing.T) {
	sink := &testSink{}
	eng, f := startTestCodexEngine(t, sink, EngineSpec{})

	f.feed(`{"id":500,"method":"item/commandExecution/requestApproval","params":{"itemId":"c1","command":"ls","cwd":"/w"}}`)
	_ = waitForEventType(t, eng.Events(), EventApprovalRequest, 2*time.Second)

	if err := eng.ReplyApproval("500", ApprovalApprove); err != nil {
		t.Fatalf("ReplyApproval: %v", err)
	}
	resolved := waitForEventType(t, eng.Events(), EventApprovalResolved, 2*time.Second)
	if resolved.Decision != ApprovalApprove {
		t.Fatalf("resolved.Decision = %q, want %q", resolved.Decision, ApprovalApprove)
	}

	// The app-server resolves the id it just got an answer for, plus one it
	// never issued as an approval. Neither may produce an EventApprovalResolved.
	f.feed(`{"method":"serverRequest/resolved","params":{"id":500}}`)
	f.feed(`{"method":"serverRequest/resolved","params":{"id":999}}`)

	deadline := time.After(300 * time.Millisecond)
	for {
		select {
		case ev := <-eng.Events():
			if ev.Type == EventApprovalResolved {
				t.Fatalf("second EventApprovalResolved for %q (decision %q) — an already-settled or non-approval id must not re-resolve", ev.ApprovalID, ev.Decision)
			}
		case <-deadline:
			return
		}
	}
}
