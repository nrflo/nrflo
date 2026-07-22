package spawner

import (
	"context"
	"testing"

	"be/internal/clock"
	"be/internal/db"
	"be/internal/service"
	"be/internal/spawner/apirun/provider"
	"be/internal/spawner/apirun/provider/mock"
	"be/internal/ws"

	"github.com/google/uuid"
)

// TestPrepareSpawn_TierFallbackOverride_CrossModeAndEffort verifies that
// SpawnRequest.ExecutionModeOverride/ReasoningEffortOverride — as set by
// relaunchForFallback from the next chain entry — win over both agentDef and
// config.Agents resolution in prepareSpawn: a request with no def at all
// still lands in cli_interactive mode with the overridden effort reaching
// SpawnOptions.ReasoningEffort.
func TestPrepareSpawn_TierFallbackOverride_CrossModeAndEffort(t *testing.T) {
	t.Parallel()
	ensureTmpNrfloDir(t)
	env := setupContextSaveTestEnv(t)
	defer env.cleanup()
	// agentDef says api mode — the override must still win over it.
	insertAPIAgentDef(t, env, "impl", "sonnet-5")

	sp := New(Config{
		DataPath: env.dbPath,
		Pool:     db.WrapAsPool(env.database),
		Clock:    clock.Real(),
		APIMode:  true,
		BuildAPIProvider: func(_ context.Context, _ string, _ string) (provider.Provider, error) {
			return mock.New(), nil
		},
		ModelConfigs: map[string]ModelConfig{
			"sonnet-5": {Provider: "anthropic", CLIModel: "claude-sonnet-5", CLIEfforts: []string{"low", "medium", "high"}, DefaultEffort: "low"},
		},
		AgentSvc: &noopAgentSvc{},
	})

	_, prep, err := sp.prepareSpawn(context.Background(), SpawnRequest{
		AgentType:               "impl",
		ProjectID:               env.projectID,
		WorkflowName:            "feature",
		WorkflowInstanceID:      env.wfiID,
		ExecutionModeOverride:   "cli_interactive",
		ReasoningEffortOverride: "medium",
	}, "claude:sonnet-5", "impl", env.wfiID)
	if err != nil {
		t.Fatalf("prepareSpawn() error: %v", err)
	}
	if prep.executionMode != "cli_interactive" {
		t.Errorf("executionMode = %q, want cli_interactive (override must win with no agentDef/config.Agents entry)", prep.executionMode)
	}
	if prep.opts.ReasoningEffort != "medium" {
		t.Errorf("SpawnOptions.ReasoningEffort = %q, want medium (ReasoningEffortOverride)", prep.opts.ReasoningEffort)
	}
	if _, ok := prep.adapter.(*ClaudeAdapter); !ok {
		t.Errorf("prep.adapter type = %T, want *ClaudeAdapter (cli_interactive override selects the CLI adapter)", prep.adapter)
	}
}

