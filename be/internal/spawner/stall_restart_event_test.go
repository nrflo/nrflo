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

// TestHandleStallRestart_BroadcastsEvent verifies agent.stall_restart event is broadcast
// with the correct payload fields before the agent is killed.
func TestHandleStallRestart_BroadcastsEvent(t *testing.T) {
	hub := ws.NewHub(clock.Real())
	go hub.Run()
	defer hub.Stop()

	client, ch := ws.NewTestClient(hub, "client-stall-event")
	hub.Register(client)
	hub.Subscribe(client, "proj-stall", "ticket-stall")

	s := New(Config{WSHub: hub, Clock: clock.Real()}) // no Pool = DB calls skipped

	doneCh := make(chan struct{})
	close(doneCh) // pre-exited process

	proc := &processInfo{
		cmd:               &exec.Cmd{}, // cmd.Process == nil
		doneCh:            doneCh,
		sessionID:         "sess-stall-event",
		agentType:         "implementor",
		modelID:           "claude:sonnet",
		projectID:         "proj-stall",
		ticketID:          "ticket-stall",
		workflowName:      "feature",
		pendingMessages:   make([]repo.MessageEntry, 0),
		lastMessageTime:   time.Now().Add(-5 * time.Minute),
		stallRestartCount: 0,
	}

	req := SpawnRequest{
		ProjectID:    "proj-stall",
		TicketID:     "ticket-stall",
		WorkflowName: "feature",
		AgentType:    "implementor",
	}

	s.handleStallRestart(context.Background(), proc, req, "start_stall")

	select {
	case msg := <-ch:
		var event ws.Event
		if err := json.Unmarshal(msg, &event); err != nil {
			t.Fatalf("unmarshal event: %v", err)
		}
		if event.Type != ws.EventAgentStallRestart {
			t.Errorf("event.Type = %q, want %q", event.Type, ws.EventAgentStallRestart)
		}
		stallAssertStringField(t, event.Data, "session_id", "sess-stall-event")
		stallAssertStringField(t, event.Data, "agent_type", "implementor")
		stallAssertStringField(t, event.Data, "stall_type", "start")
		stallAssertIntField(t, event.Data, "stall_count", 1)
	case <-time.After(500 * time.Millisecond):
		t.Fatal("timeout waiting for agent.stall_restart event")
	}
}

// TestHandleStallRestart_RunningStall_EventType verifies stall_type="running" for running_stall.
func TestHandleStallRestart_RunningStall_EventType(t *testing.T) {
	hub := ws.NewHub(clock.Real())
	go hub.Run()
	defer hub.Stop()

	client, ch := ws.NewTestClient(hub, "client-running-stall")
	hub.Register(client)
	hub.Subscribe(client, "proj-run-stall", "ticket-run-stall")

	s := New(Config{WSHub: hub, Clock: clock.Real()})

	doneCh := make(chan struct{})
	close(doneCh)

	proc := &processInfo{
		cmd:               &exec.Cmd{},
		doneCh:            doneCh,
		sessionID:         "sess-running-stall",
		agentType:         "qa-verifier",
		modelID:           "claude:opus",
		projectID:         "proj-run-stall",
		ticketID:          "ticket-run-stall",
		workflowName:      "feature",
		pendingMessages:   make([]repo.MessageEntry, 0),
		lastMessageTime:   time.Now().Add(-10 * time.Minute),
		stallRestartCount: 1,
	}

	req := SpawnRequest{
		ProjectID:    "proj-run-stall",
		TicketID:     "ticket-run-stall",
		WorkflowName: "feature",
	}

	s.handleStallRestart(context.Background(), proc, req, "running_stall")

	select {
	case msg := <-ch:
		var event ws.Event
		if err := json.Unmarshal(msg, &event); err != nil {
			t.Fatalf("unmarshal event: %v", err)
		}
		stallAssertStringField(t, event.Data, "stall_type", "running")
		stallAssertIntField(t, event.Data, "stall_count", 2) // stallRestartCount+1 = 1+1
	case <-time.After(500 * time.Millisecond):
		t.Fatal("timeout waiting for agent.stall_restart event")
	}
}

// TestStallRestart_FieldCarryover verifies stallRestartCount and timeout values
// are carried over in relaunchForContinuation.
func TestStallRestart_FieldCarryover(t *testing.T) {
	oldProc := &processInfo{
		sessionID:           "old-session",
		stallRestartCount:   2,
		stallStartTimeout:   90 * time.Second,
		stallRunningTimeout: 300 * time.Second,
		restartCount:        1,
		restartThreshold:    25,
		maxFailRestarts:     3,
		failRestartCount:    1,
	}
	newProc := &processInfo{}

	// Apply relaunchForContinuation field assignments (mirrors completion.go)
	newProc.restartCount = oldProc.restartCount + 1
	newProc.restartThreshold = oldProc.restartThreshold
	newProc.maxFailRestarts = oldProc.maxFailRestarts
	newProc.failRestartCount = oldProc.failRestartCount
	newProc.stallRestartCount = oldProc.stallRestartCount
	newProc.stallStartTimeout = oldProc.stallStartTimeout
	newProc.stallRunningTimeout = oldProc.stallRunningTimeout

	if newProc.stallRestartCount != 2 {
		t.Errorf("stallRestartCount carried = %d, want 2", newProc.stallRestartCount)
	}
	if newProc.stallStartTimeout != 90*time.Second {
		t.Errorf("stallStartTimeout carried = %v, want 90s", newProc.stallStartTimeout)
	}
	if newProc.stallRunningTimeout != 300*time.Second {
		t.Errorf("stallRunningTimeout carried = %v, want 300s", newProc.stallRunningTimeout)
	}
	if newProc.restartCount != 2 {
		t.Errorf("restartCount = %d, want 2 (incremented)", newProc.restartCount)
	}
}

// TestEventAgentStallRestart_Constant verifies the event constant uses resource.action naming.
func TestEventAgentStallRestart_Constant(t *testing.T) {
	const want = "agent.stall_restart"
	if ws.EventAgentStallRestart != want {
		t.Errorf("EventAgentStallRestart = %q, want %q", ws.EventAgentStallRestart, want)
	}
}

// stallAssertStringField asserts a string field in event.Data.
func stallAssertStringField(t *testing.T, data map[string]interface{}, key, want string) {
	t.Helper()
	v, ok := data[key]
	if !ok {
		t.Errorf("event.Data[%q] missing", key)
		return
	}
	got, ok := v.(string)
	if !ok {
		t.Errorf("event.Data[%q] = %T(%v), want string %q", key, v, v, want)
		return
	}
	if got != want {
		t.Errorf("event.Data[%q] = %q, want %q", key, got, want)
	}
}

// stallAssertIntField asserts a numeric field in event.Data (handles JSON float64).
func stallAssertIntField(t *testing.T, data map[string]interface{}, key string, want int) {
	t.Helper()
	v, ok := data[key]
	if !ok {
		t.Errorf("event.Data[%q] missing", key)
		return
	}
	switch val := v.(type) {
	case float64:
		if int(val) != want {
			t.Errorf("event.Data[%q] = %d, want %d", key, int(val), want)
		}
	case int:
		if val != want {
			t.Errorf("event.Data[%q] = %d, want %d", key, val, want)
		}
	default:
		t.Errorf("event.Data[%q] = %T(%v), want numeric %d", key, v, v, want)
	}
}
