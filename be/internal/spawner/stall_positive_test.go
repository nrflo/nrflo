package spawner

import (
	"context"
	"encoding/json"
	"os/exec"
	"testing"
	"time"

	"be/internal/clock"
	"be/internal/repo"
	"be/internal/ws"
)

// stallPositiveProc builds a processInfo whose doneCh is pre-closed so
// handleStallRestart (called internally by checkStall) does not block.
// The cliBackend with nil cmd is nil-safe on Kill.
func stallPositiveProc(clk *clock.TestClock, hasMsg bool, lastMsgOffset time.Duration, count int) *processInfo {
	doneCh := make(chan struct{})
	close(doneCh)
	return &processInfo{
		cmd:                 &exec.Cmd{},
		backend:             &cliBackend{},
		doneCh:              doneCh,
		hasReceivedMessage:  hasMsg,
		lastMessageTime:     clk.Now().Add(lastMsgOffset),
		stallStartTimeout:   2 * time.Minute,
		stallRunningTimeout: 8 * time.Minute,
		stallRestartCount:   count,
		pendingMessages:     make([]repo.MessageEntry, 0),
	}
}

// TestCheckStall_StartStall_ExceedsThreshold verifies checkStall returns true
// when no messages have been received and elapsed time exceeds stallStartTimeout.
func TestCheckStall_StartStall_ExceedsThreshold(t *testing.T) {
	clk := clock.NewTest(time.Now())
	s := New(Config{Clock: clk})

	proc := stallPositiveProc(clk, false, 0, 0) // lastMsgTime = now
	clk.Advance(3 * time.Minute)                 // elapsed > stallStartTimeout(2m)

	got := s.checkStall(context.Background(), proc, SpawnRequest{})
	if !got {
		t.Error("checkStall = false, want true when elapsed > stallStartTimeout")
	}
	if proc.stallRestartCount != 1 {
		t.Errorf("stallRestartCount = %d, want 1 after trigger", proc.stallRestartCount)
	}
	if proc.finalStatus != "CONTINUE" {
		t.Errorf("finalStatus = %q, want CONTINUE after stall", proc.finalStatus)
	}
}

// TestCheckStall_RunningStall_ExceedsThreshold verifies checkStall returns true
// when the agent received messages but then went silent past stallRunningTimeout.
func TestCheckStall_RunningStall_ExceedsThreshold(t *testing.T) {
	clk := clock.NewTest(time.Now())
	s := New(Config{Clock: clk})

	proc := stallPositiveProc(clk, true, 0, 0) // hasReceivedMessage=true
	clk.Advance(9 * time.Minute)                // elapsed > stallRunningTimeout(8m)

	got := s.checkStall(context.Background(), proc, SpawnRequest{})
	if !got {
		t.Error("checkStall = false, want true when elapsed > stallRunningTimeout and hasReceivedMessage=true")
	}
	if proc.stallRestartCount != 1 {
		t.Errorf("stallRestartCount = %d, want 1 after trigger", proc.stallRestartCount)
	}
}

// TestCheckStall_APIBackend_StartStall_IsCliAgnostic verifies that stall detection
// fires identically for API-mode agents, confirming the stall path is CLI-agnostic.
func TestCheckStall_APIBackend_StartStall_IsCliAgnostic(t *testing.T) {
	clk := clock.NewTest(time.Now())
	s := New(Config{Clock: clk})

	doneCh := make(chan struct{})
	close(doneCh)
	proc := &processInfo{
		backend:            newAPIBackend(s), // nil cancel → Kill is nil-safe
		doneCh:             doneCh,
		hasReceivedMessage: false,
		lastMessageTime:    clk.Now(),
		stallStartTimeout:  2 * time.Minute,
		stallRestartCount:  0,
		pendingMessages:    make([]repo.MessageEntry, 0),
	}

	clk.Advance(3 * time.Minute)

	got := s.checkStall(context.Background(), proc, SpawnRequest{})
	if !got {
		t.Error("checkStall (API backend) = false, want true — stall detection must be backend-agnostic")
	}
	if proc.stallRestartCount != 1 {
		t.Errorf("stallRestartCount = %d, want 1 (API backend increments same counter)", proc.stallRestartCount)
	}
}