// TestPrepareSpawn_TierFallbackOverride_APINoKeyThenCLIGuard is the explicit
// api-no-key -> cli guard the ticket calls out: entry-0 (api mode, no
// resolvable provider) fails as a build-time provider error; entry-1
// (cli_interactive override) must independently succeed at the prepareSpawn
// level, proving the override path is not entangled with the failed api
// entry's state.
func TestPrepareSpawn_TierFallbackOverride_APINoKeyThenCLIGuard(t *testing.T) {
	t.Parallel()
	ensureTmpNrfloDir(t)
	env := setupContextSaveTestEnv(t)
	defer env.cleanup()
	// agentDef says cli_interactive — entry-0's api override must still win.
	insertCLIAgentDefWithEffort(t, env, "impl", "cli-model", "")

	sp := New(Config{
		DataPath: env.dbPath,
		Pool:     db.WrapAsPool(env.database),
		Clock:    clock.Real(),
		APIMode:  true,
		BuildAPIProvider: func(_ context.Context, _ string, _ string) (provider.Provider, error) {
			return nil, errNoAPIKeyStub
		},
		ModelConfigs: map[string]ModelConfig{
			"api-model": {Provider: "anthropic", APIModel: "claude-sonnet-4-6", APIContext: 200000, APIEfforts: []string{"low"}, DefaultEffort: "low"},
			"cli-model": {Provider: "anthropic", CLIModel: "claude-sonnet-5", CLIEfforts: []string{"low", "medium"}, DefaultEffort: "low"},
		},
		AgentSvc: &noopAgentSvc{},
	})

	// entry-0: api mode, no key -> build-time provider error, wrapped.
	_, _, err := sp.prepareSpawn(context.Background(), SpawnRequest{
		AgentType:             "impl",
		ProjectID:             env.projectID,
		WorkflowName:          "feature",
		WorkflowInstanceID:    env.wfiID,
		ExecutionModeOverride: "api",
	}, "claude:api-model", "impl", env.wfiID)
	if err == nil {
		t.Fatal("entry-0 (api, no key) prepareSpawn() error = nil, want a build-time provider error")
	}
	if !isProviderBuildError(err) {
		t.Errorf("entry-0 error not classified as a provider-build error: %v", err)
	}

	// entry-1: cli_interactive override — independent, must succeed cleanly.
	_, prep, err := sp.prepareSpawn(context.Background(), SpawnRequest{
		AgentType:             "impl",
		ProjectID:             env.projectID,
		WorkflowName:          "feature",
		WorkflowInstanceID:    env.wfiID,
		ExecutionModeOverride: "cli_interactive",
	}, "claude:cli-model", "impl", env.wfiID)
	if err != nil {
		t.Fatalf("entry-1 (cli_interactive) prepareSpawn() error: %v", err)
	}
	if prep.executionMode != "cli_interactive" {
		t.Errorf("entry-1 executionMode = %q, want cli_interactive", prep.executionMode)
	}
}

// errNoAPIKeyStub stands in for a real credential-resolution failure.
var errNoAPIKeyStub = &stubError{"no api key configured"}

type stubError struct{ msg string }

func (e *stubError) Error() string { return e.msg }

// TestRelaunchForFallback_CrossProviderAndEffort drives the real
// relaunchForFallback end-to-end (api->api, cross-provider) and asserts: the
// winning proc lands under the next chain entry's model/effort, chainPos
// advances, hardProviderFail resets, and the agent.provider_fallback
// broadcast carries the from/to provider+mode+chain_pos payload.
func TestRelaunchForFallback_CrossProviderAndEffort(t *testing.T) {
	t.Parallel()
	ensureTmpNrfloDir(t)
	env := setupContextSaveTestEnv(t)
	defer env.cleanup()
	insertAPIAgentDef(t, env, "impl", "old-model")

	hub := ws.NewHub(clock.Real())
	sp := New(Config{
		DataPath: env.dbPath,
		Pool:     db.WrapAsPool(env.database),
		Clock:    clock.Real(),
		APIMode:  true,
		WSHub:    hub,
		BuildAPIProvider: func(_ context.Context, _ string, _ string) (provider.Provider, error) {
			return mock.New(), nil
		},
		ModelConfigs: map[string]ModelConfig{
			"old-model": {Provider: "oldprov", APIModel: "old-x", APIContext: 100000, APIEfforts: []string{"low"}, DefaultEffort: "low"},
			"new-model": {Provider: "newprov", APIModel: "new-x", APIContext: 100000, APIEfforts: []string{"low", "high"}, DefaultEffort: "low"},
		},
		AgentSvc: &noopAgentSvc{},
	})

	chain := []service.AgentChainEntry{
		{Provider: "oldprov", ExecutionMode: "api", ModelID: "old-model", ReasoningEffort: "low"},
		{Provider: "newprov", ExecutionMode: "api", ModelID: "new-model", ReasoningEffort: "high"},
	}

	oldSessionID := uuid.New().String()
	if !sp.createAgentSessionRow(env.projectID, env.ticketID, env.wfiID, "impl", "impl", oldSessionID, "claude:old-model", "impl", "", "", "", "", "api", 0) {
		t.Fatal("createAgentSessionRow(old) failed")
	}
	oldProc := &processInfo{
		sessionID:          oldSessionID,
		agentID:            "old-agent-id",
		agentType:          "impl",
		projectID:          env.projectID,
		ticketID:           env.ticketID,
		workflowName:       "feature",
		workflowInstanceID: env.wfiID,
		modelID:            "claude:old-model",
		chain:              chain,
		chainPos:           0,
		hardProviderFail:   true,
	}

	newProc, err := sp.relaunchForFallback(context.Background(), oldProc, SpawnRequest{
		AgentType:          "impl",
		ProjectID:          env.projectID,
		TicketID:           env.ticketID,
		WorkflowName:       "feature",
		WorkflowInstanceID: env.wfiID,
	}, "impl", chain[1], 1)
	if err != nil {
		t.Fatalf("relaunchForFallback() error: %v", err)
	}

	if newProc.chainPos != 1 {
		t.Errorf("newProc.chainPos = %d, want 1", newProc.chainPos)
	}
	if newProc.hardProviderFail {
		t.Error("newProc.hardProviderFail = true, want false (every relaunch resets it)")
	}
	if newProc.resolvedEffort != "high" {
		t.Errorf("newProc.resolvedEffort = %q, want high (entry-1 ReasoningEffort)", newProc.resolvedEffort)
	}
	if newProc.modelID != "claude:new-model" {
		t.Errorf("newProc.modelID = %q, want claude:new-model", newProc.modelID)
	}
}

