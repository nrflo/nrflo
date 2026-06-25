package service

import (
	"path/filepath"
	"testing"

	"be/internal/clock"
	"be/internal/db"
)

// TestDeepResearchFindingSchemasValid guards the bundled schemas: the seed
// inserts them via direct SQL (bypassing validation), so a malformed schema or
// non-conforming example would otherwise only surface as an emit_findings
// failure at runtime.
func TestDeepResearchFindingSchemasValid(t *testing.T) {
	t.Parallel()
	defs := parseFindingSchemas(drFindingSchemas)
	if len(defs) != 6 {
		t.Fatalf("parsed %d finding schemas, want 6", len(defs))
	}
	if err := ValidateFindingSchemas(defs); err != nil {
		t.Fatalf("bundled deep-research finding schemas are invalid: %v", err)
	}
}

func TestEnsureGlobalDeepResearch(t *testing.T) {
	t.Parallel()
	dbPath := filepath.Join(t.TempDir(), "seed.db")
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
	if err := EnsureGlobalDeepResearch(pool, clk, t.TempDir()); err != nil {
		t.Fatalf("EnsureGlobalDeepResearch: %v", err)
	}
	// Idempotent: a second call is a create-if-absent no-op.
	if err := EnsureGlobalDeepResearch(pool, clk, t.TempDir()); err != nil {
		t.Fatalf("EnsureGlobalDeepResearch (2nd): %v", err)
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
		GlobalProjectID, DeepResearchWorkflow).Scan(&agents); err != nil {
		t.Fatal(err)
	}
	if agents != len(drAgents) {
		t.Errorf("agent defs = %d, want %d (no duplication after 2 seeds)", agents, len(drAgents))
	}

	var policy string
	if err := pool.QueryRow(`SELECT pass_policy FROM workflow_layer_policies WHERE project_id=? AND workflow_id=? AND layer=2`,
		GlobalProjectID, DeepResearchWorkflow).Scan(&policy); err != nil {
		t.Fatal(err)
	}
	if policy != "quorum:2" {
		t.Errorf("L2 pass_policy = %q, want quorum:2", policy)
	}

	// Agents ship as cli_interactive (claude/codex CLIs self-auth; no server API
	// credential), and verify_b is the codex GPT-5.5 cross-provider verifier.
	var nonCLI int
	if err := pool.QueryRow(`SELECT COUNT(*) FROM agent_definitions WHERE project_id=? AND workflow_id=? AND execution_mode<>'cli_interactive'`,
		GlobalProjectID, DeepResearchWorkflow).Scan(&nonCLI); err != nil {
		t.Fatal(err)
	}
	if nonCLI != 0 {
		t.Errorf("non-cli_interactive deep-research agents = %d, want 0", nonCLI)
	}
	var verifyBModel string
	if err := pool.QueryRow(`SELECT model FROM agent_definitions WHERE project_id=? AND workflow_id=? AND id='verify_b'`,
		GlobalProjectID, DeepResearchWorkflow).Scan(&verifyBModel); err != nil {
		t.Fatal(err)
	}
	if verifyBModel != "codex_gpt55_high" {
		t.Errorf("verify_b model = %q, want codex_gpt55_high", verifyBModel)
	}

	var isGlobal bool
	if err := pool.QueryRow(`SELECT is_global FROM workflows WHERE project_id=? AND id=?`,
		GlobalProjectID, DeepResearchWorkflow).Scan(&isGlobal); err != nil {
		t.Fatal(err)
	}
	if !isGlobal {
		t.Error("deep-research workflow row is_global = false, want true")
	}

	// End-to-end: from an unrelated project, GetWorkflowDef resolves via the
	// global fallback with all phases + the report finding schema.
	def, err := NewWorkflowService(pool, clk).GetWorkflowDef("some-other-project", DeepResearchWorkflow)
	if err != nil {
		t.Fatalf("GetWorkflowDef (global fallback): %v", err)
	}
	if len(def.Phases) != len(drAgents) {
		t.Errorf("phases = %d, want %d", len(def.Phases), len(drAgents))
	}
	hasReport := false
	for _, fs := range def.FindingSchemas {
		if fs.Key == "report" {
			hasReport = true
		}
	}
	if !hasReport {
		t.Error("missing 'report' finding schema")
	}
	if !def.IsGlobal {
		t.Error("GetWorkflowDef(global fallback).IsGlobal = false, want true")
	}
}

// TestListWorkflowDefs_GlobalUnionAndPrecedence verifies that a project's
// selectable workflow list unions in global definitions (flagged IsGlobal), and
// that a project-local definition shadows a same-named global one.
func TestListWorkflowDefs_GlobalUnionAndPrecedence(t *testing.T) {
	t.Parallel()
	dbPath := filepath.Join(t.TempDir(), "list.db")
	if err := svcCopyTemplateDB(dbPath); err != nil {
		t.Fatalf("copy template DB: %v", err)
	}
	pool, err := db.OpenPoolExisting(dbPath, db.DefaultPoolConfig())
	if err != nil {
		t.Fatalf("open pool: %v", err)
	}
	t.Cleanup(func() { pool.Close() })
	clk := clock.Real()
	now := "2026-01-01T00:00:00Z"

	if err := EnsureGlobalDeepResearch(pool, clk, t.TempDir()); err != nil {
		t.Fatalf("EnsureGlobalDeepResearch: %v", err)
	}
	if _, err := pool.Exec(`INSERT INTO projects (id, name, root_path, created_at, updated_at) VALUES ('p1','P1',NULL,?,?)`, now, now); err != nil {
		t.Fatalf("insert project: %v", err)
	}
	if _, err := pool.Exec(`INSERT INTO workflows (id, project_id, description, scope_type, groups, close_ticket_on_complete, purge_on_completion, is_global, finding_schemas, created_at, updated_at)
		VALUES ('feature','p1','Local feature','project','[]',0,0,0,'[]',?,?)`, now, now); err != nil {
		t.Fatalf("insert local workflow: %v", err)
	}

	svc := NewWorkflowService(pool, clk)

	defs, err := svc.ListWorkflowDefs("p1")
	if err != nil {
		t.Fatalf("ListWorkflowDefs: %v", err)
	}
	dr, ok := defs[DeepResearchWorkflow]
	if !ok {
		t.Fatal("global deep-research not present in project p1 listing")
	}
	if !dr.IsGlobal {
		t.Error("unioned deep-research IsGlobal = false, want true")
	}
	feat, ok := defs["feature"]
	if !ok {
		t.Fatal("local 'feature' missing from listing")
	}
	if feat.IsGlobal {
		t.Error("local feature IsGlobal = true, want false")
	}

	// Local precedence: a project-local workflow named 'deep-research' must shadow
	// the global one (same name → local wins, IsGlobal=false).
	if _, err := pool.Exec(`INSERT INTO workflows (id, project_id, description, scope_type, groups, close_ticket_on_complete, purge_on_completion, is_global, finding_schemas, created_at, updated_at)
		VALUES (?,'p1','Local override','project','[]',0,0,0,'[]',?,?)`, DeepResearchWorkflow, now, now); err != nil {
		t.Fatalf("insert local override: %v", err)
	}
	defs2, err := svc.ListWorkflowDefs("p1")
	if err != nil {
		t.Fatalf("ListWorkflowDefs (2): %v", err)
	}
	if got := defs2[DeepResearchWorkflow]; got.IsGlobal || got.Description != "Local override" {
		t.Errorf("local override did not shadow global: IsGlobal=%v description=%q", got.IsGlobal, got.Description)
	}
}
