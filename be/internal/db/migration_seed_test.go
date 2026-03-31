package db

import (
	"path/filepath"
	"strings"
	"testing"
)

// TestMigration039_SeedsConflictResolver verifies that migration 000039 inserts
// the conflict-resolver system agent definition with the expected values.
func TestMigration039_SeedsConflictResolver(t *testing.T) {
	pool, err := NewPoolPath(filepath.Join(t.TempDir(), "test.db"), DefaultPoolConfig())
	if err != nil {
		t.Fatalf("NewPoolPath: %v", err)
	}
	t.Cleanup(func() { pool.Close() })

	var id, model, prompt string
	var timeout int
	err = pool.QueryRow(
		"SELECT id, model, timeout, prompt FROM system_agent_definitions WHERE id = 'conflict-resolver'",
	).Scan(&id, &model, &timeout, &prompt)
	if err != nil {
		t.Fatalf("query conflict-resolver row: %v", err)
	}

	if id != "conflict-resolver" {
		t.Errorf("id = %q, want %q", id, "conflict-resolver")
	}
	if model != "sonnet" {
		t.Errorf("model = %q, want %q", model, "sonnet")
	}
	if timeout != 20 {
		t.Errorf("timeout = %d, want 20", timeout)
	}
	// Verify prompt contains key sections from the migration SQL.
	for _, want := range []string{
		"Merge Conflict Resolver",
		"${BRANCH_NAME}",
		"${DEFAULT_BRANCH}",
		"${MERGE_ERROR}",
		"nrworkflow agent fail",
		"git merge",
	} {
		if !strings.Contains(prompt, want) {
			t.Errorf("prompt does not contain %q", want)
		}
	}
}

// TestMigration039_NullableFieldsAreNull verifies that optional configuration fields
// are NULL after seeding (spawner uses built-in defaults when NULL).
func TestMigration039_NullableFieldsAreNull(t *testing.T) {
	pool, err := NewPoolPath(filepath.Join(t.TempDir(), "test.db"), DefaultPoolConfig())
	if err != nil {
		t.Fatalf("NewPoolPath: %v", err)
	}
	t.Cleanup(func() { pool.Close() })

	var restartThreshold, maxFailRestarts, stallStart, stallRunning *int
	err = pool.QueryRow(
		`SELECT restart_threshold, max_fail_restarts, stall_start_timeout_sec, stall_running_timeout_sec
		 FROM system_agent_definitions WHERE id = 'conflict-resolver'`,
	).Scan(&restartThreshold, &maxFailRestarts, &stallStart, &stallRunning)
	if err != nil {
		t.Fatalf("query nullable fields: %v", err)
	}

	if restartThreshold != nil {
		t.Errorf("restart_threshold = %v, want NULL", *restartThreshold)
	}
	if maxFailRestarts != nil {
		t.Errorf("max_fail_restarts = %v, want NULL", *maxFailRestarts)
	}
	if stallStart != nil {
		t.Errorf("stall_start_timeout_sec = %v, want NULL", *stallStart)
	}
	if stallRunning != nil {
		t.Errorf("stall_running_timeout_sec = %v, want NULL", *stallRunning)
	}
}

// TestMigration039_TimestampsSet verifies that created_at and updated_at are populated.
func TestMigration039_TimestampsSet(t *testing.T) {
	pool, err := NewPoolPath(filepath.Join(t.TempDir(), "test.db"), DefaultPoolConfig())
	if err != nil {
		t.Fatalf("NewPoolPath: %v", err)
	}
	t.Cleanup(func() { pool.Close() })

	var createdAt, updatedAt string
	err = pool.QueryRow(
		"SELECT created_at, updated_at FROM system_agent_definitions WHERE id = 'conflict-resolver'",
	).Scan(&createdAt, &updatedAt)
	if err != nil {
		t.Fatalf("query timestamps: %v", err)
	}
	if createdAt == "" {
		t.Error("created_at is empty")
	}
	if updatedAt == "" {
		t.Error("updated_at is empty")
	}
}

// TestMigration039_DownMigrationDeletesRow verifies that the down migration SQL
// (DELETE FROM system_agent_definitions WHERE id = 'conflict-resolver') removes
// the seeded row, leaving no trace.
func TestMigration039_DownMigrationDeletesRow(t *testing.T) {
	pool, err := NewPoolPath(filepath.Join(t.TempDir(), "test.db"), DefaultPoolConfig())
	if err != nil {
		t.Fatalf("NewPoolPath: %v", err)
	}
	t.Cleanup(func() { pool.Close() })

	// Confirm the row exists before running down migration.
	var countBefore int
	if err := pool.QueryRow("SELECT COUNT(*) FROM system_agent_definitions WHERE id = 'conflict-resolver'").Scan(&countBefore); err != nil {
		t.Fatalf("count before down: %v", err)
	}
	if countBefore != 1 {
		t.Fatalf("expected 1 row before down migration, got %d", countBefore)
	}

	// Execute down migration SQL exactly as written in the .down.sql file.
	if _, err := pool.Exec("DELETE FROM system_agent_definitions WHERE id = 'conflict-resolver'"); err != nil {
		t.Fatalf("down migration DELETE: %v", err)
	}

	// Confirm the row is gone.
	var countAfter int
	if err := pool.QueryRow("SELECT COUNT(*) FROM system_agent_definitions WHERE id = 'conflict-resolver'").Scan(&countAfter); err != nil {
		t.Fatalf("count after down: %v", err)
	}
	if countAfter != 0 {
		t.Errorf("expected 0 rows after down migration, got %d", countAfter)
	}
}

// TestMigration039_DuplicateInsertFails verifies that the migration uses a plain
// INSERT (not INSERT OR IGNORE), so if the conflict-resolver row already exists,
// the insert fails loudly with a UNIQUE constraint violation.
func TestMigration039_DuplicateInsertFails(t *testing.T) {
	pool, err := NewPoolPath(filepath.Join(t.TempDir(), "test.db"), DefaultPoolConfig())
	if err != nil {
		t.Fatalf("NewPoolPath: %v", err)
	}
	t.Cleanup(func() { pool.Close() })

	// conflict-resolver is already seeded by migration. A plain INSERT must fail.
	_, insertErr := pool.Exec(
		"INSERT INTO system_agent_definitions (id, model, timeout, prompt, created_at, updated_at) VALUES (?, ?, ?, ?, datetime('now'), datetime('now'))",
		"conflict-resolver", "haiku", 5, "duplicate-test",
	)
	if insertErr == nil {
		t.Fatal("expected UNIQUE constraint error for duplicate conflict-resolver insert, got nil")
	}
	errStr := insertErr.Error()
	if !strings.Contains(strings.ToUpper(errStr), "UNIQUE") && !strings.Contains(errStr, "constraint") {
		t.Errorf("expected UNIQUE constraint error, got: %v", insertErr)
	}
}

// TestMigration039_ExactlyOneRow verifies only one conflict-resolver row is seeded.
func TestMigration039_ExactlyOneRow(t *testing.T) {
	pool, err := NewPoolPath(filepath.Join(t.TempDir(), "test.db"), DefaultPoolConfig())
	if err != nil {
		t.Fatalf("NewPoolPath: %v", err)
	}
	t.Cleanup(func() { pool.Close() })

	var count int
	if err := pool.QueryRow("SELECT COUNT(*) FROM system_agent_definitions WHERE id = 'conflict-resolver'").Scan(&count); err != nil {
		t.Fatalf("count: %v", err)
	}
	if count != 1 {
		t.Errorf("expected exactly 1 conflict-resolver row, got %d", count)
	}
}
