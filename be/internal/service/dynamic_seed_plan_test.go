package service

import (
	"path/filepath"
	"testing"
	"time"

	"be/internal/clock"
	"be/internal/db"
	"be/internal/types"
)

// seedDynamicWorkflowDB returns a pool with the global dynamic workflow seeded.
func seedDynamicWorkflowDB(t *testing.T, name string) *db.Pool {
	t.Helper()
	dbPath := filepath.Join(t.TempDir(), name)
	if err := svcCopyTemplateDB(dbPath); err != nil {
		t.Fatalf("copy template DB: %v", err)
	}
	pool, err := db.OpenPoolExisting(dbPath, db.DefaultPoolConfig())
	if err != nil {
		t.Fatalf("open pool: %v", err)
	}
	t.Cleanup(func() { pool.Close() })
	if err := EnsureGlobalDynamicWorkflow(pool, clock.Real(), t.TempDir()); err != nil {
		t.Fatalf("EnsureGlobalDynamicWorkflow: %v", err)
	}
	return pool
}

// TestAllowedTemplates_DynamicWorkflow_ExcludesPlannerReturnsAllFanoutTemplates
// verifies the plan catalog for the seeded global `dynamic` workflow is
// exactly its 10 fanout_template defs — the workflow-local planner override
// must never appear in the catalog a planner (or the plan UI) can bind a
// node to.
func TestAllowedTemplates_DynamicWorkflow_ExcludesPlannerReturnsAllFanoutTemplates(t *testing.T) {
	t.Parallel()
	pool := seedDynamicWorkflowDB(t, "allowed_templates.db")

	templates, err := AllowedTemplates(pool, GlobalProjectID, DynamicWorkflow)
	if err != nil {
		t.Fatalf("AllowedTemplates: %v", err)
	}
	if len(templates) != 10 {
		t.Fatalf("AllowedTemplates(dynamic) = %d templates, want 10", len(templates))
	}
	for _, tpl := range templates {
		if tpl.ID == "dynamic-planner" {
			t.Fatal("AllowedTemplates(dynamic) included the planner def dynamic-planner")
		}
	}
}

// TestEnabledTemplates_DynamicWorkflow_DropsDisabledCodexTemplates verifies
// graceful degradation: when OpenAI CLI-capable model rows are disabled,
// EnabledTemplates filters the -codex twins out of the ${TEMPLATE_LIBRARY}
// while every claude-backed template stays.
func TestEnabledTemplates_DynamicWorkflow_DropsDisabledCodexTemplates(t *testing.T) {
	t.Parallel()
	pool := seedDynamicWorkflowDB(t, "enabled_templates.db")

	if _, err := pool.Exec(`UPDATE models SET enabled = 0 WHERE provider = 'openai' AND cli_model <> ''`); err != nil {
		t.Fatalf("disable OpenAI CLI models: %v", err)
	}

	all, err := AllowedTemplates(pool, GlobalProjectID, DynamicWorkflow)
	if err != nil {
		t.Fatalf("AllowedTemplates: %v", err)
	}
	enabled := EnabledTemplates(pool, clock.Real(), all)

	if len(enabled) != len(all)-2 {
		t.Fatalf("EnabledTemplates count = %d, want %d (10 - 2 codex twins)", len(enabled), len(all)-2)
	}
	for _, tpl := range enabled {
		if tpl.ID == "module-reviewer-codex" || tpl.ID == "finding-verifier-codex" {
			t.Errorf("EnabledTemplates kept %q, which is bound to a disabled codex model", tpl.ID)
		}
	}
}

// TestValidatePlanManifest_AcceptsSeededDynamicWorkflow is acceptance case
// #3: a representative map -> verify(quorum:2, claude+codex) -> reduce
// manifest, bound only to seeded template ids, must pass
// ValidatePlanManifest against the real seeded global workflow with no
// edits — proving the shipped catalog + finding-schema wiring is internally
// consistent end to end.
func TestValidatePlanManifest_AcceptsSeededDynamicWorkflow(t *testing.T) {
	t.Parallel()
	pool := seedDynamicWorkflowDB(t, "accept_manifest.db")

	m := PlanManifest{
		Version: 1,
		Goal:    "Review the changed module for correctness across two providers",
		Layers: []PlanLayer{
			{
				Layer:  0,
				Policy: "all",
				Nodes: []PlanNode{
					{ID: "map", Template: "codebase-explorer", Instructions: "Locate the files touched by this change and summarize their structure."},
				},
			},
			{
				Layer:  1,
				Policy: "quorum:2",
				Nodes: []PlanNode{
					{ID: "verify-claude", Template: "module-reviewer", Instructions: "Review the module found by #{NODE_FINDINGS:map} for correctness."},
					{ID: "verify-codex", Template: "module-reviewer-codex", Instructions: "Independently review the module found by #{NODE_FINDINGS:map} for correctness."},
				},
			},
			{
				Layer:  2,
				Policy: "any",
				Nodes: []PlanNode{
					{ID: "reduce", Template: "synthesizer", Instructions: "Merge #{NODE_FINDINGS:verify-claude} and #{NODE_FINDINGS:verify-codex} into one final verdict."},
				},
			},
		},
	}

	if err := ValidatePlanManifest(pool, GlobalProjectID, DynamicWorkflow, m); err != nil {
		t.Fatalf("ValidatePlanManifest rejected a manifest bound to seeded templates: %v", err)
	}
}

// TestAllowedTemplates_ProjectLocalDynamicWorkflowShadowsGlobal verifies the
// per-workflow resolution rule (plan_templates.go AllowedTemplates /
// plan_templates.go:29-36): a project that defines its own `dynamic`
// workflow with a local fanout_template gets ITS catalog, not the global
// one — because the workflow row itself resolves locally, so
// defProjectID never falls back to __global__.
func TestAllowedTemplates_ProjectLocalDynamicWorkflowShadowsGlobal(t *testing.T) {
	t.Parallel()
	pool := seedDynamicWorkflowDB(t, "shadow.db")

	now := time.Now().UTC().Format(time.RFC3339Nano)
	if _, err := pool.Exec(`INSERT INTO projects (id, name, root_path, created_at, updated_at) VALUES ('proj-shadow','P','/tmp',?,?)`, now, now); err != nil {
		t.Fatalf("insert project: %v", err)
	}
	wfSvc := NewWorkflowService(pool, clock.Real())
	if _, err := wfSvc.CreateWorkflowDef("proj-shadow", &types.WorkflowDefCreateRequest{ID: DynamicWorkflow}); err != nil {
		t.Fatalf("create project-local dynamic workflow: %v", err)
	}
	insertFanoutTemplate(t, pool, "proj-shadow", DynamicWorkflow, "module-reviewer", "sonnet-5", "cli_interactive")

	templates, err := AllowedTemplates(pool, "proj-shadow", DynamicWorkflow)
	if err != nil {
		t.Fatalf("AllowedTemplates: %v", err)
	}
	if len(templates) != 1 {
		t.Fatalf("AllowedTemplates(proj-shadow, dynamic) = %d templates, want 1 (local catalog only)", len(templates))
	}
	if templates[0].ID != "module-reviewer" {
		t.Errorf("AllowedTemplates(proj-shadow, dynamic)[0].ID = %q, want module-reviewer", templates[0].ID)
	}

	global, err := AllowedTemplates(pool, GlobalProjectID, DynamicWorkflow)
	if err != nil {
		t.Fatalf("AllowedTemplates(global): %v", err)
	}
	if len(global) != 10 {
		t.Errorf("shadowing a project-local dynamic workflow must not affect the global catalog: got %d, want 10", len(global))
	}
}
