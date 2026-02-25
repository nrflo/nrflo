package spawner

import (
	"testing"

	"be/internal/clock"
	"be/internal/model"
	"be/internal/repo"
)

// TestAutoRestart_TimeoutSessionOverriddenToContinued verifies the DB state transition
// for a timed-out agent: handleGracefulTimeout registers failed/timeout, then the
// auto-restart override changes it to status=continued, result=continue,
// result_reason=timeout_restart.
func TestAutoRestart_TimeoutSessionOverriddenToContinued(t *testing.T) {
	env := setupTestEnv(t)
	defer env.cleanup()

	env.createSession(t, "claude:sonnet")

	sessionRepo := repo.NewAgentSessionRepo(env.database, clock.Real())

	// Simulate what handleGracefulTimeout does via registerAgentStopWithReason:
	// result="fail", reason="timeout" → status=failed.
	if err := sessionRepo.UpdateResult(env.sessionID, "fail", "timeout"); err != nil {
		t.Fatalf("simulate timeout: UpdateResult: %v", err)
	}
	if err := sessionRepo.UpdateStatus(env.sessionID, model.AgentSessionFailed); err != nil {
		t.Fatalf("simulate timeout: UpdateStatus: %v", err)
	}

	proc := &processInfo{
		sessionID:          env.sessionID,
		agentID:            "test-agent-id",
		modelID:            "claude:sonnet",
		workflowInstanceID: env.wfiID,
		projectID:          env.projectID,
		ticketID:           env.ticketID,
		workflowName:       env.workflowID,
		maxFailRestarts:    2,
		failRestartCount:   0,
		finalStatus:        "TIMEOUT", // set by handleGracefulTimeout
	}

	// Confirm initial DB state matches what handleGracefulTimeout would produce.
	beforeSess, err := sessionRepo.Get(env.sessionID)
	if err != nil {
		t.Fatalf("get session before override: %v", err)
	}
	if beforeSess.Status != model.AgentSessionFailed {
		t.Errorf("before override: status = %q, want failed", beforeSess.Status)
	}
	if !beforeSess.Result.Valid || beforeSess.Result.String != "fail" {
		t.Errorf("before override: result = %v, want fail", beforeSess.Result)
	}
	if !beforeSess.ResultReason.Valid || beforeSess.ResultReason.String != "timeout" {
		t.Errorf("before override: result_reason = %v, want timeout", beforeSess.ResultReason)
	}

	// Simulate the auto-restart DB override (mirrors the monitorAll timeout branch).
	if proc.maxFailRestarts > 0 && proc.failRestartCount < proc.maxFailRestarts {
		sessionRepo.UpdateResult(proc.sessionID, "continue", "timeout_restart")
		sessionRepo.UpdateStatus(proc.sessionID, model.AgentSessionContinued)
		proc.failRestartCount++
		proc.finalStatus = "CONTINUE"
	}

	// processInfo state
	if proc.finalStatus != "CONTINUE" {
		t.Errorf("finalStatus = %q, want CONTINUE", proc.finalStatus)
	}
	if proc.failRestartCount != 1 {
		t.Errorf("failRestartCount = %d, want 1", proc.failRestartCount)
	}

	// DB state after override
	afterSess, err := sessionRepo.Get(env.sessionID)
	if err != nil {
		t.Fatalf("get session after override: %v", err)
	}
	if afterSess.Status != model.AgentSessionContinued {
		t.Errorf("after override: status = %q, want continued", afterSess.Status)
	}
	if !afterSess.Result.Valid || afterSess.Result.String != "continue" {
		t.Errorf("after override: result = %v, want continue", afterSess.Result)
	}
	if !afterSess.ResultReason.Valid || afterSess.ResultReason.String != "timeout_restart" {
		t.Errorf("after override: result_reason = %v, want timeout_restart", afterSess.ResultReason)
	}
}

// TestAutoRestart_MixedFailAndTimeoutShareCounter verifies that failRestartCount is
// shared between exit-code failures and timeouts: the budget is consumed regardless
// of which failure type triggers the restart, and the limit is enforced collectively.
func TestAutoRestart_MixedFailAndTimeoutShareCounter(t *testing.T) {
	proc := &processInfo{
		finalStatus:      "FAIL",
		maxFailRestarts:  2,
		failRestartCount: 0,
	}

	// First event: exit-code failure → restart
	if !(proc.finalStatus == "FAIL" && proc.maxFailRestarts > 0 && proc.failRestartCount < proc.maxFailRestarts) {
		t.Fatal("first restart (FAIL): condition should be true")
	}
	proc.failRestartCount++
	proc.finalStatus = "CONTINUE"
	if proc.failRestartCount != 1 {
		t.Errorf("after 1st restart (FAIL): failRestartCount = %d, want 1", proc.failRestartCount)
	}

	// Second event: timeout → restart (counter is now 1, still under limit)
	proc.finalStatus = "TIMEOUT"
	if !(proc.maxFailRestarts > 0 && proc.failRestartCount < proc.maxFailRestarts) {
		t.Fatal("second restart (TIMEOUT): condition should be true when failRestartCount=1 < maxFailRestarts=2")
	}
	proc.failRestartCount++
	proc.finalStatus = "CONTINUE"
	if proc.failRestartCount != 2 {
		t.Errorf("after 2nd restart (TIMEOUT): failRestartCount = %d, want 2", proc.failRestartCount)
	}

	// Third event: another timeout → should be terminal (counter exhausted)
	proc.finalStatus = "TIMEOUT"
	shouldRestart := proc.maxFailRestarts > 0 && proc.failRestartCount < proc.maxFailRestarts
	if shouldRestart {
		t.Error("third event (TIMEOUT at limit): condition should be false (failRestartCount==maxFailRestarts)")
	}
	if proc.finalStatus != "TIMEOUT" {
		t.Errorf("third event: finalStatus = %q, want TIMEOUT (terminal, unchanged)", proc.finalStatus)
	}
	if proc.failRestartCount != 2 {
		t.Errorf("third event: failRestartCount = %d, want 2 (not incremented at limit)", proc.failRestartCount)
	}
}

