package spawner

import (
	"context"
	"database/sql"
	"errors"
	"strings"
	"testing"

	"be/internal/clock"
	"be/internal/db"
	"be/internal/spawner/apirun/provider"
	"be/internal/spawner/apirun/provider/mock"
)

// querySessionResolution reads back the observability columns recordResolvedSpawn writes.
func querySessionResolution(t *testing.T, database *db.DB, sessionID string) (tier sql.NullInt64, provider, mode, effort string, chainPos int, fallbackFrom sql.NullString) {
	t.Helper()
	if err := database.QueryRow(
		`SELECT tier, resolved_provider, resolved_execution_mode, resolved_effort, chain_position, fallback_from
		 FROM agent_sessions WHERE id = ?`, sessionID,
	).Scan(&tier, &provider, &mode, &effort, &chainPos, &fallbackFrom); err != nil {
		t.Fatalf("query session resolution for %s: %v", sessionID, err)
	}
	return
}

// TestRecordResolvedSpawn_ChainFallback verifies the session row written for
// a build-fallback spawn (entry 0 build-fails, entry 1 wins) carries
// chain_position=1, the winning entry's resolved provider/mode/effort/tier,
// and a fallback_from JSON array containing entry 0's provider/model.
func TestRecordResolvedSpawn_ChainFallback(t *testing.T) {
	t.Parallel()
	ensureTmpNrfloDir(t)
	env := setupContextSaveTestEnv(t)
	defer env.cleanup()
	insertAPIAgentDef(t, env, "impl", "bad-model")

	sp := New(Config{
		DataPath: env.dbPath,
		Pool:     db.WrapAsPool(env.database),
		Clock:    clock.Real(),
		APIMode:  true,
		BuildAPIProvider: func(_ context.Context, providerName, _ string) (provider.Provider, error) {
			if providerName == "badprov" {
				return nil, errors.New("missing credentials")
			}
			return mock.New(), nil
		},
		ModelConfigs: buildFallbackModelConfigs(),
		AgentSvc:     &noopAgentSvc{},
	})

	chain := buildFallbackChain()
	proc, chainPos, err := sp.spawnEntryWithBuildFallback(context.Background(), SpawnRequest{
		AgentType:          "impl",
		ProjectID:          env.projectID,
		WorkflowName:       "feature",
		WorkflowInstanceID: env.wfiID,
	}, "unused", "impl", env.wfiID, chain)
	if err != nil {
		t.Fatalf("spawnEntryWithBuildFallback() error: %v", err)
	}
	if chainPos != 1 {
		t.Fatalf("chainPos = %d, want 1", chainPos)
	}

	tier, provName, mode, effort, gotPos, fallbackFrom := querySessionResolution(t, env.database, proc.sessionID)
	if !tier.Valid || tier.Int64 != int64(chain[1].Tier) {
		t.Errorf("tier = %+v, want %d (winning entry's tier)", tier, chain[1].Tier)
	}
	if provName != "goodprov" {
		t.Errorf("resolved_provider = %q, want goodprov", provName)
	}
	if mode != "api" {
		t.Errorf("resolved_execution_mode = %q, want api", mode)
	}
	if effort != "low" {
		t.Errorf("resolved_effort = %q, want low", effort)
	}
	if gotPos != 1 {
		t.Errorf("chain_position = %d, want 1", gotPos)
	}
	if !fallbackFrom.Valid {
		t.Fatal("fallback_from is NULL, want the failed entry-0 JSON")
	}
	if !strings.Contains(fallbackFrom.String, "badprov") || !strings.Contains(fallbackFrom.String, "bad-model") {
		t.Errorf("fallback_from = %q, want it to contain entry 0's provider/model (badprov/bad-model)", fallbackFrom.String)
	}
}

// TestRecordResolvedSpawn_EmptyChain verifies the chain-less main-phase path:
// chain_position=0, fallback_from stays NULL, resolved_provider derives from
// ModelConfigs, and resolved_execution_mode is proc.effectiveMode.
func TestRecordResolvedSpawn_EmptyChain(t *testing.T) {
	t.Parallel()
	ensureTmpNrfloDir(t)
	env := setupContextSaveTestEnv(t)
	defer env.cleanup()
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
			"sonnet-5": {Provider: "anthropic", APIModel: "claude-sonnet-4-6", APIContext: 200000},
		},
		AgentSvc: &noopAgentSvc{},
	})

	proc, chainPos, err := sp.spawnEntryWithBuildFallback(context.Background(), SpawnRequest{
		AgentType:          "impl",
		ProjectID:          env.projectID,
		WorkflowName:       "feature",
		WorkflowInstanceID: env.wfiID,
	}, "claude:sonnet-5", "impl", env.wfiID, nil)
	if err != nil {
		t.Fatalf("spawnEntryWithBuildFallback() error: %v", err)
	}
	if chainPos != 0 {
		t.Fatalf("chainPos = %d, want 0", chainPos)
	}

	tier, provName, mode, _, gotPos, fallbackFrom := querySessionResolution(t, env.database, proc.sessionID)
	if tier.Valid {
		t.Errorf("tier = %+v, want NULL (empty chain never resolves a tier)", tier)
	}
	if provName != "anthropic" {
		t.Errorf("resolved_provider = %q, want anthropic (from ModelConfigs)", provName)
	}
	if mode != proc.effectiveMode {
		t.Errorf("resolved_execution_mode = %q, want proc.effectiveMode %q", mode, proc.effectiveMode)
	}
	if gotPos != 0 {
		t.Errorf("chain_position = %d, want 0", gotPos)
	}
	if fallbackFrom.Valid {
		t.Errorf("fallback_from = %+v, want NULL for an empty chain", fallbackFrom)
	}
}

// TestRecordResolvedSpawn_NilPool_NoOp verifies recordResolvedSpawn no-ops
// (never panics) when the spawner has no pool configured.
func TestRecordResolvedSpawn_NilPool_NoOp(t *testing.T) {
	t.Parallel()
	sp := New(Config{APIMode: true, AgentSvc: &noopAgentSvc{}})
	proc := &processInfo{sessionID: "sess-no-pool"}
	sp.recordResolvedSpawn(proc, buildFallbackChain(), 0)
}
