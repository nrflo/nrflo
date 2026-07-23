package spawner

import (
	"context"
	"testing"
	"time"

	"be/internal/clock"
	"be/internal/db"
	"be/internal/spawner/apirun/provider"
	"be/internal/spawner/apirun/provider/mock"
)

// TestLowConsumptionOverride_ClearsResolvedChain drives the real Spawn() path
// end-to-end (agent_definitions.low_consumption_model set + Config.Agents
// carrying a resolved 2-entry chain) and reads back the persisted
// agent_sessions row to prove the LC-override-wins invariant: chain-position
// stays 0 and tier stays NULL (i.e. spawnEntryWithBuildFallback ran with a
// nil chain), and BuildAPIProvider is only ever invoked for the LC model's
// provider — chain[0] never wins.
func TestLowConsumptionOverride_ClearsResolvedChain(t *testing.T) {
	t.Parallel()
	ensureTmpNrfloDir(t)
	env := setupContextSaveTestEnv(t)
	defer env.cleanup()
	insertAPIAgentDefWithLC(t, env, "impl", "chain-primary", "lc-model")

	var builtProviders []string
	sp := New(Config{
		DataPath:           env.dbPath,
		Pool:               db.WrapAsPool(env.database),
		Clock:              clock.Real(),
		APIMode:            true,
		LowConsumptionMode: true,
		Workflows: map[string]WorkflowDef{
			"feature": {Phases: []PhaseDef{{NodeID: "impl", Agent: "impl", Layer: 0}}},
		},
		Agents: map[string]AgentConfig{
			"impl": {
				Model: "chain-primary",
				Chain: buildFallbackChain(), // badprov/bad-model, goodprov/good-model
			},
		},
		BuildAPIProvider: func(_ context.Context, providerName, _ string) (provider.Provider, error) {
			builtProviders = append(builtProviders, providerName)
			return mock.New(mock.Script{Final: provider.FinalResponse{StopReason: "end_turn"}}), nil
		},
		ModelConfigs: mergeModelConfigs(buildFallbackModelConfigs(), map[string]ModelConfig{
			"lc-model": {Provider: "lcprov", APIModel: "lc-x", APIContext: 100000, APIEfforts: []string{"low"}, DefaultEffort: "low"},
		}),
		AgentSvc: &noopAgentSvc{},
	})

	// Spawn() blocks in monitorAll until the (mocked, scriptless) turn
	// resolves; the LC-override chain-clearing this test targets happens
	// entirely in the prep phase before that, so a FAIL completion here is
	// irrelevant — only the persisted session row (written at spawn time)
	// and the providers actually built are asserted below.
	_ = sp.Spawn(context.Background(), SpawnRequest{
		AgentType:          "impl",
		ProjectID:          env.projectID,
		WorkflowName:       "feature",
		WorkflowInstanceID: env.wfiID,
	})

	if len(builtProviders) != 1 || builtProviders[0] != "lcprov" {
		t.Errorf("BuildAPIProvider calls = %v, want exactly one call for lcprov (chain[0]'s badprov must never win)", builtProviders)
	}

	var sessionID string
	if err := env.database.QueryRow(
		`SELECT id FROM agent_sessions WHERE phase = 'impl' ORDER BY created_at DESC LIMIT 1`,
	).Scan(&sessionID); err != nil {
		t.Fatalf("query session id: %v", err)
	}
	tier, provName, _, _, chainPos, fallbackFrom := querySessionResolution(t, env.database, sessionID)
	if tier.Valid {
		t.Errorf("tier = %+v, want NULL (LC override must clear the resolved chain)", tier)
	}
	if chainPos != 0 {
		t.Errorf("chain_position = %d, want 0", chainPos)
	}
	if fallbackFrom.Valid {
		t.Errorf("fallback_from = %+v, want NULL (no chain to fall back through)", fallbackFrom)
	}
	if provName != "lcprov" {
		t.Errorf("resolved_provider = %q, want lcprov", provName)
	}
}

// insertAPIAgentDefWithLC inserts an agent_definition row with execution_mode=api,
// the given model, and a low_consumption_model override.
func insertAPIAgentDefWithLC(t *testing.T, env *contextSaveTestEnv, agentID, model, lcModel string) {
	t.Helper()
	now := time.Now().UTC().Format(time.RFC3339Nano)
	if _, err := env.database.Exec(
		`INSERT INTO agent_definitions
			(id, project_id, workflow_id, model, timeout, prompt, execution_mode, tools, low_consumption_model, created_at, updated_at)
		VALUES (?, ?, 'feature', ?, 20, '# prompt', 'api', 'agent_finished', ?, ?, ?)`,
		agentID, env.projectID, model, lcModel, now, now,
	); err != nil {
		t.Fatalf("insert agent_definition %q: %v", agentID, err)
	}
}

// mergeModelConfigs returns a new map combining a and b (b wins on key overlap).
func mergeModelConfigs(a, b map[string]ModelConfig) map[string]ModelConfig {
	out := make(map[string]ModelConfig, len(a)+len(b))
	for k, v := range a {
		out[k] = v
	}
	for k, v := range b {
		out[k] = v
	}
	return out
}
