package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
)

// logsTestDir is the hardcoded path used by handleGetLogs.
const logsTestDir = "/tmp/nrworkflow/logs"

// setupLogFile writes content to the log file and registers cleanup to restore
// the original state. The hardcoded logsDir constant in the handler forces
// tests to write directly to /tmp/nrworkflow/logs/.
func setupLogFile(t *testing.T, logType string, content string) {
	t.Helper()
	if err := os.MkdirAll(logsTestDir, 0755); err != nil {
		t.Fatalf("MkdirAll(%s): %v", logsTestDir, err)
	}
	path := fmt.Sprintf("%s/%s.log", logsTestDir, logType)

	original, readErr := os.ReadFile(path)
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatalf("WriteFile(%s): %v", path, err)
	}
	t.Cleanup(func() {
		if readErr != nil {
			os.Remove(path)
		} else {
			os.WriteFile(path, original, 0644) //nolint:errcheck
		}
	})
}

// removeLogFile ensures the log file does not exist for the test and restores it afterward.
func removeLogFile(t *testing.T, logType string) {
	t.Helper()
	path := fmt.Sprintf("%s/%s.log", logsTestDir, logType)
	original, readErr := os.ReadFile(path)
	os.Remove(path)
	t.Cleanup(func() {
		if readErr == nil {
			os.WriteFile(path, original, 0644) //nolint:errcheck
		}
	})
}

func newLogsServer() *Server {
	return &Server{}
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
		// nil slice in Go marshals to JSON null; treat as empty.
		return nil
	}
	lines, ok := raw.([]interface{})
	if !ok {
		t.Fatalf("'lines' is not an array, got %T", raw)
	}
	return lines
}

// TestHandleGetLogs_DefaultTypeBe verifies that omitting ?type= defaults to be.log.
func TestHandleGetLogs_DefaultTypeBe(t *testing.T) {
	setupLogFile(t, "be", "line1\nline2\nline3\n")

	s := newLogsServer()
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

// TestHandleGetLogs_TypeBe verifies explicit type=be reads be.log.
func TestHandleGetLogs_TypeBe(t *testing.T) {
	setupLogFile(t, "be", "alpha\nbeta\ngamma\n")

	s := newLogsServer()
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

// TestHandleGetLogs_TypeFe verifies type=fe reads fe.log.
func TestHandleGetLogs_TypeFe(t *testing.T) {
	setupLogFile(t, "fe", "fe-line1\nfe-line2\n")

	s := newLogsServer()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/logs?type=fe", nil)
	rr := httptest.NewRecorder()
	s.handleGetLogs(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rr.Code)
	}

	resp := decodeLogsResponse(t, rr)
	if resp["type"] != "fe" {
		t.Errorf("type = %v, want %q", resp["type"], "fe")
	}

	lines := getLines(t, resp)
	if len(lines) != 2 {
		t.Errorf("len(lines) = %d, want 2", len(lines))
	}
}

// TestHandleGetLogs_ReverseOrder verifies lines are returned latest-first.
func TestHandleGetLogs_ReverseOrder(t *testing.T) {
	setupLogFile(t, "be", "first\nsecond\nthird\n")

	s := newLogsServer()
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

// TestHandleGetLogs_MissingFile verifies 200 with empty lines when be.log absent.
func TestHandleGetLogs_MissingFile(t *testing.T) {
	removeLogFile(t, "be")

	s := newLogsServer()
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

// TestHandleGetLogs_MissingFileFe verifies 200 with empty lines when fe.log absent.
func TestHandleGetLogs_MissingFileFe(t *testing.T) {
	removeLogFile(t, "fe")

	s := newLogsServer()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/logs?type=fe", nil)
	rr := httptest.NewRecorder()
	s.handleGetLogs(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 for missing fe.log", rr.Code)
	}

	resp := decodeLogsResponse(t, rr)
	lines := getLines(t, resp)
	if len(lines) != 0 {
		t.Errorf("len(lines) = %d, want 0", len(lines))
	}
	if resp["type"] != "fe" {
		t.Errorf("type = %v, want %q", resp["type"], "fe")
	}
}

// TestHandleGetLogs_EmptyFile verifies 200 with empty lines for an empty file.
func TestHandleGetLogs_EmptyFile(t *testing.T) {
	setupLogFile(t, "be", "")

	s := newLogsServer()
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

// TestHandleGetLogs_InvalidType verifies 400 for unsupported type values.
// Empty string defaults to "be" (valid), so it is excluded.
func TestHandleGetLogs_InvalidType(t *testing.T) {
	cases := []struct {
		name     string
		typeVal  string
	}{
		{"all", "all"},
		{"server", "server"},
		{"uppercase_BE", "BE"},
		{"uppercase_FE", "FE"},
		{"injection", "badtype"},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			s := newLogsServer()
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

// TestHandleGetLogs_CapAt1000 verifies that >1000 lines are capped to 1000 (latest first).
func TestHandleGetLogs_CapAt1000(t *testing.T) {
	var sb strings.Builder
	for i := 1; i <= 1500; i++ {
		fmt.Fprintf(&sb, "line%d\n", i)
	}
	setupLogFile(t, "be", sb.String())

	s := newLogsServer()
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

	// After reverse + cap, the first element is the last file line ("line1500").
	first, _ := lines[0].(string)
	if first != "line1500" {
		t.Errorf("lines[0] = %q, want %q (latest line first)", first, "line1500")
	}
	// The 1000th element (index 999) should be "line501" (1500 - 999 = 501).
	last, _ := lines[999].(string)
	if last != "line501" {
		t.Errorf("lines[999] = %q, want %q", last, "line501")
	}
}

// TestHandleGetLogs_ExactlyAtCap verifies exactly 1000 lines are returned untruncated.
func TestHandleGetLogs_ExactlyAtCap(t *testing.T) {
	var sb strings.Builder
	for i := 1; i <= 1000; i++ {
		fmt.Fprintf(&sb, "line%d\n", i)
	}
	setupLogFile(t, "be", sb.String())

	s := newLogsServer()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/logs?type=be", nil)
	rr := httptest.NewRecorder()
	s.handleGetLogs(rr, req)

	resp := decodeLogsResponse(t, rr)
	lines := getLines(t, resp)

	if len(lines) != 1000 {
		t.Errorf("len(lines) = %d, want 1000", len(lines))
	}
}

// TestHandleGetLogs_ContentType verifies response Content-Type is application/json.
func TestHandleGetLogs_ContentType(t *testing.T) {
	setupLogFile(t, "be", "some log line\n")

	s := newLogsServer()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/logs?type=be", nil)
	rr := httptest.NewRecorder()
	s.handleGetLogs(rr, req)

	ct := rr.Header().Get("Content-Type")
	if ct != "application/json" {
		t.Errorf("Content-Type = %q, want %q", ct, "application/json")
	}
}

// TestHandleGetLogs_ResponseShape verifies both "lines" and "type" fields are present.
func TestHandleGetLogs_ResponseShape(t *testing.T) {
	setupLogFile(t, "fe", "log line\n")

	s := newLogsServer()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/logs?type=fe", nil)
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
