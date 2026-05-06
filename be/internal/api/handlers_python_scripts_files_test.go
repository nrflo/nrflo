package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// browsePythonScriptDir issues GET /browse and returns the recorder.
// Uses ?project= query param since middleware is not running in unit tests.
func browsePythonScriptDir(t *testing.T, s *Server, projectID, path string) *httptest.ResponseRecorder {
	t.Helper()
	url := "/api/v1/python-scripts/browse?project=" + projectID
	if path != "" {
		url += "&path=" + path
	}
	req := httptest.NewRequest(http.MethodGet, url, nil)
	rr := httptest.NewRecorder()
	s.handleBrowsePythonScriptDir(rr, req)
	return rr
}

// readPythonScriptFile issues GET /read-file and returns the recorder.
// Uses ?project= query param since middleware is not running in unit tests.
func readPythonScriptFile(t *testing.T, s *Server, projectID, path string) *httptest.ResponseRecorder {
	t.Helper()
	url := "/api/v1/python-scripts/read-file?project=" + projectID
	if path != "" {
		url += "&path=" + path
	}
	req := httptest.NewRequest(http.MethodGet, url, nil)
	rr := httptest.NewRecorder()
	s.handleReadPythonScriptFile(rr, req)
	return rr
}

// --- Browse ---

func TestHandleBrowse_MissingProject(t *testing.T) {
	s, _ := newPythonScriptServer(t)
	// No project in query or context → 400
	req := httptest.NewRequest(http.MethodGet, "/api/v1/python-scripts/browse", nil)
	rr := httptest.NewRecorder()
	s.handleBrowsePythonScriptDir(rr, req)
	if rr.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", rr.Code)
	}
}

func TestHandleBrowse_RelativePath(t *testing.T) {
	s, projectID := newPythonScriptServer(t)
	rr := browsePythonScriptDir(t, s, projectID, "relative/path")
	if rr.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", rr.Code)
	}
	assertErrorContains(t, rr, "absolute")
}

func TestHandleBrowse_NonExistentPath(t *testing.T) {
	s, projectID := newPythonScriptServer(t)
	rr := browsePythonScriptDir(t, s, projectID, "/nonexistent/absolutely/no/such/dir")
	if rr.Code != http.StatusNotFound {
		t.Errorf("status = %d, want 404", rr.Code)
	}
}

func TestHandleBrowse_NotADirectory(t *testing.T) {
	s, projectID := newPythonScriptServer(t)
	dir := t.TempDir()
	f := filepath.Join(dir, "afile.py")
	if err := os.WriteFile(f, []byte("x=1"), 0644); err != nil {
		t.Fatalf("write: %v", err)
	}
	rr := browsePythonScriptDir(t, s, projectID, f)
	if rr.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", rr.Code)
	}
	assertErrorContains(t, rr, "directory")
}