// TestCheckStall_Cap_14Triggers_15Blocks verifies the maxStallRestarts cap:
// stallRestartCount=14 (one below cap) triggers; stallRestartCount=15 (at cap) is blocked.
func TestCheckStall_Cap_14Triggers_15Blocks(t *testing.T) {
	clk := clock.NewTest(time.Now())
	s := New(Config{Clock: clk})

	makeStallProc := func(count int) *processInfo {
		return stallPositiveProc(clk, false, -10*time.Minute, count)
	}

	// At cap-1 (14): should trigger.
	proc14 := makeStallProc(maxStallRestarts - 1)
	if !s.checkStall(context.Background(), proc14, SpawnRequest{}) {
		t.Errorf("checkStall at stallRestartCount=%d should trigger (one below cap)", maxStallRestarts-1)
	}

	// At cap (15): must be blocked.
	proc15 := makeStallProc(maxStallRestarts)
	if s.checkStall(context.Background(), proc15, SpawnRequest{}) {
		t.Errorf("checkStall at stallRestartCount=%d should be blocked (cap reached)", maxStallRestarts)
	}
}

// TestCheckStall_StallEventBroadcast verifies that a triggered stall broadcasts
// EventAgentStallRestart with the correct session_id and stall_type fields.
func TestCheckStall_StallEventBroadcast(t *testing.T) {
	hub := ws.NewHub(clock.Real())
	go hub.Run()
	defer hub.Stop()

	client, ch := ws.NewTestClient(hub, "ws-stall-positive")
	hub.Register(client)
	hub.Subscribe(client, "proj-pos", "T-pos")

	clk := clock.NewTest(time.Now())
	s := New(Config{Clock: clk, WSHub: hub})

	doneCh := make(chan struct{})
	close(doneCh)
	proc := &processInfo{
		cmd:                &exec.Cmd{},
		backend:            &cliBackend{},
		doneCh:             doneCh,
		sessionID:          "stall-pos-sess",
		agentType:          "implementor",
		modelID:            "claude:sonnet",
		projectID:          "proj-pos",
		ticketID:           "T-pos",
		workflowName:       "feature",
		hasReceivedMessage: false,
		lastMessageTime:    clk.Now(),
		stallStartTimeout:  2 * time.Minute,
		stallRestartCount:  0,
		pendingMessages:    make([]repo.MessageEntry, 0),
	}

	clk.Advance(3 * time.Minute)
	s.checkStall(context.Background(), proc, SpawnRequest{
		ProjectID:    "proj-pos",
		TicketID:     "T-pos",
		WorkflowName: "feature",
	})

	select {
	case msg := <-ch:
		var event ws.Event
		if err := json.Unmarshal(msg, &event); err != nil {
			t.Fatalf("unmarshal event: %v", err)
		}
		if event.Type != ws.EventAgentStallRestart {
			t.Errorf("event.Type = %q, want %q", event.Type, ws.EventAgentStallRestart)
		}
		if v, _ := event.Data["session_id"].(string); v != "stall-pos-sess" {
			t.Errorf("session_id = %q, want stall-pos-sess", v)
		}
		if v, _ := event.Data["stall_type"].(string); v != "start" {
			t.Errorf("stall_type = %q, want start", v)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timeout waiting for EventAgentStallRestart broadcast")
	}
}

// TestCheckStall_RunningStall_EventBroadcast verifies the running-stall event
// carries stall_type="running" when triggered with hasReceivedMessage=true.
func TestCheckStall_RunningStall_EventBroadcast(t *testing.T) {
	hub := ws.NewHub(clock.Real())
	go hub.Run()
	defer hub.Stop()

	client, ch := ws.NewTestClient(hub, "ws-running-stall-pos")
	hub.Register(client)
	hub.Subscribe(client, "proj-rsp", "T-rsp")

	clk := clock.NewTest(time.Now())
	s := New(Config{Clock: clk, WSHub: hub})

	doneCh := make(chan struct{})
	close(doneCh)
	proc := &processInfo{
		cmd:                 &exec.Cmd{},
		backend:             &cliBackend{},
		doneCh:              doneCh,
		sessionID:           "running-stall-pos",
		agentType:           "qa-verifier",
		projectID:           "proj-rsp",
		ticketID:            "T-rsp",
		workflowName:        "feature",
		hasReceivedMessage:  true,
		lastMessageTime:     clk.Now(),
		stallRunningTimeout: 8 * time.Minute,
		stallRestartCount:   0,
		pendingMessages:     make([]repo.MessageEntry, 0),
	}

	clk.Advance(9 * time.Minute)
	s.checkStall(context.Background(), proc, SpawnRequest{
		ProjectID:    "proj-rsp",
		TicketID:     "T-rsp",
		WorkflowName: "feature",
	})

	select {
	case msg := <-ch:
		var event ws.Event
		if err := json.Unmarshal(msg, &event); err != nil {
			t.Fatalf("unmarshal event: %v", err)
		}
		if event.Type != ws.EventAgentStallRestart {
			t.Errorf("event.Type = %q, want %q", event.Type, ws.EventAgentStallRestart)
		}
		if v, _ := event.Data["stall_type"].(string); v != "running" {
			t.Errorf("stall_type = %q, want running", v)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timeout waiting for EventAgentStallRestart broadcast")
	}
}
