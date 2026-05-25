package db

import "testing"

// TestMigration131_ExternalIDColumns verifies external_id and external_context
// were added as nullable TEXT to workflow_instances.
func TestMigration131_ExternalIDColumns(t *testing.T) {
	pool, err := newMigratedTestPool(t)
	if err != nil {
		t.Fatalf("newMigratedTestPool: %v", err)
	}
	t.Cleanup(func() { pool.Close() })

	cols := tableColumns(t, pool, "workflow_instances")

	extID, ok := cols["external_id"]
	if !ok {
		t.Fatal("external_id column missing from workflow_instances")
	}
	if extID.colType != "TEXT" {
		t.Errorf("external_id type = %q, want TEXT", extID.colType)
	}
	if extID.notNull != 0 {
		t.Errorf("external_id notNull = %d, want 0 (nullable)", extID.notNull)
	}

	extCtx, ok := cols["external_context"]
	if !ok {
		t.Fatal("external_context column missing from workflow_instances")
	}
	if extCtx.colType != "TEXT" {
		t.Errorf("external_context type = %q, want TEXT", extCtx.colType)
	}
	if extCtx.notNull != 0 {
		t.Errorf("external_context notNull = %d, want 0 (nullable)", extCtx.notNull)
	}
}

// TestMigration131_ExternalIDIndex verifies idx_workflow_instances_external_id was created.
func TestMigration131_ExternalIDIndex(t *testing.T) {
	pool, err := newMigratedTestPool(t)
	if err != nil {
		t.Fatalf("newMigratedTestPool: %v", err)
	}
	t.Cleanup(func() { pool.Close() })

	want := map[string]bool{
		"idx_workflow_instances_external_id": false,
	}
	rows, err := pool.Query(`SELECT name FROM sqlite_master WHERE type='index'`)
	if err != nil {
		t.Fatalf("query indexes: %v", err)
	}
	defer rows.Close()
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			t.Fatalf("scan: %v", err)
		}
		if _, ok := want[name]; ok {
			want[name] = true
		}
	}
	for n, ok := range want {
		if !ok {
			t.Errorf("index %s not created by migration 131", n)
		}
	}
}
