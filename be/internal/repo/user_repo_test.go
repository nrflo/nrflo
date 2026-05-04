package repo

import (
	"testing"
	"time"

	"be/internal/auth"
	"be/internal/clock"
	"be/internal/model"
)

func setupUserRepoDB(t *testing.T) *UserRepo {
	t.Helper()
	db := newTestDB(t)
	return NewUserRepo(db, clock.Real())
}

func insertRawUser(t *testing.T, r *UserRepo, id, email, role, status string, mustChange bool) {
	t.Helper()
	hash, err := auth.Hash("testpass123")
	if err != nil {
		t.Fatalf("hash: %v", err)
	}
	u := &model.User{
		ID: id, Email: email, DisplayName: email,
		PasswordHash: hash, Role: model.UserRole(role),
		Status: model.UserStatus(status), MustChangePassword: mustChange,
	}
	if err := r.Create(u); err != nil {
		t.Fatalf("Create user %q: %v", id, err)
	}
}

func TestUserRepo_Create_GetByEmail(t *testing.T) {
	t.Parallel()
	r := setupUserRepoDB(t)

	hash, err := auth.Hash("secret")
	if err != nil {
		t.Fatalf("hash: %v", err)
	}
	u := &model.User{
		ID: "u1", Email: "alice@example.com", DisplayName: "Alice",
		PasswordHash: hash, Role: model.UserRoleViewer,
		Status: model.UserStatusActive, MustChangePassword: true,
	}
	if err := r.Create(u); err != nil {
		t.Fatalf("Create: %v", err)
	}

	got, err := r.GetByEmail("alice@example.com")
	if err != nil {
		t.Fatalf("GetByEmail: %v", err)
	}
	if got == nil {
		t.Fatal("GetByEmail returned nil, want user")
	}
	if got.ID != "u1" {
		t.Errorf("ID = %q, want u1", got.ID)
	}
	if got.Email != "alice@example.com" {
		t.Errorf("Email = %q, want alice@example.com", got.Email)
	}
	if got.Role != model.UserRoleViewer {
		t.Errorf("Role = %q, want viewer", got.Role)
	}
	if got.Status != model.UserStatusActive {
		t.Errorf("Status = %q, want active", got.Status)
	}
	if !got.MustChangePassword {
		t.Error("MustChangePassword should be true")
	}
}

func TestUserRepo_GetByEmail_CaseInsensitive(t *testing.T) {
	t.Parallel()
	r := setupUserRepoDB(t)

	insertRawUser(t, r, "u-case", "Bob@Example.COM", "viewer", "active", false)

	cases := []string{"bob@example.com", "BOB@EXAMPLE.COM", "Bob@Example.COM", "BOB@example.com"}
	for _, email := range cases {
		got, err := r.GetByEmail(email)
		if err != nil {
			t.Fatalf("GetByEmail(%q): %v", email, err)
		}
		if got == nil {
			t.Errorf("GetByEmail(%q) returned nil, want user", email)
			continue
		}
		if got.ID != "u-case" {
			t.Errorf("GetByEmail(%q).ID = %q, want u-case", email, got.ID)
		}
	}
}

func TestUserRepo_GetByEmail_NotFound(t *testing.T) {
	t.Parallel()
	r := setupUserRepoDB(t)

	got, err := r.GetByEmail("nobody@nowhere.com")
	if err != nil {
		t.Fatalf("GetByEmail not found: %v", err)
	}
	if got != nil {
		t.Errorf("GetByEmail returned %v, want nil", got)
	}
}

func TestUserRepo_Get_NotFound(t *testing.T) {
	t.Parallel()
	r := setupUserRepoDB(t)

	got, err := r.Get("nonexistent-id")
	if err != nil {
		t.Fatalf("Get not found: %v", err)
	}
	if got != nil {
		t.Errorf("Get returned %v, want nil", got)
	}
}

func TestUserRepo_UpdateProfile(t *testing.T) {
	t.Parallel()
	fixedTime := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)
	clk := clock.NewTest(fixedTime)
	db := newTestDB(t)
	r := NewUserRepo(db, clk)

	insertRawUser(t, r, "u-profile", "profile@example.com", "viewer", "active", false)

	clk.Advance(time.Second)
	if err := r.UpdateProfile("u-profile", "New Name", model.UserRoleAdmin, model.UserStatusActive); err != nil {
		t.Fatalf("UpdateProfile: %v", err)
	}

	got, err := r.Get("u-profile")
	if err != nil {
		t.Fatalf("Get after update: %v", err)
	}
	if got.DisplayName != "New Name" {
		t.Errorf("DisplayName = %q, want New Name", got.DisplayName)
	}
	if got.Role != model.UserRoleAdmin {
		t.Errorf("Role = %q, want admin", got.Role)
	}
	if got.Status != model.UserStatusActive {
		t.Errorf("Status = %q, want active", got.Status)
	}
	want := fixedTime.Add(time.Second)
	if !got.UpdatedAt.Equal(want) {
		t.Errorf("UpdatedAt = %v, want %v", got.UpdatedAt, want)
	}
}

