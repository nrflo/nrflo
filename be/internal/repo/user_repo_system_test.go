package repo

import (
	"testing"

	"be/internal/auth"
	"be/internal/clock"
	"be/internal/model"
)

// TestUserRepo_SystemFlag_Create_Get verifies that Create persists System=true
// and Get scans it back correctly.
func TestUserRepo_SystemFlag_Create_Get(t *testing.T) {
	t.Parallel()
	r := setupUserRepoDB(t)

	hash, _ := auth.Hash("testpass")
	u := &model.User{
		ID: "u-sys", Email: "sys@example.com", DisplayName: "System",
		PasswordHash: hash, Role: model.UserRoleAdmin,
		Status: model.UserStatusActive, System: true,
	}
	if err := r.Create(u); err != nil {
		t.Fatalf("Create: %v", err)
	}

	got, err := r.Get("u-sys")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got == nil {
		t.Fatal("Get returned nil, want user")
	}
	if !got.System {
		t.Error("System = false, want true after Create with System=true")
	}
}

// TestUserRepo_SystemFlag_Default verifies that System defaults to false for normal users.
func TestUserRepo_SystemFlag_Default(t *testing.T) {
	t.Parallel()
	r := setupUserRepoDB(t)

	insertRawUser(t, r, "u-normal", "normal@example.com", "viewer", "active", false)

	got, err := r.Get("u-normal")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got == nil {
		t.Fatal("Get returned nil")
	}
	if got.System {
		t.Error("System = true, want false for normal user")
	}
}

// TestUserRepo_SystemFlag_SeedAdmin verifies that the template DB has usr_admin_seed
// flagged as system=1 (set by migration 000086).
func TestUserRepo_SystemFlag_SeedAdmin(t *testing.T) {
	t.Parallel()
	db := newTestDB(t)
	r := NewUserRepo(db, clock.Real())

	got, err := r.Get("usr_admin_seed")
	if err != nil {
		t.Fatalf("Get seed admin: %v", err)
	}
	if got == nil {
		t.Fatal("seed admin not found in template DB")
	}
	if !got.System {
		t.Error("seed admin System = false, want true (set by migration 000086)")
	}
}

// TestUserRepo_SystemFlag_List verifies that List scans the system column for all rows.
func TestUserRepo_SystemFlag_List(t *testing.T) {
	t.Parallel()
	r := setupUserRepoDB(t)

	insertRawUser(t, r, "u-list-normal", "list-normal@example.com", "viewer", "active", false)

	users, err := r.List()
	if err != nil {
		t.Fatalf("List: %v", err)
	}

	for _, u := range users {
		if u.ID == "usr_admin_seed" && !u.System {
			t.Error("seed admin in List: System = false, want true")
		}
		if u.ID == "u-list-normal" && u.System {
			t.Error("normal user in List: System = true, want false")
		}
	}
}
