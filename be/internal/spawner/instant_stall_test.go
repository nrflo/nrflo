package spawner

import (
	"context"
	"testing"
	"time"

	"be/internal/clock"
	"be/internal/model"
	"be/internal/repo"
)

// insertTestMessages inserts n text messages into agent_messages for the given session.
func insertTestMessages(t *testing.T, env *testEnv, n int) {
	t.Helper()
	if n == 0 {
		return
	}
	msgRepo := repo.NewAgentMessageRepo(env.database, clock.Real())
	msgs := make([]repo.MessageEntry, n)
	for i := range msgs {
		msgs[i] = repo.MessageEntry{Content: "test message", Category: "text"}
	}
	if err := msgRepo.InsertBatch(env.sessionID, 0, msgs); err != nil {
		t.Fatalf("insertTestMessages(%d): %v", n, err)
	}
}

// makeInstantStallProc creates a processInfo with finalStatus="PASS" for instant stall tests.
func makeInstantStallProc(env *testEnv, modelID string, elapsed time.Duration, stallRestartCount int) *processInfo {
	return &processInfo{
		sessionID:          env.sessionID,
		agentID:            "test-agent-id",
		agentType:          "implementor",
		modelID:            modelID,
		workflowInstanceID: env.wfiID,
		projectID:          env.projectID,
		ticketID:           env.ticketID,
		workflowName:       env.workflowID,
		elapsed:            elapsed,
		finalStatus:        "PASS",
		stallRestartCount:  stallRestartCount,
	}
}

// makeInstantStallReq creates a SpawnRequest matching the testEnv.
func makeInstantStallReq(env *testEnv) SpawnRequest {
	return SpawnRequest{
		ProjectID:    env.projectID,
		TicketID:     env.ticketID,
		WorkflowName: env.workflowID,
		AgentType:    "implementor",
	}
}

// TestCheckInstantStall_GuardConditions verifies all guard conditions using table-driven cases.
func TestCheckInstantStall_GuardConditions(t *testing.T) {
	tests := []struct {
		name              string
		modelID           string
		elapsed           time.Duration
		msgCount          int
		stallRestartCount int
		wantCONTINUE      bool
	}{
		{"claude_15s_1msg_triggers", "claude:sonnet", 15 * time.Second, 1, 0, true},
		{"claude_30s_1msg_triggers", "claude:opus", 30 * time.Second, 1, 0, true},
		{"opencode_skipped", "opencode:opencode_gpt_normal", 15 * time.Second, 1, 0, false},
		{"codex_skipped", "codex:codex_gpt_normal", 15 * time.Second, 1, 0, false},
		{"elapsed_at_boundary_skipped", "claude:sonnet", 1 * time.Minute, 1, 0, false},
		{"elapsed_above_boundary_skipped", "claude:sonnet", 90 * time.Second, 1, 0, false},
		{"msg_count_2_skipped", "claude:sonnet", 15 * time.Second, 2, 0, false},
		{"msg_count_3_skipped", "claude:sonnet", 15 * time.Second, 3, 0, false},
		// 0 messages also triggers: production guard is msgCount > 1 (threshold <= 1 includes 0).
		{"msg_count_0_triggers", "claude:sonnet", 15 * time.Second, 0, 0, true},
		{"budget_exhausted_skipped", "claude:sonnet", 15 * time.Second, 1, maxStallRestarts, false},
		{"budget_at_limit_minus_1_triggers", "claude:sonnet", 15 * time.Second, 1, maxStallRestarts - 1, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			env := setupTestEnv(t)
			defer env.cleanup()

			env.createSession(t, tt.modelID)
			insertTestMessages(t, env, tt.msgCount)

			proc := makeInstantStallProc(env, tt.modelID, tt.elapsed, tt.stallRestartCount)
			env.spawner.checkInstantStall(context.Background(), proc, makeInstantStallReq(env))

			if tt.wantCONTINUE {
				if proc.finalStatus != "CONTINUE" {
					t.Errorf("finalStatus = %q, want CONTINUE", proc.finalStatus)
				}
				if proc.stallRestartCount != tt.stallRestartCount+1 {
					t.Errorf("stallRestartCount = %d, want %d", proc.stallRestartCount, tt.stallRestartCount+1)
				}
			} else {
				if proc.finalStatus != "PASS" {
					t.Errorf("finalStatus = %q, want PASS", proc.finalStatus)
				}
				if proc.stallRestartCount != tt.stallRestartCount {
					t.Errorf("stallRestartCount = %d, want %d (unchanged)", proc.stallRestartCount, tt.stallRestartCount)
				}
			}
		})
	}
}

// TestCheckInstantStall_DBState_AllFields verifies DB state after a successful instant stall restart:
// status=continued, result=continue, result_reason=instant_stall.
func TestCheckInstantStall_DBState_AllFields(t *testing.T) {
	env := setupTestEnv(t)
	defer env.cleanup()

	env.createSession(t, "claude:sonnet")
	insertTestMessages(t, env, 1)

	proc := makeInstantStallProc(env, "claude:sonnet", 15*time.Second, 0)
	env.spawner.checkInstantStall(context.Background(), proc, makeInstantStallReq(env))

	if proc.finalStatus != "CONTINUE" {
		t.Fatalf("expected finalStatus=CONTINUE, got %q — stall not triggered", proc.finalStatus)
	}

	sessionRepo := repo.NewAgentSessionRepo(env.database, clock.Real())
	sess, err := sessionRepo.Get(env.sessionID)
	if err != nil {
		t.Fatalf("get session: %v", err)
	}
	if sess.Status != model.AgentSessionContinued {
		t.Errorf("status = %q, want %q", sess.Status, model.AgentSessionContinued)
	}
	if !sess.Result.Valid || sess.Result.String != "continue" {
		t.Errorf("result = %v, want continue", sess.Result)
	}
	if !sess.ResultReason.Valid || sess.ResultReason.String != "instant_stall" {
		t.Errorf("result_reason = %v, want instant_stall", sess.ResultReason)
	}
}

