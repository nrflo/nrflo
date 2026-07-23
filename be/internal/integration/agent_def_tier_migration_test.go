package integration

import (
	"database/sql"
	"path/filepath"
	"testing"

	"be/internal/clock"
	"be/internal/db"
	"be/internal/model"
	"be/internal/service"

	_ "modernc.org/sqlite"
)

// TestMigration200_RetierBackfillPreservesResolvedBehavior is the load-bearing
// behavior-preservation test for migration 000200: a stock def already at
// TierMap's recommended (model, effort, template) is switched to tier-driven
// (tier set, model/effort cleared) and RESOLVES to the exact same (model,
// effort) it had before the migration; a def still on its original seed
// model, a hand-customized def, and the hotfix implementor are all left
// completely untouched.
func TestMigration200_RetierBackfillPreservesResolvedBehavior(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "migrate200.db")
	sqlDB, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatalf("sql.Open: %v", err)
	}
	t.Cleanup(func() { sqlDB.Close() })

	m := buildMigrator(t, sqlDB)
	migrateTo(t, m, 199)

	now := "2026-07-20T00:00:00Z"
	if _, err := sqlDB.Exec(
		`INSERT INTO projects (id, name, root_path, created_at, updated_at) VALUES (?, ?, ?, ?, ?)`,
		"proj-200", "p200", "/tmp", now, now); err != nil {
		t.Fatalf("insert project: %v", err)
	}
	for _, wf := range []string{"feature", "hotfix"} {
		if _, err := sqlDB.Exec(
			`INSERT INTO workflows (id, project_id, description, scope_type, groups, created_at, updated_at) VALUES (?, ?, '', 'ticket', '[]', ?, ?)`,
			wf, "proj-200", now, now); err != nil {
			t.Fatalf("insert workflow %s: %v", wf, err)
		}
	}

	type seedDef struct {
		id, workflowID, model, effort, template string
	}
	defs := []seedDef{
		// Fully re-tiered stock def: exactly at TierMap's recommendation.
		{"setup-analyzer", "feature", "sonnet-5", "low", "tier-t2-extractor"},
		// Still on its original (pre-retier) seed model — left untouched.
		{"implementor", "feature", "opus-4-8", "medium", ""},
		// Hand-customized — left untouched.
		{"qa-verifier", "feature", "fable-5", "low", "tier-t2-extractor"},
		// Hotfix implementor: matches the re-tier shape but is explicitly excluded.
		{"implementor", "hotfix", "sonnet-5", "medium", "tier-t1-executor"},
	}
	for _, d := range defs {
		if _, err := sqlDB.Exec(
			`INSERT INTO agent_definitions (id, project_id, workflow_id, model, timeout, prompt, layer, reasoning_effort, node_role, consultant, system_template_id, created_at, updated_at)
			 VALUES (?, ?, ?, ?, 20, 'p', 0, ?, 'static', 0, ?, ?, ?)`,
			d.id, "proj-200", d.workflowID, d.model, d.effort, d.template, now, now,
		); err != nil {
			t.Fatalf("insert agent_def %s/%s: %v", d.workflowID, d.id, err)
		}
	}

	migrateTo(t, m, 200)
	sqlDB.Close()

	pool, err := db.OpenPoolExisting(dbPath, db.DefaultPoolConfig())
	if err != nil {
		t.Fatalf("open pool: %v", err)
	}
	t.Cleanup(func() { pool.Close() })
	clk := clock.Real()
	modelSvc := service.NewModelService(pool, clk)

	// setup-analyzer: re-tiered.
	var saModel, saTemplate string
	var saEffort sql.NullString
	var saTier sql.NullInt64
	if err := pool.QueryRow(
		`SELECT model, reasoning_effort, system_template_id, tier FROM agent_definitions WHERE project_id=? AND workflow_id=? AND id=?`,
		"proj-200", "feature", "setup-analyzer",
	).Scan(&saModel, &saEffort, &saTemplate, &saTier); err != nil {
		t.Fatalf("query setup-analyzer: %v", err)
	}
	if saModel != "" {
		t.Errorf("setup-analyzer model = %q, want '' (re-tiered)", saModel)
	}
	if saEffort.Valid {
		t.Errorf("setup-analyzer reasoning_effort = %+v, want NULL", saEffort)
	}
	if !saTier.Valid || saTier.Int64 != 2 {
		t.Errorf("setup-analyzer tier = %+v, want 2", saTier)
	}
	if saTemplate != "tier-t2-extractor" {
		t.Errorf("setup-analyzer system_template_id = %q, want tier-t2-extractor", saTemplate)
	}

	// Resolves to the exact same (model, effort) it had pre-migration.
	tier := 2
	def := &model.AgentDefinition{ID: "setup-analyzer", ExecutionMode: "cli_interactive", Tier: &tier}
	chain, err := service.ResolveDefChain(pool, clk, modelSvc, def)
	if err != nil {
		t.Fatalf("ResolveDefChain(setup-analyzer): %v", err)
	}
	if len(chain) == 0 || chain[0].ModelID != "sonnet-5" || chain[0].ReasoningEffort != "low" {
		t.Errorf("resolved chain = %+v, want primary sonnet-5/low (pre-migration values preserved)", chain)
	}

	// implementor (original seed model): untouched.
	var implModel string
	var implTier sql.NullInt64
	if err := pool.QueryRow(
		`SELECT model, tier FROM agent_definitions WHERE project_id=? AND workflow_id=? AND id=?`,
		"proj-200", "feature", "implementor",
	).Scan(&implModel, &implTier); err != nil {
		t.Fatalf("query implementor: %v", err)
	}
	if implModel != "opus-4-8" {
		t.Errorf("implementor model = %q, want opus-4-8 (untouched)", implModel)
	}
	if implTier.Valid {
		t.Errorf("implementor tier = %+v, want NULL (untouched)", implTier)
	}

	// qa-verifier (hand-customized): untouched.
	var qaModel string
	var qaTier sql.NullInt64
	if err := pool.QueryRow(
		`SELECT model, tier FROM agent_definitions WHERE project_id=? AND workflow_id=? AND id=?`,
		"proj-200", "feature", "qa-verifier",
	).Scan(&qaModel, &qaTier); err != nil {
		t.Fatalf("query qa-verifier: %v", err)
	}
	if qaModel != "fable-5" {
		t.Errorf("qa-verifier model = %q, want fable-5 (untouched)", qaModel)
	}
	if qaTier.Valid {
		t.Errorf("qa-verifier tier = %+v, want NULL (untouched)", qaTier)
	}

	// hotfix implementor: untouched despite matching the re-tier shape.
	var hotfixModel string
	var hotfixTier sql.NullInt64
	if err := pool.QueryRow(
		`SELECT model, tier FROM agent_definitions WHERE project_id=? AND workflow_id=? AND id=?`,
		"proj-200", "hotfix", "implementor",
	).Scan(&hotfixModel, &hotfixTier); err != nil {
		t.Fatalf("query hotfix implementor: %v", err)
	}
	if hotfixModel != "sonnet-5" {
		t.Errorf("hotfix implementor model = %q, want sonnet-5 (untouched)", hotfixModel)
	}
	if hotfixTier.Valid {
		t.Errorf("hotfix implementor tier = %+v, want NULL (untouched)", hotfixTier)
	}
}
