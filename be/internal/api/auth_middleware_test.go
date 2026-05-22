package api

import (
	"context"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"be/internal/auth"
	"be/internal/config"
	"be/internal/db"
	"be/internal/model"
	"be/internal/service"
)

// newServerWithAuth creates a Server with a real sessionMgr backed by a migrated DB.
func newServerWithAuth(t *testing.T) *Server {
	t.Helper()
	dbPath := filepath.Join(t.TempDir(), "auth_mw_test.db")
	if err := apiCopyTemplateDB(dbPath); err != nil {
		t.Fatalf("newServerWithAuth: copy template DB: %v", err)
	}
	pool, err := db.OpenPoolExisting(dbPath, db.DefaultPoolConfig())
	if err != nil {
		t.Fatalf("newServerWithAuth: open pool: %v", err)
	}
	t.Cleanup(func() { pool.Close() })
	cfg := config.DefaultConfig()
	return NewServer(cfg, dbPath, t.TempDir(), pool, true)
}

// injectSession stores userID in a new session and returns the session cookie.
func injectSession(t *testing.T, s *Server, userID string) *http.Cookie {
	t.Helper()
	var cookie *http.Cookie
	// Wrap a handler that calls PutUserID inside LoadAndSave so the session is committed.
	inner := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		auth.PutUserID(r.Context(), s.sessionMgr, userID)
	})
	handler := s.sessionMgr.LoadAndSave(inner)
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)
	for _, c := range rr.Result().Cookies() {
		if c.Name == "nrflo_session" {
			cookie = c
			break
		}
	}
	if cookie == nil {
		t.Fatal("injectSession: no nrflo_session cookie in response")
	}
	return cookie
}

// createTestUser creates a user via UserService, optionally disabling it, and returns the user ID.
func createTestUser(t *testing.T, s *Server, email string, role model.UserRole, disabled bool) string {
	t.Helper()
	userSvc := service.NewUserService(s.pool, s.clock)
	id := userSvc.GenerateID()
	u, err := userSvc.Create("system", id, email, "Test User", "password123!", role)
	if err != nil {
		t.Fatalf("createTestUser(%q): %v", email, err)
	}
	// Clear must_change_password so the account is fully active.
	if _, err := s.pool.Exec(`UPDATE users SET must_change_password = 0 WHERE id = ?`, u.ID); err != nil {
		t.Fatalf("createTestUser: clear must_change_password: %v", err)
	}
	if disabled {
		if _, err := s.pool.Exec(`UPDATE users SET status = 'disabled' WHERE id = ?`, u.ID); err != nil {
			t.Fatalf("createTestUser: disable user: %v", err)
		}
	}
	return u.ID
}

// withCookieHandler executes handler with the given cookie and returns the recorder.
func withCookieHandler(t *testing.T, handler http.Handler, cookie *http.Cookie) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.AddCookie(cookie)
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)
	return rr
}

// --- requireAuth tests ---

func TestRequireAuth_NilSessionMgr_PassesThrough(t *testing.T) {
	s := &Server{config: config.DefaultConfig()}
	called := false
	handler := s.requireAuth(sentinelHandler(&called))
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)
	if !called {
		t.Error("expected next handler to be called when sessionMgr is nil")
	}
	if rr.Code != http.StatusOK {
		t.Errorf("status = %d, want 200", rr.Code)
	}
}

func TestRequireAuth_NoSession_Returns401(t *testing.T) {
	s := newServerWithAuth(t)
	called := false
	chain := s.sessionMgr.LoadAndSave(s.requireAuth(sentinelHandler(&called)))
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rr := httptest.NewRecorder()
	chain.ServeHTTP(rr, req)
	if called {
		t.Error("expected next handler NOT to be called")
	}
	if rr.Code != http.StatusUnauthorized {
		t.Errorf("status = %d, want 401", rr.Code)
	}
}

