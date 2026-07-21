package db

import "testing"

// TestMigration191_ReleaseDateColumnPresent verifies the nullable
// release_date column exists on models (PRAGMA table_info), added by
// 000191 alongside the pricing columns from 000183.
func TestMigration191_ReleaseDateColumnPresent(t *testing.T) {
	pool, err := newMigratedTestPool(t)
	if err != nil {
		t.Fatalf("NewPoolPath: %v", err)
	}
	t.Cleanup(func() { pool.Close() })

	rows, err := pool.Query(`PRAGMA table_info(models)`)
	if err != nil {
		t.Fatalf("PRAGMA table_info: %v", err)
	}
	defer rows.Close()

	found := false
	for rows.Next() {
		var cid, notnull, pk int
		var name, ctype string
		var dflt any
		if err := rows.Scan(&cid, &name, &ctype, &notnull, &dflt, &pk); err != nil {
			t.Fatalf("scan table_info row: %v", err)
		}
		if name == "release_date" {
			found = true
			if notnull != 0 {
				t.Errorf("release_date notnull = %d, want 0 (nullable)", notnull)
			}
		}
	}
	if !found {
		t.Fatal("models table missing release_date column")
	}
}

// TestMigration191_SeededReleaseDateOrdering verifies the per-id UPDATE
// seeds land in the intended chronological order: newer opus/gpt revisions
// carry a later release_date than their predecessors, and a known
// cross-provider pair compares as expected.
func TestMigration191_SeededReleaseDateOrdering(t *testing.T) {
	pool, err := newMigratedTestPool(t)
	if err != nil {
		t.Fatalf("NewPoolPath: %v", err)
	}
	t.Cleanup(func() { pool.Close() })

	releaseDate := func(id string) string {
		t.Helper()
		var d string
		if err := pool.QueryRow(`SELECT release_date FROM models WHERE id = ?`, id).Scan(&d); err != nil {
			t.Fatalf("SELECT release_date id=%s: %v", id, err)
		}
		return d
	}

	opus8, opus7 := releaseDate("opus-4-8"), releaseDate("opus-4-7")
	if opus8 == "" || opus7 == "" {
		t.Fatalf("opus-4-8/opus-4-7 release_date should be seeded, got %q/%q", opus8, opus7)
	}
	if opus8 <= opus7 {
		t.Errorf("opus-4-8 release_date %q should be after opus-4-7 release_date %q", opus8, opus7)
	}

	sol, gpt54 := releaseDate("gpt-5.6-sol"), releaseDate("gpt-5.4")
	if sol == "" || gpt54 == "" {
		t.Fatalf("gpt-5.6-sol/gpt-5.4 release_date should be seeded, got %q/%q", sol, gpt54)
	}
	if sol <= gpt54 {
		t.Errorf("gpt-5.6-sol release_date %q should be after gpt-5.4 release_date %q", sol, gpt54)
	}

	// Cross-provider pair: haiku-4-5 (2026-07-01) predates gpt-5.6-sol (2026-07-16).
	haiku := releaseDate("haiku-4-5")
	if haiku == "" {
		t.Fatal("haiku-4-5 release_date should be seeded")
	}
	if haiku >= sol {
		t.Errorf("haiku-4-5 release_date %q should be before gpt-5.6-sol release_date %q", haiku, sol)
	}
}
