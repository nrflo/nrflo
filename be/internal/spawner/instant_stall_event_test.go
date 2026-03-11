package spawner

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"be/internal/ws"
)

// TestCheckInstantStall_BroadcastsEvent verifies agent.instant_stall_restart is broadcast
// with the correct payload fields when all conditions are met.
func TestCheckInstantStall_BroadcastsEvent(t *testing.T) {
	env := setupTestEnv(t)
	defer env.cleanup()

	// Start the hub so events are delivered to subscribers.
	hub := env.spawner.config.WSHub
	go hub.Run()
	t.Cleanup(hub.Stop)

	client, ch := ws.NewTestClient(hub, "client-instant-stall-event")
	hub.Register(client)
	hub.Subscribe(client, env.projectID, env.ticketID)

	env.createSession(t, "claude:sonnet")
	insertTestMessages(t, env, 1)

	proc := makeInstantStallProc(env, "claude:sonnet", 15*time.Second, 0)
	env.spawner.checkInstantStall(context.Background(), proc, makeInstantStallReq(env))

	select {
	case msg := <-ch:
		var event ws.Event
		if err := json.Unmarshal(msg, &event); err != nil {
			t.Fatalf("unmarshal event: %v", err)
		}
		if event.Type != ws.EventAgentInstantStallRestart {
			t.Errorf("event.Type = %q, want %q", event.Type, ws.EventAgentInstantStallRestart)
		}
		stallAssertStringField(t, event.Data, "session_id", env.sessionID)
		stallAssertStringField(t, event.Data, "agent_type", "implementor")
		stallAssertStringField(t, event.Data, "elapsed", "15s")
		stallAssertIntField(t, event.Data, "message_count", 1)
		stallAssertIntField(t, event.Data, "stall_count", 1)
	case <-time.After(500 * time.Millisecond):
		t.Fatal("timeout waiting for agent.instant_stall_restart event")
	}
}

// TestCheckInstantStall_EventNotBroadcastWhenSkipped verifies no event is sent
// when the guard conditions are not met (non-Claude agent).
func TestCheckInstantStall_EventNotBroadcastWhenSkipped(t *testing.T) {
	env := setupTestEnv(t)
	defer env.cleanup()

	hub := env.spawner.config.WSHub
	go hub.Run()
	t.Cleanup(hub.Stop)

	client, ch := ws.NewTestClient(hub, "client-no-event")
	hub.Register(client)
	hub.Subscribe(client, env.projectID, env.ticketID)

	env.createSession(t, "opencode:opencode_gpt_normal")
	insertTestMessages(t, env, 1)

	proc := makeInstantStallProc(env, "opencode:opencode_gpt_normal", 15*time.Second, 0)
	env.spawner.checkInstantStall(context.Background(), proc, makeInstantStallReq(env))

	// No event should be received.
	select {
	case msg := <-ch:
		var event ws.Event
		if err := json.Unmarshal(msg, &event); err == nil && event.Type == ws.EventAgentInstantStallRestart {
			t.Errorf("unexpected agent.instant_stall_restart event for non-Claude agent")
		}
	case <-time.After(100 * time.Millisecond):
		// Expected: no event delivered.
	}
}

// TestCheckInstantStall_EventStallCountIncludesCurrentRestart verifies stall_count in the
// event payload reflects the post-increment value.
func TestCheckInstantStall_EventStallCountIncludesCurrentRestart(t *testing.T) {
	env := setupTestEnv(t)
	defer env.cleanup()

	hub := env.spawner.config.WSHub
	go hub.Run()
	t.Cleanup(hub.Stop)

	client, ch := ws.NewTestClient(hub, "client-stall-count")
	hub.Register(client)
	hub.Subscribe(client, env.projectID, env.ticketID)

	env.createSession(t, "claude:opus")
	insertTestMessages(t, env, 1)

	// Second stall restart (stallRestartCount starts at 1).
	proc := makeInstantStallProc(env, "claude:opus", 20*time.Second, 1)
	env.spawner.checkInstantStall(context.Background(), proc, makeInstantStallReq(env))

	select {
	case msg := <-ch:
		var event ws.Event
		if err := json.Unmarshal(msg, &event); err != nil {
			t.Fatalf("unmarshal event: %v", err)
		}
		if event.Type != ws.EventAgentInstantStallRestart {
			t.Errorf("event.Type = %q, want %q", event.Type, ws.EventAgentInstantStallRestart)
		}
		// stall_count is proc.stallRestartCount after increment: 1+1=2
		stallAssertIntField(t, event.Data, "stall_count", 2)
	case <-time.After(500 * time.Millisecond):
		t.Fatal("timeout waiting for agent.instant_stall_restart event")
	}
}

// TestEventAgentInstantStallRestart_Constant verifies the constant value matches the spec.
func TestEventAgentInstantStallRestart_Constant(t *testing.T) {
	const want = "agent.instant_stall_restart"
	if ws.EventAgentInstantStallRestart != want {
		t.Errorf("EventAgentInstantStallRestart = %q, want %q", ws.EventAgentInstantStallRestart, want)
	}
}

// TestCheckInstantStall_ElapsedFormattedInEvent verifies the elapsed field is formatted
// as seconds with no decimal places (e.g. "59s", not "59.123s").
func TestCheckInstantStall_ElapsedFormattedInEvent(t *testing.T) {
	env := setupTestEnv(t)
	defer env.cleanup()

	hub := env.spawner.config.WSHub
	go hub.Run()
	t.Cleanup(hub.Stop)

	client, ch := ws.NewTestClient(hub, "client-elapsed-format")
	hub.Register(client)
	hub.Subscribe(client, env.projectID, env.ticketID)

	env.createSession(t, "claude:haiku")
	insertTestMessages(t, env, 1)

	proc := makeInstantStallProc(env, "claude:haiku", 59*time.Second, 0)
	env.spawner.checkInstantStall(context.Background(), proc, makeInstantStallReq(env))

	select {
	case msg := <-ch:
		var event ws.Event
		if err := json.Unmarshal(msg, &event); err != nil {
			t.Fatalf("unmarshal event: %v", err)
		}
		stallAssertStringField(t, event.Data, "elapsed", "59s")
	case <-time.After(500 * time.Millisecond):
		t.Fatal("timeout waiting for event")
	}
}
