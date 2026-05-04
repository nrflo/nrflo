package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
)

func TestHandleGetProject_CustomerConfigDir_DefaultNull(t *testing.T) {
	s := newProjectsServer(t)
	seedTestProject(t, s, "cfg-dir-default")

	rr := httptest.NewRecorder()
	s.handleGetProject(rr, buildGetProjectReq(t, "cfg-dir-default"))

	if rr.Code != http.StatusOK {
		t.Fatalf("GET status = %d, want 200; body: %s", rr.Code, rr.Body.String())
	}

	resp := decodeProjectResp(t, rr)
	raw, ok := resp["customer_config_dir"]
	if !ok {
		t.Fatal("customer_config_dir field absent from GET response")
	}
	if raw != nil {
		t.Errorf("customer_config_dir = %v, want nil for new project", raw)
	}
}

func TestHandlePatchProject_CustomerConfigDir_Valid(t *testing.T) {
	s := newProjectsServer(t)
	seedTestProject(t, s, "cfg-dir-valid")

	dir := t.TempDir()

	body, _ := json.Marshal(map[string]interface{}{"customer_config_dir": dir})
	rr := httptest.NewRecorder()
	s.handleUpdateProject(rr, buildPatchProjectReq(t, "cfg-dir-valid", string(body)))
	if rr.Code != http.StatusOK {
		t.Fatalf("PATCH status = %d, want 200; body: %s", rr.Code, rr.Body.String())
	}

	patchResp := decodeProjectResp(t, rr)
	if val, _ := patchResp["customer_config_dir"].(string); val != dir {
		t.Errorf("PATCH response customer_config_dir = %q, want %q", val, dir)
	}

	// GET should also return the stored path
	rrGet := httptest.NewRecorder()
	s.handleGetProject(rrGet, buildGetProjectReq(t, "cfg-dir-valid"))
	getResp := decodeProjectResp(t, rrGet)
	if val, _ := getResp["customer_config_dir"].(string); val != dir {
		t.Errorf("GET customer_config_dir = %q, want %q", val, dir)
	}
}

func TestHandlePatchProject_CustomerConfigDir_NonExistent(t *testing.T) {
	s := newProjectsServer(t)
	seedTestProject(t, s, "cfg-dir-nonexist")

	body, _ := json.Marshal(map[string]interface{}{"customer_config_dir": "/definitely/does/not/exist/xyz"})
	rr := httptest.NewRecorder()
	s.handleUpdateProject(rr, buildPatchProjectReq(t, "cfg-dir-nonexist", string(body)))
	if rr.Code != http.StatusBadRequest {
		t.Errorf("PATCH non-existent dir: status = %d, want 400; body: %s", rr.Code, rr.Body.String())
	}
}

func TestHandlePatchProject_CustomerConfigDir_NotAbsolute(t *testing.T) {
	s := newProjectsServer(t)
	seedTestProject(t, s, "cfg-dir-rel")

	body, _ := json.Marshal(map[string]interface{}{"customer_config_dir": "relative/path/to/dir"})
	rr := httptest.NewRecorder()
	s.handleUpdateProject(rr, buildPatchProjectReq(t, "cfg-dir-rel", string(body)))
	if rr.Code != http.StatusBadRequest {
		t.Errorf("PATCH relative path: status = %d, want 400; body: %s", rr.Code, rr.Body.String())
	}
}

func TestHandlePatchProject_CustomerConfigDir_NotADirectory(t *testing.T) {
	s := newProjectsServer(t)
	seedTestProject(t, s, "cfg-dir-file")

	// Create a regular file (not a directory)
	tmpFile, err := os.CreateTemp(t.TempDir(), "testfile-*.txt")
	if err != nil {
		t.Fatalf("create temp file: %v", err)
	}
	tmpFile.Close()

	body, _ := json.Marshal(map[string]interface{}{"customer_config_dir": tmpFile.Name()})
	rr := httptest.NewRecorder()
	s.handleUpdateProject(rr, buildPatchProjectReq(t, "cfg-dir-file", string(body)))
	if rr.Code != http.StatusBadRequest {
		t.Errorf("PATCH file (not dir): status = %d, want 400; body: %s", rr.Code, rr.Body.String())
	}
}

