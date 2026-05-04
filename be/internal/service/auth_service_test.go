package service

import (
	"path/filepath"
	"testing"
	"time"

	"be/internal/auth"
	"be/internal/clock"
	"be/internal/db"
)

func setupAuthSvcEnv(t *testing.T) (*AuthService, *db.Pool) {
	t.Helper()
	dbPath := filepath.Join(t.TempDir(), "auth_svc.db")
	if err := svcCopyTemplateDB(dbPath); err != nil {
		t.Fatalf("copy template DB: %v", err)
	}
	pool, err := db.OpenPoolExisting(dbPath, db.DefaultPoolConfig())
	if err != nil {
		t.Fatalf("open pool: %v", err)
	}
	t.Cleanup(func() { pool.Close() })
	return NewAuthService(pool, clock.Real()), pool
}

func insertAuthUser(t *testing.T, pool *db.Pool, id, email, plain, role, status string) {
	t.Helper()
	hash, err := auth.Hash(plain)
	if err != nil {
		t.Fatalf("hash for %q: %v", id, err)
	}
	now := time.Now().UTC().Format(time.RFC3339Nano)
	_, err = pool.Exec(
		`INSERT INTO users (id, email, display_name, password_hash, role, status, must_change_password, created_at, updated_at) VALUES (?,?,?,?,?,?,?,?,?)`,
		id, email, email, hash, role, status, 0, now, now,
	)
	if err != nil {
		t.Fatalf("insert user %q: %v", id, err)
	}
}

func auditCount(t *testing.T, pool *db.Pool, userID, action string) int {
	t.Helper()
	var count int
	if err := pool.QueryRow(
		`SELECT COUNT(*) FROM audit_log WHERE user_id = ? AND action = ?`, userID, action,
	).Scan(&count); err != nil {
		t.Fatalf("auditCount(%s, %s): %v", userID, action, err)
	}
	return count
}

func TestAuthService_Login_Success(t *testing.T) {
	t.Parallel()
	svc, pool := setupAuthSvcEnv(t)
	insertAuthUser(t, pool, "u-ok", "loginok@example.com", "mypassword", "viewer", "active")

	u, err := svc.Login("loginok@example.com", "mypassword", "127.0.0.1", "testclient")
	if err != nil {
		t.Fatalf("Login: %v", err)
	}
	if u == nil {
		t.Fatal("Login returned nil user, want non-nil")
	}
	if u.Email != "loginok@example.com" {
		t.Errorf("user.Email = %q, want loginok@example.com", u.Email)
	}
	if u.LastLoginAt == nil {
		t.Error("LastLoginAt should be set after successful Login")
	}

	if c := auditCount(t, pool, "u-ok", "login_success"); c != 1 {
		t.Errorf("login_success audit count = %d, want 1", c)
	}
}

func TestAuthService_Login_UserNotFound_ReturnsInvalidCredentials(t *testing.T) {
	t.Parallel()
	svc, _ := setupAuthSvcEnv(t)

	_, err := svc.Login("nobody@example.com", "anything", "127.0.0.1", "agent")
	if err != ErrInvalidCredentials {
		t.Errorf("Login(unknown) = %v, want ErrInvalidCredentials", err)
	}
}

func TestAuthService_Login_WrongPassword_ReturnsInvalidCredentials(t *testing.T) {
	t.Parallel()
	svc, pool := setupAuthSvcEnv(t)
	insertAuthUser(t, pool, "u-badpwd", "badpwd@example.com", "correctpass", "viewer", "active")

	_, err := svc.Login("badpwd@example.com", "wrongpass", "127.0.0.1", "agent")
	if err != ErrInvalidCredentials {
		t.Errorf("Login(bad password) = %v, want ErrInvalidCredentials", err)
	}

	if c := auditCount(t, pool, "u-badpwd", "login_fail"); c != 1 {
		t.Errorf("login_fail audit count = %d, want 1", c)
	}
}

