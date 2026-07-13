package db

import (
	"database/sql"
	"fmt"
	"path/filepath"
	"testing"

	"github.com/golang-migrate/migrate/v4"
	"github.com/golang-migrate/migrate/v4/database/sqlite"
	"github.com/golang-migrate/migrate/v4/source/iofs"

	"be/internal/db/migrations"
)

// TestMigration157_NodeIDColumnSchema verifies migration 000157 adds
// agent_sessions.node_id as TEXT NOT NULL DEFAULT ”.
func TestMigration157_NodeIDColumnSchema(t *testing.T) {
	pool, err := newMigratedTestPool(t)
	if err != nil {
		t.Fatalf("NewPoolPath: %v", err)
	}
	t.Cleanup(func() { pool.Close() })

	cols := tableColumns(t, pool, "agent_sessions")
	col, ok := cols["node_id"]
	if !ok {
		t.Fatal("node_id column missing from agent_sessions; migration 000157 may not have run")
	}
	if col.colType != "TEXT" {
		t.Errorf("node_id column type = %q, want TEXT", col.colType)
	}
	if col.notNull != 1 {
		t.Errorf("node_id column notNull = %d, want 1", col.notNull)
	}
	dflt := fmt.Sprintf("%v", col.dflt)
	if dflt != "''" {
		t.Errorf("node_id column default = %q, want \"''\" (empty string literal)", dflt)
	}
}

// TestMigration157_NodeRoleColumnSchema verifies migration 000157 adds
// agent_definitions.node_role as TEXT NOT NULL DEFAULT 'static'.
func TestMigration157_NodeRoleColumnSchema(t *testing.T) {
	pool, err := newMigratedTestPool(t)
	if err != nil {
		t.Fatalf("NewPoolPath: %v", err)
	}
	t.Cleanup(func() { pool.Close() })

	cols := tableColumns(t, pool, "agent_definitions")
	col, ok := cols["node_role"]
	if !ok {
		t.Fatal("node_role column missing from agent_definitions; migration 000157 may not have run")
	}
	if col.colType != "TEXT" {
		t.Errorf("node_role column type = %q, want TEXT", col.colType)
	}
	if col.notNull != 1 {
		t.Errorf("node_role column notNull = %d, want 1", col.notNull)
	}
	dflt := fmt.Sprintf("%v", col.dflt)
	if dflt != "'static'" {
		t.Errorf("node_role column default = %q, want \"'static'\"", dflt)
	}
}

// TestMigration157_NewRowDefaultsNodeRoleStatic verifies that a row inserted
// without specifying node_role receives the DEFAULT 'static' value.
func TestMigration157_NewRowDefaultsNodeRoleStatic(t *testing.T) {
	pool, err := newMigratedTestPool(t)
	if err != nil {
		t.Fatalf("NewPoolPath: %v", err)
	}
	t.Cleanup(func() { pool.Close() })

	if _, err := pool.Exec(`INSERT INTO projects (id, name, created_at, updated_at) VALUES ('p1', 'P', datetime('now'), datetime('now'))`); err != nil {
		t.Fatalf("seed project: %v", err)
	}
	if _, err := pool.Exec(`INSERT INTO workflows (project_id, id, description, scope_type, created_at, updated_at) VALUES ('p1', 'wf1', '', 'ticket', datetime('now'), datetime('now'))`); err != nil {
		t.Fatalf("seed workflow: %v", err)
	}

	_, err = pool.Exec(`INSERT INTO agent_definitions (id, project_id, workflow_id, model, timeout, prompt, execution_mode, created_at, updated_at)
		VALUES ('ag1', 'p1', 'wf1', 'sonnet', 20, 'do stuff', 'cli_interactive', datetime('now'), datetime('now'))`)
	if err != nil {
		t.Fatalf("INSERT agent_definitions without node_role: %v", err)
	}

	var nodeRole string
	if err := pool.QueryRow(`SELECT node_role FROM agent_definitions WHERE id = 'ag1'`).Scan(&nodeRole); err != nil {
		t.Fatalf("SELECT node_role: %v", err)
	}
	if nodeRole != "static" {
		t.Errorf("node_role default = %q, want %q", nodeRole, "static")
	}
}

