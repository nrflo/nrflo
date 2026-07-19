package db

import (
	"database/sql"
	"path/filepath"
	"strings"
	"testing"

	"github.com/golang-migrate/migrate/v4"
)

// preTierPolicyPlannerPrompt mirrors the exact pre-migration anchor
// dynamic_seed_planner.go's dynPlannerPrompt carried (verbatim, per the
// REPLACE idiom in 000175_dynwf_cheap_tier_defaults.up.sql) so the migration's
// REPLACE has something real to match against.
const preTierPolicyPlannerPrompt = `# Dynamic Workflow Planner

...delegation doctrine omitted for test brevity...

- Never invent a template, model, tool, or finding key that is not in the library above. If the library is missing something you need, do not substitute a similar template silently — emit a question in questions[] describing the gap instead.

## Manifest Schema (version 1)

...schema omitted for test brevity...`

// TestMigration175_DynamicPlannerPromptGetsTierPolicy verifies the
// agent_definitions REPLACE: an existing __global__/dynamic/dynamic-planner
// row (create-if-absent seeded before this ticket, so its prompt predates the
// Tier Policy section) gets the section inserted right before the manifest
// schema.
func TestMigration175_DynamicPlannerPromptGetsTierPolicy(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "migration175-planner.db")
	sqlDB, err := sql.Open("sqlite", buildDSN(dbPath))
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	t.Cleanup(func() { sqlDB.Close() })

	m := newMigrateInstance(t, sqlDB)
	if err := m.Migrate(174); err != nil {
		t.Fatalf("migrate to 174: %v", err)
	}

	seedPre175GlobalDynamicWorkflow(t, sqlDB)

	if err := m.Up(); err != nil && err != migrate.ErrNoChange {
		t.Fatalf("migrate remaining (000175): %v", err)
	}

	var prompt string
	if err := sqlDB.QueryRow(`SELECT prompt FROM agent_definitions WHERE project_id = '__global__' AND workflow_id = 'dynamic' AND id = 'dynamic-planner'`).Scan(&prompt); err != nil {
		t.Fatalf("query dynamic-planner prompt: %v", err)
	}
	if !strings.Contains(prompt, "## Tier Policy") {
		t.Error("dynamic-planner prompt missing ## Tier Policy after migration 000175")
	}
	if !strings.Contains(prompt, "dynwf_max_premium_workers") {
		t.Error("dynamic-planner prompt missing the premium-cap directive after migration 000175")
	}
	if idx1, idx2 := strings.Index(prompt, "## Tier Policy"), strings.Index(prompt, "## Manifest Schema (version 1)"); idx1 == -1 || idx2 == -1 || idx1 >= idx2 {
		t.Errorf("expected ## Tier Policy to land before ## Manifest Schema, got indices %d, %d", idx1, idx2)
	}

	// dynamic-planner itself is never targeted by the reasoning_effort UPDATEs
	// (it isn't in either WHERE IN list) — it must stay untouched (NULL).
	var effort sql.NullString
	if err := sqlDB.QueryRow(`SELECT reasoning_effort FROM agent_definitions WHERE id = 'dynamic-planner'`).Scan(&effort); err != nil {
		t.Fatalf("query dynamic-planner reasoning_effort: %v", err)
	}
	if effort.Valid {
		t.Errorf("dynamic-planner reasoning_effort = %q, want untouched NULL", effort.String)
	}
}

// TestMigration175_SystemPlannerPromptsGetTierPolicy verifies the
// system_agent_definitions REPLACE: planner-system and planner-system-api are
// seeded by migration 000158 itself (unmodified through 174), so no manual
// seed is needed here beyond migrating to 174.
func TestMigration175_SystemPlannerPromptsGetTierPolicy(t *testing.T) {
	pool, err := newMigratedTestPool(t)
	if err != nil {
		t.Fatalf("newMigratedTestPool: %v", err)
	}
	t.Cleanup(func() { pool.Close() })

	for _, id := range []string{"planner-system", "planner-system-api"} {
		var prompt string
		if err := pool.QueryRow(`SELECT prompt FROM system_agent_definitions WHERE id = ?`, id).Scan(&prompt); err != nil {
			t.Fatalf("query %s prompt: %v", id, err)
		}
		if !strings.Contains(prompt, "## Tier Policy") {
			t.Errorf("%s prompt missing ## Tier Policy after migration 000175", id)
		}
		if !strings.Contains(prompt, "dynwf_max_premium_workers") {
			t.Errorf("%s prompt missing the premium-cap directive after migration 000175", id)
		}
	}
}

