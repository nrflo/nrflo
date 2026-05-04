package configmigrate

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"be/internal/clock"
	"be/internal/db"
	"be/internal/repo"
)

func setupMigrateDB(t *testing.T) (*repo.NrvappConfigVersionRepo, string) {
	t.Helper()
	dbPath := filepath.Join(t.TempDir(), "test.db")
	database, err := db.OpenPath(dbPath)
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	t.Cleanup(func() { database.Close() })
	_, err = database.Exec(`INSERT INTO projects (id, name, created_at, updated_at)
		VALUES ('proj-1', 'Test', datetime('now'), datetime('now'))`)
	if err != nil {
		t.Fatalf("insert project: %v", err)
	}
	clk := clock.NewTest(time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC))
	return repo.NewNrvappConfigVersionRepo(database.DB, clk), "proj-1"
}

func noop(_ context.Context, _ Deps) error { return nil }

func TestRegister_Panics_ZeroVersion(t *testing.T) {
	resetForTest()
	t.Cleanup(resetForTest)
	defer func() {
		if r := recover(); r == nil {
			t.Error("Register(0,...): expected panic, got none")
		}
	}()
	Register(0, "zero", noop)
}

func TestRegister_Panics_NegativeVersion(t *testing.T) {
	resetForTest()
	t.Cleanup(resetForTest)
	defer func() {
		if r := recover(); r == nil {
			t.Error("Register(-1,...): expected panic, got none")
		}
	}()
	Register(-1, "neg", noop)
}

func TestRegister_Panics_NilFn(t *testing.T) {
	resetForTest()
	t.Cleanup(resetForTest)
	defer func() {
		if r := recover(); r == nil {
			t.Error("Register(1, nil): expected panic, got none")
		}
	}()
	Register(1, "nil-fn", nil)
}

func TestRegister_Panics_Duplicate(t *testing.T) {
	resetForTest()
	t.Cleanup(resetForTest)
	Register(1, "first", noop)
	defer func() {
		if r := recover(); r == nil {
			t.Error("Register duplicate: expected panic, got none")
		}
	}()
	Register(1, "second", noop)
}

func TestList_SortedAscending(t *testing.T) {
	resetForTest()
	t.Cleanup(resetForTest)

	Register(3, "third", noop)
	Register(1, "first", noop)
	Register(2, "second", noop)

	migrations := List()
	if len(migrations) != 3 {
		t.Fatalf("List len = %d, want 3", len(migrations))
	}
	for i, wantVer := range []int{1, 2, 3} {
		if migrations[i].Version != wantVer {
			t.Errorf("migrations[%d].Version = %d, want %d", i, migrations[i].Version, wantVer)
		}
	}
	if migrations[0].Name != "first" || migrations[1].Name != "second" || migrations[2].Name != "third" {
		t.Errorf("names = %v, %v, %v; want first,second,third",
			migrations[0].Name, migrations[1].Name, migrations[2].Name)
	}
}

func TestList_ReturnsCopy(t *testing.T) {
	resetForTest()
	t.Cleanup(resetForTest)

	Register(1, "first", noop)
	m1 := List()
	Register(2, "second", noop)
	m2 := List()

	if len(m1) != 1 {
		t.Errorf("first List len = %d, want 1", len(m1))
	}
	if len(m2) != 2 {
		t.Errorf("second List len = %d, want 2", len(m2))
	}
}

func TestRun_NoOp_WhenPointerEqualsMax(t *testing.T) {
	resetForTest()
	t.Cleanup(resetForTest)

	r, projectID := setupMigrateDB(t)
	dir := t.TempDir()
	deps := NewDeps(dir, projectID, r, clock.NewTest(time.Now()))

	callCount := 0
	Register(1, "v1", func(_ context.Context, _ Deps) error {
		callCount++
		return nil
	})

	// Run once
	if err := Run(context.Background(), dir, deps); err != nil {
		t.Fatalf("Run first: %v", err)
	}
	// Run again — should be no-op
	if err := Run(context.Background(), dir, deps); err != nil {
		t.Fatalf("Run second: %v", err)
	}

	if callCount != 1 {
		t.Errorf("migration called %d times, want 1", callCount)
	}
}

func TestRun_AdvancesPointerPerMigration(t *testing.T) {
	resetForTest()
	t.Cleanup(resetForTest)

	r, projectID := setupMigrateDB(t)
	dir := t.TempDir()
	deps := NewDeps(dir, projectID, r, clock.NewTest(time.Now()))

	called := make(map[int]bool)
	for _, v := range []int{1, 2, 3} {
		ver := v
		Register(ver, "v"+strconv.Itoa(ver), func(_ context.Context, _ Deps) error {
			called[ver] = true
			return nil
		})
	}

	if err := Run(context.Background(), dir, deps); err != nil {
		t.Fatalf("Run: %v", err)
	}

	for _, ver := range []int{1, 2, 3} {
		if !called[ver] {
			t.Errorf("migration v%d was not called", ver)
		}
	}

	pointer, err := readPointer(deps)
	if err != nil {
		t.Fatalf("readPointer: %v", err)
	}
	if pointer != 3 {
		t.Errorf("pointer = %d, want 3", pointer)
	}
}