func TestRequireAuth_ValidSession_ActiveUser_Passes(t *testing.T) {
	s := newServerWithAuth(t)
	uid := createTestUser(t, s, "active@test.com", model.UserRoleAdmin, false)
	cookie := injectSession(t, s, uid)
	called := false
	chain := s.sessionMgr.LoadAndSave(s.requireAuth(sentinelHandler(&called)))
	rr := withCookieHandler(t, chain, cookie)
	if !called {
		t.Error("expected next handler to be called for active user")
	}
	if rr.Code != http.StatusOK {
		t.Errorf("status = %d, want 200", rr.Code)
	}
}

func TestRequireAuth_DisabledUser_Returns401(t *testing.T) {
	s := newServerWithAuth(t)
	uid := createTestUser(t, s, "disabled@test.com", model.UserRoleAdmin, true)
	cookie := injectSession(t, s, uid)
	called := false
	chain := s.sessionMgr.LoadAndSave(s.requireAuth(sentinelHandler(&called)))
	rr := withCookieHandler(t, chain, cookie)
	if called {
		t.Error("expected next handler NOT to be called for disabled user")
	}
	if rr.Code != http.StatusUnauthorized {
		t.Errorf("status = %d, want 401", rr.Code)
	}
}

// --- requireAdmin tests ---

func TestRequireAdmin_NilSessionMgr_NoUser_Returns403(t *testing.T) {
	s := &Server{config: config.DefaultConfig()}
	called := false
	handler := s.requireAdmin(sentinelHandler(&called))
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)
	if called {
		t.Error("expected next handler NOT to be called when no user in context")
	}
	if rr.Code != http.StatusForbidden {
		t.Errorf("status = %d, want 403", rr.Code)
	}
}

func TestRequireAdmin_ViewerUser_Returns403(t *testing.T) {
	s := newServerWithAuth(t)
	uid := createTestUser(t, s, "viewer@test.com", model.UserRoleViewer, false)
	cookie := injectSession(t, s, uid)
	called := false
	chain := s.sessionMgr.LoadAndSave(s.requireAdmin(sentinelHandler(&called)))
	rr := withCookieHandler(t, chain, cookie)
	if called {
		t.Error("expected next handler NOT to be called for viewer")
	}
	if rr.Code != http.StatusForbidden {
		t.Errorf("status = %d, want 403", rr.Code)
	}
}

func TestRequireAdmin_AdminUser_Passes(t *testing.T) {
	s := newServerWithAuth(t)
	uid := createTestUser(t, s, "admin2@test.com", model.UserRoleAdmin, false)
	cookie := injectSession(t, s, uid)
	called := false
	chain := s.sessionMgr.LoadAndSave(s.requireAdmin(sentinelHandler(&called)))
	rr := withCookieHandler(t, chain, cookie)
	if !called {
		t.Error("expected next handler to be called for admin")
	}
	if rr.Code != http.StatusOK {
		t.Errorf("status = %d, want 200", rr.Code)
	}
}

// --- getUser / getUserID tests ---

func TestUserContextHelpers(t *testing.T) {
	cases := []struct {
		name       string
		user       *model.User // nil = no user in context
		wantUserID string      // expected getUserID result; for present cases also the user ID
	}{
		{"user_present", &model.User{ID: "usr_abc", Email: "u@test.com", Role: model.UserRoleAdmin}, "usr_abc"},
		{"user_present_id_only", &model.User{ID: "usr_xyz", Role: model.UserRoleAdmin}, "usr_xyz"},
		{"no_user", nil, ""},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, "/", nil)
			if tc.user != nil {
				req = req.WithContext(context.WithValue(req.Context(), userKey, tc.user))
			}

			gotUser := getUser(req)
			if tc.user == nil {
				if gotUser != nil {
					t.Errorf("getUser() = %v, want nil", gotUser)
				}
			} else if gotUser == nil || gotUser.ID != tc.user.ID {
				t.Errorf("getUser() = %v, want user with ID %q", gotUser, tc.user.ID)
			}

			if got := getUserID(req); got != tc.wantUserID {
				t.Errorf("getUserID() = %q, want %q", got, tc.wantUserID)
			}
		})
	}
}
