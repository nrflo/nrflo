package repo

import (
	"path/filepath"
	"testing"
	"time"

	"be/internal/clock"
	"be/internal/db"
	"be/internal/model"
)

func setupNrvappConfigVersionDB(t *testing.T) (*NrvappConfigVersionRepo, string) {
	t.Helper()
	dbPath := filepath.Join(t.TempDir(), "test.db")
	database, err := db.OpenPath(dbPath)
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	t.Cleanup(func() { database.Close() })
	_, err = database.Exec(
		`INSERT INTO projects (id, name, created_at, updated_at) VALUES ('proj-1', 'Test', datetime('now'), datetime('now'))`)
	if err != nil {
		t.Fatalf("insert project: %v", err)
	}
	clk := clock.NewTest(time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC))
	return NewNrvappConfigVersionRepo(database.DB, clk), "proj-1"
}

func TestNrvappConfigVersionRepo_InsertAutoVersion(t *testing.T) {
	t.Parallel()
	r, projectID := setupNrvappConfigVersionDB(t)

	for i, wantVersion := range []int{1, 2, 3} {
		v := &model.NrvappConfigVersion{
			ProjectID: projectID,
			File:      "config.yaml",
			Content:   []byte("content"),
		}
		if err := r.Insert(v); err != nil {
			t.Fatalf("Insert %d: %v", i+1, err)
		}
		if v.Version != wantVersion {
			t.Errorf("Insert %d: Version = %d, want %d", i+1, v.Version, wantVersion)
		}
		if v.ID == 0 {
			t.Errorf("Insert %d: ID = 0, want non-zero", i+1)
		}
		if v.CreatedAt.IsZero() {
			t.Errorf("Insert %d: CreatedAt is zero", i+1)
		}
	}
}

func TestNrvappConfigVersionRepo_VersionsIndependentPerFile(t *testing.T) {
	t.Parallel()
	r, projectID := setupNrvappConfigVersionDB(t)

	// Two inserts for config.yaml → versions 1 and 2
	for i := 1; i <= 2; i++ {
		v := &model.NrvappConfigVersion{ProjectID: projectID, File: "config.yaml", Content: []byte("v")}
		if err := r.Insert(v); err != nil {
			t.Fatalf("Insert config.yaml %d: %v", i, err)
		}
		if v.Version != i {
			t.Errorf("config.yaml insert %d: Version = %d, want %d", i, v.Version, i)
		}
	}

	// First insert for other.yaml → version 1 (independent counter)
	v2 := &model.NrvappConfigVersion{ProjectID: projectID, File: "other.yaml", Content: []byte("v")}
	if err := r.Insert(v2); err != nil {
		t.Fatalf("Insert other.yaml: %v", err)
	}
	if v2.Version != 1 {
		t.Errorf("other.yaml first version = %d, want 1", v2.Version)
	}
}

func TestNrvappConfigVersionRepo_LatestVersionUnknown(t *testing.T) {
	t.Parallel()
	r, projectID := setupNrvappConfigVersionDB(t)

	ver, err := r.LatestVersion(projectID, "nonexistent.yaml")
	if err != nil {
		t.Fatalf("LatestVersion unknown: %v", err)
	}
	if ver != 0 {
		t.Errorf("LatestVersion unknown = %d, want 0", ver)
	}
}

func TestNrvappConfigVersionRepo_LatestVersionAfterInserts(t *testing.T) {
	t.Parallel()
	r, projectID := setupNrvappConfigVersionDB(t)

	for i := 0; i < 3; i++ {
		v := &model.NrvappConfigVersion{ProjectID: projectID, File: "f.yaml", Content: []byte("c")}
		r.Insert(v)
	}

	ver, err := r.LatestVersion(projectID, "f.yaml")
	if err != nil {
		t.Fatalf("LatestVersion: %v", err)
	}
	if ver != 3 {
		t.Errorf("LatestVersion = %d, want 3", ver)
	}
}

