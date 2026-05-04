package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"be/internal/ws"
)

const testConfigManifestYAML = `tools:
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

// configTestEnv holds a server + configDir + projectID for config handler tests.
type configTestEnv struct {
	env       *reviewTestEnv
	configDir string
	projectID string
}

func newConfigTestEnv(t *testing.T) *configTestEnv {
	t.Helper()
	env := newReviewTestEnv(t)
	const pid = "proj-cfg"
	seedReviewProject(t, env.s.pool, pid)

	configDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(configDir, "tool_manifest.yaml"),
		[]byte(testConfigManifestYAML), 0644); err != nil {
		t.Fatalf("write manifest: %v", err)
	}
	// Write a disk file so list/get can find catalog.yaml
	if err := os.WriteFile(filepath.Join(configDir, "catalog.yaml"),
		[]byte("sku: DISK-001\n"), 0644); err != nil {
		t.Fatalf("write catalog.yaml: %v", err)
	}

	_, err := env.s.pool.Exec(
		`INSERT OR REPLACE INTO config (project_id, key, value) VALUES (?, 'customer_config_dir', ?)`,
		pid, configDir)
	if err != nil {
		t.Fatalf("set customer_config_dir: %v", err)
	}
	return &configTestEnv{env: env, configDir: configDir, projectID: pid}
}

// --- Missing customer_config_dir ---

func TestHandleListConfigFiles_NoConfigDir(t *testing.T) {
	env := newReviewTestEnv(t)
	const pid = "proj-no-dir"
	seedReviewProject(t, env.s.pool, pid)
	// customer_config_dir intentionally not set
	req := httptest.NewRequest(http.MethodGet, withProject("/api/v1/config-files", pid), nil)
	rr := httptest.NewRecorder()
	env.s.handleListConfigFiles(rr, req)
	if rr.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", rr.Code)
	}
	assertErrorContains(t, rr, "customer_config_dir")
}

func TestHandleListConfigFiles_MissingProject(t *testing.T) {
	env := newReviewTestEnv(t)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/config-files", nil)
	rr := httptest.NewRecorder()
	env.s.handleListConfigFiles(rr, req)
	if rr.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", rr.Code)
	}
	assertErrorContains(t, rr, "X-Project")
}

// --- List files ---

func TestHandleListConfigFiles_ReturnsFiles(t *testing.T) {
	cenv := newConfigTestEnv(t)

	req := httptest.NewRequest(http.MethodGet, withProject("/api/v1/config-files", cenv.projectID), nil)
	rr := httptest.NewRecorder()
	cenv.env.s.handleListConfigFiles(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body: %s", rr.Code, rr.Body.String())
	}

	var files []map[string]interface{}
	json.NewDecoder(rr.Body).Decode(&files)
	paths := make(map[string]bool)
	for _, f := range files {
		if p, ok := f["Path"].(string); ok {
			paths[p] = true
		}
	}
	for _, want := range []string{"tool_manifest.yaml", "catalog.yaml"} {
		if !paths[want] {
			t.Errorf("list missing %q; got paths: %v", want, paths)
		}
	}
}

// --- GET content + version ---

func TestHandleGetConfigFile_DBVersionTakesPrecedence(t *testing.T) {
	cenv := newConfigTestEnv(t)
	s := cenv.env.s

	// PUT a DB version
	putReq := httptest.NewRequest(http.MethodPut,
		withProject("/api/v1/config-files/content/catalog.yaml", cenv.projectID),
		strings.NewReader("sku: DB-001\n"))
	putReq.SetPathValue("file", "catalog.yaml")
	putRR := httptest.NewRecorder()
	s.handlePutConfigFile(putRR, putReq)
	if putRR.Code != http.StatusOK {
		t.Fatalf("PUT status = %d; body: %s", putRR.Code, putRR.Body.String())
	}

	// GET should return DB content (version 1), not the disk "DISK-001"
	req := httptest.NewRequest(http.MethodGet,
		withProject("/api/v1/config-files/content/catalog.yaml", cenv.projectID), nil)
	req.SetPathValue("file", "catalog.yaml")
	rr := httptest.NewRecorder()
	s.handleGetConfigFile(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("GET status = %d; body: %s", rr.Code, rr.Body.String())
	}
	var resp map[string]interface{}
	json.NewDecoder(rr.Body).Decode(&resp)
	if !strings.Contains(resp["content"].(string), "DB-001") {
		t.Errorf("content = %q, want DB content with DB-001", resp["content"])
	}
	if int(resp["version"].(float64)) != 1 {
		t.Errorf("version = %v, want 1", resp["version"])
	}
}

// --- PUT auto-bumps version ---

func TestHandlePutConfigFile_AutoBumpsVersion(t *testing.T) {
	cenv := newConfigTestEnv(t)
	s := cenv.env.s

	put := func(content string) int {
		req := httptest.NewRequest(http.MethodPut,
			withProject("/api/v1/config-files/content/catalog.yaml", cenv.projectID),
			strings.NewReader(content))
		req.SetPathValue("file", "catalog.yaml")
		rr := httptest.NewRecorder()
		s.handlePutConfigFile(rr, req)
		if rr.Code != http.StatusOK {
			t.Fatalf("PUT status = %d; body: %s", rr.Code, rr.Body.String())
		}
		var resp map[string]interface{}
		json.NewDecoder(rr.Body).Decode(&resp)
		return int(resp["version"].(float64))
	}

	v1 := put("sku: V1\n")
	v2 := put("sku: V2\n")
	if v1 != 1 {
		t.Errorf("first PUT version = %d, want 1", v1)
	}
	if v2 != 2 {
		t.Errorf("second PUT version = %d, want 2", v2)
	}
}

// --- PUT schema validation failure ---

func TestHandlePutConfigFile_SchemaValidationFailure(t *testing.T) {
	cenv := newConfigTestEnv(t)
	// Write a sidecar schema requiring "items" array
	schema := `{"type":"object","properties":{"items":{"type":"array"}},"required":["items"]}`
	os.WriteFile(filepath.Join(cenv.configDir, "catalog.schema.json"), []byte(schema), 0644) //nolint

	req := httptest.NewRequest(http.MethodPut,
		withProject("/api/v1/config-files/content/catalog.yaml", cenv.projectID),
		strings.NewReader("sku: missing-items\n"))
	req.SetPathValue("file", "catalog.yaml")
	rr := httptest.NewRecorder()
	cenv.env.s.handlePutConfigFile(rr, req)
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("PUT invalid schema status = %d, want 400; body: %s", rr.Code, rr.Body.String())
	}
	var resp map[string]interface{}
	json.NewDecoder(rr.Body).Decode(&resp)
	if _, ok := resp["error"]; !ok {
		t.Error("response missing 'error' field")
	}
}

// --- Rollback broadcasts event ---

func TestHandleRollbackConfig_BroadcastsEvent(t *testing.T) {
	cenv := newConfigTestEnv(t)
	s := cenv.env.s
	pid := cenv.projectID

	// Create two versions
	for _, c := range []string{"v1 content\n", "v2 content\n"} {
		req := httptest.NewRequest(http.MethodPut,
			withProject("/api/v1/config-files/content/catalog.yaml", pid),
			strings.NewReader(c))
		req.SetPathValue("file", "catalog.yaml")
		rr := httptest.NewRecorder()
		s.handlePutConfigFile(rr, req)
		if rr.Code != http.StatusOK {
			t.Fatalf("PUT status = %d", rr.Code)
		}
	}

	rollReq := httptest.NewRequest(http.MethodPost,
		withProject("/api/v1/config-files/rollback/catalog.yaml", pid),
		strings.NewReader(`{"version":1}`))
	rollReq.SetPathValue("file", "catalog.yaml")
	rollRR := httptest.NewRecorder()
	s.handleRollbackConfig(rollRR, rollReq)
	if rollRR.Code != http.StatusOK {
		t.Fatalf("rollback status = %d; body: %s", rollRR.Code, rollRR.Body.String())
	}
	var resp map[string]interface{}
	json.NewDecoder(rollRR.Body).Decode(&resp)
	if int(resp["version"].(float64)) != 3 {
		t.Errorf("after rollback version = %v, want 3", resp["version"])
	}

	ev := cenv.env.rec.waitEvent(t, ws.EventConfigFileUpdated)
	if ev.Data["file"] != "catalog.yaml" {
		t.Errorf("event file = %v, want catalog.yaml", ev.Data["file"])
	}
}

// --- Path traversal ---

func TestHandleGetConfigFile_PathTraversal(t *testing.T) {
	cenv := newConfigTestEnv(t)
	s := cenv.env.s
	pid := cenv.projectID

	badPaths := []string{"../secret", "/etc/passwd"}
	for _, bad := range badPaths {
		req := httptest.NewRequest(http.MethodGet,
			withProject("/api/v1/config-files/content/"+bad, pid), nil)
		req.SetPathValue("file", bad)
		rr := httptest.NewRecorder()
		s.handleGetConfigFile(rr, req)
		if rr.Code != http.StatusBadRequest {
			t.Errorf("GET %q status = %d, want 400", bad, rr.Code)
		}
	}
}

func TestHandlePutConfigFile_PathTraversal(t *testing.T) {
	cenv := newConfigTestEnv(t)
	s := cenv.env.s
	pid := cenv.projectID

	req := httptest.NewRequest(http.MethodPut,
		withProject("/api/v1/config-files/content/../escape", pid),
		strings.NewReader("x: 1\n"))
	req.SetPathValue("file", "../escape")
	rr := httptest.NewRecorder()
	s.handlePutConfigFile(rr, req)
	if rr.Code != http.StatusBadRequest {
		t.Errorf("PUT traversal status = %d, want 400", rr.Code)
	}
}
