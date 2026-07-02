package db

import (
	"testing"
)

// TestMigration154_Subworkflows verifies the callable_as_subworkflow and
// launch_depth columns exist with default 0, so inserts omitting them succeed.
func TestMigration154_Subworkflows(t *testing.T) {
	pool, err := newMigratedTestPool(t)
	if err != nil {
		t.Fatalf("newMigratedTestPool: %v", err)
	}
	t.Cleanup(func() { pool.Close() })

	if _, err := pool.Exec(
		`INSERT INTO projects (id, name, created_at, updated_at) VALUES ('p154', 'P', datetime('now'), datetime('now'))`); err != nil {
		t.Fatalf("seed project: %v", err)
	}
	if _, err := pool.Exec(
		`INSERT INTO workflows (id, project_id, description, scope_type, groups, created_at, updated_at)
		 VALUES ('wf154', 'p154', '', 'project', '[]', datetime('now'), datetime('now'))`); err != nil {
		t.Fatalf("seed workflow: %v", err)
	}

	var callable int
	if err := pool.QueryRow(
		`SELECT callable_as_subworkflow FROM workflows WHERE id = 'wf154'`).Scan(&callable); err != nil {
		t.Fatalf("query callable_as_subworkflow: %v", err)
	}
	if callable != 0 {
		t.Errorf("callable_as_subworkflow default = %d, want 0", callable)
	}

	if _, err := pool.Exec(
		`INSERT INTO workflow_instances (id, project_id, ticket_id, workflow_id, status, scope_type, created_at, updated_at)
		 VALUES ('wfi154', 'p154', '', 'wf154', 'active', 'project', datetime('now'), datetime('now'))`); err != nil {
		t.Fatalf("seed instance: %v", err)
	}
	var depth int
	if err := pool.QueryRow(
		`SELECT launch_depth FROM workflow_instances WHERE id = 'wfi154'`).Scan(&depth); err != nil {
		t.Fatalf("query launch_depth: %v", err)
	}
	if depth != 0 {
		t.Errorf("launch_depth default = %d, want 0", depth)
	}
}
