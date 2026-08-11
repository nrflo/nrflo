package db

import (
	"database/sql"
	"path/filepath"
	"testing"

	"github.com/golang-migrate/migrate/v4"
)

// TestMigration212_DelegateTimeoutsRescaledToMinutes verifies the delegate
// tier seeds 000182 authored against the old seconds reading (300 / 1800) are
// rescaled to the minutes the column actually means, so they keep their
// intended wall time once spawner.SpawnDeadline reads them as minutes.
func TestMigration212_DelegateTimeoutsRescaledToMinutes(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "migration212-delegate.db")
	sqlDB, err := sql.Open("sqlite", buildDSN(dbPath))
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	t.Cleanup(func() { sqlDB.Close() })

	m := newMigrateInstance(t, sqlDB)
	if err := m.Up(); err != nil && err != migrate.ErrNoChange {
		t.Fatalf("migrate up: %v", err)
	}

	// _t1_executor lands at 60 after the full chain: 212 rescales 1800->30,
	// then 236 raises the seeded 30 to 60.
	want := map[string]int{"_t2_extractor": 5, "_t1_executor": 60}
	for id, wantTimeout := range want {
		var got int
		if err := sqlDB.QueryRow(`SELECT timeout FROM system_agent_definitions WHERE id = ?`, id).Scan(&got); err != nil {
			t.Fatalf("query %s timeout: %v", id, err)
		}
		if got != wantTimeout {
			t.Errorf("%s timeout = %d, want %d (minutes)", id, got, wantTimeout)
		}
	}
}

// TestMigration212_LeavesOperatorEditedTimeout verifies the value-matched
// UPDATE: a timeout an operator typed into the minutes-labelled agent form is
// already in the right unit and must not be rescaled again.
func TestMigration212_LeavesOperatorEditedTimeout(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "migration212-edited.db")
	sqlDB, err := sql.Open("sqlite", buildDSN(dbPath))
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	t.Cleanup(func() { sqlDB.Close() })

	m := newMigrateInstance(t, sqlDB)
	if err := m.Migrate(211); err != nil {
		t.Fatalf("migrate to 211: %v", err)
	}
	if _, err := sqlDB.Exec(`UPDATE system_agent_definitions SET timeout = 45 WHERE id = '_t1_executor'`); err != nil {
		t.Fatalf("seed operator edit: %v", err)
	}

	if err := m.Up(); err != nil && err != migrate.ErrNoChange {
		t.Fatalf("migrate remaining (000212): %v", err)
	}

	var got int
	if err := sqlDB.QueryRow(`SELECT timeout FROM system_agent_definitions WHERE id = '_t1_executor'`).Scan(&got); err != nil {
		t.Fatalf("query _t1_executor timeout: %v", err)
	}
	if got != 45 {
		t.Errorf("_t1_executor timeout = %d, want 45 (operator edit preserved)", got)
	}
}

// TestMigration212_PlannerTimeoutsUnchanged pins that the planner seeds are
// NOT rescaled: 000158 authored them as minutes, and they were only ever
// being misread at the call site.
func TestMigration212_PlannerTimeoutsUnchanged(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "migration212-planner.db")
	sqlDB, err := sql.Open("sqlite", buildDSN(dbPath))
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	t.Cleanup(func() { sqlDB.Close() })

	m := newMigrateInstance(t, sqlDB)
	if err := m.Up(); err != nil && err != migrate.ErrNoChange {
		t.Fatalf("migrate up: %v", err)
	}

	for _, id := range []string{"planner-system", "planner-system-api"} {
		var got int
		if err := sqlDB.QueryRow(`SELECT timeout FROM system_agent_definitions WHERE id = ?`, id).Scan(&got); err != nil {
			t.Fatalf("query %s timeout: %v", id, err)
		}
		if got != 10 {
			t.Errorf("%s timeout = %d, want 10 (minutes, unchanged)", id, got)
		}
	}
}
