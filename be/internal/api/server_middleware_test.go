package api

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"be/internal/config"
)

// newServerWithCORSOrigins creates a minimal Server for middleware tests.
func newServerWithCORSOrigins(origins []string) *Server {
	cfg := &config.Config{}
	cfg.Server.CORSOrigins = origins
	return &Server{config: cfg}
}

// sentinelHandler is a simple handler that records if it was called.
func sentinelHandler(called *bool) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		*called = true
		w.WriteHeader(http.StatusOK)
	})
}

// --- corsMiddleware tests ---

func TestCORSMiddleware_AllowedOrigin(t *testing.T) {
	s := newServerWithCORSOrigins([]string{"http://localhost:5175"})
	called := false
	handler := s.corsMiddleware(sentinelHandler(&called))

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("Origin", "http://localhost:5175")
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	if !called {
		t.Error("expected next handler to be called")
	}
	if got := rr.Header().Get("Access-Control-Allow-Origin"); got != "http://localhost:5175" {
		t.Errorf("Access-Control-Allow-Origin = %q, want %q", got, "http://localhost:5175")
	}
}

func TestCORSMiddleware_DisallowedOrigin(t *testing.T) {
	s := newServerWithCORSOrigins([]string{"http://localhost:5175"})
	called := false
	handler := s.corsMiddleware(sentinelHandler(&called))

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("Origin", "http://evil.example.com")
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	if !called {
		t.Error("expected next handler to be called even for disallowed origin")
	}
	if got := rr.Header().Get("Access-Control-Allow-Origin"); got != "" {
		t.Errorf("Access-Control-Allow-Origin = %q, want empty for disallowed origin", got)
	}
}

func TestCORSMiddleware_WildcardOrigin(t *testing.T) {
	s := newServerWithCORSOrigins([]string{"*"})
	called := false
	handler := s.corsMiddleware(sentinelHandler(&called))

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("Origin", "http://any.example.com")
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	if !called {
		t.Error("expected next handler to be called")
	}
	if got := rr.Header().Get("Access-Control-Allow-Origin"); got != "http://any.example.com" {
		t.Errorf("Access-Control-Allow-Origin = %q, want %q", got, "http://any.example.com")
	}
}

func TestCORSMiddleware_PreflyhtOPTIONS(t *testing.T) {
	s := newServerWithCORSOrigins([]string{"http://localhost:5175"})
	called := false
	handler := s.corsMiddleware(sentinelHandler(&called))

	req := httptest.NewRequest(http.MethodOptions, "/api/v1/tickets", nil)
	req.Header.Set("Origin", "http://localhost:5175")
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	if called {
		t.Error("expected next handler NOT to be called for OPTIONS preflight")
	}
	if rr.Code != http.StatusNoContent {
		t.Errorf("OPTIONS status = %d, want %d", rr.Code, http.StatusNoContent)
	}
}

func TestCORSMiddleware_AlwaysSetsMethodsAndHeaders(t *testing.T) {
	s := newServerWithCORSOrigins([]string{"http://localhost:5175"})
	called := false
	handler := s.corsMiddleware(sentinelHandler(&called))

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("Origin", "http://localhost:5175")
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	for _, hdr := range []struct {
		name string
		want string
	}{
		{"Access-Control-Allow-Methods", "GET, POST, PATCH, DELETE, OPTIONS"},
		{"Access-Control-Allow-Headers", "Content-Type, X-Project, X-Request-ID"},
		{"Access-Control-Max-Age", "86400"},
	} {
		if got := rr.Header().Get(hdr.name); got != hdr.want {
			t.Errorf("%s = %q, want %q", hdr.name, got, hdr.want)
		}
	}
}

