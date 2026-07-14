package spawner

import (
	"context"
	"encoding/json"
	"os"
	"strings"
	"testing"
	"time"
)

// TestCodexEngine_Start_HandshakeOrder asserts initialize -> initialized ->
// thread/start are observed on the wire in that order.
func TestCodexEngine_Start_HandshakeOrder(t *testing.T) {
	sink := &testSink{}
	_, f := startTestCodexEngine(t, sink, EngineSpec{})

	var methods []string
	deadline := time.After(2 * time.Second)
	for len(methods) < 3 {
		select {
		case env := <-f.outbound:
			methods = append(methods, env.Method)
		case <-deadline:
			t.Fatalf("only observed %v before timeout", methods)
		}
	}
	want := []string{"initialize", "initialized", "thread/start"}
	for i, m := range want {
		if methods[i] != m {
			t.Errorf("methods[%d] = %q, want %q (all: %v)", i, methods[i], m, methods)
		}
	}
}

// TestCodexEngine_Start_ThreadStartDefaults asserts the console engine's
// thread/start defaults to sandbox=workspace-write/approvalPolicy=on-request
// when EngineSpec leaves them empty (the autonomous backend instead pins
// danger-full-access/never — see TestCodexEngine_AutonomousThreadStartUnchanged).
func TestCodexEngine_Start_ThreadStartDefaults(t *testing.T) {
	sink := &testSink{}
	paramsCh := make(chan json.RawMessage, 1)
	t.Setenv("HOME", t.TempDir())
	f := newFakeAppServer(t)
	f.install(t)
	f.setOverride("thread/start", func(f *fakeAppServer, env rpcEnvelope) {
		paramsCh <- env.Params
		f.replyResult(*env.ID, `{"thread":{"id":"T1"}}`)
	})
	eng := newCodexEngine(sink)
	if err := eng.Start(context.Background(), EngineSpec{SessionID: "s1", WorkDir: t.TempDir()}); err != nil {
		t.Fatalf("Start: %v", err)
	}
	t.Cleanup(eng.Stop)

	params := mustRecvParams(t, paramsCh)
	var p struct {
		Sandbox        string `json:"sandbox"`
		ApprovalPolicy string `json:"approvalPolicy"`
	}
	_ = json.Unmarshal(params, &p)
	if p.Sandbox != "workspace-write" || p.ApprovalPolicy != "on-request" {
		t.Errorf("thread/start params = %+v, want sandbox=workspace-write approvalPolicy=on-request", p)
	}
}

// TestCodexEngine_Start_ThreadStartOverrides asserts EngineSpec.Sandbox/
// ApprovalPolicy override the engine defaults when set.
func TestCodexEngine_Start_ThreadStartOverrides(t *testing.T) {
	sink := &testSink{}
	paramsCh := make(chan json.RawMessage, 1)
	t.Setenv("HOME", t.TempDir())
	f := newFakeAppServer(t)
	f.install(t)
	f.setOverride("thread/start", func(f *fakeAppServer, env rpcEnvelope) {
		paramsCh <- env.Params
		f.replyResult(*env.ID, `{"thread":{"id":"T1"}}`)
	})
	eng := newCodexEngine(sink)
	spec := EngineSpec{SessionID: "s1", WorkDir: t.TempDir(), Sandbox: "danger-full-access", ApprovalPolicy: "never"}
	if err := eng.Start(context.Background(), spec); err != nil {
		t.Fatalf("Start: %v", err)
	}
	t.Cleanup(eng.Stop)

	params := mustRecvParams(t, paramsCh)
	var p struct {
		Sandbox        string `json:"sandbox"`
		ApprovalPolicy string `json:"approvalPolicy"`
	}
	_ = json.Unmarshal(params, &p)
	if p.Sandbox != "danger-full-access" || p.ApprovalPolicy != "never" {
		t.Errorf("thread/start params = %+v, want overrides preserved", p)
	}
}

// TestCodexEngine_Start_ThreadStartRPCError asserts a thread/start rpc error
// fails Start synchronously.
func TestCodexEngine_Start_ThreadStartRPCError(t *testing.T) {
	sink := &testSink{}
	t.Setenv("HOME", t.TempDir())
	f := newFakeAppServer(t)
	f.install(t)
	f.setOverride("thread/start", func(f *fakeAppServer, env rpcEnvelope) {
		f.replyError(*env.ID, -32000, "thread start boom")
	})
	eng := newCodexEngine(sink)
	err := eng.Start(context.Background(), EngineSpec{SessionID: "s1", WorkDir: t.TempDir()})
	if err == nil {
		t.Fatal("expected Start to fail on thread/start rpc error")
	}
	if !strings.Contains(err.Error(), "thread start boom") {
		t.Errorf("err = %v, want to contain the rpc error message", err)
	}
}

func mustRecvParams(t *testing.T, ch <-chan json.RawMessage) json.RawMessage {
	t.Helper()
	select {
	case p := <-ch:
		return p
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for params")
		return nil
	}
}

