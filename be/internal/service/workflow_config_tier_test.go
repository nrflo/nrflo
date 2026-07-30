package service

import (
	"path/filepath"
	"testing"

	"be/internal/clock"
	"be/internal/db"
	"be/internal/model"
)

func setupWorkflowConfigTierTestEnv(t *testing.T) *db.Pool {
	t.Helper()
	dbPath := filepath.Join(t.TempDir(), "workflow_config_tier_test.db")
	if err := svcCopyTemplateDB(dbPath); err != nil {
		t.Fatalf("copy template DB: %v", err)
	}
	pool, err := db.OpenPoolExisting(dbPath, db.DefaultPoolConfig())
	if err != nil {
		t.Fatalf("open pool: %v", err)
	}
	t.Cleanup(func() { pool.Close() })
	return pool
}

// TestBuildSpawnerConfig_TierOnlyDef_FillsModelEffortAndChain verifies a def
// with Model=="" && Tier!=nil resolves its chain, setting Model/ReasoningEffort
// from chain[0] and populating Chain.
func TestBuildSpawnerConfig_TierOnlyDef_FillsModelEffortAndChain(t *testing.T) {
	t.Parallel()
	pool := setupWorkflowConfigTierTestEnv(t)
	clk := clock.Real()

	tier := 1
	wf := &model.Workflow{ID: "feature", ProjectID: "p1"}
	def := &model.AgentDefinition{ID: "impl", ProjectID: "p1", WorkflowID: "feature", ExecutionMode: "api", Tier: &tier, Layer: 0}

	_, agents := BuildSpawnerConfig(pool, clk, []*model.Workflow{wf}, []*model.AgentDefinition{def})

	cfg, ok := agents["impl"]
	if !ok {
		t.Fatal("agents missing entry for impl")
	}
	if cfg.Model != "haiku-4-5" {
		t.Errorf("Model = %q, want haiku-4-5 (tier1 chain primary)", cfg.Model)
	}
	if cfg.ReasoningEffort == nil || *cfg.ReasoningEffort != "low" {
		t.Errorf("ReasoningEffort = %v, want low", cfg.ReasoningEffort)
	}
	if len(cfg.Chain) != 3 {
		t.Errorf("Chain length = %d, want 3 (tier1 chain incl. 000220 codex hop)", len(cfg.Chain))
	}
}

// TestBuildSpawnerConfig_OverrideDef_LeavesChainNilAndValuesUntouched verifies
// a def carrying an explicit model override keeps Chain nil and its raw
// model/effort untouched (byte-identical to pre-tiering behavior).
func TestBuildSpawnerConfig_OverrideDef_LeavesChainNilAndValuesUntouched(t *testing.T) {
	t.Parallel()
	pool := setupWorkflowConfigTierTestEnv(t)
	clk := clock.Real()

	effort := "high"
	wf := &model.Workflow{ID: "feature", ProjectID: "p1"}
	def := &model.AgentDefinition{
		ID: "impl", ProjectID: "p1", WorkflowID: "feature", ExecutionMode: "api",
		Model: "opus-4-8", ReasoningEffort: &effort, Layer: 0,
	}

	_, agents := BuildSpawnerConfig(pool, clk, []*model.Workflow{wf}, []*model.AgentDefinition{def})

	cfg, ok := agents["impl"]
	if !ok {
		t.Fatal("agents missing entry for impl")
	}
	if cfg.Model != "opus-4-8" {
		t.Errorf("Model = %q, want opus-4-8 (untouched override)", cfg.Model)
	}
	if cfg.ReasoningEffort == nil || *cfg.ReasoningEffort != "high" {
		t.Errorf("ReasoningEffort = %v, want high (untouched override)", cfg.ReasoningEffort)
	}
	if cfg.Chain != nil {
		t.Errorf("Chain = %+v, want nil for an override def", cfg.Chain)
	}
}

// TestBuildSpawnerConfig_TierWithNoResolvableChain_DegradesToRawModel
// verifies a def whose tier has no resolvable chain (invalid model row in
// the chain) degrades to the raw (empty) model instead of failing the whole
// call — mirroring LoadMaterializedAgentConfigs' continue-on-error style.
func TestBuildSpawnerConfig_TierWithNoResolvableChain_DegradesToRawModel(t *testing.T) {
	t.Parallel()
	pool := setupWorkflowConfigTierTestEnv(t)
	clk := clock.Real()

	if _, err := pool.Exec(
		`INSERT INTO tier_models (tier, position, provider, execution_mode, model_id, reasoning_effort)
		 VALUES (3, 9, 'anthropic', 'api', 'does-not-exist', 'low')`,
	); err != nil {
		t.Fatalf("insert bogus tier_models row: %v", err)
	}

	tier := 3
	wf := &model.Workflow{ID: "feature", ProjectID: "p1"}
	def := &model.AgentDefinition{ID: "impl", ProjectID: "p1", WorkflowID: "feature", ExecutionMode: "api", Tier: &tier, Layer: 0}

	_, agents := BuildSpawnerConfig(pool, clk, []*model.Workflow{wf}, []*model.AgentDefinition{def})

	cfg, ok := agents["impl"]
	if !ok {
		t.Fatal("agents missing entry for impl")
	}
	if cfg.Model != "" {
		t.Errorf("Model = %q, want '' (degraded, raw model was empty)", cfg.Model)
	}
	if cfg.Chain != nil {
		t.Errorf("Chain = %+v, want nil when resolution fails", cfg.Chain)
	}
}
