package spawner

import (
	"context"
	"testing"

	"be/internal/clock"
	"be/internal/model"
	"be/internal/repo"
	"be/internal/service"
)

// TestTryChainFallback_NoChain_KeepsClassifiedReason verifies the contract:
// a HARD provider fail on a proc with NO tier chain (main-phase agents,
// chain==nil) must never be re-registered as chain_exhausted — the
// completion path's already-registered classified reason (e.g. api_error)
// must survive untouched.
func TestTryChainFallback_NoChain_KeepsClassifiedReason(t *testing.T) {
	t.Parallel()
	env := setupTestEnv(t)
	defer env.cleanup()

	env.createSession(t, "claude:sonnet-5")
	// Mirrors backend.go:208 — the completion path already registered the
	// classified terminal reason before tryChainFallback ever runs.
	env.spawner.registerAgentStopWithReason(env.projectID, env.ticketID, env.workflowID,
		env.sessionID, "test-agent-id", "fail", "api_error", "claude:sonnet-5")

	proc := &processInfo{
		sessionID:          env.sessionID,
		agentID:            "test-agent-id",
		agentType:          "test-agent",
		workflowInstanceID: env.wfiID,
		projectID:          env.projectID,
		ticketID:           env.ticketID,
		workflowName:       env.workflowID,
		modelID:            "claude:sonnet-5",
		chain:              nil,
		chainPos:           0,
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
		t.Error("advanced = true, want false (no chain to advance)")
	}
	if newProc != nil {
		t.Errorf("newProc = %v, want nil", newProc)
	}
	if relaunchCalled {
		t.Error("relaunch closure was called; want it skipped when proc has no chain")
	}

	sessionRepo := repo.NewAgentSessionRepo(env.database, clock.Real())
	sess, err := sessionRepo.Get(env.sessionID)
	if err != nil {
		t.Fatalf("get session: %v", err)
	}
	if sess.Status != model.AgentSessionFailed {
		t.Errorf("session status = %q, want failed", sess.Status)
	}
	if !sess.ResultReason.Valid || sess.ResultReason.String != "api_error" {
		t.Errorf("session result_reason = %v, want api_error (classified reason must survive)", sess.ResultReason)
	}
}

// TestTryChainFallback_NeverAdvancedLength1Chain_KeepsClassifiedReason covers
// the tier-resolved main-phase agent shape: a length-1 chain still at
// chainPos==0 never advanced, so it must not be reclassified as
// chain_exhausted either.
func TestTryChainFallback_NeverAdvancedLength1Chain_KeepsClassifiedReason(t *testing.T) {
	t.Parallel()
	env := setupTestEnv(t)
	defer env.cleanup()

	env.createSession(t, "claude:sonnet-5")
	env.spawner.registerAgentStopWithReason(env.projectID, env.ticketID, env.workflowID,
		env.sessionID, "test-agent-id", "fail", "provider_error_pattern", "claude:sonnet-5")

	chain := []service.AgentChainEntry{{Provider: "anthropic", ModelID: "sonnet-5"}}
	proc := &processInfo{
		sessionID:          env.sessionID,
		agentID:            "test-agent-id",
		agentType:          "test-agent",
		workflowInstanceID: env.wfiID,
		projectID:          env.projectID,
		ticketID:           env.ticketID,
		workflowName:       env.workflowID,
		modelID:            "claude:sonnet-5",
		chain:              chain,
		chainPos:           0,
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
		t.Error("advanced = true, want false (length-1 chain never advanced)")
	}
	if newProc != nil {
		t.Errorf("newProc = %v, want nil", newProc)
	}
	if relaunchCalled {
		t.Error("relaunch closure was called; want it skipped for a never-advanced chain")
	}

	sessionRepo := repo.NewAgentSessionRepo(env.database, clock.Real())
	sess, err := sessionRepo.Get(env.sessionID)
	if err != nil {
		t.Fatalf("get session: %v", err)
	}
	if !sess.ResultReason.Valid || sess.ResultReason.String != "provider_error_pattern" {
		t.Errorf("session result_reason = %v, want provider_error_pattern (classified reason must survive)", sess.ResultReason)
	}
}

// TestTryChainFallback_RateLimit_Untouched verifies a rate-limit-classified
// session (hardProviderFail=false) is completely untouched by
// tryChainFallback's non-advance branch — no relaunch, no reason rewrite,
// row stays continue/rate_limit.
func TestTryChainFallback_RateLimit_Untouched(t *testing.T) {
	t.Parallel()
	env := setupTestEnv(t)
	defer env.cleanup()

	env.createSession(t, "claude:sonnet-5")
	env.spawner.registerAgentStopWithReason(env.projectID, env.ticketID, env.workflowID,
		env.sessionID, "test-agent-id", "continue", "rate_limit", "claude:sonnet-5")

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
		modelID:            "claude:sonnet-5",
		chain:              chain,
		chainPos:           0,
		hardProviderFail:   false,
		finalStatus:        "CONTINUE",
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
		t.Error("advanced = true, want false (rate-limit is not a hard provider fail)")
	}
	if newProc != nil {
		t.Errorf("newProc = %v, want nil", newProc)
	}
	if relaunchCalled {
		t.Error("relaunch closure was called; want it skipped for a rate-limit proc")
	}

	sessionRepo := repo.NewAgentSessionRepo(env.database, clock.Real())
	sess, err := sessionRepo.Get(env.sessionID)
	if err != nil {
		t.Fatalf("get session: %v", err)
	}
	if !sess.Result.Valid || sess.Result.String != "continue" {
		t.Errorf("session result = %v, want continue (untouched)", sess.Result)
	}
	if !sess.ResultReason.Valid || sess.ResultReason.String != "rate_limit" {
		t.Errorf("session result_reason = %v, want rate_limit (untouched)", sess.ResultReason)
	}
}
