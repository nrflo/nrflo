package integration

import (
	"database/sql"
	"errors"
	"path/filepath"
	"testing"

	"github.com/golang-migrate/migrate/v4"
	migratesqlite "github.com/golang-migrate/migrate/v4/database/sqlite"
	"github.com/golang-migrate/migrate/v4/source/iofs"
	_ "modernc.org/sqlite"

	"be/internal/db/migrations"
)

// buildMigrator wires a migrate instance against the embedded iofs source and
// the provided *sql.DB. Caller owns the sql.DB and must close it — never
// m.Close() (closes the underlying DB).
func buildMigrator(t *testing.T, sqlDB *sql.DB) *migrate.Migrate {
	t.Helper()
	src, err := iofs.New(migrations.FS, ".")
	if err != nil {
		t.Fatalf("iofs.New: %v", err)
	}
	drv, err := migratesqlite.WithInstance(sqlDB, &migratesqlite.Config{
		DatabaseName: "main",
		NoTxWrap:     true,
	})
	if err != nil {
		t.Fatalf("sqlite.WithInstance: %v", err)
	}
	m, err := migrate.NewWithInstance("iofs", src, "sqlite", drv)
	if err != nil {
		t.Fatalf("migrate.NewWithInstance: %v", err)
	}
	return m
}

// migrateTo advances a migrate instance to the given target version.
func migrateTo(t *testing.T, m *migrate.Migrate, version uint) {
	t.Helper()
	if err := m.Migrate(version); err != nil && !errors.Is(err, migrate.ErrNoChange) {
		t.Fatalf("Migrate(%d): %v", version, err)
	}
}