func TestUserRepo_UpdatePassword_ClearsMustChange(t *testing.T) {
	t.Parallel()
	fixedTime := time.Date(2025, 2, 1, 0, 0, 0, 0, time.UTC)
	clk := clock.NewTest(fixedTime)
	db := newTestDB(t)
	r := NewUserRepo(db, clk)

	insertRawUser(t, r, "u-pwd", "pwd@example.com", "viewer", "active", true)

	clk.Advance(time.Minute)
	newHash, _ := auth.Hash("newpassword")
	if err := r.UpdatePassword("u-pwd", newHash); err != nil {
		t.Fatalf("UpdatePassword: %v", err)
	}

	got, err := r.Get("u-pwd")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got.MustChangePassword {
		t.Error("MustChangePassword should be false after UpdatePassword")
	}
	if got.PasswordHash != newHash {
		t.Errorf("PasswordHash not updated")
	}
	want := fixedTime.Add(time.Minute)
	if !got.UpdatedAt.Equal(want) {
		t.Errorf("UpdatedAt = %v, want %v", got.UpdatedAt, want)
	}
}

func TestUserRepo_UpdateLastLogin(t *testing.T) {
	t.Parallel()
	fixedTime := time.Date(2025, 3, 1, 12, 0, 0, 0, time.UTC)
	clk := clock.NewTest(fixedTime)
	db := newTestDB(t)
	r := NewUserRepo(db, clk)

	insertRawUser(t, r, "u-login", "login@example.com", "viewer", "active", false)

	before, _ := r.Get("u-login")
	if before.LastLoginAt != nil {
		t.Error("LastLoginAt should be nil before first login")
	}

	clk.Advance(5 * time.Minute)
	loginTime := clk.Now()
	if err := r.UpdateLastLogin("u-login"); err != nil {
		t.Fatalf("UpdateLastLogin: %v", err)
	}

	got, err := r.Get("u-login")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got.LastLoginAt == nil {
		t.Fatal("LastLoginAt is nil after UpdateLastLogin")
	}
	if !got.LastLoginAt.Equal(loginTime) {
		t.Errorf("LastLoginAt = %v, want %v", got.LastLoginAt, loginTime)
	}
}

func TestUserRepo_CountActiveAdmins(t *testing.T) {
	t.Parallel()
	r := setupUserRepoDB(t)

	// Template DB already contains the seed admin (usr_admin_seed, active).
	count, err := r.CountActiveAdmins()
	if err != nil {
		t.Fatalf("CountActiveAdmins: %v", err)
	}
	if count < 1 {
		t.Errorf("CountActiveAdmins = %d, want >= 1 (seed admin)", count)
	}
	initial := count

	// Add a second admin.
	insertRawUser(t, r, "u-admin2", "admin2@example.com", "admin", "active", false)
	count, err = r.CountActiveAdmins()
	if err != nil {
		t.Fatalf("CountActiveAdmins after add: %v", err)
	}
	if count != initial+1 {
		t.Errorf("CountActiveAdmins = %d, want %d", count, initial+1)
	}

	// Disable the second admin — count should drop back.
	if err := r.UpdateProfile("u-admin2", "admin2@example.com", model.UserRoleAdmin, model.UserStatusDisabled); err != nil {
		t.Fatalf("UpdateProfile disable: %v", err)
	}
	count, err = r.CountActiveAdmins()
	if err != nil {
		t.Fatalf("CountActiveAdmins after disable: %v", err)
	}
	if count != initial {
		t.Errorf("CountActiveAdmins = %d, want %d (back to initial)", count, initial)
	}
}

func TestUserRepo_Delete(t *testing.T) {
	t.Parallel()
	r := setupUserRepoDB(t)

	insertRawUser(t, r, "u-del", "delete@example.com", "viewer", "active", false)

	got, err := r.Get("u-del")
	if err != nil || got == nil {
		t.Fatalf("Get before delete: err=%v user=%v", err, got)
	}

	if err := r.Delete("u-del"); err != nil {
		t.Fatalf("Delete: %v", err)
	}

	after, err := r.Get("u-del")
	if err != nil {
		t.Fatalf("Get after delete: %v", err)
	}
	if after != nil {
		t.Error("Get after Delete should return nil")
	}
}
