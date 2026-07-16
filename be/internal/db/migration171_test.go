package db

import (
	"database/sql"
	"path/filepath"
	"testing"

	"github.com/golang-migrate/migrate/v4"
)

// TestMigration171_RewritesPrefixedSessionModels verifies that 000171 remaps the
// slug suffix of prefixed agent_sessions.model_id values (the real production
// shape `<cli>:<slug>`) and leaves bare/unknown/already-canonical ids untouched.
// It also guards the schema invariant that workflow_instances carries no dead
// model_id column (the original nullable column from 000004 was dropped by the
// 000136 table rebuild and never carried forward).
func TestMigration171_RewritesPrefixedSessionModels(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "prefixed-sessions.db")
	sqlDB, err := sql.Open("sqlite", buildDSN(dbPath))
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	t.Cleanup(func() { sqlDB.Close() })
	m := newMigrateInstance(t, sqlDB)
	if err := m.Migrate(170); err != nil {
		t.Fatalf("migrate to 170: %v", err)
	}

	seedMigration171Sessions(t, sqlDB)
	if err := m.Up(); err != nil && err != migrate.ErrNoChange {
		t.Fatalf("migrate remaining: %v", err)
	}

	cases := map[string]string{
		"sess-prefixed-claude": "claude:sonnet-5",   // suffix remapped
		"sess-prefixed-codex":  "codex:gpt-5.6-sol", // suffix remapped
		"sess-bare":            "sonnet",            // no ':' → untouched
		"sess-canonical":       "claude:opus-4-8",   // suffix already canonical → untouched
		"sess-unknown":         "codex:mystery",     // suffix unknown → untouched
	}
	for id, want := range cases {
		assertScalar(t, sqlDB,
			`SELECT model_id FROM agent_sessions WHERE id = '`+id+`'`, want)
	}

	// workflow_instances must carry no model_id column.
	rows, err := sqlDB.Query("PRAGMA table_info(workflow_instances)")
	if err != nil {
		t.Fatalf("pragma table_info: %v", err)
	}
	defer rows.Close()
	for rows.Next() {
		var cid int
		var name, ctype string
		var notnull, pk int
		var dflt sql.NullString
		if err := rows.Scan(&cid, &name, &ctype, &notnull, &dflt, &pk); err != nil {
			t.Fatalf("scan table_info: %v", err)
		}
		if name == "model_id" {
			t.Fatalf("workflow_instances.model_id column still present after 000171")
		}
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("table_info rows: %v", err)
	}
}

func seedMigration171Sessions(t *testing.T, db *sql.DB) {
	t.Helper()
	now := "2026-07-15T00:00:00Z"
	statements := []string{
		`INSERT INTO projects (id, name, created_at, updated_at) VALUES ('p171', 'P171', ?, ?)`,
		`INSERT INTO workflows (project_id, id, description, scope_type, created_at, updated_at) VALUES ('p171', 'wf171', '', 'ticket', ?, ?)`,
		`INSERT INTO workflow_instances (id, project_id, def_project_id, workflow_id, status, created_at, updated_at) VALUES ('wfi171', 'p171', 'p171', 'wf171', 'completed', ?, ?)`,
		`INSERT INTO agent_sessions (id, project_id, ticket_id, workflow_instance_id, phase, agent_type, model_id, status, created_at, updated_at) VALUES ('sess-prefixed-claude', 'p171', '', 'wfi171', 'x', 'x', 'claude:sonnet', 'completed', ?, ?)`,
		`INSERT INTO agent_sessions (id, project_id, ticket_id, workflow_instance_id, phase, agent_type, model_id, status, created_at, updated_at) VALUES ('sess-prefixed-codex', 'p171', '', 'wfi171', 'x', 'x', 'codex:codex_gpt56_sol_high', 'completed', ?, ?)`,
		`INSERT INTO agent_sessions (id, project_id, ticket_id, workflow_instance_id, phase, agent_type, model_id, status, created_at, updated_at) VALUES ('sess-bare', 'p171', '', 'wfi171', 'x', 'x', 'sonnet', 'completed', ?, ?)`,
		`INSERT INTO agent_sessions (id, project_id, ticket_id, workflow_instance_id, phase, agent_type, model_id, status, created_at, updated_at) VALUES ('sess-canonical', 'p171', '', 'wfi171', 'x', 'x', 'claude:opus-4-8', 'completed', ?, ?)`,
		`INSERT INTO agent_sessions (id, project_id, ticket_id, workflow_instance_id, phase, agent_type, model_id, status, created_at, updated_at) VALUES ('sess-unknown', 'p171', '', 'wfi171', 'x', 'x', 'codex:mystery', 'completed', ?, ?)`,
	}
	for _, statement := range statements {
		if _, err := db.Exec(statement, now, now); err != nil {
			t.Fatalf("seed migration 171 session: %v", err)
		}
	}
}
