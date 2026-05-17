package spawner

import (
	"bytes"
	"context"
	"io"
	"os"
	"path/filepath"
	"testing"

	"be/internal/clock"
	"be/internal/db"
	"be/internal/model"
	"be/internal/repo"
)

// fakeArtifactStorage implements artifact.Storage in memory for tests.
type fakeArtifactStorage struct {
	data map[string][]byte
}

func newFakeArtifactStorage() *fakeArtifactStorage {
	return &fakeArtifactStorage{data: make(map[string][]byte)}
}

func (f *fakeArtifactStorage) Put(_ context.Context, key string, r io.Reader) error {
	b, err := io.ReadAll(r)
	if err != nil {
		return err
	}
	f.data[key] = b
	return nil
}

func (f *fakeArtifactStorage) Get(_ context.Context, key string) (io.ReadCloser, error) {
	b, ok := f.data[key]
	if !ok {
		return nil, os.ErrNotExist
	}
	return io.NopCloser(bytes.NewReader(b)), nil
}

func (f *fakeArtifactStorage) Delete(_ context.Context, key string) error {
	delete(f.data, key)
	return nil
}

// seedStageDB seeds project + workflow + workflow_instance for artifact_stage tests.
func seedStageDB(t *testing.T, pool *db.Pool, projID, wfiID string) {
	t.Helper()
	now := "2025-01-01T00:00:00Z"
	for _, q := range []struct {
		sql  string
		args []interface{}
	}{
		{`INSERT INTO projects (id, name, created_at, updated_at) VALUES (?, ?, ?, ?)`, []interface{}{projID, projID, now, now}},
		{`INSERT INTO workflows (project_id, id, description, scope_type, created_at, updated_at) VALUES (?, 'wf-stage', '', 'project', ?, ?)`, []interface{}{projID, now, now}},
		{`INSERT INTO workflow_instances (id, project_id, ticket_id, workflow_id, status, scope_type, findings, created_at, updated_at) VALUES (?, ?, '', 'wf-stage', 'active', 'project', '{}', ?, ?)`, []interface{}{wfiID, projID, now, now}},
	} {
		if _, err := pool.Exec(q.sql, q.args...); err != nil {
			t.Fatalf("seedStageDB(%s,%s): %v", projID, wfiID, err)
		}
	}
}

func TestEnsureStageDir_CreatesPath(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("NRFLO_HOME", dir)

	stageDir, err := EnsureStageDir("proj-cre", "wfi-cre")
	if err != nil {
		t.Fatalf("EnsureStageDir: %v", err)
	}
	if _, statErr := os.Stat(stageDir); statErr != nil {
		t.Errorf("stageDir not created: %v", statErr)
	}
	wantSuffix := filepath.Join("projects", "proj-cre", "artifacts", "wfi-cre")
	if !filepath.IsAbs(stageDir) {
		t.Errorf("stageDir not absolute: %q", stageDir)
	}
	rel, _ := filepath.Rel(dir, stageDir)
	if rel != wantSuffix {
		t.Errorf("stageDir relative = %q, want %q", rel, wantSuffix)
	}
}

func TestEnsureStageDir_Idempotent(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("NRFLO_HOME", dir)

	d1, err1 := EnsureStageDir("proj-idem", "wfi-idem")
	d2, err2 := EnsureStageDir("proj-idem", "wfi-idem")
	if err1 != nil || err2 != nil {
		t.Fatalf("EnsureStageDir twice errors: %v / %v", err1, err2)
	}
	if d1 != d2 {
		t.Errorf("paths differ: %q vs %q", d1, d2)
	}
}

func TestMaterialize_PlacesFile(t *testing.T) {
	t.Parallel()
	stageDir := t.TempDir()
	content := []byte("hello artifact")
	storage := newFakeArtifactStorage()
	storage.data["key/art"] = content

	a := &model.Artifact{ID: "a1", Name: "art.txt", PathKey: "key/art", SizeBytes: int64(len(content))}
	path, err := Materialize(context.Background(), a, stageDir, storage)
	if err != nil {
		t.Fatalf("Materialize: %v", err)
	}
	if filepath.Base(path) != "art.txt" {
		t.Errorf("path basename = %q, want art.txt", filepath.Base(path))
	}
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read materialized file: %v", err)
	}
	if !bytes.Equal(got, content) {
		t.Errorf("content = %q, want %q", got, content)
	}
}