func TestAuthService_Login_DisabledUser_ReturnsUserDisabled(t *testing.T) {
	t.Parallel()
	svc, pool := setupAuthSvcEnv(t)
	insertAuthUser(t, pool, "u-disabled", "disabled@example.com", "mypass", "viewer", "disabled")

	_, err := svc.Login("disabled@example.com", "mypass", "127.0.0.1", "agent")
	if err != ErrUserDisabled {
		t.Errorf("Login(disabled) = %v, want ErrUserDisabled", err)
	}

	// Implementation does insert login_fail for disabled accounts.
	if c := auditCount(t, pool, "u-disabled", "login_fail"); c != 1 {
		t.Errorf("login_fail audit count for disabled user = %d, want 1", c)
	}
}

func TestAuthService_Login_UpdatesLastLoginAt(t *testing.T) {
	t.Parallel()
	fixedTime := time.Date(2025, 8, 1, 10, 0, 0, 0, time.UTC)
	clk := clock.NewTest(fixedTime)

	dbPath := filepath.Join(t.TempDir(), "lastlogin.db")
	if err := svcCopyTemplateDB(dbPath); err != nil {
		t.Fatalf("copy template DB: %v", err)
	}
	pool, err := db.OpenPoolExisting(dbPath, db.DefaultPoolConfig())
	if err != nil {
		t.Fatalf("open pool: %v", err)
	}
	t.Cleanup(func() { pool.Close() })

	svc := NewAuthService(pool, clk)
	insertAuthUser(t, pool, "u-ts", "ts@example.com", "pass123", "viewer", "active")

	u, err := svc.Login("ts@example.com", "pass123", "1.2.3.4", "browser")
	if err != nil {
		t.Fatalf("Login: %v", err)
	}
	if u.LastLoginAt == nil {
		t.Fatal("LastLoginAt is nil, want non-nil")
	}
	if !u.LastLoginAt.Equal(fixedTime) {
		t.Errorf("LastLoginAt = %v, want %v", u.LastLoginAt, fixedTime)
	}
}

func TestAuthService_ChangePassword_WrongCurrent_ReturnsInvalidCredentials(t *testing.T) {
	t.Parallel()
	svc, pool := setupAuthSvcEnv(t)
	insertAuthUser(t, pool, "u-chpwd", "chpwd@example.com", "currentpass", "viewer", "active")

	err := svc.ChangePassword("u-chpwd", "wrongcurrent", "newpass", "127.0.0.1", "agent")
	if err != ErrInvalidCredentials {
		t.Errorf("ChangePassword(wrong current) = %v, want ErrInvalidCredentials", err)
	}
}

func TestAuthService_ChangePassword_Success_AuditsAndUpdatesPassword(t *testing.T) {
	t.Parallel()
	svc, pool := setupAuthSvcEnv(t)
	insertAuthUser(t, pool, "u-chpwd2", "chpwd2@example.com", "oldpass", "viewer", "active")

	err := svc.ChangePassword("u-chpwd2", "oldpass", "newpass456", "127.0.0.1", "agent")
	if err != nil {
		t.Fatalf("ChangePassword: %v", err)
	}

	if c := auditCount(t, pool, "u-chpwd2", "password_change"); c != 1 {
		t.Errorf("password_change audit count = %d, want 1", c)
	}

	// New password should work.
	u, err := svc.Login("chpwd2@example.com", "newpass456", "127.0.0.1", "agent")
	if err != nil || u == nil {
		t.Errorf("Login with new password: err=%v user=%v", err, u)
	}
}

func TestAuthService_ChangePassword_NonExistentUser_ReturnsInvalidCredentials(t *testing.T) {
	t.Parallel()
	svc, _ := setupAuthSvcEnv(t)

	err := svc.ChangePassword("nonexistent-id", "any", "new", "127.0.0.1", "agent")
	if err != ErrInvalidCredentials {
		t.Errorf("ChangePassword(nonexistent) = %v, want ErrInvalidCredentials", err)
	}
}
