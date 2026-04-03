package api

import (
	"net/http"
	"net/http/httptest"
	"regexp"
	"testing"

	"be/internal/logger"
)

var trxPattern = regexp.MustCompile(`^[0-9a-f]{8}$`)

// TestRequestIDMiddleware_HeaderPresent verifies X-Request-ID is set on every response.
func TestRequestIDMiddleware_HeaderPresent(t *testing.T) {
	s := &Server{}
	called := false
	handler := s.requestIDMiddleware(sentinelHandler(&called))

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	if !called {
		t.Error("expected next handler to be called")
	}
	if got := rr.Header().Get("X-Request-ID"); got == "" {
		t.Error("X-Request-ID header not set")
	}
}

// TestRequestIDMiddleware_HeaderIsEightCharHex verifies the header value is a valid 8-char hex string.
func TestRequestIDMiddleware_HeaderIsEightCharHex(t *testing.T) {
	s := &Server{}
	handler := s.requestIDMiddleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	got := rr.Header().Get("X-Request-ID")
	if !trxPattern.MatchString(got) {
		t.Errorf("X-Request-ID = %q, want 8-char hex string matching [0-9a-f]{8}", got)
	}
}

// TestRequestIDMiddleware_TrxInContext verifies logger.TrxFromContext returns a real trx
// (not the default "-") inside the handler.
func TestRequestIDMiddleware_TrxInContext(t *testing.T) {
	s := &Server{}
	var capturedTrx string
	handler := s.requestIDMiddleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedTrx = logger.TrxFromContext(r.Context())
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	if capturedTrx == "" || capturedTrx == "-" {
		t.Errorf("TrxFromContext() = %q, want non-empty and non-dash trx", capturedTrx)
	}
}

// TestRequestIDMiddleware_TrxMatchesHeader verifies the trx in context equals the X-Request-ID header.
func TestRequestIDMiddleware_TrxMatchesHeader(t *testing.T) {
	s := &Server{}
	var capturedTrx string
	handler := s.requestIDMiddleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedTrx = logger.TrxFromContext(r.Context())
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	headerTrx := rr.Header().Get("X-Request-ID")
	if capturedTrx != headerTrx {
		t.Errorf("context trx %q != X-Request-ID header %q", capturedTrx, headerTrx)
	}
}

// TestRequestIDMiddleware_UniquePerRequest verifies each request gets a distinct trx.
func TestRequestIDMiddleware_UniquePerRequest(t *testing.T) {
	s := &Server{}
	handler := s.requestIDMiddleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	ids := make(map[string]struct{}, 10)
	for i := 0; i < 10; i++ {
		req := httptest.NewRequest(http.MethodGet, "/", nil)
		rr := httptest.NewRecorder()
		handler.ServeHTTP(rr, req)
		id := rr.Header().Get("X-Request-ID")
		if _, dup := ids[id]; dup {
			t.Errorf("duplicate X-Request-ID %q on request %d", id, i)
		}
		ids[id] = struct{}{}
	}
}

// TestRequestIDMiddleware_CallsNextAlways verifies the handler is called for any method/path.
func TestRequestIDMiddleware_CallsNextAlways(t *testing.T) {
	cases := []struct {
		method string
		path   string
	}{
		{http.MethodGet, "/api/v1/tickets"},
		{http.MethodPost, "/api/v1/tickets"},
		{http.MethodDelete, "/api/v1/tickets/123"},
		{http.MethodOptions, "/api/v1/tickets"},
	}

	for _, tc := range cases {
		t.Run(tc.method+" "+tc.path, func(t *testing.T) {
			s := &Server{}
			called := false
			handler := s.requestIDMiddleware(sentinelHandler(&called))

			req := httptest.NewRequest(tc.method, tc.path, nil)
			rr := httptest.NewRecorder()
			handler.ServeHTTP(rr, req)

			if !called {
				t.Errorf("requestIDMiddleware did not call next handler for %s %s", tc.method, tc.path)
			}
			if got := rr.Header().Get("X-Request-ID"); got == "" {
				t.Errorf("X-Request-ID not set for %s %s", tc.method, tc.path)
			}
		})
	}
}

// TestCORSMiddleware_ExposesRequestID verifies Access-Control-Expose-Headers includes X-Request-ID.
func TestCORSMiddleware_ExposesRequestID(t *testing.T) {
	s := newServerWithCORSOrigins([]string{"http://localhost:5175"})
	handler := s.corsMiddleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("Origin", "http://localhost:5175")
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	got := rr.Header().Get("Access-Control-Expose-Headers")
	if got != "X-Request-ID" {
		t.Errorf("Access-Control-Expose-Headers = %q, want %q", got, "X-Request-ID")
	}
}

// TestRequestIDMiddleware_HeaderSetBeforeNextHandler verifies the header is set before next.ServeHTTP
// by checking it is readable even when the handler reads its own response headers.
func TestRequestIDMiddleware_HeaderSetBeforeNextHandler(t *testing.T) {
	s := &Server{}
	var seenInHandler string
	handler := s.requestIDMiddleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// The header should already be set before this runs.
		seenInHandler = w.Header().Get("X-Request-ID")
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if seenInHandler == "" {
		t.Error("X-Request-ID was not set before next.ServeHTTP was called")
	}
	if !trxPattern.MatchString(seenInHandler) {
		t.Errorf("X-Request-ID seen in handler = %q, want 8-char hex", seenInHandler)
	}
}
