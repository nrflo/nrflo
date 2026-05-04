package spawner

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"be/internal/clock"
	"be/internal/ws"
)

// TestSendNudge_BroadcastsNudgedEvent verifies EventAgentNudged is broadcast with correct fields.
func TestSendNudge_BroadcastsNudgedEvent(t *testing.T) {
	clk := clock.NewTest(time.Now())
	hub := ws.NewHub(clock.Real())
	go hub.Run()
	defer hub.Stop()

	client, ch := ws.NewTestClient(hub, "nudge-ws-client")
	hub.Register(client)
	hub.Subscribe(client, "proj-nudge", "TKT-1")

	s := New(Config{Clock: clk, WSHub: hub})

	proc := &processInfo{
		nudgeMax:   5,
		nudgeCount: 0,
		backend:    &cliInteractiveBackend{},
		sessionID:  "session-nudge-1",
		agentType:  "implementor",
		modelID:    "claude:sonnet",
		projectID:  "proj-nudge",
		ticketID:   "TKT-1",
	}

	s.sendNudge(context.Background(), proc, SpawnRequest{
		ProjectID:    "proj-nudge",
		TicketID:     "TKT-1",
		WorkflowName: "feature",
	})

	deadline := time.After(2 * time.Second)
	for {
		select {
		case msg := <-ch:
			var event ws.Event
			if err := json.Unmarshal(msg, &event); err != nil {
				continue
			}
			if event.Type != ws.EventAgentNudged {
				continue
			}
			attempt, _ := event.Data["attempt"].(float64)
			if attempt != 1 {
				t.Errorf("attempt = %v, want 1", attempt)
			}
			max, _ := event.Data["max"].(float64)
			if max != 5 {
				t.Errorf("max = %v, want 5", max)
			}
			sessID, _ := event.Data["session_id"].(string)
			if sessID != "session-nudge-1" {
				t.Errorf("session_id = %q, want 'session-nudge-1'", sessID)
			}
			agentType, _ := event.Data["agent_type"].(string)
			if agentType != "implementor" {
				t.Errorf("agent_type = %q, want 'implementor'", agentType)
			}
			return
		case <-deadline:
			t.Fatal("timeout waiting for EventAgentNudged broadcast")
		}
	}
}

// TestSendNudge_BroadcastFields_AttemptIncrements verifies that attempt reflects the
// pre-increment nudgeCount+1 on each nudge, matching the event payload.
func TestSendNudge_BroadcastFields_AttemptIncrements(t *testing.T) {
	clk := clock.NewTest(time.Now())
	hub := ws.NewHub(clock.Real())
	go hub.Run()
	defer hub.Stop()

	client, ch := ws.NewTestClient(hub, "nudge-inc-client")
	hub.Register(client)
	hub.Subscribe(client, "proj-inc", "TKT-INC")

	s := New(Config{Clock: clk, WSHub: hub})

	proc := &processInfo{
		nudgeMax:  5,
		nudgeCount: 2, // already sent 2 nudges
		backend:   &cliInteractiveBackend{},
		sessionID: "sess-inc",
		agentType: "qa-verifier",
		modelID:   "claude:opus",
		projectID: "proj-inc",
		ticketID:  "TKT-INC",
	}

	s.sendNudge(context.Background(), proc, SpawnRequest{
		ProjectID: "proj-inc", TicketID: "TKT-INC", WorkflowName: "feature",
	})

	deadline := time.After(2 * time.Second)
	for {
		select {
		case msg := <-ch:
			var event ws.Event
			if err := json.Unmarshal(msg, &event); err != nil {
				continue
			}
			if event.Type != ws.EventAgentNudged {
				continue
			}
			attempt, _ := event.Data["attempt"].(float64)
			if attempt != 3 {
				t.Errorf("attempt = %v, want 3 (nudgeCount was 2)", attempt)
			}
			return
		case <-deadline:
			t.Fatal("timeout waiting for EventAgentNudged")
		}
	}
}

// TestSendNudge_NudgeCountFallback_NilAgentSvc verifies nudgeCount is incremented via the
// fallback path when AgentSvcReal is nil.
func TestSendNudge_NudgeCountFallback_NilAgentSvc(t *testing.T) {
	clk := clock.NewTest(time.Now())
	s := New(Config{Clock: clk}) // no WSHub, no AgentSvcReal

	proc := &processInfo{
		nudgeMax:   5,
		nudgeCount: 0,
		backend:    &cliInteractiveBackend{},
		sessionID:  "s-fallback",
		agentType:  "test-agent",
		modelID:    "claude:sonnet",
	}

	s.sendNudge(context.Background(), proc, SpawnRequest{})

	if proc.nudgeCount != 1 {
		t.Errorf("nudgeCount = %d, want 1 (fallback increment when AgentSvcReal=nil)", proc.nudgeCount)
	}
}

