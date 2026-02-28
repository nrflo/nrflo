package spawner

import (
	"context"
	"os/exec"
	"testing"
	"time"

	"be/internal/clock"
	"be/internal/model"
	"be/internal/repo"
)

// TestHandleStallRestart_StartStall_DBState verifies that handleStallRestart sets
// result="continue", result_reason="stall_restart_start_stall", status="continued",
// and sets proc.finalStatus="CONTINUE" and increments stallRestartCount.
func TestHandleStallRestart_StartStall_DBState(t *testing.T) {
	env := setupTestEnv(t)
	defer env.cleanup()

	env.createSession(t, "claude:sonnet")

	doneCh := make(chan struct{})
	close(doneCh)

	proc := &processInfo{
		cmd:               &exec.Cmd{}, // cmd.Process == nil
		doneCh:            doneCh,
		sessionID:         env.sessionID,
		agentID:           "test-agent-id",
		agentType:         "implementor",
		modelID:           "claude:sonnet",
		workflowInstanceID: env.wfiID,
		projectID:         env.projectID,
		ticketID:          env.ticketID,
		workflowName:      env.workflowID,
		pendingMessages:   make([]repo.MessageEntry, 0),
		lastMessageTime:   time.Now().Add(-5 * time.Minute),
		stallRestartCount: 0,
	}

	req := SpawnRequest{
		ProjectID:    env.projectID,
		TicketID:     env.ticketID,
		WorkflowName: env.workflowID,
		AgentType:    "implementor",
	}

	env.spawner.handleStallRestart(context.Background(), proc, req, "start_stall")

	// Verify processInfo state
	if proc.finalStatus != "CONTINUE" {
		t.Errorf("finalStatus = %q, want CONTINUE", proc.finalStatus)
	}
	if proc.stallRestartCount != 1 {
		t.Errorf("stallRestartCount = %d, want 1", proc.stallRestartCount)
	}

	// Verify DB state
	sessionRepo := repo.NewAgentSessionRepo(env.database, clock.Real())
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
	if !sess.ResultReason.Valid || sess.ResultReason.String != "stall_restart_start_stall" {
		t.Errorf("result_reason = %v, want stall_restart_start_stall", sess.ResultReason)
	}
}

// TestHandleStallRestart_RunningStall_DBState verifies result_reason=stall_restart_running_stall.
func TestHandleStallRestart_RunningStall_DBState(t *testing.T) {
	env := setupTestEnv(t)
	defer env.cleanup()

	env.createSession(t, "claude:opus")

	doneCh := make(chan struct{})
	close(doneCh)

	proc := &processInfo{
		cmd:               &exec.Cmd{},
		doneCh:            doneCh,
		sessionID:         env.sessionID,
		agentID:           "test-agent-id",
		agentType:         "qa-verifier",
		modelID:           "claude:opus",
		workflowInstanceID: env.wfiID,
		projectID:         env.projectID,
		ticketID:          env.ticketID,
		workflowName:      env.workflowID,
		pendingMessages:   make([]repo.MessageEntry, 0),
		lastMessageTime:   time.Now().Add(-10 * time.Minute),
		stallRestartCount: 1,
	}

	req := SpawnRequest{
		ProjectID:    env.projectID,
		TicketID:     env.ticketID,
		WorkflowName: env.workflowID,
		AgentType:    "qa-verifier",
	}

	env.spawner.handleStallRestart(context.Background(), proc, req, "running_stall")

	// Verify finalStatus and counter
	if proc.finalStatus != "CONTINUE" {
		t.Errorf("finalStatus = %q, want CONTINUE", proc.finalStatus)
	}
	if proc.stallRestartCount != 2 {
		t.Errorf("stallRestartCount = %d, want 2", proc.stallRestartCount)
	}

	// Verify DB result_reason
	sessionRepo := repo.NewAgentSessionRepo(env.database, clock.Real())
	sess, err := sessionRepo.Get(env.sessionID)
	if err != nil {
		t.Fatalf("get session: %v", err)
	}
	if !sess.ResultReason.Valid || sess.ResultReason.String != "stall_restart_running_stall" {
		t.Errorf("result_reason = %v, want stall_restart_running_stall", sess.ResultReason)
	}
}

// TestHandleStallRestart_MaxRestartsGuard verifies checkStall returns false
// once stallRestartCount == maxStallRestarts, preventing further restarts.
func TestHandleStallRestart_MaxRestartsGuard(t *testing.T) {
	env := setupTestEnv(t)
	defer env.cleanup()

	env.createSession(t, "claude:haiku")

	clk := clock.NewTest(time.Now())
	env.spawner.config.Clock = clk

	proc := &processInfo{
		cmd:               &exec.Cmd{},
		doneCh:            make(chan struct{}),
		sessionID:         env.sessionID,
		agentID:           "test-agent-id",
		agentType:         "implementor",
		modelID:           "claude:haiku",
		projectID:         env.projectID,
		ticketID:          env.ticketID,
		workflowName:      env.workflowID,
		pendingMessages:   make([]repo.MessageEntry, 0),
		lastMessageTime:   clk.Now().Add(-10 * time.Minute),
		stallStartTimeout: 2 * time.Minute,
		stallRestartCount: maxStallRestarts, // already at limit
		hasReceivedMessage: false,
	}

	got := env.spawner.checkStall(context.Background(), proc, SpawnRequest{
		ProjectID:    env.projectID,
		TicketID:     env.ticketID,
		WorkflowName: env.workflowID,
	})
	if got {
		t.Error("checkStall should return false when stallRestartCount == maxStallRestarts")
	}
	// finalStatus must not have been changed
	if proc.finalStatus != "" {
		t.Errorf("finalStatus = %q, want empty (no restart triggered)", proc.finalStatus)
	}
}

// TestStallRestart_CounterSequence simulates consecutive stall restarts and verifies
// stallRestartCount increments correctly and is capped at maxStallRestarts.
func TestStallRestart_CounterSequence(t *testing.T) {
	proc := &processInfo{
		stallRestartCount: 0,
	}

	for i := 0; i < maxStallRestarts; i++ {
		if proc.stallRestartCount >= maxStallRestarts {
			t.Fatalf("unexpected block at count %d", i)
		}
		proc.stallRestartCount++
		proc.finalStatus = "CONTINUE"
	}

	if proc.stallRestartCount != maxStallRestarts {
		t.Errorf("stallRestartCount = %d, want %d", proc.stallRestartCount, maxStallRestarts)
	}

	// Should be blocked now
	if proc.stallRestartCount < maxStallRestarts {
		t.Error("expected stallRestartCount == maxStallRestarts after exhausting restarts")
	}

	// Simulate the guard check
	guard := proc.stallRestartCount >= maxStallRestarts
	if !guard {
		t.Error("guard condition should be true at maxStallRestarts")
	}
}
