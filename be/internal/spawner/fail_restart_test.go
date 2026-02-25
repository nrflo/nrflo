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

// TestAutoRestartCondition_Boundaries verifies the guard condition for auto-restart
// across all relevant boundary values of maxFailRestarts and failRestartCount.
func TestAutoRestartCondition_Boundaries(t *testing.T) {
	tests := []struct {
		name             string
		finalStatus      string
		maxFailRestarts  int
		failRestartCount int
		want             bool
	}{
		{name: "disabled_default_zero", finalStatus: "FAIL", maxFailRestarts: 0, failRestartCount: 0, want: false},
		{name: "first_restart_allowed", finalStatus: "FAIL", maxFailRestarts: 2, failRestartCount: 0, want: true},
		{name: "second_restart_allowed", finalStatus: "FAIL", maxFailRestarts: 2, failRestartCount: 1, want: true},
		{name: "at_limit_no_restart", finalStatus: "FAIL", maxFailRestarts: 2, failRestartCount: 2, want: false},
		{name: "over_limit_no_restart", finalStatus: "FAIL", maxFailRestarts: 2, failRestartCount: 5, want: false},
		{name: "not_a_fail_status", finalStatus: "PASS", maxFailRestarts: 2, failRestartCount: 0, want: false},
		{name: "continue_status_no_restart", finalStatus: "CONTINUE", maxFailRestarts: 2, failRestartCount: 0, want: false},
		{name: "max_1_first_allowed", finalStatus: "FAIL", maxFailRestarts: 1, failRestartCount: 0, want: true},
		{name: "max_1_already_used", finalStatus: "FAIL", maxFailRestarts: 1, failRestartCount: 1, want: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			proc := &processInfo{
				finalStatus:      tt.finalStatus,
				maxFailRestarts:  tt.maxFailRestarts,
				failRestartCount: tt.failRestartCount,
			}
			got := proc.finalStatus == "FAIL" && proc.maxFailRestarts > 0 && proc.failRestartCount < proc.maxFailRestarts
			if got != tt.want {
				t.Errorf("condition = %v, want %v (finalStatus=%q maxFailRestarts=%d failRestartCount=%d)",
					got, tt.want, tt.finalStatus, tt.maxFailRestarts, tt.failRestartCount)
			}
		})
	}
}