func TestHandlePatchProject_CustomerConfigDir_ClearedByEmpty(t *testing.T) {
	s := newProjectsServer(t)
	seedTestProject(t, s, "cfg-dir-clear")

	// Set a valid directory first
	dir := t.TempDir()
	body, _ := json.Marshal(map[string]interface{}{"customer_config_dir": dir})
	rr1 := httptest.NewRecorder()
	s.handleUpdateProject(rr1, buildPatchProjectReq(t, "cfg-dir-clear", string(body)))
	if rr1.Code != http.StatusOK {
		t.Fatalf("initial PATCH status = %d, want 200; body: %s", rr1.Code, rr1.Body.String())
	}

	// Clear with empty string
	clearBody, _ := json.Marshal(map[string]interface{}{"customer_config_dir": ""})
	rr2 := httptest.NewRecorder()
	s.handleUpdateProject(rr2, buildPatchProjectReq(t, "cfg-dir-clear", string(clearBody)))
	if rr2.Code != http.StatusOK {
		t.Fatalf("clear PATCH status = %d, want 200; body: %s", rr2.Code, rr2.Body.String())
	}

	// GET should now return null
	rrGet := httptest.NewRecorder()
	s.handleGetProject(rrGet, buildGetProjectReq(t, "cfg-dir-clear"))
	resp := decodeProjectResp(t, rrGet)
	if val, ok := resp["customer_config_dir"]; ok && val != nil {
		t.Errorf("customer_config_dir after clear = %v, want nil", val)
	}
}

func TestHandlePatchProject_CustomerConfigDir_PreservedWhenOmitted(t *testing.T) {
	s := newProjectsServer(t)
	seedTestProject(t, s, "cfg-dir-omit")

	// Set a valid directory
	dir := t.TempDir()
	body, _ := json.Marshal(map[string]interface{}{"customer_config_dir": dir})
	rr1 := httptest.NewRecorder()
	s.handleUpdateProject(rr1, buildPatchProjectReq(t, "cfg-dir-omit", string(body)))
	if rr1.Code != http.StatusOK {
		t.Fatalf("initial PATCH status = %d, want 200; body: %s", rr1.Code, rr1.Body.String())
	}

	// PATCH with only a name change — no customer_config_dir field
	rr2 := httptest.NewRecorder()
	s.handleUpdateProject(rr2, buildPatchProjectReq(t, "cfg-dir-omit", `{"name":"updated-name"}`))
	if rr2.Code != http.StatusOK {
		t.Fatalf("name PATCH status = %d, want 200; body: %s", rr2.Code, rr2.Body.String())
	}

	// GET should still return the original dir (preserved)
	rrGet := httptest.NewRecorder()
	s.handleGetProject(rrGet, buildGetProjectReq(t, "cfg-dir-omit"))
	resp := decodeProjectResp(t, rrGet)
	if val, _ := resp["customer_config_dir"].(string); val != dir {
		t.Errorf("GET customer_config_dir = %q after unrelated PATCH, want %q (preserved)", val, dir)
	}
}

func TestHandleListProjects_IncludesCustomerConfigDir(t *testing.T) {
	s := newProjectsServer(t)
	seedTestProject(t, s, "cfg-list-p1")
	seedTestProject(t, s, "cfg-list-p2")

	dir := t.TempDir()
	body, _ := json.Marshal(map[string]interface{}{"customer_config_dir": dir})
	rr := httptest.NewRecorder()
	s.handleUpdateProject(rr, buildPatchProjectReq(t, "cfg-list-p1", string(body)))
	if rr.Code != http.StatusOK {
		t.Fatalf("PATCH status = %d, want 200; body: %s", rr.Code, rr.Body.String())
	}

	listReq := httptest.NewRequest(http.MethodGet, "/api/v1/projects", nil)
	listRR := httptest.NewRecorder()
	s.handleListProjects(listRR, listReq)

	if listRR.Code != http.StatusOK {
		t.Fatalf("list status = %d, want 200; body: %s", listRR.Code, listRR.Body.String())
	}

	var listResp struct {
		Projects []map[string]interface{} `json:"projects"`
	}
	if err := json.NewDecoder(listRR.Body).Decode(&listResp); err != nil {
		t.Fatalf("decode list response: %v", err)
	}

	byID := make(map[string]map[string]interface{})
	for _, p := range listResp.Projects {
		if id, _ := p["id"].(string); id != "" {
			byID[id] = p
		}
	}

	p1 := byID["cfg-list-p1"]
	if p1 == nil {
		t.Fatal("cfg-list-p1 missing from list")
	}
	if val, _ := p1["customer_config_dir"].(string); val != dir {
		t.Errorf("cfg-list-p1: customer_config_dir = %q, want %q", val, dir)
	}

	p2 := byID["cfg-list-p2"]
	if p2 == nil {
		t.Fatal("cfg-list-p2 missing from list")
	}
	if val, ok := p2["customer_config_dir"]; ok && val != nil {
		t.Errorf("cfg-list-p2: customer_config_dir = %v, want nil (default)", val)
	}
}