func TestRun_HaltsOnError_PointerAtLastSuccess(t *testing.T) {
	resetForTest()
	t.Cleanup(resetForTest)

	r, projectID := setupMigrateDB(t)
	dir := t.TempDir()
	deps := NewDeps(dir, projectID, r, clock.NewTest(time.Now()))

	sentinel := errors.New("migration 2 failed")
	called := make(map[int]bool)

	Register(1, "ok", func(_ context.Context, _ Deps) error {
		called[1] = true
		return nil
	})
	Register(2, "fail", func(_ context.Context, _ Deps) error {
		called[2] = true
		return sentinel
	})
	Register(3, "skipped", func(_ context.Context, _ Deps) error {
		called[3] = true
		return nil
	})

	err := Run(context.Background(), dir, deps)
	if err == nil {
		t.Fatal("Run with failing migration: expected error, got nil")
	}
	if !errors.Is(err, sentinel) {
		t.Errorf("error = %v, want to wrap sentinel", err)
	}
	if !called[1] {
		t.Error("migration 1 was not called")
	}
	if !called[2] {
		t.Error("migration 2 was not called")
	}
	if called[3] {
		t.Error("migration 3 was called despite earlier failure")
	}

	pointer, err := readPointer(deps)
	if err != nil {
		t.Fatalf("readPointer: %v", err)
	}
	if pointer != 1 {
		t.Errorf("pointer = %d, want 1 (last success)", pointer)
	}
}

func TestDeps_Dir(t *testing.T) {
	r, _ := setupMigrateDB(t)
	dir := "/custom/config"
	deps := NewDeps(dir, "proj-1", r, clock.NewTest(time.Now()))
	if deps.Dir() != dir {
		t.Errorf("Dir() = %q, want %q", deps.Dir(), dir)
	}
}

func TestDeps_Backup_SnapshotsFile(t *testing.T) {
	r, projectID := setupMigrateDB(t)
	dir := t.TempDir()
	deps := NewDeps(dir, projectID, r, clock.NewTest(time.Now()))

	content := []byte("original: content\n")
	filename := "config.yaml"
	if err := os.WriteFile(filepath.Join(dir, filename), content, 0644); err != nil {
		t.Fatalf("write file: %v", err)
	}

	if err := deps.Backup(context.Background(), filename); err != nil {
		t.Fatalf("Backup: %v", err)
	}

	ver, err := r.LatestVersion(projectID, filename)
	if err != nil {
		t.Fatalf("LatestVersion: %v", err)
	}
	if ver != 1 {
		t.Errorf("LatestVersion = %d, want 1", ver)
	}

	snap, err := r.Get(projectID, filename, 1)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if string(snap.Content) != string(content) {
		t.Errorf("Content = %q, want %q", snap.Content, content)
	}
	if snap.Actor == nil || *snap.Actor != "configmigrate" {
		t.Errorf("Actor = %v, want 'configmigrate'", snap.Actor)
	}
}

func TestDeps_Backup_PathTraversal(t *testing.T) {
	r, projectID := setupMigrateDB(t)
	dir := t.TempDir()
	deps := NewDeps(dir, projectID, r, clock.NewTest(time.Now()))

	err := deps.Backup(context.Background(), "../secret.yaml")
	if err == nil {
		t.Fatal("Backup ../: expected error, got nil")
	}
	if !strings.Contains(err.Error(), "parent") {
		t.Errorf("error = %q, want to mention 'parent'", err.Error())
	}
}

func TestDeps_Backup_AbsPath(t *testing.T) {
	r, projectID := setupMigrateDB(t)
	deps := NewDeps(t.TempDir(), projectID, r, clock.NewTest(time.Now()))

	err := deps.Backup(context.Background(), "/etc/passwd")
	if err == nil {
		t.Fatal("Backup abs path: expected error, got nil")
	}
}

func TestDeps_Validate_Pass(t *testing.T) {
	r, projectID := setupMigrateDB(t)
	dir := t.TempDir()
	deps := NewDeps(dir, projectID, r, clock.NewTest(time.Now()))

	content := []byte("name: test\nvalue: 42\n")
	os.WriteFile(filepath.Join(dir, "config.yaml"), content, 0644) //nolint

	schema := []byte(`{"type":"object","properties":{"name":{"type":"string"},"value":{"type":"integer"}},"required":["name","value"]}`)
	if err := deps.Validate("config.yaml", schema); err != nil {
		t.Errorf("Validate valid: %v", err)
	}
}

func TestDeps_Validate_Fail(t *testing.T) {
	r, projectID := setupMigrateDB(t)
	dir := t.TempDir()
	deps := NewDeps(dir, projectID, r, clock.NewTest(time.Now()))

	// Write invalid YAML (missing required "value")
	os.WriteFile(filepath.Join(dir, "invalid.yaml"), []byte("name: test\n"), 0644) //nolint

	schema := []byte(`{"type":"object","properties":{"name":{"type":"string"},"value":{"type":"integer"}},"required":["name","value"]}`)
	if err := deps.Validate("invalid.yaml", schema); err == nil {
		t.Error("Validate invalid: expected error, got nil")
	}
}

func TestDeps_Validate_EmptySchema(t *testing.T) {
	r, projectID := setupMigrateDB(t)
	dir := t.TempDir()
	deps := NewDeps(dir, projectID, r, clock.NewTest(time.Now()))

	os.WriteFile(filepath.Join(dir, "any.yaml"), []byte("anything: goes\n"), 0644) //nolint

	if err := deps.Validate("any.yaml", nil); err != nil {
		t.Errorf("Validate empty schema: %v", err)
	}
}
