package db

import "testing"

// TestMigration189_RefineryAutonomousDigestsTableSchema verifies the sibling
// slot table's columns and its (workflow_instance_id, node_id) composite
// primary key — deliberately a SEPARATE table from refinery_digests (whose
// PK is console_session_id, migration179_seed_test.go) rather than a
// generalized PK, per the ticket's design decision.
func TestMigration189_RefineryAutonomousDigestsTableSchema(t *testing.T) {
	pool, err := newMigratedTestPool(t)
	if err != nil {
		t.Fatalf("NewPoolPath: %v", err)
	}
	t.Cleanup(func() { pool.Close() })

	rows, err := pool.Query(`PRAGMA table_info(refinery_autonomous_digests)`)
	if err != nil {
		t.Fatalf("PRAGMA table_info: %v", err)
	}
	defer rows.Close()

	wantCols := map[string]bool{
		"workflow_instance_id": false,
		"node_id":              false,
		"project_id":           false,
		"version":              false,
		"content":              false,
		"fold_count":           false,
		"created_at":           false,
		"updated_at":           false,
	}
	pkCols := map[string]bool{}
	for rows.Next() {
		var cid, notnull, pk int
		var name, ctype string
		var dflt any
		if err := rows.Scan(&cid, &name, &ctype, &notnull, &dflt, &pk); err != nil {
			t.Fatalf("scan table_info row: %v", err)
		}
		if _, ok := wantCols[name]; ok {
			wantCols[name] = true
		}
		if pk > 0 {
			pkCols[name] = true
		}
	}
	for col, seen := range wantCols {
		if !seen {
			t.Errorf("refinery_autonomous_digests missing expected column %q", col)
		}
	}
	if !pkCols["workflow_instance_id"] || !pkCols["node_id"] {
		t.Errorf("refinery_autonomous_digests PRIMARY KEY cols = %v, want {workflow_instance_id, node_id}", pkCols)
	}
	if len(pkCols) != 2 {
		t.Errorf("refinery_autonomous_digests PRIMARY KEY has %d columns, want exactly 2", len(pkCols))
	}
}

// TestMigration189_RefineryAutonomousEnabledSeededDefaultOn verifies the
// global config seed. The seed value is 'true', but session_sidecar.go reads
// it with default-ON semantics (val != "false") — this test only asserts the
// literal seeded row, not the read semantics (covered in refinery package
// tests).
func TestMigration189_RefineryAutonomousEnabledSeededDefaultOn(t *testing.T) {
	pool, err := newMigratedTestPool(t)
	if err != nil {
		t.Fatalf("NewPoolPath: %v", err)
	}
	t.Cleanup(func() { pool.Close() })

	var value string
	err = pool.QueryRow(
		`SELECT value FROM config WHERE project_id = '' AND key = 'refinery_autonomous_enabled'`,
	).Scan(&value)
	if err != nil {
		t.Fatalf("SELECT config refinery_autonomous_enabled: %v", err)
	}
	if value != "true" {
		t.Errorf("refinery_autonomous_enabled seed value = %q, want %q", value, "true")
	}
}
