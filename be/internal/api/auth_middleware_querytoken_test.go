package api

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"be/internal/model"
)

// reqWithQueryToken builds a GET request with ?token=<token> in the URL.
func reqWithQueryToken(path, token string) *http.Request {
	return httptest.NewRequest(http.MethodGet, path+"?token="+token, nil)
}

func TestRequireAuthWith_QueryToken_OnWSPath_ServiceToken_Passes(t *testing.T) {
	s := newServerWithAuth(t)
	_, plain := seedServiceToken(t, s, "proj-ws-svc", "ws-ci")

	called := false
	chain := s.requireAuthWith(true, sentinelHandler(&called))
	rr := httptest.NewRecorder()
	chain.ServeHTTP(rr, reqWithQueryToken("/api/v1/ws", plain))

	if rr.Code != http.StatusOK {
		t.Errorf("status = %d, want 200; body=%s", rr.Code, rr.Body.String())
	}
	if !called {
		t.Error("expected next handler to be called for service token via ?token=")
	}
}

func TestRequireAuthWith_QueryToken_OnWSPath_AgentToken_Passes(t *testing.T) {
	s := newServerWithAuth(t)
	seedTokenSession(t, s, "proj-ws-agent", "tok-ws-agent", model.AgentSessionRunning)

	called := false
	chain := s.requireAuthWith(true, sentinelHandler(&called))
	rr := httptest.NewRecorder()
	chain.ServeHTTP(rr, reqWithQueryToken("/api/v1/ws", "tok-ws-agent"))

	if rr.Code != http.StatusOK {
		t.Errorf("status = %d, want 200; body=%s", rr.Code, rr.Body.String())
	}
	if !called {
		t.Error("expected next handler to be called for agent session token via ?token=")
	}
}

func TestRequireAuthWith_QueryToken_UnknownToken_Returns401(t *testing.T) {
	s := newServerWithAuth(t)

	called := false
	chain := s.sessionMgr.LoadAndSave(s.requireAuthWith(true, sentinelHandler(&called)))
	rr := httptest.NewRecorder()
	chain.ServeHTTP(rr, reqWithQueryToken("/api/v1/ws", "nope"))

	if called {
		t.Error("next handler must not be called for unknown ?token=")
	}
	if rr.Code != http.StatusUnauthorized {
		t.Errorf("status = %d, want 401", rr.Code)
	}
}

func TestRequireAuthWith_QueryToken_ProjectMismatch_Returns403(t *testing.T) {
	s := newServerWithAuth(t)
	seedTokenSession(t, s, "proj-ws-mismatch", "tok-ws-mismatch", model.AgentSessionRunning)

	called := false
	chain := s.requireAuthWith(true, sentinelHandler(&called))
	r := reqWithQueryToken("/api/v1/ws", "tok-ws-mismatch")
	r.Header.Set("X-Project", "proj-other")
	rr := httptest.NewRecorder()
	chain.ServeHTTP(rr, r)

	if called {
		t.Error("project mismatch must not pass through")
	}
	if rr.Code != http.StatusForbidden {
		t.Errorf("status = %d, want 403", rr.Code)
	}
}

func TestRequireAuth_QueryTokenOnRESTPath_Returns401(t *testing.T) {
	s := newServerWithAuth(t)
	seedTokenSession(t, s, "proj-rest-qt", "tok-rest-qt", model.AgentSessionRunning)

	called := false
	// requireAuth wraps requireAuthWith(false, ...) — query token must be ignored.
	chain := s.sessionMgr.LoadAndSave(s.requireAuth(sentinelHandler(&called)))
	rr := httptest.NewRecorder()
	chain.ServeHTTP(rr, reqWithQueryToken("/api/v1/tickets", "tok-rest-qt"))

	if called {
		t.Error("requireAuth must not accept ?token= query param")
	}
	if rr.Code != http.StatusUnauthorized {
		t.Errorf("status = %d, want 401", rr.Code)
	}
}

func TestRequireAuthWith_HeaderBearer_StillWorks(t *testing.T) {
	s := newServerWithAuth(t)
	seedTokenSession(t, s, "proj-ws-hdr", "tok-ws-hdr", model.AgentSessionRunning)

	called := false
	chain := s.requireAuthWith(true, sentinelHandler(&called))
	rr := httptest.NewRecorder()
	chain.ServeHTTP(rr, reqWithBearer("tok-ws-hdr"))

	if rr.Code != http.StatusOK {
		t.Errorf("status = %d, want 200", rr.Code)
	}
	if !called {
		t.Error("Authorization: Bearer must still work through requireAuthWith(true, ...)")
	}
}

func TestCORSMiddleware_AllowHeaders_IncludesAuthorization(t *testing.T) {
	s := newServerWithCORSOrigins([]string{"http://localhost:5175"})
	handler := s.corsMiddleware(sentinelHandler(new(bool)))

	req := httptest.NewRequest(http.MethodOptions, "/api/v1/ws", nil)
	req.Header.Set("Origin", "http://localhost:5175")
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	got := rr.Header().Get("Access-Control-Allow-Headers")
	want := "Content-Type, X-Project, X-Request-ID, Authorization"
	if got != want {
		t.Errorf("Access-Control-Allow-Headers = %q, want %q", got, want)
	}
}
