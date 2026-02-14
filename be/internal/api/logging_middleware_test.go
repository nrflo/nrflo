package api

import (
	"bytes"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"be/internal/logger"
)

// TestLoggingMiddleware_GeneratesTrxAndLogsRequestResponse verifies middleware generates trx and logs.
func TestLoggingMiddleware_GeneratesTrxAndLogsRequestResponse(t *testing.T) {
	var logBuf bytes.Buffer
	logger.SetWriter(&logBuf)
	defer logger.SetWriter(os.Stderr)

	// Create test handler
	testHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Verify trx is in context
		trx := logger.TrxFromContext(r.Context())
		if trx == "-" {
			t.Error("trx not found in context")
		}
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("ok"))
	})

	// Create middleware
	srv := &Server{}
	handler := srv.loggingMiddleware(testHandler)

	// Make request
	req := httptest.NewRequest("POST", "/api/v1/tickets", nil)
	req.Header.Set("X-Project", "test-project")
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	logs := logBuf.String()

	// Verify request log
	if !strings.Contains(logs, "api request") {
		t.Errorf("logs missing 'api request': %s", logs)
	}
	if !strings.Contains(logs, "method=POST") {
		t.Errorf("logs missing method=POST: %s", logs)
	}
	if !strings.Contains(logs, "path=/api/v1/tickets") {
		t.Errorf("logs missing path: %s", logs)
	}
	if !strings.Contains(logs, "project=test-project") {
		t.Errorf("logs missing project: %s", logs)
	}

	// Verify response log
	if !strings.Contains(logs, "api response") {
		t.Errorf("logs missing 'api response': %s", logs)
	}
	if !strings.Contains(logs, "status=200") {
		t.Errorf("logs missing status=200: %s", logs)
	}
	if !strings.Contains(logs, "duration_ms=") {
		t.Errorf("logs missing duration_ms: %s", logs)
	}

	// Verify both logs have same trx
	requestLines := extractLogLines(logs, "api request")
	responseLines := extractLogLines(logs, "api response")

	if len(requestLines) != 1 || len(responseLines) != 1 {
		t.Fatalf("expected 1 request and 1 response log")
	}

	reqTrx := extractTrx(requestLines[0])
	respTrx := extractTrx(responseLines[0])

	if reqTrx != respTrx {
		t.Errorf("request trx %s != response trx %s", reqTrx, respTrx)
	}
	if len(reqTrx) != 8 {
		t.Errorf("trx length = %d, want 8", len(reqTrx))
	}
}

// TestLoggingMiddleware_SkipsOPTIONS verifies OPTIONS requests skip logging.
func TestLoggingMiddleware_SkipsOPTIONS(t *testing.T) {
	var logBuf bytes.Buffer
	logger.SetWriter(&logBuf)
	defer logger.SetWriter(os.Stderr)

	testHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	})

	srv := &Server{}
	handler := srv.loggingMiddleware(testHandler)

	req := httptest.NewRequest("OPTIONS", "/api/v1/tickets", nil)
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	logs := logBuf.String()

	if strings.Contains(logs, "api request") || strings.Contains(logs, "api response") {
		t.Errorf("OPTIONS request should not be logged: %s", logs)
	}
}

// TestLoggingMiddleware_SkipsWebSocketUpgrade verifies WebSocket upgrade skips logging.
func TestLoggingMiddleware_SkipsWebSocketUpgrade(t *testing.T) {
	var logBuf bytes.Buffer
	logger.SetWriter(&logBuf)
	defer logger.SetWriter(os.Stderr)

	testHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusSwitchingProtocols)
	})

	srv := &Server{}
	handler := srv.loggingMiddleware(testHandler)

	req := httptest.NewRequest("GET", "/api/v1/ws", nil)
	req.Header.Set("Upgrade", "websocket")
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	logs := logBuf.String()

	if strings.Contains(logs, "api request") || strings.Contains(logs, "api response") {
		t.Errorf("WebSocket upgrade should not be logged: %s", logs)
	}
}

// TestLoggingMiddleware_ExcludesHighFrequencyGETEndpoints verifies exclusion logic.
func TestLoggingMiddleware_ExcludesHighFrequencyGETEndpoints(t *testing.T) {
	testCases := []struct {
		name     string
		path     string
		excluded bool
	}{
		{"agents recent", "/api/v1/agents/recent", true},
		{"status", "/api/v1/status", true},
		{"daily stats", "/api/v1/daily-stats", true},
		{"ticket workflow", "/api/v1/tickets/TICKET-1/workflow", true},
		{"session messages", "/api/v1/sessions/sess-123/messages", true},
		{"project workflow", "/api/v1/projects/proj-1/workflow", true},
		{"project agents", "/api/v1/projects/proj-1/agents", true},
		{"regular endpoint", "/api/v1/tickets", false},
		{"ticket get", "/api/v1/tickets/TICKET-1", false},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			var logBuf bytes.Buffer
			logger.SetWriter(&logBuf)
			defer logger.SetWriter(os.Stderr)

			testHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusOK)
			})

			srv := &Server{}
			handler := srv.loggingMiddleware(testHandler)

			req := httptest.NewRequest("GET", tc.path, nil)
			req.Header.Set("X-Project", "test-project")
			rr := httptest.NewRecorder()

			handler.ServeHTTP(rr, req)

			logs := logBuf.String()
			hasLogs := strings.Contains(logs, "api request")

			if tc.excluded && hasLogs {
				t.Errorf("path %s should be excluded but was logged: %s", tc.path, logs)
			}
			if !tc.excluded && !hasLogs {
				t.Errorf("path %s should be logged but was excluded", tc.path)
			}
		})
	}
}

