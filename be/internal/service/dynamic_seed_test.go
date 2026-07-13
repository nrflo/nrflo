package service

import (
	"path/filepath"
	"strings"
	"testing"

	"be/internal/clock"
	"be/internal/db"
)

func TestEnsureGlobalDynamicWorkflow(t *testing.T) {
	t.Parallel()
	dbPath := filepath.Join(t.TempDir(), "dynamic_seed.db")
	if err := svcCopyTemplateDB(dbPath); err != nil {
		t.Fatalf("copy template DB: %v", err)
	}
	pool, err := db.OpenPoolExisting(dbPath, db.DefaultPoolConfig())
	if err != nil {
		t.Fatalf("open pool: %v", err)
	}
	t.Cleanup(func() { pool.Close() })
	clk := clock.Real()

	// Seeds against the real migrated schema; a missing NOT-NULL column would error here.
	if err := EnsureGlobalDynamicWorkflow(pool, clk, t.TempDir()); err != nil {
		t.Fatalf("EnsureGlobalDynamicWorkflow: %v", err)
	}
	// Idempotent: a second call is a create-if-absent no-op.
	if err := EnsureGlobalDynamicWorkflow(pool, clk, t.TempDir()); err != nil {
		t.Fatalf("EnsureGlobalDynamicWorkflow (2nd): %v", err)
	}

	var projects int
	if err := pool.QueryRow(`SELECT COUNT(*) FROM projects WHERE id=?`, GlobalProjectID).Scan(&projects); err != nil {
		t.Fatal(err)
	}
	if projects != 1 {
		t.Errorf("global project count = %d, want 1", projects)
	}

	var agents int
	if err := pool.QueryRow(`SELECT COUNT(*) FROM agent_definitions WHERE project_id=? AND workflow_id=?`,
		GlobalProjectID, DynamicWorkflow).Scan(&agents); err != nil {
		t.Fatal(err)
	}
	if agents != len(dynAgents) {
		t.Errorf("agent defs = %d, want %d (no duplication after 2 seeds)", agents, len(dynAgents))
	}

	// All seeded defs ship cli_interactive (no server-side API credential
	// needed; self-authenticating CLIs), and every def is either the
	// workflow-local planner override or a fanout_template — never static
	// (a plan-driven workflow must have zero executable phases).
	var nonTemplate int
	if err := pool.QueryRow(`SELECT COUNT(*) FROM agent_definitions WHERE project_id=? AND workflow_id=? AND node_role NOT IN ('fanout_template','planner')`,
		GlobalProjectID, DynamicWorkflow).Scan(&nonTemplate); err != nil {
		t.Fatal(err)
	}
	if nonTemplate != 0 {
		t.Errorf("dynamic agents outside {fanout_template,planner} = %d, want 0", nonTemplate)
	}
	var fanoutCount int
	if err := pool.QueryRow(`SELECT COUNT(*) FROM agent_definitions WHERE project_id=? AND workflow_id=? AND node_role='fanout_template'`,
		GlobalProjectID, DynamicWorkflow).Scan(&fanoutCount); err != nil {
		t.Fatal(err)
	}
	if fanoutCount != 10 {
		t.Errorf("fanout_template dynamic agents = %d, want 10", fanoutCount)
	}
	var plannerCount int
	if err := pool.QueryRow(`SELECT COUNT(*) FROM agent_definitions WHERE project_id=? AND workflow_id=? AND node_role='planner'`,
		GlobalProjectID, DynamicWorkflow).Scan(&plannerCount); err != nil {
		t.Fatal(err)
	}
	if plannerCount != 1 {
		t.Errorf("planner dynamic agents = %d, want 1", plannerCount)
	}
	var nonCLI int
	if err := pool.QueryRow(`SELECT COUNT(*) FROM agent_definitions WHERE project_id=? AND workflow_id=? AND execution_mode<>'cli_interactive'`,
		GlobalProjectID, DynamicWorkflow).Scan(&nonCLI); err != nil {
		t.Fatal(err)
	}
	if nonCLI != 0 {
		t.Errorf("non-cli_interactive dynamic agents = %d, want 0", nonCLI)
	}

	// The synthesizer template must carry the workflow_final_result completion
	// contract — it's the only value the caller of a dynamic sub-run reads back.
	var synthPrompt string
	if err := pool.QueryRow(`SELECT prompt FROM agent_definitions WHERE project_id=? AND workflow_id=? AND id='synthesizer'`,
		GlobalProjectID, DynamicWorkflow).Scan(&synthPrompt); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(synthPrompt, "workflow_final_result") {
		t.Error("synthesizer prompt missing 'workflow_final_result' completion contract")
	}

	var isGlobal, callableAsSubworkflow bool
	var purgeOnCompletion bool
	if err := pool.QueryRow(`SELECT is_global, callable_as_subworkflow, purge_on_completion FROM workflows WHERE project_id=? AND id=?`,
		GlobalProjectID, DynamicWorkflow).Scan(&isGlobal, &callableAsSubworkflow, &purgeOnCompletion); err != nil {
		t.Fatal(err)
	}
	if !isGlobal {
		t.Error("dynamic workflow row is_global = false, want true")
	}
	if !callableAsSubworkflow {
		t.Error("dynamic workflow row callable_as_subworkflow = false, want true")
	}
	if purgeOnCompletion {
		t.Error("dynamic workflow row purge_on_completion = true, want false")
	}

	// End-to-end: from an unrelated project, GetWorkflowDef resolves via the
	// global fallback. The workflow is intentionally phase-less (plan-driven,
	// not static): zero agent_definitions with node_role='static'.
	def, err := NewWorkflowService(pool, clk).GetWorkflowDef("some-other-project", DynamicWorkflow)
	if err != nil {
		t.Fatalf("GetWorkflowDef (global fallback): %v", err)
	}
	if !def.IsGlobal {
		t.Error("GetWorkflowDef(global fallback).IsGlobal = false, want true")
	}
	if len(def.Phases) != 0 {
		t.Errorf("phases = %d, want 0 (plan-driven workflow has no static phases)", len(def.Phases))
	}

	planDriven, err := IsPlanDriven(pool, GlobalProjectID, DynamicWorkflow)
	if err != nil {
		t.Fatalf("IsPlanDriven: %v", err)
	}
	if !planDriven {
		t.Error("IsPlanDriven(dynamic) = false, want true")
	}
}
