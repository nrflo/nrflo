package service

import (
	"context"
	"path/filepath"
	"reflect"
	"testing"
	"time"

	"be/internal/clock"
	"be/internal/db"
	"be/internal/types"
)

// setupPlainProjectNoFanout builds a fresh project/workflow with a single
// static agent_definitions row and no fanout_template — the negative fixture
// for IsPlanDriven.
func setupPlainProjectNoFanout(t *testing.T) (*db.Pool, string, string) {
	t.Helper()
	dbPath := filepath.Join(t.TempDir(), "no_fanout_test.db")
	if err := svcCopyTemplateDB(dbPath); err != nil {
		t.Fatalf("copy template DB: %v", err)
	}
	pool, err := db.OpenPoolExisting(dbPath, db.DefaultPoolConfig())
	if err != nil {
		t.Fatalf("open pool: %v", err)
	}
	t.Cleanup(func() { pool.Close() })

	const projectID = "no-fanout-proj"
	const workflowID = "no-fanout-wf"
	now := time.Now().UTC().Format(time.RFC3339Nano)
	mustExec(t, pool, `INSERT INTO projects (id, name, root_path, created_at, updated_at) VALUES (?, 'P', '/tmp', ?, ?)`,
		projectID, now, now)
	mustExec(t, pool, `INSERT INTO workflows (id, project_id, description, scope_type, created_at, updated_at) VALUES (?, ?, '', 'ticket', ?, ?)`,
		workflowID, projectID, now, now)
	mustExec(t, pool, `INSERT INTO agent_definitions (id, project_id, workflow_id, prompt, layer, model, execution_mode, created_at, updated_at)
		 VALUES ('static-1', ?, ?, 'p', 0, 'sonnet', 'cli_interactive', ?, ?)`, projectID, workflowID, now, now)

	return pool, projectID, workflowID
}

// TestIsPlanDriven_TrueWithFanoutTemplate_FalseWithout pairs setupPlanTestEnv's
// project (has a fanout_template def -> true) against a plain project with
// only static defs (-> false).
func TestIsPlanDriven_TrueWithFanoutTemplate_FalseWithout(t *testing.T) {
	t.Parallel()

	pool, _ := setupPlanTestEnv(t)
	driven, err := IsPlanDriven(pool, planTestProjectID, planTestWorkflowID)
	if err != nil {
		t.Fatalf("IsPlanDriven (with fanout_template): %v", err)
	}
	if !driven {
		t.Error("IsPlanDriven with a fanout_template def = false, want true")
	}

	pool2, projectID, workflowID := setupPlainProjectNoFanout(t)
	driven2, err := IsPlanDriven(pool2, projectID, workflowID)
	if err != nil {
		t.Fatalf("IsPlanDriven (no fanout_template): %v", err)
	}
	if driven2 {
		t.Error("IsPlanDriven with only static defs = true, want false")
	}
}

// TestLoadInstanceNodePhases_EmptyBeforeMaterialize_PopulatedAfter asserts the
// (empty, empty, nil) contract before any materialization, and correct field
// mapping from workflow_instance_nodes/_layer_policies after Revise+Approve.
func TestLoadInstanceNodePhases_EmptyBeforeMaterialize_PopulatedAfter(t *testing.T) {
	t.Parallel()
	pool, instanceID := setupPlanTestEnv(t)
	clk := clock.Real()

	phases, policies, err := LoadInstanceNodePhases(pool, clk, instanceID)
	if err != nil {
		t.Fatalf("LoadInstanceNodePhases before materialize: %v", err)
	}
	if len(phases) != 0 {
		t.Errorf("phases before materialize = %+v, want empty", phases)
	}
	if len(policies) != 0 {
		t.Errorf("policies before materialize = %+v, want empty", policies)
	}

	svc := NewPlanService(pool, clk, nil)
	rev, err := svc.Revise(context.Background(), instanceID, types.PlanReviseRequest{
		Revision: 0, Manifest: validPlanManifestJSON("goal", "do it"),
	})
	if err != nil {
		t.Fatalf("revise: %v", err)
	}
	if _, err := svc.Approve(instanceID, rev.Revision); err != nil {
		t.Fatalf("approve: %v", err)
	}

	phases2, policies2, err := LoadInstanceNodePhases(pool, clk, instanceID)
	if err != nil {
		t.Fatalf("LoadInstanceNodePhases after materialize: %v", err)
	}
	if len(phases2) != 1 {
		t.Fatalf("len(phases2) = %d, want 1", len(phases2))
	}
	got := phases2[0]
	want := SpawnerPhaseDef{NodeID: "step1", Agent: planTestTemplateID, Layer: 0, Instructions: "do it"}
	if got != want {
		t.Errorf("phases2[0] = %+v, want %+v", got, want)
	}
	if policies2[0] != "all" {
		t.Errorf("policies2[0] = %q, want %q", policies2[0], "all")
	}
}

