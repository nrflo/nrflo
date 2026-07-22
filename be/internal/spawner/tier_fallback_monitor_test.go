package spawner

import (
	"context"
	"testing"

	"be/internal/clock"
	"be/internal/model"
	"be/internal/repo"
	"be/internal/service"
)

// deleteContextSaverDefs removes the seeded context-saver system agent defs
// so contextSaveViaAgent's spawnContextSaver returns false immediately
// (TestSpawnContextSaver_SystemAgentNotFound's pattern) instead of attempting
// a real child spawn — keeping tryChainFallback tests fast and free of any
// CLI process launch.
func deleteContextSaverDefs(t *testing.T, env *testEnv) {
	t.Helper()
	if _, err := env.database.Exec(`DELETE FROM system_agent_definitions WHERE id IN ('context-saver', 'context-saver-api')`); err != nil {
		t.Fatalf("delete context-saver defs: %v", err)
	}
}

// TestTryChainFallback_AdvancesAndUpdatesDB is the full-flow case: a proc
// with hardProviderFail=true mid-chain drives tryChainFallback end-to-end —
// DB session flips to continue/provider_fallback + continued, proc.finalStatus
// becomes CONTINUE, and the injected relaunch closure is called with the
// correct next entry/position.
func TestTryChainFallback_AdvancesAndUpdatesDB(t *testing.T) {
	t.Parallel()
	env := setupTestEnv(t)
	defer env.cleanup()
	deleteContextSaverDefs(t, env)

	env.createSession(t, "claude:m0")

	chain := []service.AgentChainEntry{
		{Provider: "anthropic", ModelID: "m0"},
		{Provider: "openai", ModelID: "m1"},
	}
	proc := &processInfo{
		sessionID:          env.sessionID,
		agentID:            "test-agent-id",
		agentType:          "test-agent",
		workflowInstanceID: env.wfiID,
		projectID:          env.projectID,
		ticketID:           env.ticketID,
		workflowName:       env.workflowID,
		modelID:            "claude:m0",
		chain:              chain,
		chainPos:           0,
		hardProviderFail:   true,
		finalStatus:        "FAIL",
	}

	var relaunchCalledWith struct {
		nextPos int
		entry   service.AgentChainEntry
		called  bool
	}
	fakeNewProc := &processInfo{sessionID: "new-sess"}
	relaunch := func(_ *processInfo, entry service.AgentChainEntry, nextPos int) (*processInfo, error) {
		relaunchCalledWith.called = true
		relaunchCalledWith.entry = entry
		relaunchCalledWith.nextPos = nextPos
		return fakeNewProc, nil
	}

	newProc, advanced := env.spawner.tryChainFallback(context.Background(), proc, SpawnRequest{
		ProjectID:    env.projectID,
		TicketID:     env.ticketID,
		WorkflowName: env.workflowID,
		AgentType:    "test-agent",
	}, relaunch)

	if !advanced {
		t.Fatal("advanced = false, want true")
	}
	if newProc != fakeNewProc {
		t.Errorf("newProc = %v, want the relaunch closure's returned proc", newProc)
	}
	if !relaunchCalledWith.called {
		t.Fatal("relaunch closure was not called")
	}
	if relaunchCalledWith.nextPos != 1 || relaunchCalledWith.entry.Provider != "openai" {
		t.Errorf("relaunch called with nextPos=%d entry=%+v, want nextPos=1 entry.Provider=openai",
			relaunchCalledWith.nextPos, relaunchCalledWith.entry)
	}
	if proc.finalStatus != "CONTINUE" {
		t.Errorf("proc.finalStatus = %q, want CONTINUE", proc.finalStatus)
	}

	sessionRepo := repo.NewAgentSessionRepo(env.database, clock.Real())
	sess, err := sessionRepo.Get(env.sessionID)
	if err != nil {
		t.Fatalf("get session: %v", err)
	}
	if sess.Status != model.AgentSessionContinued {
		t.Errorf("session status = %q, want continued", sess.Status)
	}
	if !sess.Result.Valid || sess.Result.String != "continue" {
		t.Errorf("session result = %v, want continue", sess.Result)
	}
	if !sess.ResultReason.Valid || sess.ResultReason.String != "provider_fallback" {
		t.Errorf("session result_reason = %v, want provider_fallback", sess.ResultReason)
	}
}

