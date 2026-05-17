package api

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/cookiejar"
	"path/filepath"
	"testing"
	"time"

	"be/internal/clock"
	"be/internal/config"
	"be/internal/db"
	"be/internal/model"
	"be/internal/service"
)

// authServer is a real HTTP server backed by a temp DB, used by auth handler tests.
type authServer struct {
	baseURL string
	pool    *db.Pool
	srv     *Server
	client  *http.Client // cookie-jar client for session-aware requests
}

func newAuthServer(t *testing.T) *authServer {
	t.Helper()
	dbPath := filepath.Join(t.TempDir(), "auth_test.db")
	if err := apiCopyTemplateDB(dbPath); err != nil {
		t.Fatalf("copy template DB: %v", err)
	}
	pool, err := db.OpenPoolExisting(dbPath, db.DefaultPoolConfig())
	if err != nil {
		t.Fatalf("newAuthServer: pool: %v", err)
	}
	t.Cleanup(func() { pool.Close() })

	srv := NewServer(config.DefaultConfig(), dbPath, t.TempDir(), pool, true)
	port := findFreePort(t)
	go func() { _ = srv.Start("127.0.0.1", port) }()
	baseURL := fmt.Sprintf("http://127.0.0.1:%d", port)
	waitHTTP(t, baseURL, 3*time.Second)
	t.Cleanup(func() { srv.Stop(nil) })

	jar, _ := cookiejar.New(nil)
	return &authServer{baseURL: baseURL, pool: pool, srv: srv, client: &http.Client{Jar: jar}}
}

func seedUser(t *testing.T, pool *db.Pool, email, pass string, role model.UserRole, disabled bool) string {
	t.Helper()
	svc := service.NewUserService(pool, clock.Real())
	id := svc.GenerateID()
	u, err := svc.Create("", id, email, email, pass, role)
	if err != nil {
		t.Fatalf("seedUser: %v", err)
	}
	if _, err := pool.Exec("UPDATE users SET must_change_password=0 WHERE id=?", u.ID); err != nil {
		t.Fatalf("seedUser clear must_change_password: %v", err)
	}
	if disabled {
		if _, err := pool.Exec("UPDATE users SET status='disabled' WHERE id=?", u.ID); err != nil {
			t.Fatalf("seedUser disable: %v", err)
		}
	}
	return u.ID
}

func loginHTTP(t *testing.T, client *http.Client, baseURL, email, pass string) *http.Response {
	t.Helper()
	body, _ := json.Marshal(map[string]string{"email": email, "password": pass})
	resp, err := client.Post(baseURL+"/api/v1/auth/login", "application/json", bytes.NewReader(body))
	if err != nil {
		t.Fatalf("loginHTTP: %v", err)
	}
	return resp
}

// drain discards the body and closes it.
func drain(r *http.Response) { io.Copy(io.Discard, r.Body); r.Body.Close() }

// mustLogin logs in and fails the test if login does not return 200.
func mustLogin(t *testing.T, as *authServer, email, pass string) {
	t.Helper()
	resp := loginHTTP(t, as.client, as.baseURL, email, pass)
	defer drain(resp)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("login %s: status = %d, want 200", email, resp.StatusCode)
	}
}

// postJSON posts JSON body to path using client, drains and closes body, returns response.
func postJSON(t *testing.T, client *http.Client, url, body string) *http.Response {
	t.Helper()
	resp, err := client.Post(url, "application/json", bytes.NewBufferString(body))
	if err != nil {
		t.Fatalf("POST %s: %v", url, err)
	}
	return resp
}

func TestHandleAuthLogin_Success(t *testing.T) {
	as := newAuthServer(t)
	seedUser(t, as.pool, "ok@test.com", "hunter2", model.UserRoleViewer, false)

	resp := loginHTTP(t, as.client, as.baseURL, "ok@test.com", "hunter2")
	defer drain(resp)

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", resp.StatusCode)
	}
	found := false
	for _, c := range resp.Cookies() {
		if c.Name == "nrflo_session" {
			found = true
		}
	}
	if !found {
		t.Error("expected nrflo_session cookie, not found")
	}
	var out map[string]map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		t.Fatalf("decode body: %v", err)
	}
	if out["user"]["email"] != "ok@test.com" {
		t.Errorf("user.email = %v, want ok@test.com", out["user"]["email"])
	}
}

func TestHandleAuthLogin_BadPassword(t *testing.T) {
	as := newAuthServer(t)
	seedUser(t, as.pool, "bp@test.com", "correcthorse", model.UserRoleViewer, false)
	resp := loginHTTP(t, as.client, as.baseURL, "bp@test.com", "wrongpassword")
	defer drain(resp)
	if resp.StatusCode != http.StatusUnauthorized {
		t.Errorf("status = %d, want 401", resp.StatusCode)
	}
}

func TestHandleAuthLogin_RateLimit(t *testing.T) {
	as := newAuthServer(t)
	seedUser(t, as.pool, "rl@test.com", "pass", model.UserRoleViewer, false)

	// Plain client (no jar) — each request shares the same loopback RemoteAddr.
	client := &http.Client{}
	var last *http.Response
	for i := 0; i < 6; i++ {
		if last != nil {
			drain(last)
		}
		last = loginHTTP(t, client, as.baseURL, "rl@test.com", "wrong")
	}
	defer drain(last)
	if last.StatusCode != http.StatusTooManyRequests {
		t.Errorf("6th attempt: status = %d, want 429", last.StatusCode)
	}
	if ra := last.Header.Get("Retry-After"); ra == "" || ra == "0" {
		t.Errorf("expected Retry-After > 0, got %q", ra)
	}
}

