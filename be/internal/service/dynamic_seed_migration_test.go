package service

import (
	"path/filepath"
	"testing"
	"time"

	"be/internal/clock"
	"be/internal/db"
)

// TestAgentDefinitionsDescription_DefaultsEmptyForPreexistingRows verifies
// migration 000161's `ALTER TABLE agent_definitions ADD COLUMN description
// TEXT NOT NULL DEFAULT ”`: a row inserted without naming the column (as
// every agent_definitions row created before the migration would have been)
// backfills to ”, not NULL or an error.
func TestAgentDefinitionsDescription_DefaultsEmptyForPreexistingRows(t *testing.T) {
	t.Parallel()
	dbPath := filepath.Join(t.TempDir(), "dyn_migration_backfill.db")
	if err := svcCopyTemplateDB(dbPath); err != nil {
		t.Fatalf("copy template DB: %v", err)
	}
	pool, err := db.OpenPoolExisting(dbPath, db.DefaultPoolConfig())
	if err != nil {
		t.Fatalf("open pool: %v", err)
	}
	t.Cleanup(func() { pool.Close() })

	now := time.Now().UTC().Format(time.RFC3339Nano)
	if _, err := pool.Exec(`INSERT INTO projects (id, name, root_path, created_at, updated_at) VALUES ('proj-mig','P','/tmp',?,?)`, now, now); err != nil {
		t.Fatalf("insert project: %v", err)
	}
	if _, err := pool.Exec(`INSERT INTO workflows (id, project_id, description, scope_type, groups, close_ticket_on_complete, purge_on_completion, finding_schemas, created_at, updated_at)
		VALUES ('wf-mig','proj-mig','','project','[]',0,0,'[]',?,?)`, now, now); err != nil {
		t.Fatalf("insert workflow: %v", err)
	}
	// Deliberately omit the description column, mirroring a row written before
	// migration 000161 existed.
	if _, err := pool.Exec(`INSERT INTO agent_definitions (id, project_id, workflow_id, model, timeout, prompt, layer, execution_mode, tools, created_at, updated_at)
		VALUES ('old-agent','proj-mig','wf-mig','sonnet',20,'do stuff',0,'cli_interactive','',?,?)`, now, now); err != nil {
		t.Fatalf("insert pre-migration-shaped agent_definitions row: %v", err)
	}

	var description string
	if err := pool.QueryRow(`SELECT description FROM agent_definitions WHERE project_id='proj-mig' AND id='old-agent'`).Scan(&description); err != nil {
		t.Fatalf("select description: %v", err)
	}
	if description != "" {
		t.Errorf("description for a pre-migration row = %q, want '' (column default)", description)
	}
}

// TestEnsureGlobalDynamicWorkflow_LeavesUnrelatedAgentDefsIntact verifies
// seeding the dynamic workflow into a DB that already has agent_definitions
// rows (an existing project's own workflow) does not touch them.
func TestEnsureGlobalDynamicWorkflow_LeavesUnrelatedAgentDefsIntact(t *testing.T) {
	t.Parallel()
	dbPath := filepath.Join(t.TempDir(), "dyn_migration_preexisting.db")
	if err := svcCopyTemplateDB(dbPath); err != nil {
		t.Fatalf("copy template DB: %v", err)
	}
	pool, err := db.OpenPoolExisting(dbPath, db.DefaultPoolConfig())
	if err != nil {
		t.Fatalf("open pool: %v", err)
	}
	t.Cleanup(func() { pool.Close() })
	clk := clock.Real()

	now := time.Now().UTC().Format(time.RFC3339Nano)
	if _, err := pool.Exec(`INSERT INTO projects (id, name, root_path, created_at, updated_at) VALUES ('proj-existing','P','/tmp',?,?)`, now, now); err != nil {
		t.Fatalf("insert project: %v", err)
	}
	if _, err := pool.Exec(`INSERT INTO workflows (id, project_id, description, scope_type, groups, close_ticket_on_complete, purge_on_completion, finding_schemas, created_at, updated_at)
		VALUES ('wf-existing','proj-existing','','project','[]',0,0,'[]',?,?)`, now, now); err != nil {
		t.Fatalf("insert workflow: %v", err)
	}
	if _, err := pool.Exec(`INSERT INTO agent_definitions (id, project_id, workflow_id, model, timeout, prompt, layer, execution_mode, tools, description, created_at, updated_at)
		VALUES ('existing-agent','proj-existing','wf-existing','sonnet',20,'do stuff',0,'cli_interactive','','pre-existing description',?,?)`, now, now); err != nil {
		t.Fatalf("insert existing agent_definitions row: %v", err)
	}

	if err := EnsureGlobalDynamicWorkflow(pool, clk, t.TempDir()); err != nil {
		t.Fatalf("EnsureGlobalDynamicWorkflow: %v", err)
	}

	var count int
	var description string
	if err := pool.QueryRow(`SELECT COUNT(*), description FROM agent_definitions WHERE project_id='proj-existing' AND id='existing-agent' GROUP BY description`).Scan(&count, &description); err != nil {
		t.Fatalf("select existing-agent after seed: %v", err)
	}
	if count != 1 {
		t.Errorf("existing-agent row count after seed = %d, want 1 (untouched)", count)
	}
	if description != "pre-existing description" {
		t.Errorf("existing-agent description after seed = %q, want unchanged", description)
	}
}
