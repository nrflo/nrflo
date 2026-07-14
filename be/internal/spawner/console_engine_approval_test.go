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
// ApprovalDecision to the correct wire vocabulary for both protocol
// generations (v2: accept/acceptForSession/decline/cancel; legacy:
// approved/approved_for_session/denied/abort).
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
		{"legacy_exec_approve", "execCommandApproval", ApprovalApprove, "approved"},
		{"legacy_exec_approve_for_session", "execCommandApproval", ApprovalApproveForSession, "approved_for_session"},
		{"legacy_exec_deny", "execCommandApproval", ApprovalDeny, "denied"},
		{"legacy_exec_abort", "execCommandApproval", ApprovalAbort, "abort"},
		{"legacy_patch_deny", "applyPatchApproval", ApprovalDeny, "denied"},
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
// that isn't one of the four approval-shaped methods gets a JSON-RPC error
// reply — never a fabricated decision — and emits no event.
func TestCodexEngine_Approval_UnknownMethodRejected(t *testing.T) {
	sink := &testSink{}
	eng, f := startTestCodexEngine(t, sink, EngineSpec{})

	cases := []struct{ id, method string }{
		{"201", "item/permissions/requestApproval"},
		{"202", "item/tool/requestUserInput"},
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
	// Notifications on notifyCh are FIFO behind a single consumer (runLoop):
	// feed a marker delta right after and wait for it, which guarantees the
	// resolved notification (enqueued first) was already processed.
	f.feed(`{"method":"item/agentMessage/delta","params":{"itemId":"marker","delta":"sync-marker"}}`)
	marker := waitForEventType(t, eng.Events(), EventTextDelta, 2*time.Second)
	if marker.Text != "sync-marker" {
		t.Fatalf("sync marker delta = %+v", marker)
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
