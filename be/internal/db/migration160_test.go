package db

import (
	"fmt"
	"testing"
)

// TestMigration160_PlanAutoApproveColumn verifies the plan_auto_approve
// column added to workflow_instances: NOT NULL INTEGER defaulting to 0.
func TestMigration160_PlanAutoApproveColumn(t *testing.T) {
	t.Parallel()
	pool, err := newMigratedTestPool(t)
	if err != nil {
		t.Fatalf("NewPoolPath: %v", err)
	}
	t.Cleanup(func() { pool.Close() })

	cols := tableColumns(t, pool, "workflow_instances")

	col, ok := cols["plan_auto_approve"]
	if !ok {
		t.Fatal("plan_auto_approve column missing from workflow_instances; migration 000160 may not have run")
	}
	if col.colType != "INTEGER" {
		t.Errorf("plan_auto_approve column type = %q, want INTEGER", col.colType)
	}
	if col.notNull != 1 {
		t.Errorf("plan_auto_approve notNull = %d, want 1", col.notNull)
	}
	if got := fmt.Sprintf("%v", col.dflt); got != "0" {
		t.Errorf("plan_auto_approve default = %q, want \"0\"", got)
	}
}

// TestMigration160_PlanAutoApproveRoundTrip verifies plan_auto_approve
// round-trips through an explicit insert and through the column DEFAULT
// when omitted from the insert list.
func TestMigration160_PlanAutoApproveRoundTrip(t *testing.T) {
	t.Parallel()
	pool, err := newMigratedTestPool(t)
	if err != nil {
		t.Fatalf("NewPoolPath: %v", err)
	}
	t.Cleanup(func() { pool.Close() })

	seedProjectWorkflow(t, pool)

	if _, err := pool.Exec(`INSERT INTO workflow_instances (id, project_id, ticket_id, workflow_id, scope_type, status, retry_count, plan_auto_approve, created_at, updated_at)
		VALUES ('wfi-auto160-explicit', 'p1', '', 'wf1', 'ticket', 'active', 0, 1, datetime('now'), datetime('now'))`); err != nil {
		t.Fatalf("insert explicit plan_auto_approve=1: %v", err)
	}

	if _, err := pool.Exec(`INSERT INTO workflow_instances (id, project_id, ticket_id, workflow_id, scope_type, status, retry_count, created_at, updated_at)
		VALUES ('wfi-auto160-default', 'p1', '', 'wf1', 'ticket', 'active', 0, datetime('now'), datetime('now'))`); err != nil {
		t.Fatalf("insert without plan_auto_approve: %v", err)
	}

	cases := []struct {
		instanceID string
		want       int
	}{
		{"wfi-auto160-explicit", 1},
		{"wfi-auto160-default", 0},
	}
	for _, tc := range cases {
		var got int
		row := pool.QueryRow(`SELECT plan_auto_approve FROM workflow_instances WHERE id = ?`, tc.instanceID)
		if err := row.Scan(&got); err != nil {
			t.Fatalf("select plan_auto_approve for %s: %v", tc.instanceID, err)
		}
		if got != tc.want {
			t.Errorf("plan_auto_approve for %s = %d, want %d", tc.instanceID, got, tc.want)
		}
	}
}