// TestEffectivePhases_ConcatenatesStaticThenMaterialized_EmptyMaterializedReturnsStaticUnchanged
// is a pure-function test: no DB. Static entries come first in the given
// order, materialized second, and an empty/nil materialized slice returns a
// value-equal (not necessarily pointer-identical) copy of static.
func TestEffectivePhases_ConcatenatesStaticThenMaterialized_EmptyMaterializedReturnsStaticUnchanged(t *testing.T) {
	t.Parallel()
	static := []SpawnerPhaseDef{
		{NodeID: "a", Agent: "agent-a", Layer: 0},
		{NodeID: "b", Agent: "agent-b", Layer: 1},
	}
	materialized := []SpawnerPhaseDef{
		{NodeID: "step1", Agent: "template-1", Layer: 2, Instructions: "do it"},
	}

	combined := EffectivePhases(static, materialized)
	wantCombined := []SpawnerPhaseDef{static[0], static[1], materialized[0]}
	if !reflect.DeepEqual(combined, wantCombined) {
		t.Errorf("EffectivePhases(static, materialized) = %+v, want %+v", combined, wantCombined)
	}

	for _, empty := range [][]SpawnerPhaseDef{nil, {}} {
		got := EffectivePhases(static, empty)
		if !reflect.DeepEqual(got, static) {
			t.Errorf("EffectivePhases(static, %#v) = %+v, want %+v (value-equal to static)", empty, got, static)
		}
	}
}

// TestLoadMaterializedAgentConfigs_ResolvesModelTimeoutTag_SkipsDeletedTemplate
// asserts model/timeout/tag resolution for a materialized node's template, and
// that deleting the template def afterward makes LoadMaterializedAgentConfigs
// silently skip it (no entry, no error) rather than fail the whole read.
func TestLoadMaterializedAgentConfigs_ResolvesModelTimeoutTag_SkipsDeletedTemplate(t *testing.T) {
	t.Parallel()
	pool, instanceID := setupPlanTestEnv(t)
	clk := clock.Real()
	mustExec(t, pool, `UPDATE agent_definitions SET tag = 'ops', timeout = 45 WHERE id = ?`, planTestTemplateID)

	svc := NewPlanService(pool, clk, nil)
	rev, err := svc.Revise(context.Background(), instanceID, types.PlanReviseRequest{
		Revision: 0, Manifest: validPlanManifestJSON("goal", "do it"),
	})
	if err != nil {
		t.Fatalf("revise: %v", err)
	}
	if _, err := svc.Approve(instanceID, rev.Revision); err != nil {
		t.Fatalf("approve: %v", err)
	}

	materialized, _, err := LoadInstanceNodePhases(pool, clk, instanceID)
	if err != nil {
		t.Fatalf("LoadInstanceNodePhases: %v", err)
	}
	if len(materialized) != 1 {
		t.Fatalf("len(materialized) = %d, want 1", len(materialized))
	}

	configs := LoadMaterializedAgentConfigs(pool, clk, planTestProjectID, planTestWorkflowID, materialized)
	cfg, ok := configs[planTestTemplateID]
	if !ok {
		t.Fatalf("configs missing entry for %q: %+v", planTestTemplateID, configs)
	}
	want := SpawnerAgentConfig{Model: "sonnet", Timeout: 45, Tag: "ops"}
	if cfg != want {
		t.Errorf("configs[%q] = %+v, want %+v", planTestTemplateID, cfg, want)
	}

	mustExec(t, pool, `DELETE FROM agent_definitions WHERE id = ?`, planTestTemplateID)
	configs2 := LoadMaterializedAgentConfigs(pool, clk, planTestProjectID, planTestWorkflowID, materialized)
	if _, ok := configs2[planTestTemplateID]; ok {
		t.Errorf("configs2 still contains entry for deleted template %q: %+v", planTestTemplateID, configs2)
	}
}

// TestLoadMaterializedAgentConfigs_EmptyMaterialized_ReturnsNil is the pure
// edge case: nil and empty materialized slices both return a nil map.
func TestLoadMaterializedAgentConfigs_EmptyMaterialized_ReturnsNil(t *testing.T) {
	t.Parallel()
	pool, _ := setupPlanTestEnv(t)
	clk := clock.Real()

	if got := LoadMaterializedAgentConfigs(pool, clk, planTestProjectID, planTestWorkflowID, nil); got != nil {
		t.Errorf("LoadMaterializedAgentConfigs(nil) = %+v, want nil", got)
	}
	if got := LoadMaterializedAgentConfigs(pool, clk, planTestProjectID, planTestWorkflowID, []SpawnerPhaseDef{}); got != nil {
		t.Errorf("LoadMaterializedAgentConfigs(empty) = %+v, want nil", got)
	}
}
