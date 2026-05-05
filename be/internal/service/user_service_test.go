package service

import (
	"path/filepath"
	"testing"
	"time"

	"be/internal/auth"
	"be/internal/clock"
	"be/internal/db"
	"be/internal/model"
)

const svcSeedAdminID = "usr_admin_seed"

func setupUserSvcEnv(t *testing.T) (*UserService, *db.Pool) {
	t.Helper()
	dbPath := filepath.Join(t.TempDir(), "user_svc.db")
	if err := svcCopyTemplateDB(dbPath); err != nil {
		t.Fatalf("copy template DB: %v", err)
	}
	pool, err := db.OpenPoolExisting(dbPath, db.DefaultPoolConfig())
	if err != nil {
		t.Fatalf("open pool: %v", err)
	}
	t.Cleanup(func() { pool.Close() })
	return NewUserService(pool, clock.Real()), pool
}

func insertUserSvcUser(t *testing.T, pool *db.Pool, id, email, role, status string) {
	t.Helper()
	hash, err := auth.Hash("testpassword")
	if err != nil {
		t.Fatalf("hash: %v", err)
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

func getMustChangePwd(t *testing.T, pool *db.Pool, userID string) bool {
	t.Helper()
	var v int
	if err := pool.QueryRow(`SELECT must_change_password FROM users WHERE id = ?`, userID).Scan(&v); err != nil {
		t.Fatalf("getMustChangePwd(%s): %v", userID, err)
	}
	return v != 0
}

// --- Last-admin protection ---

func TestUserService_Update_LastAdmin_DemoteToViewer_Rejected(t *testing.T) {
	t.Parallel()
	svc, _ := setupUserSvcEnv(t)

	// Seed admin is the only active admin — demote to viewer should fail.
	err := svc.Update("acting-user", svcSeedAdminID, "Admin", model.UserRoleViewer, model.UserStatusActive)
	if err != ErrLastAdmin {
		t.Errorf("Update demote last admin = %v, want ErrLastAdmin", err)
	}
}

func TestUserService_Update_LastAdmin_Disable_Rejected(t *testing.T) {
	t.Parallel()
	svc, _ := setupUserSvcEnv(t)

	err := svc.Update("acting-user", svcSeedAdminID, "Admin", model.UserRoleAdmin, model.UserStatusDisabled)
	if err != ErrLastAdmin {
		t.Errorf("Update disable last admin = %v, want ErrLastAdmin", err)
	}
}

func TestUserService_Update_LastAdmin_AllowedWhenSecondAdminExists(t *testing.T) {
	t.Parallel()
	svc, pool := setupUserSvcEnv(t)

	// Add a second active admin; now the seed admin can be demoted.
	insertUserSvcUser(t, pool, "u-admin2", "admin2@test.com", "admin", "active")

	err := svc.Update("acting-user", svcSeedAdminID, "Admin", model.UserRoleViewer, model.UserStatusActive)
	if err != nil {
		t.Errorf("Update demote admin with two admins = %v, want nil", err)
	}
}

func TestUserService_Delete_LastAdmin_Rejected(t *testing.T) {
	t.Parallel()
	svc, pool := setupUserSvcEnv(t)

	// Seed admin is system=1, so it returns ErrSystemUser, not ErrLastAdmin.
	// Insert a non-system admin and disable the seed admin so u-laststop is the sole active admin.
	insertUserSvcUser(t, pool, "u-laststop", "laststop@test.com", "admin", "active")
	if _, err := pool.Exec(`UPDATE users SET status='disabled' WHERE id=?`, svcSeedAdminID); err != nil {
		t.Fatalf("disable seed admin: %v", err)
	}

	// u-laststop is now the only active admin; deleting it must return ErrLastAdmin.
	err := svc.Delete("some-viewer-id", "u-laststop")
	if err != ErrLastAdmin {
		t.Errorf("Delete last admin = %v, want ErrLastAdmin", err)
	}
}

func TestUserService_Delete_LastAdmin_AllowedWhenSecondAdminExists(t *testing.T) {
	t.Parallel()
	svc, pool := setupUserSvcEnv(t)

	// Two admins: seed admin (system=1) and u-admin3 (non-system).
	// Deleting u-admin3 should succeed; seed admin remains as active admin.
	insertUserSvcUser(t, pool, "u-admin3", "admin3@test.com", "admin", "active")

	err := svc.Delete(svcSeedAdminID, "u-admin3")
	if err != nil {
		t.Errorf("Delete non-system admin with two admins = %v, want nil", err)
	}

	// Verify u-admin3 is gone.
	var count int
	pool.QueryRow(`SELECT COUNT(*) FROM users WHERE id = ?`, "u-admin3").Scan(&count)
	if count != 0 {
		t.Errorf("u-admin3 count = %d, want 0 after Delete", count)
	}
}

// --- Self-delete ---

func TestUserService_Delete_SelfDelete_Rejected(t *testing.T) {
	t.Parallel()
	svc, pool := setupUserSvcEnv(t)
	insertUserSvcUser(t, pool, "u-self", "self@test.com", "viewer", "active")

	err := svc.Delete("u-self", "u-self")
	if err != ErrSelfDelete {
		t.Errorf("Delete self = %v, want ErrSelfDelete", err)
	}
}

// --- ResetPassword ---

func TestUserService_ResetPassword_SetsMustChangePwd(t *testing.T) {
	t.Parallel()
	svc, pool := setupUserSvcEnv(t)
	insertUserSvcUser(t, pool, "u-reset", "reset@test.com", "viewer", "active")

	// Confirm must_change_password starts at 0 (we inserted with 0).
	if getMustChangePwd(t, pool, "u-reset") {
		t.Fatal("precondition: must_change_password should be 0")
	}

	if err := svc.ResetPassword("acting-admin", "u-reset", "newResetPass"); err != nil {
		t.Fatalf("ResetPassword: %v", err)
	}

	if !getMustChangePwd(t, pool, "u-reset") {
		t.Error("must_change_password should be 1 after ResetPassword")
	}
}

func TestUserService_ResetPassword_NewPasswordWorks(t *testing.T) {
	t.Parallel()
	svc, pool := setupUserSvcEnv(t)
	insertUserSvcUser(t, pool, "u-reset2", "reset2@test.com", "viewer", "active")

	newPass := "freshNewPassword123"
	if err := svc.ResetPassword("admin", "u-reset2", newPass); err != nil {
		t.Fatalf("ResetPassword: %v", err)
	}

	// Verify the new hash is in the DB.
	var hash string
	if err := pool.QueryRow(`SELECT password_hash FROM users WHERE id = ?`, "u-reset2").Scan(&hash); err != nil {
		t.Fatalf("query hash: %v", err)
	}
	if err := auth.Verify(hash, newPass); err != nil {
		t.Errorf("Verify new password after ResetPassword = %v, want nil", err)
	}
}

// --- Create ---

func TestUserService_Create_SetsMustChangePwd(t *testing.T) {
	t.Parallel()
	svc, pool := setupUserSvcEnv(t)

	u, err := svc.Create("admin", "u-new", "new@test.com", "New User", "initpass", model.UserRoleViewer)
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if !u.MustChangePassword {
		t.Error("Create should set MustChangePassword=true")
	}

	// Verify in DB.
	if !getMustChangePwd(t, pool, "u-new") {
		t.Error("must_change_password in DB should be 1 after Create")
	}
}

func TestUserService_Create_PasswordHashVerifies(t *testing.T) {
	t.Parallel()
	svc, pool := setupUserSvcEnv(t)

	_, err := svc.Create("admin", "u-hashcheck", "hashcheck@test.com", "HC", "secretpass", model.UserRoleViewer)
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	var hash string
	if err := pool.QueryRow(`SELECT password_hash FROM users WHERE id = ?`, "u-hashcheck").Scan(&hash); err != nil {
		t.Fatalf("query hash: %v", err)
	}
	if err := auth.Verify(hash, "secretpass"); err != nil {
		t.Errorf("Verify hash after Create = %v, want nil", err)
	}
}

// --- Update profile (non-last-admin cases) ---

func TestUserService_Update_ViewerToAdmin_Works(t *testing.T) {
	t.Parallel()
	svc, pool := setupUserSvcEnv(t)
	insertUserSvcUser(t, pool, "u-promote", "promote@test.com", "viewer", "active")

	err := svc.Update("admin", "u-promote", "Promoted", model.UserRoleAdmin, model.UserStatusActive)
	if err != nil {
		t.Errorf("Update viewer to admin = %v, want nil", err)
	}

	var role string
	pool.QueryRow(`SELECT role FROM users WHERE id = ?`, "u-promote").Scan(&role)
	if role != "admin" {
		t.Errorf("role = %q, want admin after Update", role)
	}
}

// --- Acceptance: seed admin authenticates with known password ---

func TestSeedAdminLoginWithKnownCredentials(t *testing.T) {
	t.Parallel()
	dbPath := filepath.Join(t.TempDir(), "seed_accept.db")
	if err := svcCopyTemplateDB(dbPath); err != nil {
		t.Fatalf("copy: %v", err)
	}
	pool, err := db.OpenPoolExisting(dbPath, db.DefaultPoolConfig())
	if err != nil {
		t.Fatalf("open pool: %v", err)
	}
	t.Cleanup(func() { pool.Close() })

	svc := NewAuthService(pool, clock.Real())
	u, err := svc.Login("admin@nrflo.com", "nrfloAdmin", "127.0.0.1", "test")
	if err != nil {
		t.Fatalf("seed admin login: %v", err)
	}
	if u.ID != svcSeedAdminID {
		t.Errorf("user.ID = %q, want %q", u.ID, svcSeedAdminID)
	}
}