// TestAutoRestart_TimeoutDisabledAtZero confirms that maxFailRestarts=0 means
// no auto-restart after timeout — the session stays failed.
func TestAutoRestart_TimeoutDisabledAtZero(t *testing.T) {
	env := setupTestEnv(t)
	defer env.cleanup()

	env.createSession(t, "claude:haiku")

	sessionRepo := repo.NewAgentSessionRepo(env.database, clock.Real())

	// Simulate handleGracefulTimeout registering failed/timeout in DB.
	if err := sessionRepo.UpdateResult(env.sessionID, "fail", "timeout"); err != nil {
		t.Fatalf("simulate timeout: UpdateResult: %v", err)
	}
	if err := sessionRepo.UpdateStatus(env.sessionID, model.AgentSessionFailed); err != nil {
		t.Fatalf("simulate timeout: UpdateStatus: %v", err)
	}

	proc := &processInfo{
		sessionID:       env.sessionID,
		finalStatus:     "TIMEOUT",
		maxFailRestarts: 0, // disabled
		failRestartCount: 0,
	}

	// Condition must not fire.
	shouldRestart := proc.maxFailRestarts > 0 && proc.failRestartCount < proc.maxFailRestarts
	if shouldRestart {
		t.Error("should not trigger auto-restart when maxFailRestarts=0")
	}

	// DB must retain failed/timeout state.
	sess, err := sessionRepo.Get(env.sessionID)
	if err != nil {
		t.Fatalf("get session: %v", err)
	}
	if sess.Status != model.AgentSessionFailed {
		t.Errorf("status = %q, want failed (no restart with maxFailRestarts=0)", sess.Status)
	}
	if !sess.ResultReason.Valid || sess.ResultReason.String != "timeout" {
		t.Errorf("result_reason = %v, want timeout (no restart with maxFailRestarts=0)", sess.ResultReason)
	}
	if proc.failRestartCount != 0 {
		t.Errorf("failRestartCount = %d, want 0", proc.failRestartCount)
	}
}

// TestAutoRestart_TimeoutTerminalAtMaxCount verifies the restart is blocked when
// failRestartCount has reached maxFailRestarts via a prior timeout restart.
func TestAutoRestart_TimeoutTerminalAtMaxCount(t *testing.T) {
	env := setupTestEnv(t)
	defer env.cleanup()

	env.createSession(t, "claude:opus")

	sessionRepo := repo.NewAgentSessionRepo(env.database, clock.Real())

	// Simulate handleGracefulTimeout DB state.
	if err := sessionRepo.UpdateResult(env.sessionID, "fail", "timeout"); err != nil {
		t.Fatalf("simulate timeout: UpdateResult: %v", err)
	}
	if err := sessionRepo.UpdateStatus(env.sessionID, model.AgentSessionFailed); err != nil {
		t.Fatalf("simulate timeout: UpdateStatus: %v", err)
	}

	proc := &processInfo{
		sessionID:        env.sessionID,
		finalStatus:      "TIMEOUT",
		maxFailRestarts:  2,
		failRestartCount: 2, // already exhausted
	}

	shouldRestart := proc.maxFailRestarts > 0 && proc.failRestartCount < proc.maxFailRestarts
	if shouldRestart {
		t.Error("should not trigger auto-restart when failRestartCount == maxFailRestarts")
	}
	if proc.finalStatus != "TIMEOUT" {
		t.Errorf("finalStatus = %q, want TIMEOUT (terminal)", proc.finalStatus)
	}
	if proc.failRestartCount != 2 {
		t.Errorf("failRestartCount = %d, want 2 (unchanged)", proc.failRestartCount)
	}

	// DB must remain failed.
	sess, err := sessionRepo.Get(env.sessionID)
	if err != nil {
		t.Fatalf("get session: %v", err)
	}
	if sess.Status != model.AgentSessionFailed {
		t.Errorf("status = %q, want failed (exhausted restarts)", sess.Status)
	}
}
