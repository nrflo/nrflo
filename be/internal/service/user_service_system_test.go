package service

import (
	"testing"

	"be/internal/model"
)

// TestUserService_Delete_SystemUser_Rejected verifies that deleting a system user
// returns ErrSystemUser even when a second admin exists (last-admin check would pass).
func TestUserService_Delete_SystemUser_Rejected(t *testing.T) {
	t.Parallel()
	svc, pool := setupUserSvcEnv(t)

	// Insert a second non-system admin so the last-admin check does not trigger.
	insertUserSvcUser(t, pool, "u-admin-extra", "extra@test.com", "admin", "active")

	err := svc.Delete("some-user", svcSeedAdminID)
	if err != ErrSystemUser {
		t.Errorf("Delete system user = %v, want ErrSystemUser", err)
	}
}

// TestUserService_Delete_SystemUser_BeforeLastAdmin proves the check order:
// system-user check fires before last-admin check. Even with only one active admin
// (the system user), ErrSystemUser is returned, not ErrLastAdmin.
func TestUserService_Delete_SystemUser_BeforeLastAdmin(t *testing.T) {
	t.Parallel()
	svc, _ := setupUserSvcEnv(t)

	// Seed admin is the only active admin and also system=1.
	// System check must fire first and return ErrSystemUser, not ErrLastAdmin.
	err := svc.Delete("some-user", svcSeedAdminID)
	if err != ErrSystemUser {
		t.Errorf("Delete system-only-admin = %v, want ErrSystemUser (not ErrLastAdmin)", err)
	}
}

// TestUserService_Update_SystemUser_NotBlocked confirms that Update is not blocked
// for system users — only Delete enforces the system flag.
func TestUserService_Update_SystemUser_NotBlocked(t *testing.T) {
	t.Parallel()
	svc, _ := setupUserSvcEnv(t)

	// Updating the seed admin's display name (keeping admin+active) must succeed.
	err := svc.Update("acting-user", svcSeedAdminID, "System Admin Renamed", model.UserRoleAdmin, model.UserStatusActive)
	if err != nil {
		t.Errorf("Update system user = %v, want nil", err)
	}
}
