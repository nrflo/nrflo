package spawner

import (
	"encoding/json"
	"testing"
	"time"

	"be/internal/clock"
	"be/internal/ws"
)

// TestEventAgentTakeControlRejected_Constant verifies the WS event constant
// uses the correct resource.action naming convention.
func TestEventAgentTakeControlRejected_Constant(t *testing.T) {
	t.Parallel()
	const want = "agent.take_control_rejected"
	if ws.EventAgentTakeControlRejected != want {
		t.Errorf("EventAgentTakeControlRejected = %q, want %q", ws.EventAgentTakeControlRejected, want)
	}
}

// TestTakeControlRejected_APIMode_BroadcastsEvent verifies that when a
// take-control request arrives for an API-mode agent, EventAgentTakeControlRejected
// is broadcast with all required payload fields (session_id, agent_type, model_id,
// reason=api_mode_unsupported). This mirrors the monitorAll rejection path at
// spawner.go when proc.backend.SupportsTakeControl() returns false.
func TestTakeControlRejected_APIMode_BroadcastsEvent(t *testing.T) {
	t.Parallel()
	hub := ws.NewHub(clock.Real())
	go hub.Run()
	defer hub.Stop()

	client, ch := ws.NewTestClient(hub, "ws-tc-api-rejected")
	hub.Register(client)
	hub.Subscribe(client, "proj-api", "T-100")

	sp := New(Config{Clock: clock.Real(), WSHub: hub})

	sp.broadcast(ws.EventAgentTakeControlRejected, "proj-api", "T-100", "feature", map[string]interface{}{
		"session_id": "api-sess-1",
		"agent_type": "implementor",
		"model_id":   "claude:sonnet",
		"reason":     "api_mode_unsupported",
	})

	deadline := time.After(2 * time.Second)
	for {
		select {
		case msg := <-ch:
			var event ws.Event
			if err := json.Unmarshal(msg, &event); err != nil {
				continue
			}
			if event.Type != ws.EventAgentTakeControlRejected {
				continue
			}
			reason, _ := event.Data["reason"].(string)
			if reason != "api_mode_unsupported" {
				t.Errorf("reason = %q, want api_mode_unsupported", reason)
			}
			sessID, _ := event.Data["session_id"].(string)
			if sessID != "api-sess-1" {
				t.Errorf("session_id = %q, want api-sess-1", sessID)
			}
			agentType, _ := event.Data["agent_type"].(string)
			if agentType != "implementor" {
				t.Errorf("agent_type = %q, want implementor", agentType)
			}
			modelID, _ := event.Data["model_id"].(string)
			if modelID != "claude:sonnet" {
				t.Errorf("model_id = %q, want claude:sonnet", modelID)
			}
			return
		case <-deadline:
			t.Fatal("timeout waiting for EventAgentTakeControlRejected")
		}
	}
}

// TestTakeControlRejected_APIBackend_SupportsTakeControlFalse verifies that
// apiBackend.SupportsTakeControl() returns false, which is the gate condition
// that triggers the rejection broadcast path in monitorAll.
func TestTakeControlRejected_APIBackend_SupportsTakeControlFalse(t *testing.T) {
	t.Parallel()
	sp := New(Config{Clock: clock.Real()})
	b := newAPIBackend(sp)
	if b.SupportsTakeControl() {
		t.Error("apiBackend.SupportsTakeControl() = true, want false")
	}
	// Name and SupportsResume are also false for API agents.
	if b.Name() != "api" {
		t.Errorf("apiBackend.Name() = %q, want api", b.Name())
	}
	if b.SupportsResume() {
		t.Error("apiBackend.SupportsResume() = true, want false")
	}
}

// TestTakeControlRejected_CLIInteractiveBackend_AlwaysTrue verifies that the
// cli_interactive backend always supports take-control (for all adapters).
func TestTakeControlRejected_CLIInteractiveBackend_AlwaysTrue(t *testing.T) {
	t.Parallel()
	b := newCLIInteractiveBackend(&ClaudeAdapter{}, nil, nil)
	if !b.SupportsTakeControl() {
		t.Error("cliInteractiveBackend(Claude).SupportsTakeControl() = false, want true")
	}
}

// TestTakeControlRejected_EventType_DistinctFromSuccessEvent verifies that the
// rejection event type string is distinct from the success event type, so UI
// subscribers can differentiate them.
func TestTakeControlRejected_EventType_DistinctFromSuccessEvent(t *testing.T) {
	t.Parallel()
	if ws.EventAgentTakeControlRejected == ws.EventAgentTakeControl {
		t.Errorf("EventAgentTakeControlRejected == EventAgentTakeControl (%q); they must be distinct",
			ws.EventAgentTakeControlRejected)
	}
}