// TestMigration175_ReasoningEffortDefaults verifies the two effort UPDATEs:
// the 7 non-codex worker/verifier templates land at "low", the synthesizer at
// "medium", and a codex twin (outside both WHERE IN lists) is left untouched.
func TestMigration175_ReasoningEffortDefaults(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "migration175-efforts.db")
	sqlDB, err := sql.Open("sqlite", buildDSN(dbPath))
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	t.Cleanup(func() { sqlDB.Close() })

	m := newMigrateInstance(t, sqlDB)
	if err := m.Migrate(174); err != nil {
		t.Fatalf("migrate to 174: %v", err)
	}

	seedPre175GlobalDynamicWorkflow(t, sqlDB)

	if err := m.Up(); err != nil && err != migrate.ErrNoChange {
		t.Fatalf("migrate remaining (000175): %v", err)
	}

	lowEffortIDs := []string{
		"codebase-explorer", "module-reviewer", "implementor-worker",
		"web-researcher", "finding-verifier", "generic-worker", "cross-checker",
	}
	for _, id := range lowEffortIDs {
		var effort string
		if err := sqlDB.QueryRow(`SELECT reasoning_effort FROM agent_definitions WHERE id = ?`, id).Scan(&effort); err != nil {
			t.Fatalf("query %s reasoning_effort: %v", id, err)
		}
		if effort != "low" {
			t.Errorf("agent_definitions[%q].reasoning_effort = %q, want %q", id, effort, "low")
		}
	}

	var synthEffort string
	if err := sqlDB.QueryRow(`SELECT reasoning_effort FROM agent_definitions WHERE id = 'synthesizer'`).Scan(&synthEffort); err != nil {
		t.Fatalf("query synthesizer reasoning_effort: %v", err)
	}
	if synthEffort != "medium" {
		t.Errorf("agent_definitions[synthesizer].reasoning_effort = %q, want %q", synthEffort, "medium")
	}

	var codexEffort string
	if err := sqlDB.QueryRow(`SELECT reasoning_effort FROM agent_definitions WHERE id = 'module-reviewer-codex'`).Scan(&codexEffort); err != nil {
		t.Fatalf("query module-reviewer-codex reasoning_effort: %v", err)
	}
	if codexEffort != "high" {
		t.Errorf("agent_definitions[module-reviewer-codex].reasoning_effort = %q, want unchanged %q (not in either UPDATE's WHERE IN list)", codexEffort, "high")
	}
}

// seedPre175GlobalDynamicWorkflow seeds the __global__ project + dynamic
// workflow plus the agent_definitions rows migration 000175's UPDATEs target,
// mimicking a pre-ticket EnsureGlobalDynamicWorkflow seed (Go-only,
// create-if-absent — never re-run by a migration test).
func seedPre175GlobalDynamicWorkflow(t *testing.T, sqlDB *sql.DB) {
	t.Helper()
	now := "2026-01-01T00:00:00Z"
	if _, err := sqlDB.Exec(`INSERT INTO projects (id, name, created_at, updated_at) VALUES ('__global__', 'Global', ?, ?)`, now, now); err != nil {
		t.Fatalf("seed __global__ project: %v", err)
	}
	if _, err := sqlDB.Exec(`INSERT INTO workflows (project_id, id, description, scope_type, created_at, updated_at) VALUES ('__global__', 'dynamic', '', 'project', ?, ?)`, now, now); err != nil {
		t.Fatalf("seed dynamic workflow: %v", err)
	}

	insert := func(id, model, nodeRole, effort string) {
		t.Helper()
		var effortArg interface{}
		if effort != "" {
			effortArg = effort
		}
		_, err := sqlDB.Exec(`INSERT INTO agent_definitions
			(id, project_id, workflow_id, model, timeout, prompt, execution_mode, tools, layer, consultant, node_role, reasoning_effort, created_at, updated_at)
			VALUES (?, '__global__', 'dynamic', ?, 20, ?, 'cli_interactive', '', 0, 0, ?, ?, ?, ?)`,
			id, model, preTierPolicyPlannerPrompt, nodeRole, effortArg, now, now)
		if err != nil {
			t.Fatalf("seed agent_definitions %q: %v", id, err)
		}
	}

	insert("dynamic-planner", "opus-4-8", "planner", "")
	for _, id := range []string{"codebase-explorer", "module-reviewer", "implementor-worker", "web-researcher", "finding-verifier", "generic-worker", "cross-checker"} {
		insert(id, "sonnet-5", "fanout_template", "high")
	}
	insert("synthesizer", "opus-4-8", "fanout_template", "high")
	insert("module-reviewer-codex", "gpt-5.6-terra", "fanout_template", "high")
}
