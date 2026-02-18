package spawner

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"be/internal/clock"
	"be/internal/model"
	"be/internal/repo"
	"be/internal/ws"
)

// TestRequestTakeControl_SendsToChannel verifies that RequestTakeControl puts
// the sessionID onto takeControlCh in a non-blocking manner.
func TestRequestTakeControl_SendsToChannel(t *testing.T) {
	sp := New(Config{Clock: clock.Real()})

	sp.RequestTakeControl("sess-abc")

	select {
	case id := <-sp.takeControlCh:
		if id != "sess-abc" {
			t.Errorf("expected 'sess-abc', got %q", id)
		}
	default:
		t.Fatal("expected sessionID on takeControlCh, channel was empty")
	}
}

// TestRequestTakeControl_NonBlockingWhenFull verifies that calling RequestTakeControl
// a second time when the channel is full is a no-op (no panic, no block).
func TestRequestTakeControl_NonBlockingWhenFull(t *testing.T) {
	sp := New(Config{Clock: clock.Real()})

	sp.RequestTakeControl("first")
	// Channel is now full (capacity 1). Second call must not block or panic.
	sp.RequestTakeControl("second")

	select {
	case id := <-sp.takeControlCh:
		if id != "first" {
			t.Errorf("expected 'first', got %q", id)
		}
	default:
		t.Fatal("expected 'first' on takeControlCh")
	}

	// Channel should be empty now — 'second' was dropped.
	select {
	case id := <-sp.takeControlCh:
		t.Errorf("expected channel empty, got %q", id)
	default:
	}
}

// TestCompleteInteractive_ClosesChannel verifies that CompleteInteractive closes
// the wait channel registered under the given sessionID.
func TestCompleteInteractive_ClosesChannel(t *testing.T) {
	sp := New(Config{Clock: clock.Real()})

	waitCh := make(chan struct{})
	sp.mu.Lock()
	sp.interactiveWaits["sess-xyz"] = waitCh
	sp.mu.Unlock()

	sp.CompleteInteractive("sess-xyz")

	select {
	case <-waitCh:
		// closed — good
	case <-time.After(500 * time.Millisecond):
		t.Fatal("CompleteInteractive did not close the wait channel")
	}
}

// TestCompleteInteractive_IdempotentDoubleClose verifies that calling
// CompleteInteractive twice on the same sessionID does not panic.
func TestCompleteInteractive_IdempotentDoubleClose(t *testing.T) {
	sp := New(Config{Clock: clock.Real()})

	waitCh := make(chan struct{})
	sp.mu.Lock()
	sp.interactiveWaits["sess-dup"] = waitCh
	sp.mu.Unlock()

	sp.CompleteInteractive("sess-dup")
	// Second call must not panic (channel already closed).
	sp.CompleteInteractive("sess-dup")
}

// TestCompleteInteractive_UnknownSession verifies that calling CompleteInteractive
// for a session that has no registered wait channel is a no-op.
func TestCompleteInteractive_UnknownSession(t *testing.T) {
	sp := New(Config{Clock: clock.Real()})
	// No wait channel registered — must not panic.
	sp.CompleteInteractive("no-such-session")
}

// TestRegisterAgentStopWithReason_UserInteractive documents the production bug:
// the DB schema is missing a migration to allow 'user_interactive' and
// 'interactive_completed' as valid values for the status and result columns.
// The CHECK constraints reject these values, causing all DB updates in
// registerAgentStopWithReason to fail silently (errors are not checked).
//
// Production bug: a migration is missing. See be_production_bugs in ticket findings.
func TestRegisterAgentStopWithReason_UserInteractive_DBConstraintBug(t *testing.T) {
	env := setupTestEnv(t)
	defer env.cleanup()

	env.createSession(t, "claude:sonnet")

	// Verify that updating status to user_interactive fails with a DB constraint.
	sessionRepo := repo.NewAgentSessionRepo(env.database, clock.Real())
	err := sessionRepo.UpdateStatus(env.sessionID, model.AgentSessionUserInteractive)
	if err == nil {
		t.Fatal("expected UpdateStatus to fail for user_interactive (missing migration)")
	}
	// The error should mention a constraint violation.
	if !containsConstraintError(err) {
		t.Errorf("expected constraint error, got: %v", err)
	}

	// Similarly, UpdateStatusToInteractiveCompleted should fail.
	err2 := sessionRepo.UpdateStatusToInteractiveCompleted(env.sessionID)
	if err2 == nil {
		t.Fatal("expected UpdateStatusToInteractiveCompleted to fail (missing migration)")
	}
}

