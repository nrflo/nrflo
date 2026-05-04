package configeditor_test

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"be/internal/clock"
	"be/internal/db"
	"be/internal/manifest/config"
	"be/internal/configeditor"
	"be/internal/repo"
)

const testManifestYAML = `
tools:
  - name: lookup_sku
    type: python_script
    description: Look up SKU
    script: tools/lookup_sku.py
    input_schema:
      type: object
      properties:
        sku: {type: string}
      required: [sku]
    config_files:
      - path: catalog.yaml
`

func setupConfigEditor(t *testing.T) (*configeditor.Service, string, string) {
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

	configDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(configDir, "tool_manifest.yaml"), []byte(testManifestYAML), 0644); err != nil {
		t.Fatalf("write manifest: %v", err)
	}

	manifest, err := config.Load(configDir)
	if err != nil {
		t.Fatalf("config.Load: %v", err)
	}

	clk := clock.NewTest(time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC))
	r := repo.NewConfigVersionRepo(database.DB, clk)
	svc := configeditor.NewService(configDir, manifest, r, clk)

	return svc, configDir, "proj-1"
}

func TestService_Put_NewFile(t *testing.T) {
	svc, _, projectID := setupConfigEditor(t)

	content := []byte("sku: ABC-123\n")
	if err := svc.Put(projectID, "catalog.yaml", "alice", content); err != nil {
		t.Fatalf("Put: %v", err)
	}

	got, err := svc.Get(projectID, "catalog.yaml")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if string(got) != string(content) {
		t.Errorf("Get = %q, want %q", got, content)
	}
}

func TestService_Put_AutoVersions(t *testing.T) {
	svc, _, projectID := setupConfigEditor(t)

	for i, c := range []string{"v1 content", "v2 content"} {
		if err := svc.Put(projectID, "catalog.yaml", "actor", []byte(c)); err != nil {
			t.Fatalf("Put %d: %v", i+1, err)
		}
	}

	hist, err := svc.History(projectID, "catalog.yaml")
	if err != nil {
		t.Fatalf("History: %v", err)
	}
	if len(hist) != 2 {
		t.Fatalf("History len = %d, want 2", len(hist))
	}
	if hist[0].Version != 2 {
		t.Errorf("hist[0].Version = %d, want 2 (newest first)", hist[0].Version)
	}
	if hist[1].Version != 1 {
		t.Errorf("hist[1].Version = %d, want 1", hist[1].Version)
	}
}

func TestService_Get_DiskFallback(t *testing.T) {
	svc, configDir, projectID := setupConfigEditor(t)

	diskContent := []byte("disk content\n")
	if err := os.WriteFile(filepath.Join(configDir, "catalog.yaml"), diskContent, 0644); err != nil {
		t.Fatalf("write disk file: %v", err)
	}

	got, err := svc.Get(projectID, "catalog.yaml")
	if err != nil {
		t.Fatalf("Get disk fallback: %v", err)
	}
	if string(got) != string(diskContent) {
		t.Errorf("Get disk = %q, want %q", got, diskContent)
	}
}

func TestService_Get_DBFirst(t *testing.T) {
	svc, configDir, projectID := setupConfigEditor(t)

	if err := os.WriteFile(filepath.Join(configDir, "catalog.yaml"), []byte("disk"), 0644); err != nil {
		t.Fatalf("write disk: %v", err)
	}

	dbContent := []byte("db content")
	if err := svc.Put(projectID, "catalog.yaml", "actor", dbContent); err != nil {
		t.Fatalf("Put: %v", err)
	}

	got, err := svc.Get(projectID, "catalog.yaml")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if string(got) != string(dbContent) {
		t.Errorf("Get = %q, want DB content %q", got, dbContent)
	}
}

func TestService_History_NewestFirst(t *testing.T) {
	svc, _, projectID := setupConfigEditor(t)

	for _, c := range []string{"v1", "v2", "v3"} {
		if err := svc.Put(projectID, "catalog.yaml", "actor", []byte(c)); err != nil {
			t.Fatalf("Put %q: %v", c, err)
		}
	}

	hist, err := svc.History(projectID, "catalog.yaml")
	if err != nil {
		t.Fatalf("History: %v", err)
	}
	if len(hist) != 3 {
		t.Fatalf("History len = %d, want 3", len(hist))
	}
	for i, want := range []string{"v3", "v2", "v1"} {
		if string(hist[i].Content) != want {
			t.Errorf("hist[%d] = %q, want %q", i, hist[i].Content, want)
		}
	}
}

