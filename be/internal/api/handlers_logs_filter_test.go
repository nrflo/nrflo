package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// TestHandleGetLogs_FilterMatchingLines verifies that when a filter is provided,
// all matching lines from the entire file are returned with no 1000-line cap.
func TestHandleGetLogs_FilterMatchingLines(t *testing.T) {
	dir := t.TempDir()
	// Write 1500 lines; every 3rd line contains the target token.
	// 500 lines will match — more than maxLogLines proves no cap applies.
	var sb strings.Builder
	for i := 1; i <= 1500; i++ {
		if i%3 == 0 {
			fmt.Fprintf(&sb, "line%d session_id=target-uuid\n", i)
		} else {
			fmt.Fprintf(&sb, "line%d noise\n", i)
		}
	}
	setupLogFile(t, dir, "be", sb.String())

	s := newLogsServer(dir)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/logs?type=be&filter=target-uuid", nil)
	rr := httptest.NewRecorder()
	s.handleGetLogs(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rr.Code)
	}

	resp := decodeLogsResponse(t, rr)
	lines := getLines(t, resp)

	// 500 matching lines (1500/3) — well above maxLogLines=1000
	if len(lines) != 500 {
		t.Errorf("len(lines) = %d, want 500 (no 1000-line cap when filter set)", len(lines))
	}
}

// TestHandleGetLogs_FilterReverseOrder verifies that filtered results are reversed
// (latest matching line appears first).
func TestHandleGetLogs_FilterReverseOrder(t *testing.T) {
	dir := t.TempDir()
	setupLogFile(t, dir, "be", "match-first\nskip-me\nmatch-second\nskip-me-too\nmatch-third\n")

	s := newLogsServer(dir)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/logs?type=be&filter=match", nil)
	rr := httptest.NewRecorder()
	s.handleGetLogs(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rr.Code)
	}

	resp := decodeLogsResponse(t, rr)
	lines := getLines(t, resp)

	want := []string{"match-third", "match-second", "match-first"}
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

// TestHandleGetLogs_FilterCaseInsensitive verifies that filter matching is case-insensitive.
func TestHandleGetLogs_FilterCaseInsensitive(t *testing.T) {
	cases := []struct {
		name   string
		filter string
	}{
		{"uppercase_filter", "SESSION_ID=ABC-123"},
		{"mixed_filter", "Session_ID=abc-123"},
		{"lowercase_filter", "session_id=abc-123"},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			// Lines use mixed case; all three filter variants should match.
			setupLogFile(t, dir, "be",
				"unrelated noise\n"+
					"2026-01-01 INFO [trx] spawner started session_id=abc-123 phase=L0\n"+
					"another unrelated line\n"+
					"2026-01-01 INFO [trx] spawner ended Session_Id=abc-123 result=pass\n",
			)

			s := newLogsServer(dir)
			req := httptest.NewRequest(http.MethodGet, "/api/v1/logs", nil)
			q := req.URL.Query()
			q.Set("type", "be")
			q.Set("filter", tc.filter)
			req.URL.RawQuery = q.Encode()
			rr := httptest.NewRecorder()
			s.handleGetLogs(rr, req)

			if rr.Code != http.StatusOK {
				t.Fatalf("filter=%q: status = %d, want 200", tc.filter, rr.Code)
			}

			resp := decodeLogsResponse(t, rr)
			lines := getLines(t, resp)

			if len(lines) != 2 {
				t.Errorf("filter=%q: len(lines) = %d, want 2", tc.filter, len(lines))
			}
		})
	}
}

// TestHandleGetLogs_FilterNoMatches verifies that a filter with no matches returns an empty array.
func TestHandleGetLogs_FilterNoMatches(t *testing.T) {
	dir := t.TempDir()
	setupLogFile(t, dir, "be", "line1\nline2\nline3\n")

	s := newLogsServer(dir)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/logs?type=be&filter=nonexistent-string-xyz", nil)
	rr := httptest.NewRecorder()
	s.handleGetLogs(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rr.Code)
	}

	resp := decodeLogsResponse(t, rr)
	lines := getLines(t, resp)

	if len(lines) != 0 {
		t.Errorf("len(lines) = %d, want 0 for filter with no matches", len(lines))
	}
}

// TestHandleGetLogs_FilterEmptyStringPreservesCap verifies that an explicit empty
// filter= param still applies the 1000-line cap (same as no filter param).
func TestHandleGetLogs_FilterEmptyStringPreservesCap(t *testing.T) {
	dir := t.TempDir()
	var sb strings.Builder
	for i := 1; i <= 1500; i++ {
		fmt.Fprintf(&sb, "line%d\n", i)
	}
	setupLogFile(t, dir, "be", sb.String())

	s := newLogsServer(dir)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/logs?type=be&filter=", nil)
	rr := httptest.NewRecorder()
	s.handleGetLogs(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rr.Code)
	}

	resp := decodeLogsResponse(t, rr)
	lines := getLines(t, resp)

	if len(lines) != maxLogLines {
		t.Errorf("len(lines) = %d, want %d (cap applies for empty filter)", len(lines), maxLogLines)
	}
	// Latest line should be first
	first, _ := lines[0].(string)
	if first != "line1500" {
		t.Errorf("lines[0] = %q, want %q", first, "line1500")
	}
}

// TestHandleGetLogs_FilterMissingFileReturnsEmpty verifies that a filter on a missing
// log file still returns an empty lines array (not an error).
func TestHandleGetLogs_FilterMissingFileReturnsEmpty(t *testing.T) {
	dir := t.TempDir()
	// no log file created

	s := newLogsServer(dir)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/logs?type=be&filter=something", nil)
	rr := httptest.NewRecorder()
	s.handleGetLogs(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rr.Code)
	}

	resp := decodeLogsResponse(t, rr)
	lines := getLines(t, resp)
	if len(lines) != 0 {
		t.Errorf("len(lines) = %d, want 0 for missing file with filter", len(lines))
	}
}

// TestHandleGetLogs_FeErrorMessage verifies that type=fe returns 400 with the exact
// error message "type must be 'be'" (acceptance criterion).
func TestHandleGetLogs_FeErrorMessage(t *testing.T) {
	s := newLogsServer(t.TempDir())
	req := httptest.NewRequest(http.MethodGet, "/api/v1/logs?type=fe", nil)
	rr := httptest.NewRecorder()
	s.handleGetLogs(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Fatalf("type=fe: status = %d, want 400", rr.Code)
	}

	var resp map[string]interface{}
	if err := json.NewDecoder(rr.Body).Decode(&resp); err != nil {
		t.Fatalf("type=fe: failed to decode error response: %v", err)
	}
	errMsg, _ := resp["error"].(string)
	if errMsg != "type must be 'be'" {
		t.Errorf("type=fe: error = %q, want %q", errMsg, "type must be 'be'")
	}
}