// TestRegisterAgentStopWithReason_UserInteractive_BroadcastsEvent verifies
// that registerAgentStopWithReason broadcasts an EventAgentCompleted event
// with result=user_interactive when hub is configured.
// NOTE: The DB updates fail silently due to the missing migration, but the
// WS broadcast still happens (it uses the function parameter, not DB state).
func TestRegisterAgentStopWithReason_UserInteractive_BroadcastsEvent(t *testing.T) {
	env := setupTestEnv(t)
	defer env.cleanup()

	env.createSession(t, "claude:sonnet")

	hub := ws.NewHub(clock.Real())
	go hub.Run()
	defer hub.Stop()

	client, ch := ws.NewTestClient(hub, "test-ws-client")
	hub.Register(client)
	time.Sleep(50 * time.Millisecond)
	hub.Subscribe(client, env.projectID, env.ticketID)
	time.Sleep(50 * time.Millisecond)

	env.spawner.config.WSHub = hub

	env.spawner.registerAgentStopWithReason(
		env.projectID, env.ticketID, env.workflowID,
		env.sessionID, "agent-id-2",
		"user_interactive", "take_control", "claude:sonnet",
	)

	// Expect EventAgentCompleted with result=user_interactive in WS event data.
	deadline := time.After(2 * time.Second)
	for {
		select {
		case msg := <-ch:
			var event ws.Event
			if err := json.Unmarshal(msg, &event); err != nil {
				continue
			}
			if event.Type == ws.EventAgentCompleted {
				result, _ := event.Data["result"].(string)
				if result != "user_interactive" {
					t.Errorf("expected result='user_interactive' in WS event, got %q", result)
				}
				return
			}
		case <-deadline:
			t.Fatal("timeout waiting for EventAgentCompleted with result=user_interactive")
		}
	}
}

// TestTakeControlBroadcastsEventAgentTakeControl verifies that the
// EventAgentTakeControl broadcast carries the correct fields.
func TestTakeControlBroadcastsEventAgentTakeControl(t *testing.T) {
	env := setupTestEnv(t)
	defer env.cleanup()

	hub := ws.NewHub(clock.Real())
	go hub.Run()
	defer hub.Stop()

	client, ch := ws.NewTestClient(hub, "ws-take-ctrl")
	hub.Register(client)
	time.Sleep(50 * time.Millisecond)
	hub.Subscribe(client, env.projectID, env.ticketID)
	time.Sleep(50 * time.Millisecond)

	env.spawner.config.WSHub = hub

	// Simulate what monitorAll does when take-control is processed.
	env.spawner.broadcast(ws.EventAgentTakeControl, env.projectID, env.ticketID, env.workflowID, map[string]interface{}{
		"session_id": env.sessionID,
		"agent_type": "test-agent",
		"model_id":   "claude:sonnet",
	})

	deadline := time.After(2 * time.Second)
	for {
		select {
		case msg := <-ch:
			var event ws.Event
			if err := json.Unmarshal(msg, &event); err != nil {
				continue
			}
			if event.Type == ws.EventAgentTakeControl {
				sessID, _ := event.Data["session_id"].(string)
				if sessID != env.sessionID {
					t.Errorf("expected session_id=%q, got %q", env.sessionID, sessID)
				}
				agentType, _ := event.Data["agent_type"].(string)
				if agentType != "test-agent" {
					t.Errorf("expected agent_type='test-agent', got %q", agentType)
				}
				modelID, _ := event.Data["model_id"].(string)
				if modelID != "claude:sonnet" {
					t.Errorf("expected model_id='claude:sonnet', got %q", modelID)
				}
				return
			}
		case <-deadline:
			t.Fatal("timeout waiting for EventAgentTakeControl")
		}
	}
}

// TestInteractiveWaits_ConcurrentAccess verifies that registering and completing
// multiple interactive waits concurrently does not race or panic.
func TestInteractiveWaits_ConcurrentAccess(t *testing.T) {
	sp := New(Config{Clock: clock.Real()})

	sessions := []string{"sess-1", "sess-2", "sess-3"}
	done := make(chan struct{}, len(sessions))

	for _, sid := range sessions {
		waitCh := make(chan struct{})
		sp.mu.Lock()
		sp.interactiveWaits[sid] = waitCh
		sp.mu.Unlock()

		go func(id string, ch chan struct{}) {
			sp.CompleteInteractive(id)
			<-ch
			done <- struct{}{}
		}(sid, waitCh)
	}

	deadline := time.After(2 * time.Second)
	for i := 0; i < len(sessions); i++ {
		select {
		case <-done:
		case <-deadline:
			t.Fatalf("timeout waiting for concurrent interactive completions (completed %d/%d)", i, len(sessions))
		}
	}
}

// containsConstraintError returns true if err looks like a DB CHECK constraint failure.
func containsConstraintError(err error) bool {
	if err == nil {
		return false
	}
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "constraint") || strings.Contains(msg, "check")
}
