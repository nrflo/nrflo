package spawner

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"be/internal/clock"
	"be/internal/ws"
)

// TestDefaultFailRetryDelay_Is15Seconds verifies the retry delay constant is 15 seconds.
func TestDefaultFailRetryDelay_Is15Seconds(t *testing.T) {
	if defaultFailRetryDelay != 15*time.Second {
		t.Errorf("defaultFailRetryDelay = %v, want 15s", defaultFailRetryDelay)
	}
}

// TestWaitBeforeRetry_CancelledContext_ReturnsFalse verifies that an already-cancelled
// context causes waitBeforeRetry to return false without waiting the full 15s delay.
func TestWaitBeforeRetry_CancelledContext_ReturnsFalse(t *testing.T) {
	hub := ws.NewHub(clock.Real())
	go hub.Run()
	defer hub.Stop()

	s := New(Config{WSHub: hub, Clock: clock.Real()})
	proc := &processInfo{
		agentType:        "implementor",
		sessionID:        "sess-cancel",
		modelID:          "claude:sonnet",
		projectID:        "proj-1",
		ticketID:         "ticket-1",
		workflowName:     "feature",
		maxFailRestarts:  2,
		failRestartCount: 0,
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // already cancelled before call

	start := time.Now()
	got := s.waitBeforeRetry(ctx, proc)
	elapsed := time.Since(start)

	if got {
		t.Error("waitBeforeRetry should return false when context is cancelled")
	}
	// Should return well under the 15s delay; 500ms is a generous bound.
	if elapsed > 500*time.Millisecond {
		t.Errorf("waitBeforeRetry took %v with cancelled context, want <500ms", elapsed)
	}
}

// TestWaitBeforeRetry_BroadcastEvent_AllFields verifies that waitBeforeRetry broadcasts
// an agent.retry_waiting event with all required payload fields.
func TestWaitBeforeRetry_BroadcastEvent_AllFields(t *testing.T) {
	hub := ws.NewHub(clock.Real())
	go hub.Run()
	defer hub.Stop()

	client, ch := ws.NewTestClient(hub, "client-retry-allfields")
	hub.Register(client)
	hub.Subscribe(client, "proj-retry", "ticket-retry")

	s := New(Config{WSHub: hub, Clock: clock.Real()})
	proc := &processInfo{
		agentType:        "implementor",
		sessionID:        "sess-retry-event",
		modelID:          "claude:sonnet",
		projectID:        "proj-retry",
		ticketID:         "ticket-retry",
		workflowName:     "feature",
		maxFailRestarts:  3,
		failRestartCount: 1,
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	s.waitBeforeRetry(ctx, proc)

	select {
	case msg := <-ch:
		var event ws.Event
		if err := json.Unmarshal(msg, &event); err != nil {
			t.Fatalf("unmarshal event: %v", err)
		}
		if event.Type != ws.EventAgentRetryWaiting {
			t.Errorf("event.Type = %q, want %q", event.Type, ws.EventAgentRetryWaiting)
		}
		retryAssertStringField(t, event.Data, "agent_type", "implementor")
		retryAssertStringField(t, event.Data, "session_id", "sess-retry-event")
		retryAssertStringField(t, event.Data, "model_id", "claude:sonnet")
		retryAssertIntField(t, event.Data, "delay_seconds", 15)
		retryAssertIntField(t, event.Data, "fail_restart_count", 1)
		retryAssertIntField(t, event.Data, "max_fail_restarts", 3)
	case <-time.After(500 * time.Millisecond):
		t.Fatal("timeout waiting for agent.retry_waiting broadcast")
	}
}

// TestWaitBeforeRetry_NoHub_CancelledContext verifies that waitBeforeRetry returns
// false with a cancelled context when no hub is configured (nil hub, no panic).
func TestWaitBeforeRetry_NoHub_CancelledContext(t *testing.T) {
	s := New(Config{Clock: clock.Real()}) // WSHub intentionally nil

	proc := &processInfo{
		agentType:        "implementor",
		sessionID:        "sess-no-hub",
		modelID:          "claude:haiku",
		projectID:        "proj-1",
		ticketID:         "ticket-1",
		workflowName:     "feature",
		maxFailRestarts:  1,
		failRestartCount: 0,
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	got := s.waitBeforeRetry(ctx, proc)
	if got {
		t.Error("waitBeforeRetry should return false when context is cancelled (nil hub)")
	}
}

// TestWaitBeforeRetry_EventConstant_Format verifies EventAgentRetryWaiting matches
// the resource.action naming convention used by all event constants.
func TestWaitBeforeRetry_EventConstant_Format(t *testing.T) {
	const want = "agent.retry_waiting"
	if ws.EventAgentRetryWaiting != want {
		t.Errorf("EventAgentRetryWaiting = %q, want %q", ws.EventAgentRetryWaiting, want)
	}
}

// TestWaitBeforeRetry_BroadcastPayload_DelayMatchesConstant verifies delay_seconds
// in the broadcast equals int(defaultFailRetryDelay.Seconds()), not a hardcoded value.
func TestWaitBeforeRetry_BroadcastPayload_DelayMatchesConstant(t *testing.T) {
	hub := ws.NewHub(clock.Real())
	go hub.Run()
	defer hub.Stop()

	client, ch := ws.NewTestClient(hub, "client-delay-const")
	hub.Register(client)
	hub.Subscribe(client, "proj-delay", "ticket-delay")

	s := New(Config{WSHub: hub, Clock: clock.Real()})
	proc := &processInfo{
		projectID:    "proj-delay",
		ticketID:     "ticket-delay",
		workflowName: "feature",
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	s.waitBeforeRetry(ctx, proc)

	select {
	case msg := <-ch:
		var event ws.Event
		if err := json.Unmarshal(msg, &event); err != nil {
			t.Fatalf("unmarshal event: %v", err)
		}
		retryAssertIntField(t, event.Data, "delay_seconds", int(defaultFailRetryDelay.Seconds()))
	case <-time.After(500 * time.Millisecond):
		t.Fatal("timeout waiting for agent.retry_waiting event")
	}
}

// TestWaitBeforeRetry_CounterValues_ReflectedInEvent verifies fail_restart_count and
// max_fail_restarts are taken from the processInfo at call time, not hardcoded.
func TestWaitBeforeRetry_CounterValues_ReflectedInEvent(t *testing.T) {
	tests := []struct {
		name             string
		failRestartCount int
		maxFailRestarts  int
	}{
		{"first_retry", 0, 1},
		{"second_retry", 1, 3},
		{"last_retry", 2, 3},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			hub := ws.NewHub(clock.Real())
			go hub.Run()
			defer hub.Stop()

			client, ch := ws.NewTestClient(hub, "client-counter-"+tt.name)
			hub.Register(client)
			hub.Subscribe(client, "proj-ctr", "ticket-ctr")

			s := New(Config{WSHub: hub, Clock: clock.Real()})
			proc := &processInfo{
				projectID:        "proj-ctr",
				ticketID:         "ticket-ctr",
				workflowName:     "feature",
				failRestartCount: tt.failRestartCount,
				maxFailRestarts:  tt.maxFailRestarts,
			}

			ctx, cancel := context.WithCancel(context.Background())
			cancel()
			s.waitBeforeRetry(ctx, proc)

			select {
			case msg := <-ch:
				var event ws.Event
				if err := json.Unmarshal(msg, &event); err != nil {
					t.Fatalf("unmarshal event: %v", err)
				}
				retryAssertIntField(t, event.Data, "fail_restart_count", tt.failRestartCount)
				retryAssertIntField(t, event.Data, "max_fail_restarts", tt.maxFailRestarts)
			case <-time.After(500 * time.Millisecond):
				t.Fatal("timeout waiting for event")
			}
		})
	}
}

// retryAssertStringField asserts a string-valued field in an event data map.
func retryAssertStringField(t *testing.T, data map[string]interface{}, key, want string) {
	t.Helper()
	v, ok := data[key]
	if !ok {
		t.Errorf("data[%q] is missing", key)
		return
	}
	got, ok := v.(string)
	if !ok {
		t.Errorf("data[%q] = %T(%v), want string %q", key, v, v, want)
		return
	}
	if got != want {
		t.Errorf("data[%q] = %q, want %q", key, got, want)
	}
}

// retryAssertIntField asserts a numeric field in an event data map.
// JSON numbers decode as float64 when target is interface{}.
func retryAssertIntField(t *testing.T, data map[string]interface{}, key string, want int) {
	t.Helper()
	v, ok := data[key]
	if !ok {
		t.Errorf("data[%q] is missing", key)
		return
	}
	// JSON roundtrip produces float64; direct map construction may produce int.
	switch val := v.(type) {
	case float64:
		if int(val) != want {
			t.Errorf("data[%q] = %d, want %d", key, int(val), want)
		}
	case int:
		if val != want {
			t.Errorf("data[%q] = %d, want %d", key, val, want)
		}
	default:
		t.Errorf("data[%q] = %T(%v), want numeric %d", key, v, v, want)
	}
}