// TestMigration057MigratesAgentDefModelColumns exercises the UPDATE statements
// in migration 000057 by seeding agent_definitions rows under migration 56
// (when opus/opus_1m still exist), then running 57 and verifying the
// model + low_consumption_model columns were rewritten to the versioned IDs.
func TestMigration057MigratesAgentDefModelColumns(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "migrate057.db")
	sqlDB, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatalf("sql.Open: %v", err)
	}
	t.Cleanup(func() { sqlDB.Close() })

	m := buildMigrator(t, sqlDB)
	migrateTo(t, m, 56)

	// Workflow FK requires a parent project + workflow row.
	now := "2026-04-01T00:00:00Z"
	if _, err := sqlDB.Exec(
		`INSERT INTO projects (id, name, root_path, created_at, updated_at) VALUES (?, ?, ?, ?, ?)`,
		"proj-57", "p57", "/tmp", now, now); err != nil {
		t.Fatalf("insert project: %v", err)
	}
	if _, err := sqlDB.Exec(
		`INSERT INTO workflows (id, project_id, description, created_at, updated_at) VALUES (?, ?, ?, ?, ?)`,
		"wf-57", "proj-57", "d", now, now); err != nil {
		t.Fatalf("insert workflow: %v", err)
	}

	// Seed agent_definitions rows that reference the legacy opus/opus_1m IDs.
	agents := []struct {
		id, model, lcModel string
	}{
		{"builder", "opus", ""},
		{"planner", "opus_1m", ""},
		{"writer", "sonnet", "opus"},
		{"analyst", "haiku", "opus_1m"},
		{"untouched", "sonnet", "haiku"},
	}
	for _, a := range agents {
		if _, err := sqlDB.Exec(
			`INSERT INTO agent_definitions (id, project_id, workflow_id, model, timeout, prompt, low_consumption_model, layer, created_at, updated_at)
			 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
			a.id, "proj-57", "wf-57", a.model, 60, "p", a.lcModel, 0, now, now); err != nil {
			t.Fatalf("insert agent_def %q: %v", a.id, err)
		}
	}

	// Seed system_agent_definitions rows (no FK).
	sysAgents := []struct{ id, model string }{
		{"sys-1", "opus"},
		{"sys-2", "opus_1m"},
		{"sys-3", "sonnet"},
	}
	for _, s := range sysAgents {
		if _, err := sqlDB.Exec(
			`INSERT INTO system_agent_definitions (id, model, timeout, prompt, created_at, updated_at)
			 VALUES (?, ?, ?, ?, ?, ?)`,
			s.id, s.model, 60, "p", now, now); err != nil {
			t.Fatalf("insert system_agent_def %q: %v", s.id, err)
		}
	}

	// Apply migration 57.
	migrateTo(t, m, 57)

	// Verify agent_definitions.model UPDATEs.
	wantModel := map[string]string{
		"builder":   "opus_4_7",
		"planner":   "opus_4_7_1m",
		"writer":    "sonnet",
		"analyst":   "haiku",
		"untouched": "sonnet",
	}
	for id, want := range wantModel {
		var got string
		if err := sqlDB.QueryRow(
			`SELECT model FROM agent_definitions WHERE id = ? AND project_id = ? AND workflow_id = ?`,
			id, "proj-57", "wf-57").Scan(&got); err != nil {
			t.Fatalf("scan agent_def %q: %v", id, err)
		}
		if got != want {
			t.Errorf("agent_def %q: model = %q, want %q", id, got, want)
		}
	}

	// Verify agent_definitions.low_consumption_model UPDATEs.
	wantLC := map[string]string{
		"builder":   "",
		"planner":   "",
		"writer":    "opus_4_7",
		"analyst":   "opus_4_7_1m",
		"untouched": "haiku",
	}
	for id, want := range wantLC {
		var got string
		if err := sqlDB.QueryRow(
			`SELECT low_consumption_model FROM agent_definitions WHERE id = ? AND project_id = ? AND workflow_id = ?`,
			id, "proj-57", "wf-57").Scan(&got); err != nil {
			t.Fatalf("scan agent_def lc %q: %v", id, err)
		}
		if got != want {
			t.Errorf("agent_def %q: low_consumption_model = %q, want %q", id, got, want)
		}
	}

	// Verify system_agent_definitions.model UPDATEs.
	wantSys := map[string]string{
		"sys-1": "opus_4_7",
		"sys-2": "opus_4_7_1m",
		"sys-3": "sonnet",
	}
	for id, want := range wantSys {
		var got string
		if err := sqlDB.QueryRow(
			`SELECT model FROM system_agent_definitions WHERE id = ?`, id).Scan(&got); err != nil {
			t.Fatalf("scan system_agent_def %q: %v", id, err)
		}
		if got != want {
			t.Errorf("system_agent_def %q: model = %q, want %q", id, got, want)
		}
	}

	// Verify old opus/opus_1m rows removed from cli_models after migration.
	for _, id := range []string{"opus", "opus_1m"} {
		var count int
		if err := sqlDB.QueryRow(
			`SELECT COUNT(*) FROM cli_models WHERE id = ?`, id).Scan(&count); err != nil {
			t.Fatalf("count cli_models %q: %v", id, err)
		}
		if count != 0 {
			t.Errorf("cli_models %q: count = %d, want 0 after migration 57", id, count)
		}
	}
}

// TestMigration057NoOpWithoutLegacyRows verifies that migration 000057 is a
// no-op for agent definitions that don't reference the legacy opus/opus_1m
// IDs — sonnet, haiku, codex_gpt_*, opencode_* rows are left untouched.
func TestMigration057NoOpWithoutLegacyRows(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "migrate057_noop.db")
	sqlDB, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatalf("sql.Open: %v", err)
	}
	t.Cleanup(func() { sqlDB.Close() })

	m := buildMigrator(t, sqlDB)
	migrateTo(t, m, 56)

	now := "2026-04-01T00:00:00Z"
	if _, err := sqlDB.Exec(
		`INSERT INTO projects (id, name, root_path, created_at, updated_at) VALUES (?, ?, ?, ?, ?)`,
		"proj-x", "px", "/tmp", now, now); err != nil {
		t.Fatalf("insert project: %v", err)
	}
	if _, err := sqlDB.Exec(
		`INSERT INTO workflows (id, project_id, description, created_at, updated_at) VALUES (?, ?, ?, ?, ?)`,
		"wf-x", "proj-x", "d", now, now); err != nil {
		t.Fatalf("insert workflow: %v", err)
	}
	if _, err := sqlDB.Exec(
		`INSERT INTO agent_definitions (id, project_id, workflow_id, model, timeout, prompt, low_consumption_model, layer, created_at, updated_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		"keeper", "proj-x", "wf-x", "sonnet", 60, "p", "haiku", 0, now, now); err != nil {
		t.Fatalf("insert agent_def: %v", err)
	}

	migrateTo(t, m, 57)

	var gotModel, gotLC string
	if err := sqlDB.QueryRow(
		`SELECT model, low_consumption_model FROM agent_definitions WHERE id = ?`,
		"keeper").Scan(&gotModel, &gotLC); err != nil {
		t.Fatalf("scan agent_def: %v", err)
	}
	if gotModel != "sonnet" {
		t.Errorf("model = %q, want %q (migration should not touch non-opus models)", gotModel, "sonnet")
	}
	if gotLC != "haiku" {
		t.Errorf("low_consumption_model = %q, want %q", gotLC, "haiku")
	}
}
