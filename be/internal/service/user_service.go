package service

import (
	"errors"
	"fmt"

	"github.com/google/uuid"

	"be/internal/auth"
	"be/internal/clock"
	"be/internal/db"
	"be/internal/model"
	"be/internal/repo"
)

var (
	ErrLastAdmin   = errors.New("service: cannot remove or demote the last active admin")
	ErrSelfDelete  = errors.New("service: cannot delete your own account")
	ErrSystemUser  = errors.New("service: user is system; cannot be deleted")
)

// UserService handles user management operations.
type UserService struct {
	pool  *db.Pool
	clock clock.Clock
}

// NewUserService creates a new UserService.
func NewUserService(pool *db.Pool, clk clock.Clock) *UserService {
	return &UserService{pool: pool, clock: clk}
}

// Create adds a new user with a hashed password and must_change_password=1.
func (s *UserService) Create(actingUserID, id, email, displayName, plainPassword string, role model.UserRole) (*model.User, error) {
	hash, err := auth.Hash(plainPassword)
	if err != nil {
		return nil, fmt.Errorf("user create: hash: %w", err)
	}

	u := &model.User{
		ID:                 id,
		Email:              email,
		DisplayName:        displayName,
		PasswordHash:       hash,
		Role:               role,
		Status:             model.UserStatusActive,
		MustChangePassword: true,
	}

	r := repo.NewUserRepo(s.pool, s.clock)
	if err := r.Create(u); err != nil {
		return nil, fmt.Errorf("user create: %w", err)
	}
	return u, nil
}

// Update modifies a user's profile. Enforces last-admin protection on demote/disable.
func (s *UserService) Update(actingUserID, targetID, displayName string, role model.UserRole, status model.UserStatus) error {
	r := repo.NewUserRepo(s.pool, s.clock)

	existing, err := r.Get(targetID)
	if err != nil {
		return fmt.Errorf("user update: %w", err)
	}
	if existing == nil {
		return fmt.Errorf("user update: not found")
	}

	// Last-admin protection: demoting or disabling the last active admin.
	if existing.Role == model.UserRoleAdmin && existing.Status == model.UserStatusActive {
		if role != model.UserRoleAdmin || status != model.UserStatusActive {
			count, err := r.CountActiveAdmins()
			if err != nil {
				return fmt.Errorf("user update: count admins: %w", err)
			}
			if count <= 1 {
				return ErrLastAdmin
			}
		}
	}

	return r.UpdateProfile(targetID, displayName, role, status)
}

// ResetPassword sets a new password (admin force-reset) and marks must_change_password=1.
func (s *UserService) ResetPassword(actingUserID, targetID, newPlain string) error {
	hash, err := auth.Hash(newPlain)
	if err != nil {
		return fmt.Errorf("user reset-password: hash: %w", err)
	}

	r := repo.NewUserRepo(s.pool, s.clock)
	if err := r.UpdatePassword(targetID, hash); err != nil {
		return fmt.Errorf("user reset-password: %w", err)
	}

	// Force must_change_password back to 1 after admin reset.
	_, err = s.pool.Exec(
		`UPDATE users SET must_change_password = 1 WHERE id = ?`, targetID,
	)
	return err
}

// Delete removes a user. Rejects self-delete and last-admin deletion.
func (s *UserService) Delete(actingUserID, targetID string) error {
	if actingUserID == targetID {
		return ErrSelfDelete
	}

	r := repo.NewUserRepo(s.pool, s.clock)

	existing, err := r.Get(targetID)
	if err != nil {
		return fmt.Errorf("user delete: %w", err)
	}
	if existing == nil {
		return fmt.Errorf("user delete: not found")
	}

	if existing.System {
		return ErrSystemUser
	}

	if existing.Role == model.UserRoleAdmin && existing.Status == model.UserStatusActive {
		count, err := r.CountActiveAdmins()
		if err != nil {
			return fmt.Errorf("user delete: count admins: %w", err)
		}
		if count <= 1 {
			return ErrLastAdmin
		}
	}

	return r.Delete(targetID)
}

// GenerateID returns a new user ID.
func (s *UserService) GenerateID() string {
	return "usr_" + uuid.New().String()
}