func TestNrvappConfigVersionRepo_GetSpecificVersion(t *testing.T) {
	t.Parallel()
	r, projectID := setupNrvappConfigVersionDB(t)

	actor := "alice"
	for i, content := range []string{"first", "second", "third"} {
		v := &model.NrvappConfigVersion{
			ProjectID: projectID,
			File:      "config.yaml",
			Content:   []byte(content),
		}
		if i == 1 {
			v.Actor = &actor
		}
		if err := r.Insert(v); err != nil {
			t.Fatalf("Insert %d: %v", i+1, err)
		}
	}

	got, err := r.Get(projectID, "config.yaml", 2)
	if err != nil {
		t.Fatalf("Get version 2: %v", err)
	}
	if got.Version != 2 {
		t.Errorf("Version = %d, want 2", got.Version)
	}
	if string(got.Content) != "second" {
		t.Errorf("Content = %q, want 'second'", got.Content)
	}
	if got.Actor == nil || *got.Actor != "alice" {
		t.Errorf("Actor = %v, want 'alice'", got.Actor)
	}
	if got.CreatedAt.IsZero() {
		t.Errorf("CreatedAt is zero")
	}
}

func TestNrvappConfigVersionRepo_GetNotFound(t *testing.T) {
	t.Parallel()
	r, projectID := setupNrvappConfigVersionDB(t)

	_, err := r.Get(projectID, "missing.yaml", 99)
	if err == nil {
		t.Fatal("Get missing version: expected error, got nil")
	}
}

func TestNrvappConfigVersionRepo_History_DescOrder(t *testing.T) {
	t.Parallel()
	r, projectID := setupNrvappConfigVersionDB(t)

	for _, content := range []string{"v1", "v2", "v3"} {
		v := &model.NrvappConfigVersion{ProjectID: projectID, File: "h.yaml", Content: []byte(content)}
		if err := r.Insert(v); err != nil {
			t.Fatalf("Insert: %v", err)
		}
	}

	history, err := r.History(projectID, "h.yaml")
	if err != nil {
		t.Fatalf("History: %v", err)
	}
	if len(history) != 3 {
		t.Fatalf("History len = %d, want 3", len(history))
	}
	// Newest first: version 3, 2, 1
	for i, wantVer := range []int{3, 2, 1} {
		if history[i].Version != wantVer {
			t.Errorf("history[%d].Version = %d, want %d", i, history[i].Version, wantVer)
		}
	}
	// Contents match insertion order in reverse
	for i, wantContent := range []string{"v3", "v2", "v1"} {
		if string(history[i].Content) != wantContent {
			t.Errorf("history[%d].Content = %q, want %q", i, history[i].Content, wantContent)
		}
	}
}

func TestNrvappConfigVersionRepo_HistoryEmpty(t *testing.T) {
	t.Parallel()
	r, projectID := setupNrvappConfigVersionDB(t)

	history, err := r.History(projectID, "empty.yaml")
	if err != nil {
		t.Fatalf("History empty: %v", err)
	}
	if len(history) != 0 {
		t.Errorf("History empty: len = %d, want 0", len(history))
	}
}

func TestNrvappConfigVersionRepo_SequentialInsertsMonotonic(t *testing.T) {
	t.Parallel()
	r, projectID := setupNrvappConfigVersionDB(t)

	v1 := &model.NrvappConfigVersion{ProjectID: projectID, File: "seq.yaml", Content: []byte("first")}
	if err := r.Insert(v1); err != nil {
		t.Fatalf("Insert v1: %v", err)
	}
	if v1.Version != 1 {
		t.Errorf("v1.Version = %d, want 1", v1.Version)
	}

	v2 := &model.NrvappConfigVersion{ProjectID: projectID, File: "seq.yaml", Content: []byte("second")}
	if err := r.Insert(v2); err != nil {
		t.Fatalf("Insert v2: %v", err)
	}
	if v2.Version != 2 {
		t.Errorf("v2.Version = %d, want 2", v2.Version)
	}

	if v2.ID <= v1.ID {
		t.Errorf("v2.ID %d not greater than v1.ID %d", v2.ID, v1.ID)
	}
}