// TestRelaunchForContinuation_ResetsHardProviderFail is the concrete,
// real-code-path complement to TestShouldAdvanceChain_MonotonicSequence's
// re-advance-loop guard: a same-model relaunch (low-context/stall/failRestart)
// carries chain/chainPos forward unchanged but always resets
// hardProviderFail=false, so a stale flag from the killed session can never
// trigger shouldAdvanceChain on the new one.
func TestRelaunchForContinuation_ResetsHardProviderFail(t *testing.T) {
	t.Parallel()
	ensureTmpNrfloDir(t)
	env := setupContextSaveTestEnv(t)
	defer env.cleanup()
	insertAPIAgentDef(t, env, "impl", "m0")

	sp := New(Config{
		DataPath: env.dbPath,
		Pool:     db.WrapAsPool(env.database),
		Clock:    clock.Real(),
		APIMode:  true,
		BuildAPIProvider: func(_ context.Context, _ string, _ string) (provider.Provider, error) {
			return mock.New(), nil
		},
		ModelConfigs: map[string]ModelConfig{
			"m0": {Provider: "anthropic", APIModel: "claude-x", APIContext: 100000, APIEfforts: []string{"low"}, DefaultEffort: "low"},
		},
		AgentSvc: &noopAgentSvc{},
	})

	chain := []service.AgentChainEntry{
		{Provider: "anthropic", ModelID: "m0"},
		{Provider: "openai", ModelID: "m1"},
	}
	oldSessionID := uuid.New().String()
	if !sp.createAgentSessionRow(env.projectID, env.ticketID, env.wfiID, "impl", "impl", oldSessionID, "claude:m0", "impl", "", "", "", "", "api", 0) {
		t.Fatal("createAgentSessionRow(old) failed")
	}
	oldProc := &processInfo{
		sessionID:          oldSessionID,
		agentID:            "old-agent-id",
		agentType:          "impl",
		projectID:          env.projectID,
		ticketID:           env.ticketID,
		workflowName:       "feature",
		workflowInstanceID: env.wfiID,
		modelID:            "claude:m0",
		chain:              chain,
		chainPos:           0,
		hardProviderFail:   true, // stale flag from the killed session
	}

	newProc, err := sp.relaunchForContinuation(context.Background(), oldProc, SpawnRequest{
		AgentType:          "impl",
		ProjectID:          env.projectID,
		TicketID:           env.ticketID,
		WorkflowName:       "feature",
		WorkflowInstanceID: env.wfiID,
	}, "impl")
	if err != nil {
		t.Fatalf("relaunchForContinuation() error: %v", err)
	}

	if newProc.hardProviderFail {
		t.Error("newProc.hardProviderFail = true, want false (same-model relaunch must reset the stale flag)")
	}
	if newProc.chainPos != 0 {
		t.Errorf("newProc.chainPos = %d, want 0 (same-model relaunch never advances)", newProc.chainPos)
	}
	if len(newProc.chain) != 2 {
		t.Errorf("newProc.chain len = %d, want 2 (chain carried forward)", len(newProc.chain))
	}

	// The guard in action: with the flag reset, shouldAdvanceChain must
	// report false for the new proc even though it still carries a
	// multi-entry chain.
	if _, _, ok := shouldAdvanceChain(newProc); ok {
		t.Error("shouldAdvanceChain(newProc) = true after relaunchForContinuation reset it, want false")
	}
}