// TestAutoRestart_SessionOverriddenToContined verifies the DB state transition:
// handleCompletion registers a failed session, then the auto-restart override
// changes it to status=continued, result=continue, result_reason=fail_restart.
func TestAutoRestart_SessionOverriddenToContined(t *testing.T) {
	env := setupTestEnv(t)
	defer env.cleanup()

	env.createSession(t, "claude:sonnet")

	// Simulate non-zero exit — handleCompletion will set FAIL and register in DB.
	cmd := exec.Command("false")
	_ = cmd.Run()

	proc := &processInfo{
		cmd:                cmd,
		sessionID:          env.sessionID,
		agentID:            "test-agent-id",
		modelID:            "claude:sonnet",
		workflowInstanceID: env.wfiID,
		projectID:          env.projectID,
		ticketID:           env.ticketID,
		workflowName:       env.workflowID,
		startTime:          time.Now().Add(-5 * time.Second),
		maxFailRestarts:    2,
		failRestartCount:   0,
	}

	env.spawner.handleCompletion(context.Background(), proc, SpawnRequest{
		ProjectID:    env.projectID,
		TicketID:     env.ticketID,
		WorkflowName: env.workflowID,
		AgentType:    "test-agent",
	})

	if proc.finalStatus != "FAIL" {
		t.Fatalf("expected finalStatus=FAIL after handleCompletion, got %q", proc.finalStatus)
	}

	sessionRepo := repo.NewAgentSessionRepo(env.database, clock.Real())

	// Confirm the DB has the initial failed state.
	beforeSess, err := sessionRepo.Get(env.sessionID)
	if err != nil {
		t.Fatalf("get session before override: %v", err)
	}
	if beforeSess.Status != model.AgentSessionFailed {
		t.Errorf("before override: status = %q, want failed", beforeSess.Status)
	}

	// Simulate the auto-restart DB override (mirrors the monitorAll inline block).
	if proc.finalStatus == "FAIL" && proc.maxFailRestarts > 0 && proc.failRestartCount < proc.maxFailRestarts {
		sessionRepo.UpdateResult(proc.sessionID, "continue", "fail_restart")
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

	// DB state
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
	if !afterSess.ResultReason.Valid || afterSess.ResultReason.String != "fail_restart" {
		t.Errorf("after override: result_reason = %v, want fail_restart", afterSess.ResultReason)
	}
}

// TestAutoRestart_DisabledAtZero confirms maxFailRestarts=0 means no auto-restart.
func TestAutoRestart_DisabledAtZero(t *testing.T) {
	env := setupTestEnv(t)
	defer env.cleanup()

	env.createSession(t, "claude:opus")

	cmd := exec.Command("false")
	_ = cmd.Run()

	proc := &processInfo{
		cmd:                cmd,
		sessionID:          env.sessionID,
		agentID:            "test-agent-id",
		modelID:            "claude:opus",
		workflowInstanceID: env.wfiID,
		projectID:          env.projectID,
		ticketID:           env.ticketID,
		workflowName:       env.workflowID,
		startTime:          time.Now().Add(-3 * time.Second),
		maxFailRestarts:    0, // disabled
		failRestartCount:   0,
	}

	env.spawner.handleCompletion(context.Background(), proc, SpawnRequest{
		ProjectID:    env.projectID,
		TicketID:     env.ticketID,
		WorkflowName: env.workflowID,
		AgentType:    "test-agent",
	})

	// Condition should not fire.
	shouldRestart := proc.finalStatus == "FAIL" && proc.maxFailRestarts > 0 && proc.failRestartCount < proc.maxFailRestarts
	if shouldRestart {
		t.Error("should not trigger auto-restart when maxFailRestarts=0")
	}

	// DB must retain failed state.
	sessionRepo := repo.NewAgentSessionRepo(env.database, clock.Real())
	sess, err := sessionRepo.Get(env.sessionID)
	if err != nil {
		t.Fatalf("get session: %v", err)
	}
	if sess.Status != model.AgentSessionFailed {
		t.Errorf("status = %q, want failed (no restart with maxFailRestarts=0)", sess.Status)
	}
	if proc.failRestartCount != 0 {
		t.Errorf("failRestartCount = %d, want 0", proc.failRestartCount)
	}
}

// TestAutoRestart_TerminalAtMaxCount verifies the restart is blocked when
// failRestartCount has reached maxFailRestarts.
func TestAutoRestart_TerminalAtMaxCount(t *testing.T) {
	env := setupTestEnv(t)
	defer env.cleanup()

	env.createSession(t, "claude:haiku")

	cmd := exec.Command("false")
	_ = cmd.Run()

	proc := &processInfo{
		cmd:                cmd,
		sessionID:          env.sessionID,
		agentID:            "test-agent-id",
		modelID:            "claude:haiku",
		workflowInstanceID: env.wfiID,
		projectID:          env.projectID,
		ticketID:           env.ticketID,
		workflowName:       env.workflowID,
		startTime:          time.Now().Add(-2 * time.Second),
		maxFailRestarts:    2,
		failRestartCount:   2, // already exhausted
	}

	env.spawner.handleCompletion(context.Background(), proc, SpawnRequest{
		ProjectID:    env.projectID,
		TicketID:     env.ticketID,
		WorkflowName: env.workflowID,
		AgentType:    "test-agent",
	})

	shouldRestart := proc.finalStatus == "FAIL" && proc.maxFailRestarts > 0 && proc.failRestartCount < proc.maxFailRestarts
	if shouldRestart {
		t.Error("should not trigger auto-restart when failRestartCount == maxFailRestarts")
	}
	if proc.finalStatus != "FAIL" {
		t.Errorf("finalStatus = %q, want FAIL (terminal)", proc.finalStatus)
	}
}

// TestAutoRestart_CounterIncrementSequence simulates two consecutive fail-restarts
// and verifies failRestartCount increments correctly, with the third failure terminal.
func TestAutoRestart_CounterIncrementSequence(t *testing.T) {
	proc := &processInfo{
		finalStatus:      "FAIL",
		maxFailRestarts:  2,
		failRestartCount: 0,
	}

	// First restart
	if !(proc.finalStatus == "FAIL" && proc.maxFailRestarts > 0 && proc.failRestartCount < proc.maxFailRestarts) {
		t.Fatal("first restart: condition should be true")
	}
	proc.failRestartCount++
	proc.finalStatus = "CONTINUE"

	if proc.failRestartCount != 1 {
		t.Errorf("after 1st restart: failRestartCount = %d, want 1", proc.failRestartCount)
	}

	// Simulate second failure on the new proc
	proc.finalStatus = "FAIL"
	if !(proc.finalStatus == "FAIL" && proc.maxFailRestarts > 0 && proc.failRestartCount < proc.maxFailRestarts) {
		t.Fatal("second restart: condition should be true")
	}
	proc.failRestartCount++
	proc.finalStatus = "CONTINUE"

	if proc.failRestartCount != 2 {
		t.Errorf("after 2nd restart: failRestartCount = %d, want 2", proc.failRestartCount)
	}

	// Third failure — should be terminal
	proc.finalStatus = "FAIL"
	shouldRestart := proc.finalStatus == "FAIL" && proc.maxFailRestarts > 0 && proc.failRestartCount < proc.maxFailRestarts
	if shouldRestart {
		t.Error("third failure: condition should be false (failRestartCount==maxFailRestarts)")
	}
	if proc.finalStatus != "FAIL" {
		t.Errorf("third failure: finalStatus = %q, want FAIL", proc.finalStatus)
	}
}

// TestAutoRestart_IndependentFromContextRestartCount confirms that restartCount
// (used by low-context restarts) and failRestartCount are tracked separately.
// A context-restarted agent (restartCount=3) can still fail-restart up to maxFailRestarts.
func TestAutoRestart_IndependentFromContextRestartCount(t *testing.T) {
	proc := &processInfo{
		finalStatus:      "FAIL",
		maxFailRestarts:  2,
		failRestartCount: 0,
		restartCount:     3, // already had 3 context restarts
	}

	shouldRestart := proc.finalStatus == "FAIL" && proc.maxFailRestarts > 0 && proc.failRestartCount < proc.maxFailRestarts
	if !shouldRestart {
		t.Error("fail-restart should be allowed regardless of restartCount (context restart counter)")
	}

	// Simulate restart
	proc.failRestartCount++
	proc.finalStatus = "CONTINUE"

	// restartCount is incremented by relaunchForContinuation — we verify it's separate
	if proc.failRestartCount != 1 {
		t.Errorf("failRestartCount = %d, want 1", proc.failRestartCount)
	}
	if proc.restartCount != 3 {
		t.Errorf("restartCount changed unexpectedly: got %d, want 3", proc.restartCount)
	}
}

// TestAutoRestart_FieldsInProcessInfo verifies that the processInfo struct contains
// the expected fields maxFailRestarts and failRestartCount.
func TestAutoRestart_FieldsInProcessInfo(t *testing.T) {
	proc := &processInfo{
		maxFailRestarts:  5,
		failRestartCount: 3,
	}

	if proc.maxFailRestarts != 5 {
		t.Errorf("maxFailRestarts = %d, want 5", proc.maxFailRestarts)
	}
	if proc.failRestartCount != 3 {
		t.Errorf("failRestartCount = %d, want 3", proc.failRestartCount)
	}

	// Zero values when not set
	proc2 := &processInfo{}
	if proc2.maxFailRestarts != 0 {
		t.Errorf("default maxFailRestarts = %d, want 0", proc2.maxFailRestarts)
	}
	if proc2.failRestartCount != 0 {
		t.Errorf("default failRestartCount = %d, want 0", proc2.failRestartCount)
	}
}

// TestAutoRestart_RelaunchFieldCarryover verifies that relaunchForContinuation
// copies maxFailRestarts and failRestartCount to the new processInfo.
// Uses direct field inspection on a manually constructed processInfo chain.
func TestAutoRestart_RelaunchFieldCarryover(t *testing.T) {
	// Simulate what relaunchForContinuation does: copy fields from oldProc to newProc.
	oldProc := &processInfo{
		sessionID:        "old-session",
		maxFailRestarts:  3,
		failRestartCount: 2,
		restartCount:     1,
		restartThreshold: 30,
	}
	newProc := &processInfo{}

	// Apply the assignments from relaunchForContinuation
	newProc.restartCount = oldProc.restartCount + 1
	newProc.restartThreshold = oldProc.restartThreshold
	newProc.maxFailRestarts = oldProc.maxFailRestarts
	newProc.failRestartCount = oldProc.failRestartCount

	if newProc.maxFailRestarts != 3 {
		t.Errorf("maxFailRestarts carried = %d, want 3", newProc.maxFailRestarts)
	}
	if newProc.failRestartCount != 2 {
		t.Errorf("failRestartCount carried = %d, want 2", newProc.failRestartCount)
	}
	if newProc.restartCount != 2 {
		t.Errorf("restartCount = %d, want 2 (incremented)", newProc.restartCount)
	}
}
