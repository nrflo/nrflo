package spawner

import (
	"context"
	"encoding/json"
	"os/exec"
	"testing"
	"time"

	"be/internal/clock"
	"be/internal/model"
	"be/internal/repo"
	"be/internal/ws"
)

// TestHandleRateLimitRetry_BroadcastsEvent verifies agent.rate_limited event with all payload fields.
func TestHandleRateLimitRetry_BroadcastsEvent(t *testing.T) {
	t.Parallel()
	hub := ws.NewHub(clock.Real())
	go hub.Run()
	defer hub.Stop()

	client, ch := ws.NewTestClient(hub, "client-rl-event")
	hub.Register(client)
	hub.Subscribe(client, "proj-rl", "ticket-rl")

	clk := clock.NewTest(time.Now())
	s := New(Config{WSHub: hub, Clock: clk})

	doneCh := make(chan struct{})
	close(doneCh)

	proc := &processInfo{
		cmd:                 &exec.Cmd{},
		backend:             fakeBackend{name: "cli_interactive"},
		doneCh:              doneCh,
		sessionID:           "sess-rl-event",
		agentType:           "implementor",
		modelID:             "claude:sonnet",
		projectID:           "proj-rl",
		ticketID:            "ticket-rl",
		workflowName:        "feature",
		pendingMessages:     make([]repo.MessageEntry, 0),
		rateLimitRetryCount: 0,
		rateLimitTotalWait:  30 * time.Second,
		rateLimitConfig: rateLimitConfig{
			Enabled:        true,
			InitialBackoff: 60 * time.Second,
			MaxWait:        3600 * time.Second,
		},
	}

	req := SpawnRequest{ProjectID: "proj-rl", TicketID: "ticket-rl", WorkflowName: "feature"}

	s.handleRateLimitRetry(context.Background(), proc, req, "You've hit your limit")

	select {
	case msg := <-ch:
		var event ws.Event
		if err := json.Unmarshal(msg, &event); err != nil {
			t.Fatalf("unmarshal event: %v", err)
		}
		if event.Type != ws.EventAgentRateLimited {
			t.Errorf("event.Type = %q, want %q", event.Type, ws.EventAgentRateLimited)
		}
		rlAssertString(t, event.Data, "session_id", "sess-rl-event")
		rlAssertString(t, event.Data, "agent_type", "implementor")
		rlAssertString(t, event.Data, "matched_pattern", "You've hit your limit")
		// upcomingCount = 0+1=1, delay = InitialBackoff * 2^0 = 60s
		rlAssertInt(t, event.Data, "wait_seconds", 60)
		// total_wait = existing(30) + delay(60) = 90
		rlAssertInt(t, event.Data, "total_wait_seconds", 90)
		rlAssertInt(t, event.Data, "retry_count", 1)
	case <-time.After(500 * time.Millisecond):
		t.Fatal("timeout waiting for agent.rate_limited event")
	}
}

// TestHandleRateLimitRetry_DBState verifies DB status, result, result_reason, and rate-limit fields.
func TestHandleRateLimitRetry_DBState(t *testing.T) {
	t.Parallel()
	env := setupTestEnv(t)
	defer env.cleanup()
	env.createSession(t, "claude:sonnet")

	doneCh := make(chan struct{})
	close(doneCh)

	clk := clock.NewTest(time.Now())
	env.spawner.config.Clock = clk

	proc := &processInfo{
		cmd:                 &exec.Cmd{},
		backend:             fakeBackend{name: "cli_interactive"},
		doneCh:              doneCh,
		sessionID:           env.sessionID,
		agentID:             "test-agent-id",
		agentType:           "implementor",
		modelID:             "claude:sonnet",
		workflowInstanceID:  env.wfiID,
		projectID:           env.projectID,
		ticketID:            env.ticketID,
		workflowName:        env.workflowID,
		pendingMessages:     make([]repo.MessageEntry, 0),
		rateLimitRetryCount: 0,
		rateLimitTotalWait:  0,
		rateLimitConfig: rateLimitConfig{
			Enabled:        true,
			InitialBackoff: 60 * time.Second,
			MaxWait:        3600 * time.Second,
		},
	}

	req := SpawnRequest{
		ProjectID:    env.projectID,
		TicketID:     env.ticketID,
		WorkflowName: env.workflowID,
		AgentType:    "implementor",
	}

	env.spawner.handleRateLimitRetry(context.Background(), proc, req, "You've hit your limit")

	// Verify proc state.
	if proc.finalStatus != "CONTINUE" {
		t.Errorf("finalStatus = %q, want CONTINUE", proc.finalStatus)
	}
	if proc.rateLimitRetryCount != 1 {
		t.Errorf("rateLimitRetryCount = %d, want 1", proc.rateLimitRetryCount)
	}

	// Verify DB.
	sessionRepo := repo.NewAgentSessionRepo(env.database, clk)
	sess, err := sessionRepo.Get(env.sessionID)
	if err != nil {
		t.Fatalf("get session: %v", err)
	}
	if sess.Status != model.AgentSessionContinued {
		t.Errorf("status = %q, want continued", sess.Status)
	}
	if !sess.Result.Valid || sess.Result.String != "continue" {
		t.Errorf("result = %v, want continue", sess.Result)
	}
	if !sess.ResultReason.Valid || sess.ResultReason.String != "rate_limit" {
		t.Errorf("result_reason = %v, want rate_limit", sess.ResultReason)
	}
	if sess.RateLimitRetryCount != 1 {
		t.Errorf("rate_limit_retry_count = %d, want 1", sess.RateLimitRetryCount)
	}
	if !sess.LastRetryClass.Valid || sess.LastRetryClass.String != "You've hit your limit" {
		t.Errorf("last_retry_class = %v, want 'You've hit your limit'", sess.LastRetryClass)
	}
	if !sess.RateLimitUntilTs.Valid || sess.RateLimitUntilTs.String == "" {
		t.Errorf("rate_limit_until_ts not set")
	}
}

