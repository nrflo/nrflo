package db

import "testing"

// 000236 raises the executor tier's seeded timeout from 30 to 60 minutes —
// real work packages run 25-30 min and were dying at the old budget.
func TestMigration236_ExecutorTimeout60(t *testing.T) {
	pool, err := newMigratedTestPool(t)
	if err != nil {
		t.Fatalf("NewPoolPath: %v", err)
	}
	t.Cleanup(func() { pool.Close() })

	var timeout int
	if err := pool.QueryRow(
		`SELECT timeout FROM system_agent_definitions WHERE id = '_t1_executor'`,
	).Scan(&timeout); err != nil {
		t.Fatalf("SELECT _t1_executor: %v", err)
	}
	if timeout != 60 {
		t.Errorf("_t1_executor timeout = %d, want 60 (minutes)", timeout)
	}
}
