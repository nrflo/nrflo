package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func setupLogFile(t *testing.T, dir string, logType string, content string) {
	t.Helper()
	if err := os.MkdirAll(dir, 0755); err != nil {
		t.Fatalf("MkdirAll(%s): %v", dir, err)
	}
	path := filepath.Join(dir, logType+".log")
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatalf("WriteFile(%s): %v", path, err)
	}
}

func removeLogFile(t *testing.T, dir string, logType string) {
	t.Helper()
	path := filepath.Join(dir, logType+".log")
	os.Remove(path)
}

func newLogsServer(logsDir string) *Server {
	return &Server{logsDir: logsDir}
}

func decodeLogsResponse(t *testing.T, rr *httptest.ResponseRecorder) map[string]interface{} {
	t.Helper()
	var resp map[string]interface{}
	if err := json.NewDecoder(rr.Body).Decode(&resp); err != nil {
		t.Fatalf("failed to decode response body: %v", err)
	}
	return resp
}

func getLines(t *testing.T, resp map[string]interface{}) []interface{} {
	t.Helper()
	raw, ok := resp["lines"]
	if !ok {
		t.Fatal("response missing 'lines' key")
	}
	if raw == nil {
		return nil
	}
	lines, ok := raw.([]interface{})
	if !ok {
		t.Fatalf("'lines' is not an array, got %T", raw)
	}
	return lines
}

func TestHandleGetLogs_DefaultTypeBe(t *testing.T) {
	dir := t.TempDir()
	setupLogFile(t, dir, "be", "line1\nline2\nline3\n")

	s := newLogsServer(dir)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/logs", nil)
	rr := httptest.NewRecorder()
	s.handleGetLogs(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body: %s", rr.Code, rr.Body.String())
	}

	resp := decodeLogsResponse(t, rr)
	logType, _ := resp["type"].(string)
	if logType != "be" {
		t.Errorf("type = %q, want %q", logType, "be")
	}

	lines := getLines(t, resp)
	if len(lines) != 3 {
		t.Errorf("len(lines) = %d, want 3", len(lines))
	}
}

func TestHandleGetLogs_TypeBe(t *testing.T) {
	dir := t.TempDir()
	setupLogFile(t, dir, "be", "alpha\nbeta\ngamma\n")

	s := newLogsServer(dir)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/logs?type=be", nil)
	rr := httptest.NewRecorder()
	s.handleGetLogs(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rr.Code)
	}

	resp := decodeLogsResponse(t, rr)
	if resp["type"] != "be" {
		t.Errorf("type = %v, want %q", resp["type"], "be")
	}
}

func TestHandleGetLogs_ReverseOrder(t *testing.T) {
	dir := t.TempDir()
	setupLogFile(t, dir, "be", "first\nsecond\nthird\n")

	s := newLogsServer(dir)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/logs?type=be", nil)
	rr := httptest.NewRecorder()
	s.handleGetLogs(rr, req)

	resp := decodeLogsResponse(t, rr)
	lines := getLines(t, resp)

	want := []string{"third", "second", "first"}
	if len(lines) != len(want) {
		t.Fatalf("len(lines) = %d, want %d", len(lines), len(want))
	}
	for i, w := range want {
		got, _ := lines[i].(string)
		if got != w {
			t.Errorf("lines[%d] = %q, want %q", i, got, w)
		}
	}
}

func TestHandleGetLogs_MissingFile(t *testing.T) {
	dir := t.TempDir()
	removeLogFile(t, dir, "be")

	s := newLogsServer(dir)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/logs?type=be", nil)
	rr := httptest.NewRecorder()
	s.handleGetLogs(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 for missing file", rr.Code)
	}

	resp := decodeLogsResponse(t, rr)
	lines := getLines(t, resp)
	if len(lines) != 0 {
		t.Errorf("len(lines) = %d, want 0 for missing file", len(lines))
	}
}