// TestHandleRateLimitRetry_FailRestartCountUnchanged verifies failRestartCount is not touched.
func TestHandleRateLimitRetry_FailRestartCountUnchanged(t *testing.T) {
	t.Parallel()
	s := New(Config{WSHub: nil, Clock: clock.Real()})

	doneCh := make(chan struct{})
	close(doneCh)

	proc := &processInfo{
		cmd:                 &exec.Cmd{},
		backend:             fakeBackend{name: "cli_interactive"},
		doneCh:              doneCh,
		sessionID:           "sess-frc",
		agentType:           "implementor",
		projectID:           "proj-1",
		ticketID:            "ticket-1",
		workflowName:        "feature",
		pendingMessages:     make([]repo.MessageEntry, 0),
		failRestartCount:    3,
		rateLimitRetryCount: 0,
		rateLimitConfig: rateLimitConfig{
			Enabled:        true,
			InitialBackoff: 60 * time.Second,
			MaxWait:        3600 * time.Second,
		},
	}

	req := SpawnRequest{ProjectID: "proj-1", TicketID: "ticket-1", WorkflowName: "feature"}
	s.handleRateLimitRetry(context.Background(), proc, req, "You've hit your limit")

	if proc.failRestartCount != 3 {
		t.Errorf("failRestartCount = %d, want 3 (unchanged)", proc.failRestartCount)
	}
	if proc.rateLimitRetryCount != 1 {
		t.Errorf("rateLimitRetryCount = %d, want 1 (incremented)", proc.rateLimitRetryCount)
	}
}

// TestHandleRateLimitRetry_CounterIncrements verifies multiple calls increment rateLimitRetryCount.
func TestHandleRateLimitRetry_CounterIncrements(t *testing.T) {
	t.Parallel()
	s := New(Config{WSHub: nil, Clock: clock.Real()})

	for n := 0; n < 3; n++ {
		doneCh := make(chan struct{})
		close(doneCh)
		proc := &processInfo{
			cmd:                 &exec.Cmd{},
			backend:             fakeBackend{name: "cli_interactive"},
			doneCh:              doneCh,
			sessionID:           "sess-incr",
			agentType:           "implementor",
			projectID:           "proj-1",
			ticketID:            "ticket-1",
			workflowName:        "feature",
			pendingMessages:     make([]repo.MessageEntry, 0),
			rateLimitRetryCount: n,
			rateLimitConfig: rateLimitConfig{
				Enabled:        true,
				InitialBackoff: 60 * time.Second,
				MaxWait:        3600 * time.Second,
			},
		}
		req := SpawnRequest{ProjectID: "proj-1", TicketID: "ticket-1", WorkflowName: "feature"}
		s.handleRateLimitRetry(context.Background(), proc, req, "You've hit your limit")
		if proc.rateLimitRetryCount != n+1 {
			t.Errorf("after call %d: rateLimitRetryCount = %d, want %d", n, proc.rateLimitRetryCount, n+1)
		}
	}
}

// TestEventAgentRateLimited_Constant verifies the event constant value.
func TestEventAgentRateLimited_Constant(t *testing.T) {
	t.Parallel()
	const want = "agent.rate_limited"
	if ws.EventAgentRateLimited != want {
		t.Errorf("EventAgentRateLimited = %q, want %q", ws.EventAgentRateLimited, want)
	}
}

// rlAssertString asserts a string field in event data.
func rlAssertString(t *testing.T, data map[string]interface{}, key, want string) {
	t.Helper()
	v, ok := data[key]
	if !ok {
		t.Errorf("data[%q] missing", key)
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

// rlAssertInt asserts a numeric field in event data.
func rlAssertInt(t *testing.T, data map[string]interface{}, key string, want int) {
	t.Helper()
	v, ok := data[key]
	if !ok {
		t.Errorf("data[%q] missing", key)
		return
	}
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