// TestTryChainFallback_ChainExhausted_RegistersFailReason verifies that when
// shouldAdvanceChain's guard fails (chain exhausted: proc at the last
// position), tryChainFallback registers the session as fail/chain_exhausted,
// returns advanced=false, and never calls the relaunch closure.
func TestTryChainFallback_ChainExhausted_RegistersFailReason(t *testing.T) {
	t.Parallel()
	env := setupTestEnv(t)
	defer env.cleanup()

	env.createSession(t, "claude:m1")

	chain := []service.AgentChainEntry{
		{Provider: "anthropic", ModelID: "m0"},
		{Provider: "openai", ModelID: "m1"},
	}
	proc := &processInfo{
		sessionID:          env.sessionID,
		agentID:            "test-agent-id",
		agentType:          "test-agent",
		workflowInstanceID: env.wfiID,
		projectID:          env.projectID,
		ticketID:           env.ticketID,
		workflowName:       env.workflowID,
		modelID:            "claude:m1",
		chain:              chain,
		chainPos:           1, // last entry — no further chain to advance to
		hardProviderFail:   true,
		finalStatus:        "FAIL",
	}

	relaunchCalled := false
	relaunch := func(_ *processInfo, _ service.AgentChainEntry, _ int) (*processInfo, error) {
		relaunchCalled = true
		return nil, nil
	}

	newProc, advanced := env.spawner.tryChainFallback(context.Background(), proc, SpawnRequest{
		ProjectID:    env.projectID,
		TicketID:     env.ticketID,
		WorkflowName: env.workflowID,
		AgentType:    "test-agent",
	}, relaunch)

	if advanced {
		t.Error("advanced = true, want false (chain exhausted)")
	}
	if newProc != nil {
		t.Errorf("newProc = %v, want nil", newProc)
	}
	if relaunchCalled {
		t.Error("relaunch closure was called; want it skipped on chain exhaustion")
	}
	// proc.finalStatus is left untouched by tryChainFallback on the
	// non-advance path — the caller (monitorAll) treats it as terminal FAIL.
	if proc.finalStatus != "FAIL" {
		t.Errorf("proc.finalStatus = %q, want FAIL (unchanged, terminal)", proc.finalStatus)
	}

	sessionRepo := repo.NewAgentSessionRepo(env.database, clock.Real())
	sess, err := sessionRepo.Get(env.sessionID)
	if err != nil {
		t.Fatalf("get session: %v", err)
	}
	if sess.Status != model.AgentSessionFailed {
		t.Errorf("session status = %q, want failed", sess.Status)
	}
	if !sess.ResultReason.Valid || sess.ResultReason.String != "chain_exhausted" {
		t.Errorf("session result_reason = %v, want chain_exhausted", sess.ResultReason)
	}
}

// TestTryChainFallback_RelaunchFailure_LeavesProcTerminal verifies that when
// the relaunch closure itself fails, tryChainFallback reports advanced=true
// (caller must not fall through to ordinary fail-restart handling) with a
// nil newProc (caller treats proc as terminally completed).
func TestTryChainFallback_RelaunchFailure_LeavesProcTerminal(t *testing.T) {
	t.Parallel()
	env := setupTestEnv(t)
	defer env.cleanup()
	deleteContextSaverDefs(t, env)

	env.createSession(t, "claude:m0")

	chain := []service.AgentChainEntry{
		{Provider: "anthropic", ModelID: "m0"},
		{Provider: "openai", ModelID: "m1"},
	}
	proc := &processInfo{
		sessionID:          env.sessionID,
		agentID:            "test-agent-id",
		agentType:          "test-agent",
		workflowInstanceID: env.wfiID,
		projectID:          env.projectID,
		ticketID:           env.ticketID,
		workflowName:       env.workflowID,
		modelID:            "claude:m0",
		chain:              chain,
		chainPos:           0,
		hardProviderFail:   true,
		finalStatus:        "FAIL",
	}

	relaunch := func(_ *processInfo, _ service.AgentChainEntry, _ int) (*processInfo, error) {
		return nil, errRelaunchStub
	}

	newProc, advanced := env.spawner.tryChainFallback(context.Background(), proc, SpawnRequest{
		ProjectID:    env.projectID,
		TicketID:     env.ticketID,
		WorkflowName: env.workflowID,
		AgentType:    "test-agent",
	}, relaunch)

	if !advanced {
		t.Error("advanced = false, want true (relaunch attempted)")
	}
	if newProc != nil {
		t.Errorf("newProc = %v, want nil on relaunch failure", newProc)
	}
}

var errRelaunchStub = &stubError{"relaunch failed"}