func TestMaterialize_SkipsIfSizeMatches(t *testing.T) {
	t.Parallel()
	stageDir := t.TempDir()
	content := []byte("original")
	storage := newFakeArtifactStorage()
	storage.data["k"] = content

	a := &model.Artifact{Name: "f.txt", PathKey: "k", SizeBytes: int64(len(content))}
	path, _ := Materialize(context.Background(), a, stageDir, storage)

	// Corrupt storage so re-download gives different bytes; size unchanged
	altContent := bytes.Repeat([]byte("X"), len(content))
	storage.data["k"] = altContent

	path2, err := Materialize(context.Background(), a, stageDir, storage)
	if err != nil {
		t.Fatalf("second Materialize: %v", err)
	}
	if path != path2 {
		t.Errorf("paths differ: %q vs %q", path, path2)
	}
	got, _ := os.ReadFile(path2)
	if !bytes.Equal(got, content) {
		t.Errorf("file was re-downloaded unexpectedly")
	}
}

func TestMaterialize_RedownloadsOnSizeMismatch(t *testing.T) {
	t.Parallel()
	stageDir := t.TempDir()
	original := []byte("old")
	storage := newFakeArtifactStorage()
	storage.data["k"] = original

	a := &model.Artifact{Name: "f.txt", PathKey: "k", SizeBytes: int64(len(original))}
	Materialize(context.Background(), a, stageDir, storage) //nolint

	updated := []byte("updated-and-longer")
	storage.data["k"] = updated
	a.SizeBytes = int64(len(updated))

	path, err := Materialize(context.Background(), a, stageDir, storage)
	if err != nil {
		t.Fatalf("Materialize after size change: %v", err)
	}
	got, _ := os.ReadFile(path)
	if !bytes.Equal(got, updated) {
		t.Errorf("file not re-downloaded: got %q, want %q", got, updated)
	}
}

func TestMaterializeAll_EmptyArtifacts(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("NRFLO_HOME", dir)

	pool, err := db.NewPoolPath(filepath.Join(dir, "test.db"), db.DefaultPoolConfig())
	if err != nil {
		t.Fatalf("NewPoolPath: %v", err)
	}
	t.Cleanup(func() { pool.Close() })
	seedStageDB(t, pool, "p-empty", "wfi-empty")

	stageDir, err := MaterializeAll(context.Background(), "wfi-empty", "p-empty",
		repo.NewArtifactRepo(pool, clock.Real()), newFakeArtifactStorage())
	if err != nil {
		t.Fatalf("MaterializeAll: %v", err)
	}
	if stageDir == "" {
		t.Error("stageDir empty")
	}
	if _, statErr := os.Stat(stageDir); statErr != nil {
		t.Errorf("stageDir not created: %v", statErr)
	}
}

func TestMaterializeAll_BestEffortOnStorageError(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("NRFLO_HOME", dir)

	pool, err := db.NewPoolPath(filepath.Join(dir, "test.db"), db.DefaultPoolConfig())
	if err != nil {
		t.Fatalf("NewPoolPath: %v", err)
	}
	t.Cleanup(func() { pool.Close() })
	seedStageDB(t, pool, "p-berr", "wfi-berr")

	now := "2025-01-01T00:00:00Z"
	_, err = pool.Exec(`INSERT INTO artifacts (id, project_id, workflow_instance_id, name, type, path_key, size_bytes, source, created_at, updated_at)
		VALUES ('art-miss', 'p-berr', 'wfi-berr', 'missing.txt', 'internal', 'no/such/key', 10, 'agent', ?, ?)`, now, now)
	if err != nil {
		t.Fatalf("insert artifact: %v", err)
	}

	// Storage doesn't have the key — best-effort means no error returned.
	stageDir, err := MaterializeAll(context.Background(), "wfi-berr", "p-berr",
		repo.NewArtifactRepo(pool, clock.Real()), newFakeArtifactStorage())
	if err != nil {
		t.Fatalf("MaterializeAll best-effort returned error: %v", err)
	}
	if stageDir == "" {
		t.Error("stageDir empty")
	}
}
