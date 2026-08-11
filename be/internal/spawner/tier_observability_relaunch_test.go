package spawner

import (
	"context"
	"testing"

	"be/internal/clock"
	"be/internal/db"
	"be/internal/service"
	"be/internal/spawner/apirun/provider"
	"be/internal/spawner/apirun/provider/mock"

	"github.com/google/uuid"
)

// TestRelaunchForContinuation_PersistsResolvedEffortOnNewSession is a
// regression guard for the missing recordResolvedSpawn call on the
// continuation relaunch path: createAgentSessionRow only ran for oldProc,
// so without the fix the new session row's resolution columns stay at
// their zero values even though the old row carried real chain resolution.
func TestRelaunchForContinuation_PersistsResolvedEffortOnNewSession(t *testing.T) {
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
		{Provider: "anthropic", ModelID: "m0", ExecutionMode: "api", ReasoningEffort: "low", Tier: 1, Position: 0},
		{Provider: "openai", ModelID: "m1", ExecutionMode: "api", ReasoningEffort: "medium", Tier: 2, Position: 1},
	}
	oldSessionID := uuid.New().String()
	if !sp.createAgentSessionRow(env.projectID, env.ticketID, env.wfiID, "impl", "impl", oldSessionID, "claude:m0", "impl", "", "", "", "", "api", 0) {
		t.Fatal("createAgentSessionRow(old) failed")
	}
	// Simulate the old session having resolved chain position 1 (a prior
	// fallback), so the regression is distinguishable from a coincidental
	// zero-value default on the new row.
	sp.recordResolvedSpawn(&processInfo{sessionID: oldSessionID}, chain, 1)

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
		chainPos:           1,
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

	_, provName, mode, effort, gotPos, _ := querySessionResolution(t, env.database, newProc.sessionID)
	if effort == "" {
		t.Error("new session resolved_effort is empty, want non-empty (recordResolvedSpawn must run on relaunch)")
	}
	if effort != chain[1].ReasoningEffort {
		t.Errorf("new session resolved_effort = %q, want %q (chain[oldProc.chainPos])", effort, chain[1].ReasoningEffort)
	}
	if gotPos != oldProc.chainPos {
		t.Errorf("new session chain_position = %d, want %d (oldProc.chainPos)", gotPos, oldProc.chainPos)
	}
	if provName != chain[1].Provider {
		t.Errorf("new session resolved_provider = %q, want %q", provName, chain[1].Provider)
	}
	if mode != chain[1].ExecutionMode {
		t.Errorf("new session resolved_execution_mode = %q, want %q", mode, chain[1].ExecutionMode)
	}
}