func TestService_Rollback_AppendsVersion(t *testing.T) {
	svc, _, projectID := setupConfigEditor(t)

	svc.Put(projectID, "catalog.yaml", "actor", []byte("v1")) //nolint
	svc.Put(projectID, "catalog.yaml", "actor", []byte("v2")) //nolint

	if err := svc.Rollback(projectID, "catalog.yaml", "alice", 1); err != nil {
		t.Fatalf("Rollback: %v", err)
	}

	hist, err := svc.History(projectID, "catalog.yaml")
	if err != nil {
		t.Fatalf("History after rollback: %v", err)
	}
	if len(hist) != 3 {
		t.Fatalf("History len = %d, want 3 after rollback", len(hist))
	}
	// Newest version should have content of v1
	if string(hist[0].Content) != "v1" {
		t.Errorf("After rollback: hist[0] = %q, want 'v1'", hist[0].Content)
	}
	if hist[0].Version != 3 {
		t.Errorf("After rollback: hist[0].Version = %d, want 3", hist[0].Version)
	}
}

func TestService_PathTraversal_Rejected(t *testing.T) {
	svc, _, projectID := setupConfigEditor(t)

	badPaths := []string{"", "../secret.yaml", "/absolute.yaml"}
	for _, path := range badPaths {
		if err := svc.Put(projectID, path, "actor", []byte("x")); err == nil {
			t.Errorf("Put(%q): expected error, got nil", path)
		}
		if _, err := svc.Get(projectID, path); err == nil {
			t.Errorf("Get(%q): expected error, got nil", path)
		}
		if _, err := svc.History(projectID, path); err == nil {
			t.Errorf("History(%q): expected error, got nil", path)
		}
		if err := svc.Rollback(projectID, path, "actor", 1); err == nil {
			t.Errorf("Rollback(%q): expected error, got nil", path)
		}
	}
}

func TestService_Put_SchemaValidation_WithSidecar(t *testing.T) {
	svc, configDir, projectID := setupConfigEditor(t)

	schema := `{"type":"object","properties":{"items":{"type":"array"}},"required":["items"]}`
	os.WriteFile(filepath.Join(configDir, "catalog.schema.json"), []byte(schema), 0644) //nolint

	// Valid content
	if err := svc.Put(projectID, "catalog.yaml", "actor", []byte("items:\n  - a\n  - b\n")); err != nil {
		t.Errorf("Put valid with sidecar: %v", err)
	}

	// Invalid content (missing required 'items')
	if err := svc.Put(projectID, "catalog.yaml", "actor", []byte("other: value\n")); err == nil {
		t.Error("Put invalid schema: expected error, got nil")
	}
}

func TestService_Put_NoSidecarAcceptsAnything(t *testing.T) {
	svc, _, projectID := setupConfigEditor(t)

	if err := svc.Put(projectID, "catalog.yaml", "actor", []byte("anything: goes\n")); err != nil {
		t.Errorf("Put without sidecar: %v", err)
	}
}

func TestService_List_ContainsExpectedFiles(t *testing.T) {
	svc, configDir, projectID := setupConfigEditor(t)

	os.WriteFile(filepath.Join(configDir, "extra.yaml"), []byte("x: 1\n"), 0644) //nolint

	files, err := svc.List(projectID)
	if err != nil {
		t.Fatalf("List: %v", err)
	}

	paths := make(map[string]bool)
	for _, f := range files {
		paths[f.Path] = true
	}

	for _, want := range []string{"tool_manifest.yaml", "catalog.yaml", "extra.yaml"} {
		if !paths[want] {
			t.Errorf("List missing %q; got paths: %v", want, paths)
		}
	}
}

func TestService_List_Deduplicates(t *testing.T) {
	svc, _, projectID := setupConfigEditor(t)

	files, err := svc.List(projectID)
	if err != nil {
		t.Fatalf("List: %v", err)
	}

	seen := make(map[string]int)
	for _, f := range files {
		seen[f.Path]++
	}
	for path, count := range seen {
		if count > 1 {
			t.Errorf("List has duplicate path %q (count=%d)", path, count)
		}
	}
}

func TestService_List_LatestVersion(t *testing.T) {
	svc, _, projectID := setupConfigEditor(t)

	svc.Put(projectID, "catalog.yaml", "actor", []byte("v1")) //nolint
	svc.Put(projectID, "catalog.yaml", "actor", []byte("v2")) //nolint

	files, _ := svc.List(projectID)
	for _, f := range files {
		if f.Path == "catalog.yaml" {
			if f.LatestVersion != 2 {
				t.Errorf("catalog.yaml LatestVersion = %d, want 2", f.LatestVersion)
			}
			return
		}
	}
	t.Error("catalog.yaml not found in List")
}

func TestService_List_SchemaAutoDetect(t *testing.T) {
	svc, configDir, projectID := setupConfigEditor(t)

	os.WriteFile(filepath.Join(configDir, "catalog.schema.json"), []byte(`{}`), 0644) //nolint

	files, _ := svc.List(projectID)
	for _, f := range files {
		if f.Path == "catalog.yaml" {
			if f.SchemaPath == "" {
				t.Error("catalog.yaml SchemaPath not auto-detected")
			}
			return
		}
	}
	t.Error("catalog.yaml not found in List")
}
