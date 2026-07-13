package db

import (
	"fmt"
	"testing"
)

// TestMigration158_PlanRevisionsSchema verifies the append-only plan_revisions
// table shape: PK (instance_id, revision), author CHECK, planner_session_id
// default.
func TestMigration158_PlanRevisionsSchema(t *testing.T) {
	pool, err := newMigratedTestPool(t)
	if err != nil {
		t.Fatalf("NewPoolPath: %v", err)
	}
	t.Cleanup(func() { pool.Close() })

	cols := tableColumns(t, pool, "plan_revisions")

	want := map[string]struct {
		colType string
		notNull int
	}{
		"instance_id":        {"TEXT", 1},
		"revision":           {"INTEGER", 1},
		"manifest":           {"TEXT", 1},
		"hash":               {"TEXT", 1},
		"author":             {"TEXT", 1},
		"planner_session_id": {"TEXT", 1},
		"created_at":         {"TEXT", 1},
	}
	for name, w := range want {
		col, ok := cols[name]
		if !ok {
			t.Fatalf("%s column missing from plan_revisions; migration 000158 may not have run", name)
		}
		if col.colType != w.colType {
			t.Errorf("%s column type = %q, want %q", name, col.colType, w.colType)
		}
		if col.notNull != w.notNull {
			t.Errorf("%s column notNull = %d, want %d", name, col.notNull, w.notNull)
		}
	}

	dflt := fmt.Sprintf("%v", cols["planner_session_id"].dflt)
	if dflt != "''" {
		t.Errorf("planner_session_id default = %q, want \"''\"", dflt)
	}
}

// TestMigration158_WorkflowPlansSchema verifies the mutable workflow_plans
// head row shape: PK instance_id, status CHECK/default, revision counters,
// goal default.
func TestMigration158_WorkflowPlansSchema(t *testing.T) {
	pool, err := newMigratedTestPool(t)
	if err != nil {
		t.Fatalf("NewPoolPath: %v", err)
	}
	t.Cleanup(func() { pool.Close() })

	cols := tableColumns(t, pool, "workflow_plans")

	want := map[string]struct {
		colType string
		notNull int
	}{
		// instance_id is the PRIMARY KEY but, per SQLite semantics, a
		// non-INTEGER PRIMARY KEY does not imply NOT NULL in
		// PRAGMA table_info output.
		"instance_id":       {"TEXT", 0},
		"status":            {"TEXT", 1},
		"latest_revision":   {"INTEGER", 1},
		"approved_revision": {"INTEGER", 1},
		"goal":              {"TEXT", 1},
		"created_at":        {"TEXT", 1},
		"updated_at":        {"TEXT", 1},
	}
	for name, w := range want {
		col, ok := cols[name]
		if !ok {
			t.Fatalf("%s column missing from workflow_plans; migration 000158 may not have run", name)
		}
		if col.colType != w.colType {
			t.Errorf("%s column type = %q, want %q", name, col.colType, w.colType)
		}
		if col.notNull != w.notNull {
			t.Errorf("%s column notNull = %d, want %d", name, col.notNull, w.notNull)
		}
	}

	if got := fmt.Sprintf("%v", cols["status"].dflt); got != "'draft'" {
		t.Errorf("status default = %q, want \"'draft'\"", got)
	}
	if got := fmt.Sprintf("%v", cols["latest_revision"].dflt); got != "0" {
		t.Errorf("latest_revision default = %q, want \"0\"", got)
	}
	if got := fmt.Sprintf("%v", cols["approved_revision"].dflt); got != "0" {
		t.Errorf("approved_revision default = %q, want \"0\"", got)
	}
	if got := fmt.Sprintf("%v", cols["goal"].dflt); got != "''" {
		t.Errorf("goal default = %q, want \"''\"", got)
	}
}

// seedProjectWorkflow seeds a fixed project 'p1' + workflow 'wf1' pair, the
// minimal parents required by workflow_instances (columns mirrored from
// migration157_test.go).
func seedProjectWorkflow(t *testing.T, pool *Pool) {
	t.Helper()
	if _, err := pool.Exec(`INSERT INTO projects (id, name, created_at, updated_at) VALUES ('p1', 'P', datetime('now'), datetime('now'))`); err != nil {
		t.Fatalf("seed project: %v", err)
	}
	if _, err := pool.Exec(`INSERT INTO workflows (project_id, id, description, scope_type, created_at, updated_at) VALUES ('p1', 'wf1', '', 'ticket', datetime('now'), datetime('now'))`); err != nil {
		t.Fatalf("seed workflow: %v", err)
	}
}

// seedInstance inserts a workflow_instances row with the given id, referencing
// the p1/wf1 parents created by seedProjectWorkflow.
func seedInstance(t *testing.T, pool *Pool, instanceID string) {
	t.Helper()
	_, err := pool.Exec(`INSERT INTO workflow_instances (id, project_id, ticket_id, workflow_id, scope_type, status, retry_count, created_at, updated_at)
		VALUES (?, 'p1', '', 'wf1', 'ticket', 'active', 0, datetime('now'), datetime('now'))`, instanceID)
	if err != nil {
		t.Fatalf("seed workflow_instance %s: %v", instanceID, err)
	}
}

