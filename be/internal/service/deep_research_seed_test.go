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
	if err := EnsureGlobalDeepResearch(pool, clk); err != nil {
		t.Fatalf("EnsureGlobalDeepResearch: %v", err)
	}
	// Idempotent: a second call is a create-if-absent no-op.
	if err := EnsureGlobalDeepResearch(pool, clk); err != nil {
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
}
