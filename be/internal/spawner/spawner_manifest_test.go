package spawner

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

const testManifestYAML = `
tools:
  - name: sample_tool
    type: python_script
    description: A sample tool
    script: tools/sample.py
    input_schema:
      type: object
      properties:
        input:
          type: string
      required: [input]
`

// newManifestDir writes a tool_manifest.yaml to a temp dir and returns the dir.
func newManifestDir(t *testing.T, content string) string {
	t.Helper()
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "tool_manifest.yaml"), []byte(content), 0644); err != nil {
		t.Fatalf("write manifest: %v", err)
	}
	return dir
}

func newTestSpawner() *Spawner {
	return New(Config{})
}

func TestLoadManifestCached_HappyPath(t *testing.T) {
	t.Parallel()
	dir := newManifestDir(t, testManifestYAML)
	s := newTestSpawner()

	m, err := s.loadManifestCached(dir)
	if err != nil {
		t.Fatalf("loadManifestCached: %v", err)
	}
	if m == nil {
		t.Fatalf("manifest is nil")
	}

	tool, ok := m.Tool("sample_tool")
	if !ok {
		t.Errorf("sample_tool not found in manifest")
	}
	if tool.Description != "A sample tool" {
		t.Errorf("description = %q, want 'A sample tool'", tool.Description)
	}
}

func TestLoadManifestCached_MissingFile_ReturnsNilNil(t *testing.T) {
	t.Parallel()
	dir := t.TempDir() // empty dir — no tool_manifest.yaml
	s := newTestSpawner()

	m, err := s.loadManifestCached(dir)
	if err != nil {
		t.Fatalf("loadManifestCached: %v", err)
	}
	if m != nil {
		t.Errorf("manifest = %v, want nil for missing file", m)
	}
}

func TestLoadManifestCached_CacheHitOnUnchangedMtime(t *testing.T) {
	t.Parallel()
	dir := newManifestDir(t, testManifestYAML)
	s := newTestSpawner()

	// First load populates the cache.
	m1, err := s.loadManifestCached(dir)
	if err != nil || m1 == nil {
		t.Fatalf("first load: err=%v manifest=%v", err, m1)
	}

	// Second load with unchanged mtime must return the same pointer.
	m2, err := s.loadManifestCached(dir)
	if err != nil {
		t.Fatalf("second load: %v", err)
	}
	if m2 != m1 {
		t.Errorf("cache miss on unchanged mtime: got different *Manifest pointer")
	}
}

func TestLoadManifestCached_CacheMissOnMtimeChange(t *testing.T) {
	t.Parallel()
	dir := newManifestDir(t, testManifestYAML)
	s := newTestSpawner()

	m1, err := s.loadManifestCached(dir)
	if err != nil || m1 == nil {
		t.Fatalf("first load: err=%v", err)
	}

	// Artificially advance the mtime by setting it to a future time.
	future := time.Now().Add(2 * time.Second)
	manifestPath := filepath.Join(dir, "tool_manifest.yaml")
	if err := os.Chtimes(manifestPath, future, future); err != nil {
		t.Skipf("cannot change mtime: %v", err)
	}

	m2, err := s.loadManifestCached(dir)
	if err != nil {
		t.Fatalf("second load after mtime change: %v", err)
	}
	// Different pointer expected (reload).
	if m2 == m1 {
		t.Errorf("expected cache miss after mtime change, got same pointer")
	}
}

func TestLoadManifestCached_InvalidManifest_ReturnsError(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	// Write invalid YAML to trigger a parse/validation error.
	invalid := []byte("tools:\n  - name: bad\n    type: builtin\n    description: x\n    input_schema: {type: object}\n")
	if err := os.WriteFile(filepath.Join(dir, "tool_manifest.yaml"), invalid, 0644); err != nil {
		t.Fatalf("write: %v", err)
	}
	s := newTestSpawner()
	_, err := s.loadManifestCached(dir)
	if err == nil {
		t.Fatalf("expected error for invalid manifest, got nil")
	}
}

func TestLoadManifestCached_MultipleTools(t *testing.T) {
	t.Parallel()
	multiYAML := `
tools:
  - name: tool_a
    type: python_script
    description: Tool A
    script: a.py
    input_schema:
      type: object
  - name: tool_b
    type: python_script
    description: Tool B
    script: b.py
    input_schema:
      type: object
`
	dir := newManifestDir(t, multiYAML)
	s := newTestSpawner()

	m, err := s.loadManifestCached(dir)
	if err != nil {
		t.Fatalf("loadManifestCached: %v", err)
	}
	if len(m.Tools) != 2 {
		t.Errorf("tools count = %d, want 2", len(m.Tools))
	}
	if _, ok := m.Tool("tool_a"); !ok {
		t.Errorf("tool_a not found")
	}
	if _, ok := m.Tool("tool_b"); !ok {
		t.Errorf("tool_b not found")
	}
}

// TestLoadManifestCached_SetsDir verifies that config.Load sets Dir correctly.
func TestLoadManifestCached_SetsDir(t *testing.T) {
	t.Parallel()
	dir := newManifestDir(t, testManifestYAML)
	s := newTestSpawner()

	m, err := s.loadManifestCached(dir)
	if err != nil {
		t.Fatalf("loadManifestCached: %v", err)
	}
	if m.Dir != dir {
		t.Errorf("manifest.Dir = %q, want %q", m.Dir, dir)
	}
}
