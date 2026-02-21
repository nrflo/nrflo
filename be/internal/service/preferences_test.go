package service

import (
	"path/filepath"
	"testing"
	"time"

	"be/internal/clock"
	"be/internal/db"
)

func setupPreferencesTestDB(t *testing.T) *db.Pool {
	t.Helper()
	dbPath := filepath.Join(t.TempDir(), "preferences_test.db")
	pool, err := db.NewPoolPath(dbPath, db.DefaultPoolConfig())
	if err != nil {
		t.Fatalf("failed to create pool: %v", err)
	}
	t.Cleanup(func() { pool.Close() })
	return pool
}

func TestPreferences_SetAndGet(t *testing.T) {
	pool := setupPreferencesTestDB(t)
	fixedTime := time.Date(2026, 1, 15, 10, 0, 0, 0, time.UTC)
	clk := clock.NewTest(fixedTime)
	svc := NewPreferencesService(pool, clk)

	if err := svc.Set("my-key", "my-value"); err != nil {
		t.Fatalf("Set() error = %v", err)
	}

	pref, err := svc.Get("my-key")
	if err != nil {
		t.Fatalf("Get() error = %v", err)
	}
	if pref == nil {
		t.Fatal("Get() returned nil, want non-nil")
	}
	if pref.Name != "my-key" {
		t.Errorf("Get().Name = %q, want %q", pref.Name, "my-key")
	}
	if pref.Value != "my-value" {
		t.Errorf("Get().Value = %q, want %q", pref.Value, "my-value")
	}
	if !pref.CreatedAt.Equal(fixedTime) {
		t.Errorf("Get().CreatedAt = %v, want %v", pref.CreatedAt, fixedTime)
	}
	if !pref.UpdatedAt.Equal(fixedTime) {
		t.Errorf("Get().UpdatedAt = %v, want %v", pref.UpdatedAt, fixedTime)
	}
}

func TestPreferences_GetNonExistent(t *testing.T) {
	pool := setupPreferencesTestDB(t)
	svc := NewPreferencesService(pool, clock.Real())

	pref, err := svc.Get("does-not-exist")
	if err != nil {
		t.Fatalf("Get() on missing key error = %v, want nil", err)
	}
	if pref != nil {
		t.Errorf("Get() = %+v, want nil for missing key", pref)
	}
}

func TestPreferences_SetUpsertPreservesCreatedAt(t *testing.T) {
	pool := setupPreferencesTestDB(t)
	t1 := time.Date(2026, 1, 15, 10, 0, 0, 0, time.UTC)
	clk := clock.NewTest(t1)
	svc := NewPreferencesService(pool, clk)

	// First Set — establishes created_at = t1
	if err := svc.Set("config-key", "initial-value"); err != nil {
		t.Fatalf("first Set() error = %v", err)
	}

	// Advance clock before second Set
	t2 := t1.Add(5 * time.Minute)
	clk.Set(t2)

	// Second Set — should update value and updated_at, not created_at
	if err := svc.Set("config-key", "updated-value"); err != nil {
		t.Fatalf("second Set() error = %v", err)
	}

	pref, err := svc.Get("config-key")
	if err != nil {
		t.Fatalf("Get() error = %v", err)
	}
	if pref == nil {
		t.Fatal("Get() returned nil after upsert")
	}
	if pref.Value != "updated-value" {
		t.Errorf("Get().Value = %q, want %q", pref.Value, "updated-value")
	}
	if !pref.CreatedAt.Equal(t1) {
		t.Errorf("Get().CreatedAt = %v, want %v (original, must not change on update)", pref.CreatedAt, t1)
	}
	if !pref.UpdatedAt.Equal(t2) {
		t.Errorf("Get().UpdatedAt = %v, want %v (must reflect second Set time)", pref.UpdatedAt, t2)
	}
}

func TestPreferences_SetMultipleKeys(t *testing.T) {
	pool := setupPreferencesTestDB(t)
	clk := clock.NewTest(time.Date(2026, 1, 15, 10, 0, 0, 0, time.UTC))
	svc := NewPreferencesService(pool, clk)

	entries := []struct{ name, value string }{
		{"key-a", "val-a"},
		{"key-b", `{"json": true}`},
		{"key-c", ""},
	}

	for _, e := range entries {
		if err := svc.Set(e.name, e.value); err != nil {
			t.Fatalf("Set(%q, %q) error = %v", e.name, e.value, err)
		}
	}

	for _, e := range entries {
		pref, err := svc.Get(e.name)
		if err != nil {
			t.Fatalf("Get(%q) error = %v", e.name, err)
		}
		if pref == nil {
			t.Fatalf("Get(%q) = nil, want non-nil", e.name)
		}
		if pref.Value != e.value {
			t.Errorf("Get(%q).Value = %q, want %q", e.name, pref.Value, e.value)
		}
	}
}

func TestPreferences_GetMissingKeyDoesNotAffectOthers(t *testing.T) {
	pool := setupPreferencesTestDB(t)
	clk := clock.NewTest(time.Date(2026, 1, 15, 10, 0, 0, 0, time.UTC))
	svc := NewPreferencesService(pool, clk)

	if err := svc.Set("existing", "value"); err != nil {
		t.Fatalf("Set() error = %v", err)
	}

	missing, err := svc.Get("nonexistent")
	if err != nil {
		t.Fatalf("Get(nonexistent) error = %v", err)
	}
	if missing != nil {
		t.Errorf("Get(nonexistent) = %+v, want nil", missing)
	}

	existing, err := svc.Get("existing")
	if err != nil {
		t.Fatalf("Get(existing) error = %v", err)
	}
	if existing == nil || existing.Value != "value" {
		t.Errorf("Get(existing).Value = %v, want %q", existing, "value")
	}
}