// TestCodexEngine_SendUserTurn_WireAndPersistence asserts turn/start carries
// {threadId, input:[{type:"text",text:...}]} and the user text is persisted
// exactly once as category "user_input".
func TestCodexEngine_SendUserTurn_WireAndPersistence(t *testing.T) {
	sink := &testSink{}
	eng, f := startTestCodexEngine(t, sink, EngineSpec{})

	paramsCh := make(chan json.RawMessage, 1)
	f.setOverride("turn/start", func(f *fakeAppServer, env rpcEnvelope) {
		paramsCh <- env.Params
		f.replyResult(*env.ID, `{}`)
	})

	if err := eng.SendUserTurn(context.Background(), "hello codex"); err != nil {
		t.Fatalf("SendUserTurn: %v", err)
	}

	params := mustRecvParams(t, paramsCh)
	var p struct {
		ThreadID string `json:"threadId"`
		Input    []struct {
			Type string `json:"type"`
			Text string `json:"text"`
		} `json:"input"`
	}
	if err := json.Unmarshal(params, &p); err != nil {
		t.Fatalf("unmarshal turn/start params: %v", err)
	}
	if p.ThreadID == "" {
		t.Errorf("threadId empty in turn/start params")
	}
	if len(p.Input) != 1 || p.Input[0].Type != "text" || p.Input[0].Text != "hello codex" {
		t.Errorf("input = %+v, want one text block with %q", p.Input, "hello codex")
	}

	if n := countCategory(sink, "user_input"); n != 1 {
		t.Errorf("user_input rows = %d, want exactly 1", n)
	}
}

// TestCodexEngine_SendUserTurn_ErrTurnActive asserts a second SendUserTurn
// while a turn is in flight is rejected without touching the wire.
func TestCodexEngine_SendUserTurn_ErrTurnActive(t *testing.T) {
	sink := &testSink{}
	eng, f := startTestCodexEngine(t, sink, EngineSpec{})

	// Let the first turn/start hang (never replies) so the turn stays active.
	f.setOverride("turn/start", func(f *fakeAppServer, env rpcEnvelope) {})

	done := make(chan error, 1)
	go func() { done <- eng.SendUserTurn(context.Background(), "first") }()

	// SendUserTurn marks turnActive BEFORE it writes turn/start, so seeing the
	// turn/start envelope on the wire already proves the flag is set — no poll.
	waitForOutbound(t, f, "turn/start", 2*time.Second)

	if err := eng.SendUserTurn(context.Background(), "second"); err != ErrTurnActive {
		t.Errorf("second SendUserTurn err = %v, want ErrTurnActive", err)
	}

	// Unblock the first call so its goroutine doesn't leak past the test.
	f.setOverride("turn/start", nil)
	// Cancel via Stop in cleanup; drain the first call's result if it ever returns.
	select {
	case <-done:
	case <-time.After(50 * time.Millisecond):
	}
}

// TestCodexEngine_Stop_IdempotentAndCleansUp asserts Stop cancels the run
// context, removes the profile dir, closes Events() exactly once, and is
// safe to call twice.
func TestCodexEngine_Stop_IdempotentAndCleansUp(t *testing.T) {
	sink := &testSink{}
	eng, _ := startTestCodexEngine(t, sink, EngineSpec{})

	eng.mu.Lock()
	profileDir := eng.profileDir
	eng.mu.Unlock()
	if profileDir == "" {
		t.Fatal("profileDir not set after Start")
	}
	if _, err := os.Stat(profileDir); err != nil {
		t.Fatalf("profile dir missing before Stop: %v", err)
	}

	eng.Stop()

	if _, err := os.Stat(profileDir); !os.IsNotExist(err) {
		t.Errorf("profile dir still exists after Stop: err=%v", err)
	}
	if _, ok := <-eng.Events(); ok {
		t.Errorf("Events() should be closed after Stop")
	}

	eng.Stop() // must not panic
}

// TestCodexEngine_ErrorNotification asserts an `error` notification yields
// EventError carrying the message.
func TestCodexEngine_ErrorNotification(t *testing.T) {
	sink := &testSink{}
	eng, f := startTestCodexEngine(t, sink, EngineSpec{})
	f.feed(`{"method":"error","params":{"error":{"message":"boom generic"}}}`)

	ev := waitForEventType(t, eng.Events(), EventError, 2*time.Second)
	if ev.Text != "boom generic" || !ev.IsError {
		t.Errorf("event = %+v, want text=%q isError=true", ev, "boom generic")
	}
}

// TestCodexEngine_TurnCompletedError asserts turn/completed carrying
// turn.error yields EventError as well as EventTurnCompleted.
func TestCodexEngine_TurnCompletedError(t *testing.T) {
	sink := &testSink{}
	eng, f := startTestCodexEngine(t, sink, EngineSpec{})
	f.feed(`{"method":"turn/completed","params":{"turn":{"id":"t1","status":"failed","error":{"message":"turn boom"}}}}`)

	ev := waitForEventType(t, eng.Events(), EventError, 2*time.Second)
	if ev.Text != "turn boom" {
		t.Errorf("error event text = %q, want %q", ev.Text, "turn boom")
	}
	_ = waitForEventType(t, eng.Events(), EventTurnCompleted, 2*time.Second)
}

// TestCodexEngine_ConnectionClosed_EmitsErrorThenChannelCloses asserts a
// closed app-server connection (EOF) yields an EventError, and once the
// caller reacts by calling Stop, the Events() channel closes.
func TestCodexEngine_ConnectionClosed_EmitsErrorThenChannelCloses(t *testing.T) {
	sink := &testSink{}
	eng, f := startTestCodexEngine(t, sink, EngineSpec{})

	_ = f.stdoutW.Close() // simulate the app-server process exiting (EOF)

	ev := waitForEventType(t, eng.Events(), EventError, 2*time.Second)
	if !ev.IsError || ev.Text == "" {
		t.Errorf("connection-closed event = %+v, want a non-empty error", ev)
	}

	eng.Stop()
	select {
	case _, ok := <-eng.Events():
		if ok {
			t.Errorf("Events() should be closed after Stop following connection close")
		}
	case <-time.After(2 * time.Second):
		t.Fatal("Events() did not close after Stop")
	}
}
