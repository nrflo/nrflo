package spawner

import (
	"context"
	"path/filepath"
	"strings"
	"testing"

	"be/internal/clock"
	"be/internal/service"

	"github.com/google/uuid"
)

// insertArtifactRow inserts an artifact row directly into the test DB.
func insertArtifactRow(t *testing.T, env *spawnerTestEnv, artID, wfiID, name, pathKey string, size int64) {
	t.Helper()
	now := "2025-01-01T00:00:00Z"
	_, err := env.pool.Exec(`
		INSERT INTO artifacts (id, project_id, workflow_instance_id, name, type, path_key, size_bytes, source, created_at, updated_at)
		VALUES (?, ?, ?, ?, 'internal', ?, ?, 'agent', ?, ?)`,
		artID, env.project, wfiID, name, pathKey, size, now, now)
	if err != nil {
		t.Fatalf("insertArtifactRow(%s): %v", name, err)
	}
}

func TestExpandArtifacts_NoPattern_PassThrough(t *testing.T) {
	t.Parallel()
	env := newSpawnerTestEnv(t)
	sp := env.newSpawner()
	in := "no artifact patterns here"
	got, err := sp.expandArtifacts(in, env.project, "wfi-1")
	if err != nil {
		t.Fatalf("expandArtifacts: %v", err)
	}
	if got != in {
		t.Errorf("got %q, want unchanged %q", got, in)
	}
}

func TestExpandArtifacts_EmptyWFI_Placeholder(t *testing.T) {
	t.Parallel()
	env := newSpawnerTestEnv(t)
	sp := env.newSpawner()

	got, err := sp.expandArtifacts("#{ARTIFACTS}", env.project, "")
	if err != nil {
		t.Fatalf("expandArtifacts: %v", err)
	}
	if !strings.Contains(got, "_No artifacts available") {
		t.Errorf("expected placeholder for empty wfiID, got: %q", got)
	}
	if strings.Contains(got, "#{ARTIFACTS}") {
		t.Error("pattern not consumed")
	}
}

func TestExpandArtifacts_EmptyWFI_NamedPatternEmpty(t *testing.T) {
	t.Parallel()
	env := newSpawnerTestEnv(t)
	sp := env.newSpawner()

	got, err := sp.expandArtifacts("prefix #{ARTIFACT:foo} suffix", env.project, "")
	if err != nil {
		t.Fatalf("expandArtifacts: %v", err)
	}
	if strings.Contains(got, "#{ARTIFACT:foo}") {
		t.Error("named pattern not consumed")
	}
	// Named pattern expands to empty when wfiID is empty
	if got != "prefix  suffix" {
		t.Errorf("got %q, want \"prefix  suffix\"", got)
	}
}

func TestExpandArtifacts_NoArtifacts_Placeholder(t *testing.T) {
	t.Parallel()
	env := newSpawnerTestEnv(t)
	ticketID := "ART-" + uuid.New().String()[:6]
	wfiID := env.initWorkflow(t, ticketID)
	sp := env.newSpawner()

	// No artifact rows inserted; expect placeholder
	got, err := sp.expandArtifacts("#{ARTIFACTS}", env.project, wfiID)
	if err != nil {
		t.Fatalf("expandArtifacts: %v", err)
	}
	if !strings.Contains(got, "_No artifacts available") {
		t.Errorf("expected placeholder for empty artifact list, got: %q", got)
	}
}

func TestExpandArtifacts_FullListNotMaterialized(t *testing.T) {
	t.Parallel()
	env := newSpawnerTestEnv(t)
	ticketID := "ART-" + uuid.New().String()[:6]
	wfiID := env.initWorkflow(t, ticketID)

	// Insert two artifact rows; no files in storage → not materialized paths
	insertArtifactRow(t, env, "id-b", wfiID, "b.txt", "wfi/id-b__b.txt", 5)
	insertArtifactRow(t, env, "id-a", wfiID, "a.txt", "wfi/id-a__a.txt", 3)

	sp := env.newSpawner() // no ArtifactSvc → nameToPath stays empty
	got, err := sp.expandArtifacts("#{ARTIFACTS}", env.project, wfiID)
	if err != nil {
		t.Fatalf("expandArtifacts: %v", err)
	}
	if !strings.Contains(got, "b.txt\t") {
		t.Errorf("b.txt not in output: %q", got)
	}
	if !strings.Contains(got, "a.txt\t") {
		t.Errorf("a.txt not in output: %q", got)
	}
	// Tab-separated format (name\tpath)
	lines := strings.Split(got, "\n")
	for _, l := range lines {
		if l == "" {
			continue
		}
		if !strings.Contains(l, "\t") {
			t.Errorf("line %q is not tab-separated", l)
		}
	}
}