// TestSendNudge_ResetsLastMessageTime verifies sendNudge resets lastMessageTime to Now()
// and sets hasReceivedMessage=true so the idle window restarts.
func TestSendNudge_ResetsLastMessageTime(t *testing.T) {
	clk := clock.NewTest(time.Now())
	s := New(Config{Clock: clk})

	proc := &processInfo{
		nudgeMax:           5,
		nudgeCount:         0,
		backend:            &cliInteractiveBackend{},
		sessionID:          "s-reset",
		agentType:          "test-agent",
		lastMessageTime:    clk.Now().Add(-5 * time.Minute),
		hasReceivedMessage: false,
	}

	clk.Advance(1 * time.Minute)
	expectedTime := clk.Now()
	s.sendNudge(context.Background(), proc, SpawnRequest{})

	proc.messagesMutex.Lock()
	lmt := proc.lastMessageTime
	hasMsg := proc.hasReceivedMessage
	proc.messagesMutex.Unlock()

	if lmt != expectedTime {
		t.Errorf("lastMessageTime = %v, want %v (reset to clock.Now() by nudge)", lmt, expectedTime)
	}
	if !hasMsg {
		t.Error("hasReceivedMessage should be true after nudge")
	}
}

// TestSendNudge_LastNudgeAtSet verifies lastNudgeAt is set on each nudge.
func TestSendNudge_LastNudgeAtSet(t *testing.T) {
	clk := clock.NewTest(time.Now())
	s := New(Config{Clock: clk})

	proc := &processInfo{
		nudgeMax:  5,
		backend:   &cliInteractiveBackend{},
		sessionID: "s-lna",
		agentType: "test-agent",
	}

	if !proc.lastNudgeAt.IsZero() {
		t.Error("lastNudgeAt should be zero before first nudge")
	}

	s.sendNudge(context.Background(), proc, SpawnRequest{})

	if proc.lastNudgeAt.IsZero() {
		t.Error("lastNudgeAt should be set after nudge")
	}
	if proc.lastNudgeAt != clk.Now() {
		t.Errorf("lastNudgeAt = %v, want %v", proc.lastNudgeAt, clk.Now())
	}
}

// TestSendNudge_NilPTYManager_NoPanic verifies no panic when PTYManager is nil.
func TestSendNudge_NilPTYManager_NoPanic(t *testing.T) {
	clk := clock.NewTest(time.Now())
	s := New(Config{Clock: clk}) // PTYManager is nil

	proc := &processInfo{
		nudgeMax:   5,
		nudgeCount: 2,
		backend:    &cliInteractiveBackend{},
		sessionID:  "s-nopty",
		agentType:  "test-agent",
		modelID:    "sonnet",
	}

	// Must not panic; nudgeCount still incremented via fallback.
	s.sendNudge(context.Background(), proc, SpawnRequest{})

	if proc.nudgeCount != 3 {
		t.Errorf("nudgeCount = %d, want 3 (incremented even with nil PTYManager)", proc.nudgeCount)
	}
}

// TestHandleNudgeAutoFail_DispatchesTerminalSignal verifies RequestTerminalSignal is called
// with the correct sessionID and "fail" result.
func TestHandleNudgeAutoFail_DispatchesTerminalSignal(t *testing.T) {
	clk := clock.NewTest(time.Now())
	s := New(Config{Clock: clk})

	proc := &processInfo{
		sessionID:  "sess-auto-fail",
		agentType:  "implementor",
		projectID:  "proj-1",
		nudgeMax:   5,
		nudgeCount: 5,
	}

	ch := make(chan terminalSignal, 1)
	s.registerTerminalSignal(proc.sessionID, ch)

	s.handleNudgeAutoFail(context.Background(), proc, SpawnRequest{})

	select {
	case sig := <-ch:
		if sig.SessionID != "sess-auto-fail" {
			t.Errorf("signal.SessionID = %q, want 'sess-auto-fail'", sig.SessionID)
		}
		if sig.Result != "fail" {
			t.Errorf("signal.Result = %q, want 'fail'", sig.Result)
		}
	default:
		t.Error("registered channel empty: RequestTerminalSignal not dispatched by handleNudgeAutoFail")
	}
}

// TestHandleNudgeAutoFail_NilServices_NoPanic verifies no panic when AgentSvcReal and
// ErrorSvc are nil (the common test configuration).
func TestHandleNudgeAutoFail_NilServices_NoPanic(t *testing.T) {
	clk := clock.NewTest(time.Now())
	s := New(Config{Clock: clk}) // AgentSvcReal=nil, ErrorSvc=nil

	proc := &processInfo{
		sessionID: "sess-nil-svc",
		agentType: "test-agent",
		projectID: "proj-1",
		nudgeMax:  5,
	}

	// Must not panic regardless of nil services.
	s.handleNudgeAutoFail(context.Background(), proc, SpawnRequest{})
}