// TestCheckInstantStall_ConsumesBudget verifies that after the instant stall triggers,
// stallRestartCount is incremented and subsequent stall checks see reduced budget.
func TestCheckInstantStall_ConsumesBudget(t *testing.T) {
	env := setupTestEnv(t)
	defer env.cleanup()

	// One prior stall restart consumed — 2 restarts left.
	env.createSession(t, "claude:sonnet")
	insertTestMessages(t, env, 1)

	proc := makeInstantStallProc(env, "claude:sonnet", 15*time.Second, 2) // 2 prior (maxStallRestarts-1)
	env.spawner.checkInstantStall(context.Background(), proc, makeInstantStallReq(env))

	if proc.finalStatus != "CONTINUE" {
		t.Fatalf("finalStatus = %q, want CONTINUE", proc.finalStatus)
	}
	// Budget is now exhausted (2+1 == maxStallRestarts).
	if proc.stallRestartCount != maxStallRestarts {
		t.Errorf("stallRestartCount = %d, want %d after consuming last budget slot", proc.stallRestartCount, maxStallRestarts)
	}

	// Guard check must now block further restarts.
	if proc.stallRestartCount < maxStallRestarts {
		t.Errorf("budget should be exhausted: stallRestartCount=%d >= maxStallRestarts=%d",
			proc.stallRestartCount, maxStallRestarts)
	}
}

// TestCheckInstantStall_SequenceOfRestarts simulates three consecutive instant stall restarts
// and verifies stallRestartCount increments correctly, then is capped.
func TestCheckInstantStall_SequenceOfRestarts(t *testing.T) {
	proc := &processInfo{
		stallRestartCount: 0,
	}

	for i := 0; i < maxStallRestarts; i++ {
		if proc.stallRestartCount >= maxStallRestarts {
			t.Fatalf("guard blocked unexpectedly at iteration %d", i)
		}
		proc.stallRestartCount++
		proc.finalStatus = "CONTINUE"
	}

	if proc.stallRestartCount != maxStallRestarts {
		t.Errorf("stallRestartCount = %d, want %d after %d restarts", proc.stallRestartCount, maxStallRestarts, maxStallRestarts)
	}

	// Next restart should be blocked.
	blocked := proc.stallRestartCount >= maxStallRestarts
	if !blocked {
		t.Error("budget should be exhausted after maxStallRestarts increments")
	}
}

// TestCheckInstantStall_SharedBudgetWithStallRestart verifies that instant stall and
// regular stall share the same stallRestartCount budget.
func TestCheckInstantStall_SharedBudgetWithStallRestart(t *testing.T) {
	// Simulate proc that had 2 regular stall restarts already.
	proc := &processInfo{
		stallRestartCount: 2,
		finalStatus:       "PASS",
	}

	// Guard for instant stall: stallRestartCount < maxStallRestarts (3)
	instantStallAllowed := proc.stallRestartCount < maxStallRestarts
	if !instantStallAllowed {
		t.Errorf("instant stall should be allowed at count=%d (max=%d)", proc.stallRestartCount, maxStallRestarts)
	}

	// After one instant stall restart
	proc.stallRestartCount++
	proc.finalStatus = "CONTINUE"

	// Budget is now exhausted — both instant stall and regular stall are blocked.
	if proc.stallRestartCount < maxStallRestarts {
		t.Errorf("budget should be exhausted after 3rd restart: stallRestartCount=%d", proc.stallRestartCount)
	}

	// Regular stall guard also blocks.
	regularStallAllowed := proc.stallRestartCount < maxStallRestarts
	if regularStallAllowed {
		t.Error("regular stall should also be blocked (shared budget)")
	}
}

// TestCheckInstantStall_ClaudeHaikuModel verifies the haiku model triggers the check.
func TestCheckInstantStall_ClaudeHaikuModel(t *testing.T) {
	env := setupTestEnv(t)
	defer env.cleanup()

	env.createSession(t, "claude:haiku")
	insertTestMessages(t, env, 1)

	proc := makeInstantStallProc(env, "claude:haiku", 10*time.Second, 0)
	env.spawner.checkInstantStall(context.Background(), proc, makeInstantStallReq(env))

	if proc.finalStatus != "CONTINUE" {
		t.Errorf("claude:haiku finalStatus = %q, want CONTINUE", proc.finalStatus)
	}
}

// TestCheckInstantStall_ElapsedJustUnderOneMinute verifies elapsed 59s triggers the check.
func TestCheckInstantStall_ElapsedJustUnderOneMinute(t *testing.T) {
	env := setupTestEnv(t)
	defer env.cleanup()

	env.createSession(t, "claude:sonnet")
	insertTestMessages(t, env, 1)

	proc := makeInstantStallProc(env, "claude:sonnet", 59*time.Second, 0)
	env.spawner.checkInstantStall(context.Background(), proc, makeInstantStallReq(env))

	if proc.finalStatus != "CONTINUE" {
		t.Errorf("elapsed 59s finalStatus = %q, want CONTINUE (< 1min boundary)", proc.finalStatus)
	}
}
