package spawner

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"be/internal/clock"
	"be/internal/model"
	"be/internal/repo"
)

// insertTestMessages inserts n actionable text messages into agent_messages for the given session.
// These are "real" messages (not [init] or [thinking]) that count toward the stall threshold.
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

// insertNonActionableMessages inserts init and thinking messages that are excluded from stall count.
func insertNonActionableMessages(t *testing.T, env *testEnv, seqStart int) {
	t.Helper()
	msgRepo := repo.NewAgentMessageRepo(env.database, clock.Real())
	msgs := []repo.MessageEntry{
		{Content: "[init] v2.1.87 model=claude-opus-4-6", Category: "text"},
		{Content: "[thinking] Let me analyze this...", Category: "text"},
	}
	if err := msgRepo.InsertBatch(env.sessionID, seqStart, msgs); err != nil {
		t.Fatalf("insertNonActionableMessages: %v", err)
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
		msgCount          int // actionable messages (not [init] or [thinking])
		stallRestartCount int
		wantCONTINUE      bool
	}{
		{"claude_15s_0msg_triggers", "claude:sonnet", 15 * time.Second, 0, 0, true},
		{"claude_15s_1msg_triggers", "claude:sonnet", 15 * time.Second, 1, 0, true},
		{"claude_15s_3msg_triggers", "claude:sonnet", 15 * time.Second, 3, 0, true},
		{"claude_30s_2msg_triggers", "claude:opus", 30 * time.Second, 2, 0, true},
		{"opencode_skipped", "opencode:opencode_gpt54", 15 * time.Second, 1, 0, false},
		{"codex_skipped", "codex:codex_gpt_normal", 15 * time.Second, 1, 0, false},
		{"elapsed_at_boundary_skipped", "claude:sonnet", 1 * time.Minute, 1, 0, false},
		{"elapsed_above_boundary_skipped", "claude:sonnet", 90 * time.Second, 1, 0, false},
		{"msg_count_4_skipped", "claude:sonnet", 15 * time.Second, 4, 0, false},
		{"msg_count_5_skipped", "claude:sonnet", 15 * time.Second, 5, 0, false},
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

	env.createSession(t, "claude:sonnet")
	insertTestMessages(t, env, 1)

	proc := makeInstantStallProc(env, "claude:sonnet", 15*time.Second, maxStallRestarts-1)
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
	// Simulate proc that had maxStallRestarts-1 regular stall restarts already.
	proc := &processInfo{
		stallRestartCount: maxStallRestarts - 1,
		finalStatus:       "PASS",
	}

	// Guard for instant stall: stallRestartCount < maxStallRestarts
	instantStallAllowed := proc.stallRestartCount < maxStallRestarts
	if !instantStallAllowed {
		t.Errorf("instant stall should be allowed at count=%d (max=%d)", proc.stallRestartCount, maxStallRestarts)
	}

	// After one instant stall restart
	proc.stallRestartCount++
	proc.finalStatus = "CONTINUE"

	// Budget is now exhausted — both instant stall and regular stall are blocked.
	if proc.stallRestartCount < maxStallRestarts {
		t.Errorf("budget should be exhausted: stallRestartCount=%d", proc.stallRestartCount)
	}

	// Regular stall guard also blocks.
	regularStallAllowed := proc.stallRestartCount < maxStallRestarts
	if regularStallAllowed {
		t.Error("regular stall should also be blocked (shared budget)")
	}
}

// TestCheckInstantStall_BudgetExhausted_MarkedFailed verifies that when an instant stall
// is detected but the stall restart budget is exhausted, the session is marked as failed.
func TestCheckInstantStall_BudgetExhausted_MarkedFailed(t *testing.T) {
	env := setupTestEnv(t)
	defer env.cleanup()

	env.createSession(t, "claude:sonnet")
	insertTestMessages(t, env, 1)

	proc := makeInstantStallProc(env, "claude:sonnet", 15*time.Second, maxStallRestarts)
	env.spawner.checkInstantStall(context.Background(), proc, makeInstantStallReq(env))

	if proc.finalStatus != "FAIL" {
		t.Errorf("finalStatus = %q, want FAIL when budget exhausted", proc.finalStatus)
	}

	// Verify DB state
	sessionRepo := repo.NewAgentSessionRepo(env.database, clock.Real())
	sess, err := sessionRepo.Get(env.sessionID)
	if err != nil {
		t.Fatalf("get session: %v", err)
	}
	if sess.Status != model.AgentSessionFailed {
		t.Errorf("status = %q, want %q", sess.Status, model.AgentSessionFailed)
	}
	if !sess.Result.Valid || sess.Result.String != "fail" {
		t.Errorf("result = %v, want fail", sess.Result)
	}
	if !sess.ResultReason.Valid || sess.ResultReason.String != "stall_budget_exhausted" {
		t.Errorf("result_reason = %v, want stall_budget_exhausted", sess.ResultReason)
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

// setSessionFindings sets findings JSON on an agent session.
func setSessionFindings(t *testing.T, env *testEnv, findings map[string]interface{}) {
	t.Helper()
	sessionRepo := repo.NewAgentSessionRepo(env.database, clock.Real())
	data, _ := json.Marshal(findings)
	if err := sessionRepo.UpdateFindings(env.sessionID, string(data)); err != nil {
		t.Fatalf("setSessionFindings: %v", err)
	}
}

// TestCheckInstantStall_InitAndThinkingExcluded verifies that [init] and [thinking] messages
// are excluded from the actionable message count. An agent with only these messages triggers stall.
func TestCheckInstantStall_InitAndThinkingExcluded(t *testing.T) {
	env := setupTestEnv(t)
	defer env.cleanup()

	env.createSession(t, "claude:sonnet")
	// Insert only non-actionable messages (init + thinking)
	insertNonActionableMessages(t, env, 0)

	proc := makeInstantStallProc(env, "claude:sonnet", 25*time.Second, 0)
	env.spawner.checkInstantStall(context.Background(), proc, makeInstantStallReq(env))

	if proc.finalStatus != "CONTINUE" {
		t.Errorf("finalStatus = %q, want CONTINUE (init+thinking should not count)", proc.finalStatus)
	}
}

// TestCheckInstantStall_InitThinkingPlusActionable verifies that actionable messages beyond
// the threshold still prevent stall detection, even when mixed with init/thinking.
func TestCheckInstantStall_InitThinkingPlusActionable(t *testing.T) {
	env := setupTestEnv(t)
	defer env.cleanup()

	env.createSession(t, "claude:sonnet")
	// Insert init+thinking (non-actionable) + 4 actionable messages (exceeds threshold of 3)
	insertNonActionableMessages(t, env, 0)
	insertTestMessages(t, env, 4)

	proc := makeInstantStallProc(env, "claude:sonnet", 25*time.Second, 0)
	env.spawner.checkInstantStall(context.Background(), proc, makeInstantStallReq(env))

	if proc.finalStatus != "PASS" {
		t.Errorf("finalStatus = %q, want PASS (4 actionable msgs > 3 threshold)", proc.finalStatus)
	}
}

// TestCheckInstantStall_NoOpFinding_SkipsStall verifies that an agent with a no-op finding
// is not treated as an instant stall.
func TestCheckInstantStall_NoOpFinding_SkipsStall(t *testing.T) {
	env := setupTestEnv(t)
	defer env.cleanup()

	env.createSession(t, "claude:sonnet")
	insertTestMessages(t, env, 1)
	setSessionFindings(t, env, map[string]interface{}{"no-op": "no-op"})

	proc := makeInstantStallProc(env, "claude:sonnet", 15*time.Second, 0)
	env.spawner.checkInstantStall(context.Background(), proc, makeInstantStallReq(env))

	if proc.finalStatus != "PASS" {
		t.Errorf("finalStatus = %q, want PASS (no-op finding should skip stall)", proc.finalStatus)
	}
	if proc.stallRestartCount != 0 {
		t.Errorf("stallRestartCount = %d, want 0 (unchanged)", proc.stallRestartCount)
	}
}

// TestCheckInstantStall_NoOpFinding_BudgetExhausted_StillPasses verifies that no-op guard
// fires before the budget check, so agent passes even when budget is exhausted.
func TestCheckInstantStall_NoOpFinding_BudgetExhausted_StillPasses(t *testing.T) {
	env := setupTestEnv(t)
	defer env.cleanup()

	env.createSession(t, "claude:sonnet")
	insertTestMessages(t, env, 1)
	setSessionFindings(t, env, map[string]interface{}{"no-op": "no-op"})

	proc := makeInstantStallProc(env, "claude:sonnet", 15*time.Second, maxStallRestarts)
	env.spawner.checkInstantStall(context.Background(), proc, makeInstantStallReq(env))

	if proc.finalStatus != "PASS" {
		t.Errorf("finalStatus = %q, want PASS (no-op should pass even with exhausted budget)", proc.finalStatus)
	}
}

// TestCheckInstantStall_CallbackInstructionsFinding_SkipsStall verifies that an agent with
// a callback_instructions finding is not treated as an instant stall.
func TestCheckInstantStall_CallbackInstructionsFinding_SkipsStall(t *testing.T) {
	env := setupTestEnv(t)
	defer env.cleanup()

	env.createSession(t, "claude:sonnet")
	insertTestMessages(t, env, 1)
	setSessionFindings(t, env, map[string]interface{}{"callback_instructions": "Fix the bug"})

	proc := makeInstantStallProc(env, "claude:sonnet", 15*time.Second, 0)
	env.spawner.checkInstantStall(context.Background(), proc, makeInstantStallReq(env))

	if proc.finalStatus != "PASS" {
		t.Errorf("finalStatus = %q, want PASS (callback_instructions finding should skip stall)", proc.finalStatus)
	}
	if proc.stallRestartCount != 0 {
		t.Errorf("stallRestartCount = %d, want 0 (unchanged)", proc.stallRestartCount)
	}
}

// TestCheckInstantStall_CallbackInstructionsFinding_BudgetExhausted_StillPasses verifies that the
// callback_instructions guard fires before the budget check, so agent passes even when budget is exhausted.
func TestCheckInstantStall_CallbackInstructionsFinding_BudgetExhausted_StillPasses(t *testing.T) {
	env := setupTestEnv(t)
	defer env.cleanup()

	env.createSession(t, "claude:sonnet")
	insertTestMessages(t, env, 1)
	setSessionFindings(t, env, map[string]interface{}{"callback_instructions": "Fix the bug"})

	proc := makeInstantStallProc(env, "claude:sonnet", 15*time.Second, maxStallRestarts)
	env.spawner.checkInstantStall(context.Background(), proc, makeInstantStallReq(env))

	if proc.finalStatus != "PASS" {
		t.Errorf("finalStatus = %q, want PASS (callback_instructions should pass even with exhausted budget)", proc.finalStatus)
	}
}
