package db

import (
	"fmt"
	"testing"
)

// TestMigration159_WorkflowInstanceNodesSchema verifies the insert-only
// materialized-node table shape: PK (instance_id, node_id), instructions
// default, layer/plan_revision as NOT NULL INTEGER.
func TestMigration159_WorkflowInstanceNodesSchema(t *testing.T) {
	t.Parallel()
	pool, err := newMigratedTestPool(t)
	if err != nil {
		t.Fatalf("NewPoolPath: %v", err)
	}
	t.Cleanup(func() { pool.Close() })

	cols := tableColumns(t, pool, "workflow_instance_nodes")

	want := map[string]struct {
		colType string
		notNull int
	}{
		"instance_id":   {"TEXT", 1},
		"node_id":       {"TEXT", 1},
		"layer":         {"INTEGER", 1},
		"agent_type":    {"TEXT", 1},
		"instructions":  {"TEXT", 1},
		"plan_revision": {"INTEGER", 1},
		"created_at":    {"TEXT", 1},
	}
	for name, w := range want {
		col, ok := cols[name]
		if !ok {
			t.Fatalf("%s column missing from workflow_instance_nodes; migration 000159 may not have run", name)
		}
		if col.colType != w.colType {
			t.Errorf("%s column type = %q, want %q", name, col.colType, w.colType)
		}
		if col.notNull != w.notNull {
			t.Errorf("%s column notNull = %d, want %d", name, col.notNull, w.notNull)
		}
	}

	if got := fmt.Sprintf("%v", cols["instructions"].dflt); got != "''" {
		t.Errorf("instructions default = %q, want \"''\"", got)
	}
}

// TestMigration159_WorkflowInstanceLayerPoliciesSchema verifies the
// materialized layer-policy table shape: PK (instance_id, layer),
// pass_policy default 'any'.
func TestMigration159_WorkflowInstanceLayerPoliciesSchema(t *testing.T) {
	t.Parallel()
	pool, err := newMigratedTestPool(t)
	if err != nil {
		t.Fatalf("NewPoolPath: %v", err)
	}
	t.Cleanup(func() { pool.Close() })

	cols := tableColumns(t, pool, "workflow_instance_layer_policies")

	want := map[string]struct {
		colType string
		notNull int
	}{
		"instance_id": {"TEXT", 1},
		"layer":       {"INTEGER", 1},
		"pass_policy": {"TEXT", 1},
	}
	for name, w := range want {
		col, ok := cols[name]
		if !ok {
			t.Fatalf("%s column missing from workflow_instance_layer_policies; migration 000159 may not have run", name)
		}
		if col.colType != w.colType {
			t.Errorf("%s column type = %q, want %q", name, col.colType, w.colType)
		}
		if col.notNull != w.notNull {
			t.Errorf("%s column notNull = %d, want %d", name, col.notNull, w.notNull)
		}
	}

	if got := fmt.Sprintf("%v", cols["pass_policy"].dflt); got != "'any'" {
		t.Errorf("pass_policy default = %q, want \"'any'\"", got)
	}
}

// TestMigration159_WorkflowPlansMaterializationColumns verifies the
// exactly-once materialization stamp added to workflow_plans: both columns
// NOT NULL with zero-value defaults.
func TestMigration159_WorkflowPlansMaterializationColumns(t *testing.T) {
	t.Parallel()
	pool, err := newMigratedTestPool(t)
	if err != nil {
		t.Fatalf("NewPoolPath: %v", err)
	}
	t.Cleanup(func() { pool.Close() })

	cols := tableColumns(t, pool, "workflow_plans")

	rev, ok := cols["materialized_revision"]
	if !ok {
		t.Fatal("materialized_revision column missing from workflow_plans; migration 000159 may not have run")
	}
	if rev.colType != "INTEGER" {
		t.Errorf("materialized_revision column type = %q, want INTEGER", rev.colType)
	}
	if rev.notNull != 1 {
		t.Errorf("materialized_revision notNull = %d, want 1", rev.notNull)
	}
	if got := fmt.Sprintf("%v", rev.dflt); got != "0" {
		t.Errorf("materialized_revision default = %q, want \"0\"", got)
	}

	hash, ok := cols["materialized_hash"]
	if !ok {
		t.Fatal("materialized_hash column missing from workflow_plans; migration 000159 may not have run")
	}
	if hash.colType != "TEXT" {
		t.Errorf("materialized_hash column type = %q, want TEXT", hash.colType)
	}
	if hash.notNull != 1 {
		t.Errorf("materialized_hash notNull = %d, want 1", hash.notNull)
	}
	if got := fmt.Sprintf("%v", hash.dflt); got != "''" {
		t.Errorf("materialized_hash default = %q, want \"''\"", got)
	}
}

// TestMigration159_WorkflowInstancesStatusCheckWidened verifies the rebuilt
// workflow_instances status CHECK still accepts every pre-existing status
// (regression: widening must not drop old values) plus the four new
// plan-boundary suspend statuses, and still rejects bogus values. Each case
// targets its own instance id since id is the PRIMARY KEY.
func TestMigration159_WorkflowInstancesStatusCheckWidened(t *testing.T) {
	t.Parallel()
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
		{"active accepted (pre-existing)", "active", false},
		{"completed accepted (pre-existing)", "completed", false},
		{"failed accepted (pre-existing)", "failed", false},
		{"project_completed accepted (pre-existing)", "project_completed", false},
		{"waiting accepted (pre-existing)", "waiting", false},
		{"planning accepted (new)", "planning", false},
		{"plan_ready rejected (dropped in 000211)", "plan_ready", true},
		{"waiting_input accepted (new)", "waiting_input", false},
		{"waiting_approval accepted (new)", "waiting_approval", false},
		{"bogus rejected", "bogus-status", true},
	}
	for i, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			instanceID := fmt.Sprintf("wfi-status159-%d", i)
			_, err := pool.Exec(`INSERT INTO workflow_instances (id, project_id, ticket_id, workflow_id, scope_type, status, retry_count, created_at, updated_at)
				VALUES (?, 'p1', '', 'wf1', 'ticket', ?, 0, datetime('now'), datetime('now'))`,
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