func TestExpandArtifacts_WithMaterialization(t *testing.T) {
	// Cannot parallelize: sets NRFLO_HOME.
	dir := t.TempDir()
	t.Setenv("NRFLO_HOME", dir)

	env := newSpawnerTestEnv(t)
	ticketID := "ART-" + uuid.New().String()[:6]
	wfiID := env.initWorkflow(t, ticketID)

	dataPath := filepath.Join(dir, "nrflo.data")
	var hub service.WSHub // nil hub; BroadcastFromCtx is nil-safe
	artSvc := service.NewArtifactService(env.pool, clock.Real(), hub, dataPath)

	ctx := context.Background()
	_, err := artSvc.AddFromAgent(ctx, "sess1", env.project, wfiID, "report.txt", "text/plain", []byte("report content"))
	if err != nil {
		t.Fatalf("AddFromAgent: %v", err)
	}

	sp := &Spawner{
		config: Config{
			DataPath:    env.dbPath,
			Pool:        env.pool,
			ArtifactSvc: artSvc,
			Clock:       clock.Real(),
		},
	}

	got, err := sp.expandArtifacts("#{ARTIFACTS}", env.project, wfiID)
	if err != nil {
		t.Fatalf("expandArtifacts: %v", err)
	}
	if !strings.Contains(got, "report.txt\t") {
		t.Errorf("report.txt not in output: %q", got)
	}
	// Path must not be [not materialized:...]
	if strings.Contains(got, "[not materialized:") {
		t.Errorf("artifact not materialized: %q", got)
	}
}

func TestExpandArtifacts_KnownNamePath(t *testing.T) {
	// Cannot parallelize: sets NRFLO_HOME.
	dir := t.TempDir()
	t.Setenv("NRFLO_HOME", dir)

	env := newSpawnerTestEnv(t)
	ticketID := "ART-" + uuid.New().String()[:6]
	wfiID := env.initWorkflow(t, ticketID)

	dataPath := filepath.Join(dir, "nrflo.data")
	var hub service.WSHub
	artSvc := service.NewArtifactService(env.pool, clock.Real(), hub, dataPath)

	ctx := context.Background()
	_, err := artSvc.AddFromAgent(ctx, "sess", env.project, wfiID, "data.csv", "text/csv", []byte("a,b,c"))
	if err != nil {
		t.Fatalf("AddFromAgent: %v", err)
	}

	sp := &Spawner{
		config: Config{
			DataPath:    env.dbPath,
			Pool:        env.pool,
			ArtifactSvc: artSvc,
			Clock:       clock.Real(),
		},
	}

	got, err := sp.expandArtifacts("path=#{ARTIFACT:data.csv}", env.project, wfiID)
	if err != nil {
		t.Fatalf("expandArtifacts: %v", err)
	}
	if !strings.HasPrefix(got, "path=") {
		t.Errorf("prefix missing: %q", got)
	}
	expanded := strings.TrimPrefix(got, "path=")
	if expanded == "" {
		t.Error("#{ARTIFACT:data.csv} expanded to empty string")
	}
	if strings.Contains(got, "#{ARTIFACT:") {
		t.Error("pattern not consumed")
	}
}

func TestExpandArtifacts_UnknownName_EmptyNoPanic(t *testing.T) {
	t.Parallel()
	env := newSpawnerTestEnv(t)
	ticketID := "ART-" + uuid.New().String()[:6]
	wfiID := env.initWorkflow(t, ticketID)

	sp := env.newSpawner()
	got, err := sp.expandArtifacts("path=#{ARTIFACT:nonexistent}", env.project, wfiID)
	if err != nil {
		t.Fatalf("expandArtifacts: %v", err)
	}
	// Unknown name → empty string expansion, no panic
	if strings.Contains(got, "#{ARTIFACT:") {
		t.Error("pattern not consumed")
	}
	// Should expand to empty, leaving "path="
	if got != "path=" {
		t.Errorf("got %q, want %q", got, "path=")
	}
}

// Verify expandArtifacts also uses repo.ArtifactRepo via pool()
func TestExpandArtifacts_PoolNil_ReturnsError(t *testing.T) {
	t.Parallel()
	sp := &Spawner{
		config: Config{
			// No Pool set → pool() returns nil → expandArtifacts returns error
			Clock: clock.Real(),
		},
	}
	_, err := sp.expandArtifacts("#{ARTIFACTS}", "proj", "wfi")
	if err == nil {
		t.Error("expected error when pool is nil")
	}
}