// TestResponseWriter_CapturesStatusCode verifies responseWriter captures status codes.
func TestResponseWriter_CapturesStatusCode(t *testing.T) {
	testCases := []struct {
		name           string
		writeHeader    bool
		statusCode     int
		expectedStatus int
	}{
		{"explicit 200", true, 200, 200},
		{"explicit 201", true, 201, 201},
		{"explicit 400", true, 400, 400},
		{"explicit 404", true, 404, 404},
		{"explicit 500", true, 500, 500},
		{"default 200", false, 0, 200},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			var logBuf bytes.Buffer
			logger.SetWriter(&logBuf)
			defer logger.SetWriter(os.Stderr)

			testHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if tc.writeHeader {
					w.WriteHeader(tc.statusCode)
				}
				w.Write([]byte("body"))
			})

			srv := &Server{}
			handler := srv.loggingMiddleware(testHandler)

			req := httptest.NewRequest("POST", "/api/v1/test", nil)
			req.Header.Set("X-Project", "test")
			rr := httptest.NewRecorder()

			handler.ServeHTTP(rr, req)

			logs := logBuf.String()

			expectedLog := fmt.Sprintf("status=%d", tc.expectedStatus)
			if !strings.Contains(logs, expectedLog) {
				t.Errorf("logs missing status=%d: %s", tc.expectedStatus, logs)
			}
		})
	}
}

// TestResponseWriter_MultipleWriteHeader verifies only first WriteHeader is captured.
func TestResponseWriter_MultipleWriteHeader(t *testing.T) {
	var logBuf bytes.Buffer
	logger.SetWriter(&logBuf)
	defer logger.SetWriter(os.Stderr)

	testHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.WriteHeader(http.StatusInternalServerError) // Should be ignored
		w.Write([]byte("ok"))
	})

	srv := &Server{}
	handler := srv.loggingMiddleware(testHandler)

	req := httptest.NewRequest("GET", "/api/v1/test", nil)
	req.Header.Set("X-Project", "test")
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	logs := logBuf.String()

	if !strings.Contains(logs, "status=200") {
		t.Errorf("logs should capture first status 200: %s", logs)
	}
	if strings.Contains(logs, "status=500") {
		t.Errorf("logs should not capture second status 500: %s", logs)
	}
}

// TestExcludedFromLogging_POSTNotExcluded verifies only GET requests are excluded.
func TestExcludedFromLogging_POSTNotExcluded(t *testing.T) {
	// POST to excluded path should not be excluded
	if excludedFromLogging("POST", "/api/v1/status") {
		t.Error("POST to /api/v1/status should not be excluded")
	}
	if excludedFromLogging("DELETE", "/api/v1/agents/recent") {
		t.Error("DELETE to /api/v1/agents/recent should not be excluded")
	}
}

// TestExcludedFromLogging_ExactMatches verifies exact path matching.
func TestExcludedFromLogging_ExactMatches(t *testing.T) {
	testCases := []struct {
		path     string
		excluded bool
	}{
		{"/api/v1/agents/recent", true},
		{"/api/v1/status", true},
		{"/api/v1/daily-stats", true},
		{"/api/v1/agents/recent/extra", false},
		{"/api/v1/status/extra", false},
	}

	for _, tc := range testCases {
		result := excludedFromLogging("GET", tc.path)
		if result != tc.excluded {
			t.Errorf("excludedFromLogging(GET, %s) = %v, want %v", tc.path, result, tc.excluded)
		}
	}
}

// TestExcludedFromLogging_ParameterizedPaths verifies parameterized path matching.
func TestExcludedFromLogging_ParameterizedPaths(t *testing.T) {
	testCases := []struct {
		path     string
		excluded bool
	}{
		{"/api/v1/tickets/TICKET-1/workflow", true},
		{"/api/v1/tickets/ABC-123/workflow", true},
		{"/api/v1/tickets/TICKET-1/workflow/extra", false},
		{"/api/v1/sessions/sess-abc/messages", true},
		{"/api/v1/sessions/sess-123/messages", true},
		{"/api/v1/sessions/sess-abc/messages/extra", false},
		{"/api/v1/projects/proj-1/workflow", true},
		{"/api/v1/projects/proj-1/agents", true},
		{"/api/v1/projects/proj-1/other", false},
	}

	for _, tc := range testCases {
		result := excludedFromLogging("GET", tc.path)
		if result != tc.excluded {
			t.Errorf("excludedFromLogging(GET, %s) = %v, want %v", tc.path, result, tc.excluded)
		}
	}
}

// Helper functions

func extractLogLines(logs, substr string) []string {
	var lines []string
	for _, line := range strings.Split(logs, "\n") {
		if strings.Contains(line, substr) {
			lines = append(lines, line)
		}
	}
	return lines
}

func extractTrx(logLine string) string {
	start := strings.Index(logLine, "[")
	end := strings.Index(logLine, "]")
	if start == -1 || end == -1 || end <= start {
		return ""
	}
	return logLine[start+1 : end]
}