func TestHandleAuthMe_NoSession(t *testing.T) {
	as := newAuthServer(t)
	resp, err := http.Get(as.baseURL + "/api/v1/auth/me")
	if err != nil {
		t.Fatalf("GET /auth/me: %v", err)
	}
	defer drain(resp)
	if resp.StatusCode != http.StatusUnauthorized {
		t.Errorf("status = %d, want 401", resp.StatusCode)
	}
}

func TestHandleAuthMe_ValidSession(t *testing.T) {
	as := newAuthServer(t)
	seedUser(t, as.pool, "me@test.com", "pass123", model.UserRoleViewer, false)
	mustLogin(t, as, "me@test.com", "pass123")

	resp, err := as.client.Get(as.baseURL + "/api/v1/auth/me")
	if err != nil {
		t.Fatalf("GET /auth/me: %v", err)
	}
	defer drain(resp)
	if resp.StatusCode != http.StatusOK {
		t.Errorf("status = %d, want 200", resp.StatusCode)
	}
	var out map[string]map[string]interface{}
	json.NewDecoder(resp.Body).Decode(&out)
	if out["user"]["email"] != "me@test.com" {
		t.Errorf("user.email = %v, want me@test.com", out["user"]["email"])
	}
}

func TestHandleAuthLogout_DestroysSession(t *testing.T) {
	as := newAuthServer(t)
	seedUser(t, as.pool, "lo@test.com", "pass123", model.UserRoleViewer, false)
	mustLogin(t, as, "lo@test.com", "pass123")

	logoutResp := postJSON(t, as.client, as.baseURL+"/api/v1/auth/logout", "{}")
	defer drain(logoutResp)
	if logoutResp.StatusCode != http.StatusNoContent {
		t.Errorf("logout status = %d, want 204", logoutResp.StatusCode)
	}

	meResp, err := as.client.Get(as.baseURL + "/api/v1/auth/me")
	if err != nil {
		t.Fatalf("GET /auth/me: %v", err)
	}
	defer drain(meResp)
	if meResp.StatusCode != http.StatusUnauthorized {
		t.Errorf("after logout: status = %d, want 401", meResp.StatusCode)
	}
}

func TestHandleAuthChangePassword_WrongCurrent(t *testing.T) {
	as := newAuthServer(t)
	seedUser(t, as.pool, "cp@test.com", "correct", model.UserRoleViewer, false)
	mustLogin(t, as, "cp@test.com", "correct")

	body, _ := json.Marshal(map[string]string{"current_password": "wrong", "new_password": "newpass"})
	resp := postJSON(t, as.client, as.baseURL+"/api/v1/auth/change-password", string(body))
	defer drain(resp)
	if resp.StatusCode != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", resp.StatusCode)
	}
}

func TestHandleAuthChangePassword_Success(t *testing.T) {
	as := newAuthServer(t)
	seedUser(t, as.pool, "cps@test.com", "oldpass", model.UserRoleViewer, false)
	mustLogin(t, as, "cps@test.com", "oldpass")

	body, _ := json.Marshal(map[string]string{"current_password": "oldpass", "new_password": "newpass"})
	resp := postJSON(t, as.client, as.baseURL+"/api/v1/auth/change-password", string(body))
	defer drain(resp)
	if resp.StatusCode != http.StatusNoContent {
		t.Errorf("change-password status = %d, want 204", resp.StatusCode)
	}

	jar, _ := cookiejar.New(nil)
	newClient := &http.Client{Jar: jar}
	resp2 := loginHTTP(t, newClient, as.baseURL, "cps@test.com", "newpass")
	defer drain(resp2)
	if resp2.StatusCode != http.StatusOK {
		t.Errorf("login with new password: status = %d, want 200", resp2.StatusCode)
	}
}

func TestAdminGate_ViewerForbidden(t *testing.T) {
	as := newAuthServer(t)
	seedUser(t, as.pool, "viewer@test.com", "pass", model.UserRoleViewer, false)
	mustLogin(t, as, "viewer@test.com", "pass")

	resp := postJSON(t, as.client, as.baseURL+"/api/v1/system-agents", "{}")
	defer drain(resp)
	if resp.StatusCode != http.StatusForbidden {
		t.Errorf("viewer: status = %d, want 403", resp.StatusCode)
	}
}

func TestAdminGate_AdminAllowed(t *testing.T) {
	as := newAuthServer(t)
	seedUser(t, as.pool, "admin2@test.com", "pass", model.UserRoleAdmin, false)
	mustLogin(t, as, "admin2@test.com", "pass")

	resp := postJSON(t, as.client, as.baseURL+"/api/v1/system-agents", "{}")
	defer drain(resp)
	if resp.StatusCode == http.StatusForbidden {
		t.Error("admin: got 403, should not be forbidden")
	}
}

// TestRequireAuth_NoSession_ProtectedEndpoint verifies that a protected endpoint
// returns 401 when accessed without a session cookie.
func TestRequireAuth_NoSession_ProtectedEndpoint(t *testing.T) {
	as := newAuthServer(t)
	resp, err := http.Get(as.baseURL + "/api/v1/auth/me")
	if err != nil {
		t.Fatalf("GET /auth/me: %v", err)
	}
	defer drain(resp)
	if resp.StatusCode != http.StatusUnauthorized {
		t.Errorf("status = %d, want 401", resp.StatusCode)
	}
}
