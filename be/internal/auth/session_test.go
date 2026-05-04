package auth

import (
	"context"
	"database/sql"
	"net/http"
	"path/filepath"
	"testing"
	"time"

	_ "modernc.org/sqlite"
)

func openSessionDB(t *testing.T) *sql.DB {
	t.Helper()
	path := filepath.Join(t.TempDir(), "sessions.db")
	db, err := sql.Open("sqlite", "file:"+path+"?_pragma=busy_timeout(10000)")
	if err != nil {
		t.Fatalf("open session db: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	if _, err := db.Exec(`CREATE TABLE IF NOT EXISTS sessions (token TEXT PRIMARY KEY, data BLOB NOT NULL, expiry REAL NOT NULL)`); err != nil {
		t.Fatalf("create sessions table: %v", err)
	}
	if _, err := db.Exec(`CREATE INDEX IF NOT EXISTS sessions_expiry_idx ON sessions (expiry)`); err != nil {
		t.Fatalf("create sessions index: %v", err)
	}
	return db
}

func TestNewManager_CookieSettings(t *testing.T) {
	t.Parallel()
	db := openSessionDB(t)
	sm := NewManager(db, false)
	if sm.Cookie.Name != sessionCookieName {
		t.Errorf("Cookie.Name = %q, want %q", sm.Cookie.Name, sessionCookieName)
	}
	if !sm.Cookie.HttpOnly {
		t.Error("Cookie.HttpOnly should be true")
	}
	if !sm.Cookie.Secure {
		t.Error("Cookie.Secure should be true when dev=false")
	}
	if sm.Cookie.SameSite != http.SameSiteStrictMode {
		t.Errorf("Cookie.SameSite = %v, want SameSiteStrictMode", sm.Cookie.SameSite)
	}
	if sm.Lifetime != 24*time.Hour {
		t.Errorf("Lifetime = %v, want 24h", sm.Lifetime)
	}
	if sm.IdleTimeout != 8*time.Hour {
		t.Errorf("IdleTimeout = %v, want 8h", sm.IdleTimeout)
	}
}

func TestNewManager_DevMode_SecureFalse(t *testing.T) {
	t.Parallel()
	db := openSessionDB(t)
	sm := NewManager(db, true) // dev=true
	if sm.Cookie.Secure {
		t.Error("Cookie.Secure should be false in dev mode")
	}
}

func TestPutUserID_UserID_RoundTrip(t *testing.T) {
	t.Parallel()
	db := openSessionDB(t)
	sm := NewManager(db, true)

	ctx, err := sm.Load(context.Background(), "")
	if err != nil {
		t.Fatalf("Load: %v", err)
	}

	PutUserID(ctx, sm, "usr-abc-123")

	token, _, err := sm.Commit(ctx)
	if err != nil {
		t.Fatalf("Commit: %v", err)
	}

	ctx2, err := sm.Load(context.Background(), token)
	if err != nil {
		t.Fatalf("Load with token: %v", err)
	}

	got := UserID(ctx2, sm)
	if got != "usr-abc-123" {
		t.Errorf("UserID() = %q, want %q", got, "usr-abc-123")
	}
}

func TestUserID_EmptyWhenNotSet(t *testing.T) {
	t.Parallel()
	db := openSessionDB(t)
	sm := NewManager(db, true)

	ctx, err := sm.Load(context.Background(), "")
	if err != nil {
		t.Fatalf("Load: %v", err)
	}

	got := UserID(ctx, sm)
	if got != "" {
		t.Errorf("UserID() = %q, want empty string", got)
	}
}

func TestRenew_RotatesToken(t *testing.T) {
	t.Parallel()
	db := openSessionDB(t)
	sm := NewManager(db, true)

	// Create session and commit to get token1.
	ctx, err := sm.Load(context.Background(), "")
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	PutUserID(ctx, sm, "usr-renew-test")
	token1, _, err := sm.Commit(ctx)
	if err != nil {
		t.Fatalf("Commit: %v", err)
	}

	// Load with token1, renew, commit to get token2.
	ctx2, err := sm.Load(context.Background(), token1)
	if err != nil {
		t.Fatalf("Load with token1: %v", err)
	}
	if err := Renew(ctx2, sm); err != nil {
		t.Fatalf("Renew: %v", err)
	}
	token2, _, err := sm.Commit(ctx2)
	if err != nil {
		t.Fatalf("Commit after renew: %v", err)
	}

	if token1 == token2 {
		t.Error("Renew() did not rotate token (token1 == token2)")
	}

	// Load with the new token and verify data is preserved.
	ctx3, err := sm.Load(context.Background(), token2)
	if err != nil {
		t.Fatalf("Load with token2: %v", err)
	}
	got := UserID(ctx3, sm)
	if got != "usr-renew-test" {
		t.Errorf("UserID after Renew = %q, want %q", got, "usr-renew-test")
	}
}