func TestHandleGetLogs_EmptyFile(t *testing.T) {
	dir := t.TempDir()
	setupLogFile(t, dir, "be", "")

	s := newLogsServer(dir)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/logs?type=be", nil)
	rr := httptest.NewRecorder()
	s.handleGetLogs(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rr.Code)
	}

	resp := decodeLogsResponse(t, rr)
	lines := getLines(t, resp)
	if len(lines) != 0 {
		t.Errorf("len(lines) = %d, want 0 for empty file", len(lines))
	}
}

func TestHandleGetLogs_InvalidType(t *testing.T) {
	cases := []struct {
		name    string
		typeVal string
	}{
		{"all", "all"},
		{"fe", "fe"},
		{"server", "server"},
		{"uppercase_BE", "BE"},
		{"uppercase_FE", "FE"},
		{"injection", "badtype"},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			s := newLogsServer(t.TempDir())
			req := httptest.NewRequest(http.MethodGet, "/api/v1/logs", nil)
			q := req.URL.Query()
			q.Set("type", tc.typeVal)
			req.URL.RawQuery = q.Encode()
			rr := httptest.NewRecorder()
			s.handleGetLogs(rr, req)

			if rr.Code != http.StatusBadRequest {
				t.Errorf("type=%q: status = %d, want 400", tc.typeVal, rr.Code)
			}

			var resp map[string]interface{}
			if err := json.NewDecoder(rr.Body).Decode(&resp); err != nil {
				t.Fatalf("type=%q: failed to decode error response: %v", tc.typeVal, err)
			}
			if _, ok := resp["error"]; !ok {
				t.Errorf("type=%q: response missing 'error' key", tc.typeVal)
			}
		})
	}
}

func TestHandleGetLogs_CapAt1000(t *testing.T) {
	dir := t.TempDir()
	var sb strings.Builder
	for i := 1; i <= 1500; i++ {
		fmt.Fprintf(&sb, "line%d\n", i)
	}
	setupLogFile(t, dir, "be", sb.String())

	s := newLogsServer(dir)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/logs?type=be", nil)
	rr := httptest.NewRecorder()
	s.handleGetLogs(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rr.Code)
	}

	resp := decodeLogsResponse(t, rr)
	lines := getLines(t, resp)

	if len(lines) != 1000 {
		t.Errorf("len(lines) = %d, want 1000", len(lines))
	}

	first, _ := lines[0].(string)
	if first != "line1500" {
		t.Errorf("lines[0] = %q, want %q (latest line first)", first, "line1500")
	}
	last, _ := lines[999].(string)
	if last != "line501" {
		t.Errorf("lines[999] = %q, want %q", last, "line501")
	}
}

func TestHandleGetLogs_ExactlyAtCap(t *testing.T) {
	dir := t.TempDir()
	var sb strings.Builder
	for i := 1; i <= 1000; i++ {
		fmt.Fprintf(&sb, "line%d\n", i)
	}
	setupLogFile(t, dir, "be", sb.String())

	s := newLogsServer(dir)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/logs?type=be", nil)
	rr := httptest.NewRecorder()
	s.handleGetLogs(rr, req)

	resp := decodeLogsResponse(t, rr)
	lines := getLines(t, resp)

	if len(lines) != 1000 {
		t.Errorf("len(lines) = %d, want 1000", len(lines))
	}
}

func TestHandleGetLogs_ContentType(t *testing.T) {
	dir := t.TempDir()
	setupLogFile(t, dir, "be", "some log line\n")

	s := newLogsServer(dir)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/logs?type=be", nil)
	rr := httptest.NewRecorder()
	s.handleGetLogs(rr, req)

	ct := rr.Header().Get("Content-Type")
	if ct != "application/json" {
		t.Errorf("Content-Type = %q, want %q", ct, "application/json")
	}
}

func TestHandleGetLogs_ResponseShape(t *testing.T) {
	dir := t.TempDir()
	setupLogFile(t, dir, "be", "log line\n")

	s := newLogsServer(dir)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/logs?type=be", nil)
	rr := httptest.NewRecorder()
	s.handleGetLogs(rr, req)

	resp := decodeLogsResponse(t, rr)

	if _, ok := resp["lines"]; !ok {
		t.Error("response missing 'lines' key")
	}
	if _, ok := resp["type"]; !ok {
		t.Error("response missing 'type' key")
	}
}
