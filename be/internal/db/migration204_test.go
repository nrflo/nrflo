package db

import (
	"fmt"
	"testing"
)

// 000204 adds agent_step_cursors.rejections: a JSON map step_id->count
// tracking the durable per-step evidence-rejection counter.

// TestMigration204_RejectionsColumnDefaultsToEmptyObject verifies the column
// shape on a fresh migrated DB: TEXT, NOT NULL, default '{}'.
func TestMigration204_RejectionsColumnDefaultsToEmptyObject(t *testing.T) {
	t.Parallel()
	pool, err := newMigratedTestPool(t)
	if err != nil {
		t.Fatalf("NewPoolPath: %v", err)
	}
	t.Cleanup(func() { pool.Close() })

	cols := tableColumns(t, pool, "agent_step_cursors")
	rejections, ok := cols["rejections"]
	if !ok {
		t.Fatal("rejections column missing from agent_step_cursors; migration 000204 may not have run")
	}
	if rejections.colType != "TEXT" {
		t.Errorf("rejections column type = %q, want TEXT", rejections.colType)
	}
	if rejections.notNull != 1 {
		t.Errorf("rejections notNull = %d, want 1", rejections.notNull)
	}
	if got := fmt.Sprintf("%v", rejections.dflt); got != "'{}'" {
		t.Errorf("rejections default = %q, want \"'{}'\"", got)
	}
}

// TestMigration204_FreshInsertBackfillsDefaultEmptyObject verifies a row
// inserted without specifying rejections gets the '{}' default (the
// pre-migration row shape a relaunch might still write via an older code
// path, guarding against a NOT NULL failure).
func TestMigration204_FreshInsertBackfillsDefaultEmptyObject(t *testing.T) {
	t.Parallel()
	pool, err := newMigratedTestPool(t)
	if err != nil {
		t.Fatalf("NewPoolPath: %v", err)
	}
	t.Cleanup(func() { pool.Close() })

	seedProjectWorkflow(t, pool)
	if _, err := pool.Exec(`INSERT INTO workflow_instances (id, project_id, ticket_id, workflow_id, scope_type, status, retry_count, created_at, updated_at)
		VALUES ('wfi-rej204', 'p1', '', 'wf1', 'ticket', 'active', 0, datetime('now'), datetime('now'))`); err != nil {
		t.Fatalf("insert workflow_instance: %v", err)
	}
	if _, err := pool.Exec(`INSERT INTO agent_step_cursors
		(workflow_instance_id, node_id, steps_snapshot, revision, current_index, completed, created_at, updated_at)
		VALUES ('wfi-rej204', 'node-a', '[]', 1, 0, '[]', datetime('now'), datetime('now'))`); err != nil {
		t.Fatalf("insert agent_step_cursors without rejections: %v", err)
	}

	var rejections string
	if err := pool.QueryRow(`SELECT rejections FROM agent_step_cursors WHERE workflow_instance_id = 'wfi-rej204' AND node_id = 'node-a'`).Scan(&rejections); err != nil {
		t.Fatalf("select rejections: %v", err)
	}
	if rejections != "{}" {
		t.Errorf("rejections = %q, want default {}", rejections)
	}
}

// TestMigration204_JSONSetExtractRoundTrips exercises the exact json_set/
// json_extract idiom AgentStepCursorRepo.RecordRejection relies on (JSON1
// availability, per repo/agent_message.go's precedent).
func TestMigration204_JSONSetExtractRoundTrips(t *testing.T) {
	t.Parallel()
	pool, err := newMigratedTestPool(t)
	if err != nil {
		t.Fatalf("NewPoolPath: %v", err)
	}
	t.Cleanup(func() { pool.Close() })

	seedProjectWorkflow(t, pool)
	if _, err := pool.Exec(`INSERT INTO workflow_instances (id, project_id, ticket_id, workflow_id, scope_type, status, retry_count, created_at, updated_at)
		VALUES ('wfi-rej204b', 'p1', '', 'wf1', 'ticket', 'active', 0, datetime('now'), datetime('now'))`); err != nil {
		t.Fatalf("insert workflow_instance: %v", err)
	}
	if _, err := pool.Exec(`INSERT INTO agent_step_cursors
		(workflow_instance_id, node_id, steps_snapshot, revision, current_index, completed, created_at, updated_at)
		VALUES ('wfi-rej204b', 'node-a', '[]', 1, 0, '[]', datetime('now'), datetime('now'))`); err != nil {
		t.Fatalf("insert agent_step_cursors: %v", err)
	}

	path := "$.step-1"
	for i := 0; i < 3; i++ {
		if _, err := pool.Exec(`UPDATE agent_step_cursors
			SET rejections = json_set(rejections, ?, COALESCE(json_extract(rejections, ?), 0) + 1)
			WHERE workflow_instance_id = 'wfi-rej204b' AND node_id = 'node-a'`, path, path); err != nil {
			t.Fatalf("increment %d: %v", i, err)
		}
	}

	var count int
	if err := pool.QueryRow(`SELECT json_extract(rejections, ?) FROM agent_step_cursors WHERE workflow_instance_id = 'wfi-rej204b' AND node_id = 'node-a'`, path).Scan(&count); err != nil {
		t.Fatalf("json_extract: %v", err)
	}
	if count != 3 {
		t.Errorf("count = %d, want 3", count)
	}

	var rejections string
	if err := pool.QueryRow(`SELECT rejections FROM agent_step_cursors WHERE workflow_instance_id = 'wfi-rej204b' AND node_id = 'node-a'`).Scan(&rejections); err != nil {
		t.Fatalf("select rejections: %v", err)
	}
	if rejections != `{"step-1":3}` {
		t.Errorf("rejections = %q, want {\"step-1\":3}", rejections)
	}
}