// TestMigration157_BackfillNodeIDFromPhase verifies the "no backfill hazard"
// acceptance clause: an agent_sessions row inserted BEFORE migration 000157 runs
// comes out the other side with node_id == phase (the migration's UPDATE
// agent_sessions SET node_id = phase step), not the column's own zero-value default.
func TestMigration157_BackfillNodeIDFromPhase(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "backfill.db")
	sqlDB, err := sql.Open("sqlite", buildDSN(dbPath))
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	t.Cleanup(func() { sqlDB.Close() })

	m := newMigrateInstance(t, sqlDB)

	// Migrate up to (and including) 000156, one version before node_identity.
	if err := m.Migrate(156); err != nil {
		t.Fatalf("migrate to 156: %v", err)
	}

	// Seed a pre-existing agent_sessions row the way the pre-migration schema
	// required (phase is populated, node_id does not exist yet).
	now := "2025-01-01T00:00:00Z"
	if _, err := sqlDB.Exec(`INSERT INTO projects (id, name, created_at, updated_at) VALUES ('p1', 'P', ?, ?)`, now, now); err != nil {
		t.Fatalf("seed project: %v", err)
	}
	if _, err := sqlDB.Exec(`INSERT INTO workflows (project_id, id, description, scope_type, created_at, updated_at) VALUES ('p1', 'wf1', '', 'ticket', ?, ?)`, now, now); err != nil {
		t.Fatalf("seed workflow: %v", err)
	}
	if _, err := sqlDB.Exec(`INSERT INTO workflow_instances (id, project_id, ticket_id, workflow_id, scope_type, status, retry_count, created_at, updated_at)
		VALUES ('wfi1', 'p1', '', 'wf1', 'ticket', 'active', 0, ?, ?)`, now, now); err != nil {
		t.Fatalf("seed workflow_instance: %v", err)
	}
	if _, err := sqlDB.Exec(`INSERT INTO agent_sessions (id, project_id, ticket_id, workflow_instance_id, phase, agent_type, status, restart_count, created_at, updated_at)
		VALUES ('sess-pre', 'p1', '', 'wfi1', 'analyzer', 'analyzer', 'completed', 0, ?, ?)`, now, now); err != nil {
		t.Fatalf("seed pre-existing agent_session: %v", err)
	}

	// Now run the remaining migrations, including 000157.
	if err := m.Up(); err != nil && err != migrate.ErrNoChange {
		t.Fatalf("migrate up (remaining): %v", err)
	}

	var nodeID, phase string
	if err := sqlDB.QueryRow(`SELECT node_id, phase FROM agent_sessions WHERE id = 'sess-pre'`).Scan(&nodeID, &phase); err != nil {
		t.Fatalf("SELECT node_id, phase: %v", err)
	}
	if nodeID != phase {
		t.Errorf("node_id = %q, want it to equal phase %q (backfill)", nodeID, phase)
	}
	if nodeID != "analyzer" {
		t.Errorf("node_id = %q, want %q", nodeID, "analyzer")
	}
}

// newMigrateInstance builds a *migrate.Migrate over sqlDB using the embedded
// migration source, mirroring RunMigrations but exposing Migrate(version) for
// partial-migration tests.
func newMigrateInstance(t *testing.T, sqlDB *sql.DB) *migrate.Migrate {
	t.Helper()
	sourceDriver, err := iofs.New(migrations.FS, ".")
	if err != nil {
		t.Fatalf("migration source: %v", err)
	}
	dbDriver, err := sqlite.WithInstance(sqlDB, &sqlite.Config{DatabaseName: "main", NoTxWrap: true})
	if err != nil {
		t.Fatalf("migration db driver: %v", err)
	}
	m, err := migrate.NewWithInstance("iofs", sourceDriver, "sqlite", dbDriver)
	if err != nil {
		t.Fatalf("migration init: %v", err)
	}
	return m
}