// TestMigration158_PlanRevisionsAuthorCheck verifies the author CHECK
// constraint: 'planner' and 'caller' succeed, anything else is rejected.
func TestMigration158_PlanRevisionsAuthorCheck(t *testing.T) {
	pool, err := newMigratedTestPool(t)
	if err != nil {
		t.Fatalf("NewPoolPath: %v", err)
	}
	t.Cleanup(func() { pool.Close() })

	seedProjectWorkflow(t, pool)
	seedInstance(t, pool, "wfi1")

	cases := []struct {
		name    string
		author  string
		wantErr bool
	}{
		{"bogus rejected", "bogus-author", true},
		{"planner accepted", "planner", false},
		{"caller accepted", "caller", false},
	}
	for i, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := pool.Exec(`INSERT INTO plan_revisions (instance_id, revision, manifest, hash, author, created_at) VALUES ('wfi1', ?, '{}', 'h', ?, datetime('now'))`,
				i+1, tc.author)
			if tc.wantErr && err == nil {
				t.Errorf("insert author=%q: expected CHECK constraint error, got nil", tc.author)
			}
			if !tc.wantErr && err != nil {
				t.Errorf("insert author=%q: unexpected error: %v", tc.author, err)
			}
		})
	}
}

// TestMigration158_WorkflowPlansStatusCheck verifies the status CHECK
// constraint: 'draft', 'approved', 'cancelled' succeed, anything else is
// rejected. Each case targets its own instance_id since instance_id is the
// PRIMARY KEY of workflow_plans.
func TestMigration158_WorkflowPlansStatusCheck(t *testing.T) {
	pool, err := newMigratedTestPool(t)
	if err != nil {
		t.Fatalf("NewPoolPath: %v", err)
	}
	t.Cleanup(func() { pool.Close() })

	seedProjectWorkflow(t, pool)

	cases := []struct {
		name    string
		status  string
		wantErr bool
	}{
		{"bogus rejected", "bogus-status", true},
		{"draft accepted", "draft", false},
		{"approved accepted", "approved", false},
		{"cancelled accepted", "cancelled", false},
	}
	for i, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			instanceID := fmt.Sprintf("wfi-status-%d", i)
			seedInstance(t, pool, instanceID)
			_, err := pool.Exec(`INSERT INTO workflow_plans (instance_id, status, created_at, updated_at) VALUES (?, ?, datetime('now'), datetime('now'))`,
				instanceID, tc.status)
			if tc.wantErr && err == nil {
				t.Errorf("insert status=%q: expected CHECK constraint error, got nil", tc.status)
			}
			if !tc.wantErr && err != nil {
				t.Errorf("insert status=%q: unexpected error: %v", tc.status, err)
			}
		})
	}
}

// TestMigration158_CascadeDeleteWorkflowInstance verifies that deleting a
// workflow_instances row cascades to both plan_revisions and workflow_plans.
func TestMigration158_CascadeDeleteWorkflowInstance(t *testing.T) {
	pool, err := newMigratedTestPool(t)
	if err != nil {
		t.Fatalf("NewPoolPath: %v", err)
	}
	t.Cleanup(func() { pool.Close() })

	seedProjectWorkflow(t, pool)
	seedInstance(t, pool, "wfi-cascade")

	if _, err := pool.Exec(`INSERT INTO plan_revisions (instance_id, revision, manifest, hash, author, created_at) VALUES ('wfi-cascade', 1, '{}', 'h', 'planner', datetime('now'))`); err != nil {
		t.Fatalf("seed plan_revisions: %v", err)
	}
	if _, err := pool.Exec(`INSERT INTO workflow_plans (instance_id, status, created_at, updated_at) VALUES ('wfi-cascade', 'draft', datetime('now'), datetime('now'))`); err != nil {
		t.Fatalf("seed workflow_plans: %v", err)
	}

	if _, err := pool.Exec(`DELETE FROM workflow_instances WHERE id = 'wfi-cascade'`); err != nil {
		t.Fatalf("delete workflow_instances: %v", err)
	}

	var revCount, planCount int
	if err := pool.QueryRow(`SELECT COUNT(*) FROM plan_revisions WHERE instance_id = 'wfi-cascade'`).Scan(&revCount); err != nil {
		t.Fatalf("count plan_revisions: %v", err)
	}
	if revCount != 0 {
		t.Errorf("plan_revisions rows after cascade delete = %d, want 0", revCount)
	}
	if err := pool.QueryRow(`SELECT COUNT(*) FROM workflow_plans WHERE instance_id = 'wfi-cascade'`).Scan(&planCount); err != nil {
		t.Fatalf("count workflow_plans: %v", err)
	}
	if planCount != 0 {
		t.Errorf("workflow_plans rows after cascade delete = %d, want 0", planCount)
	}
}