func TestHandleBrowse_FiltersEntries(t *testing.T) {
	s, projectID := newPythonScriptServer(t)
	dir := t.TempDir()

	// Create: .py file (included), subdir (included), .txt file (excluded), dotfile (excluded), dot-dir (excluded)
	if err := os.WriteFile(filepath.Join(dir, "script.py"), []byte("x=1"), 0644); err != nil {
		t.Fatalf("write script.py: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "readme.txt"), []byte("text"), 0644); err != nil {
		t.Fatalf("write readme.txt: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, ".hidden.py"), []byte("x=2"), 0644); err != nil {
		t.Fatalf("write .hidden.py: %v", err)
	}
	if err := os.Mkdir(filepath.Join(dir, "subdir"), 0755); err != nil {
		t.Fatalf("mkdir subdir: %v", err)
	}
	if err := os.Mkdir(filepath.Join(dir, ".dotdir"), 0755); err != nil {
		t.Fatalf("mkdir .dotdir: %v", err)
	}

	rr := browsePythonScriptDir(t, s, projectID, dir)
	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body: %s", rr.Code, rr.Body.String())
	}

	var resp browseResponse
	if err := json.NewDecoder(rr.Body).Decode(&resp); err != nil {
		t.Fatalf("decode: %v", err)
	}

	if resp.Path != dir {
		t.Errorf("path = %q, want %q", resp.Path, dir)
	}

	names := make(map[string]browseEntry)
	for _, e := range resp.Entries {
		names[e.Name] = e
	}

	if _, ok := names["script.py"]; !ok {
		t.Error("expected script.py in entries")
	}
	if _, ok := names["subdir"]; !ok {
		t.Error("expected subdir in entries")
	}
	if _, ok := names["readme.txt"]; ok {
		t.Error("readme.txt should be excluded")
	}
	if _, ok := names[".hidden.py"]; ok {
		t.Error(".hidden.py (dotfile) should be excluded")
	}
	if _, ok := names[".dotdir"]; ok {
		t.Error(".dotdir should be excluded")
	}

	if e, ok := names["script.py"]; ok {
		if !e.IsPython {
			t.Error("script.py is_python = false, want true")
		}
		if e.IsDir {
			t.Error("script.py is_dir = true, want false")
		}
	}
	if e, ok := names["subdir"]; ok {
		if !e.IsDir {
			t.Error("subdir is_dir = false, want true")
		}
		if e.IsPython {
			t.Error("subdir is_python = true, want false")
		}
	}
}

// --- Read File ---

func TestHandleReadFile_MissingProject(t *testing.T) {
	s, _ := newPythonScriptServer(t)
	// No project in query or context → 400
	req := httptest.NewRequest(http.MethodGet, "/api/v1/python-scripts/read-file", nil)
	rr := httptest.NewRecorder()
	s.handleReadPythonScriptFile(rr, req)
	if rr.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", rr.Code)
	}
}

func TestHandleReadFile_MissingPath(t *testing.T) {
	s, projectID := newPythonScriptServer(t)
	rr := readPythonScriptFile(t, s, projectID, "")
	if rr.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", rr.Code)
	}
	assertErrorContains(t, rr, "path is required")
}

func TestHandleReadFile_RelativePath(t *testing.T) {
	s, projectID := newPythonScriptServer(t)
	rr := readPythonScriptFile(t, s, projectID, "relative/script.py")
	if rr.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", rr.Code)
	}
	assertErrorContains(t, rr, "absolute")
}

func TestHandleReadFile_NotPyExtension(t *testing.T) {
	s, projectID := newPythonScriptServer(t)
	dir := t.TempDir()
	f := filepath.Join(dir, "script.sh")
	if err := os.WriteFile(f, []byte("#!/bin/sh"), 0644); err != nil {
		t.Fatalf("write: %v", err)
	}
	rr := readPythonScriptFile(t, s, projectID, f)
	if rr.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", rr.Code)
	}
	assertErrorContains(t, rr, ".py")
}

func TestHandleReadFile_NotFound(t *testing.T) {
	s, projectID := newPythonScriptServer(t)
	rr := readPythonScriptFile(t, s, projectID, "/nonexistent/script.py")
	if rr.Code != http.StatusNotFound {
		t.Errorf("status = %d, want 404", rr.Code)
	}
}

func TestHandleReadFile_Oversized(t *testing.T) {
	s, projectID := newPythonScriptServer(t)
	dir := t.TempDir()
	f := filepath.Join(dir, "big.py")
	// Write slightly over 1 MiB
	data := []byte(strings.Repeat("x", (1<<20)+1))
	if err := os.WriteFile(f, data, 0644); err != nil {
		t.Fatalf("write: %v", err)
	}
	rr := readPythonScriptFile(t, s, projectID, f)
	if rr.Code != http.StatusRequestEntityTooLarge {
		t.Errorf("status = %d, want 413", rr.Code)
	}
}

func TestHandleReadFile_ValidFile(t *testing.T) {
	s, projectID := newPythonScriptServer(t)
	dir := t.TempDir()
	content := "print('hello')\nx = 42\n"
	f := filepath.Join(dir, "hello.py")
	if err := os.WriteFile(f, []byte(content), 0644); err != nil {
		t.Fatalf("write: %v", err)
	}
	rr := readPythonScriptFile(t, s, projectID, f)
	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body: %s", rr.Code, rr.Body.String())
	}

	var resp readFileResponse
	if err := json.NewDecoder(rr.Body).Decode(&resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if resp.Path != f {
		t.Errorf("path = %q, want %q", resp.Path, f)
	}
	if resp.Content != content {
		t.Errorf("content = %q, want %q", resp.Content, content)
	}
}