func TestCORSMiddleware_NoOriginHeader(t *testing.T) {
	s := newServerWithCORSOrigins([]string{"http://localhost:5175"})
	called := false
	handler := s.corsMiddleware(sentinelHandler(&called))

	// Request without Origin header (e.g. server-to-server)
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	if !called {
		t.Error("expected next handler to be called")
	}
	if got := rr.Header().Get("Access-Control-Allow-Origin"); got != "" {
		t.Errorf("Access-Control-Allow-Origin = %q, want empty when no Origin header", got)
	}
}

func TestCORSMiddleware_MultipleAllowedOrigins(t *testing.T) {
	origins := []string{"http://localhost:5175", "http://localhost:3000", "https://app.example.com"}
	s := newServerWithCORSOrigins(origins)

	cases := []struct {
		origin      string
		wantAllowed bool
	}{
		{"http://localhost:5175", true},
		{"http://localhost:3000", true},
		{"https://app.example.com", true},
		{"http://evil.com", false},
	}

	for _, tc := range cases {
		t.Run(tc.origin, func(t *testing.T) {
			called := false
			handler := s.corsMiddleware(sentinelHandler(&called))

			req := httptest.NewRequest(http.MethodGet, "/", nil)
			req.Header.Set("Origin", tc.origin)
			rr := httptest.NewRecorder()

			handler.ServeHTTP(rr, req)

			got := rr.Header().Get("Access-Control-Allow-Origin")
			if tc.wantAllowed {
				if got != tc.origin {
					t.Errorf("origin %q: Access-Control-Allow-Origin = %q, want %q", tc.origin, got, tc.origin)
				}
			} else {
				if got != "" {
					t.Errorf("origin %q: Access-Control-Allow-Origin = %q, want empty", tc.origin, got)
				}
			}
		})
	}
}

// --- projectMiddleware tests ---

func TestProjectMiddleware_HeaderExtracted(t *testing.T) {
	s := &Server{}
	var capturedProject string
	handler := s.projectMiddleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedProject = getProjectID(r)
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("X-Project", "my-project")
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	if capturedProject != "my-project" {
		t.Errorf("getProjectID() = %q, want %q", capturedProject, "my-project")
	}
}

func TestProjectMiddleware_MissingHeader(t *testing.T) {
	s := &Server{}
	var capturedProject string
	handler := s.projectMiddleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedProject = getProjectID(r)
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	if capturedProject != "" {
		t.Errorf("getProjectID() = %q, want empty string when no X-Project header", capturedProject)
	}
}

func TestProjectMiddleware_DoesNotBlockRequest(t *testing.T) {
	s := &Server{}
	called := false
	handler := s.projectMiddleware(sentinelHandler(&called))

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	if !called {
		t.Error("expected next handler to be called by projectMiddleware")
	}
	if rr.Code != http.StatusOK {
		t.Errorf("status = %d, want 200", rr.Code)
	}
}

// --- getProjectID tests ---

func TestGetProjectID_FromQueryParam(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/?project=from-query", nil)
	got := getProjectID(req)
	if got != "from-query" {
		t.Errorf("getProjectID() = %q, want %q", got, "from-query")
	}
}

func TestGetProjectID_QueryParamTakesPrecedence(t *testing.T) {
	// Query param should win over header-injected context value
	s := &Server{}
	var capturedProject string
	handler := s.projectMiddleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedProject = getProjectID(r)
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodGet, "/?project=query-project", nil)
	req.Header.Set("X-Project", "header-project")
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	if capturedProject != "query-project" {
		t.Errorf("getProjectID() = %q, want query param %q to win", capturedProject, "query-project")
	}
}

func TestGetProjectID_FallsBackToContext(t *testing.T) {
	s := &Server{}
	var capturedProject string
	handler := s.projectMiddleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedProject = getProjectID(r)
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("X-Project", "context-project")
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	if capturedProject != "context-project" {
		t.Errorf("getProjectID() = %q, want %q from context", capturedProject, "context-project")
	}
}

func TestGetProjectID_EmptyWhenNeitherSet(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	got := getProjectID(req)
	if got != "" {
		t.Errorf("getProjectID() = %q, want empty string", got)
	}
}
