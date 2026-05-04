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
	ErrInvalidCredentials = errors.New("service: invalid email or password")
	ErrUserDisabled       = errors.New("service: user account is disabled")
)

// AuthService handles authentication operations.
type AuthService struct {
	pool  *db.Pool
	clock clock.Clock
}

// NewAuthService creates a new AuthService.
func NewAuthService(pool *db.Pool, clk clock.Clock) *AuthService {
	return &AuthService{pool: pool, clock: clk}
}

// Login authenticates a user by email and password.
// Returns the user on success. Audits login_success / login_fail.
func (s *AuthService) Login(email, plain, ip, ua string) (*model.User, error) {
	userRepo := repo.NewUserRepo(s.pool, s.clock)
	auditRepo := repo.NewAuditRepo(s.pool, s.clock)

	u, err := userRepo.GetByEmail(email)
	if err != nil {
		return nil, fmt.Errorf("auth login: %w", err)
	}

	if u == nil {
		_ = auditRepo.Append(&model.AuditEntry{
			ID:     uuid.New().String(),
			Action: "login_fail",
			IP:     ip, UserAgent: ua,
			Metadata: fmt.Sprintf(`{"reason":"user_not_found","email":%q}`, email),
		})
		return nil, ErrInvalidCredentials
	}

	if u.Status == model.UserStatusDisabled {
		_ = auditRepo.Append(&model.AuditEntry{
			ID:     uuid.New().String(),
			UserID: u.ID,
			Action: "login_fail",
			IP:     ip, UserAgent: ua,
			Metadata: `{"reason":"account_disabled"}`,
		})
		return nil, ErrUserDisabled
	}

	if err := auth.Verify(u.PasswordHash, plain); err != nil {
		_ = auditRepo.Append(&model.AuditEntry{
			ID:     uuid.New().String(),
			UserID: u.ID,
			Action: "login_fail",
			IP:     ip, UserAgent: ua,
			Metadata: `{"reason":"bad_password"}`,
		})
		return nil, ErrInvalidCredentials
	}

	if err := userRepo.UpdateLastLogin(u.ID); err != nil {
		return nil, fmt.Errorf("auth login: update last login: %w", err)
	}

	_ = auditRepo.Append(&model.AuditEntry{
		ID:     uuid.New().String(),
		UserID: u.ID,
		Action: "login_success",
		IP:     ip, UserAgent: ua,
	})

	// Refresh user to get updated last_login_at.
	u, err = userRepo.Get(u.ID)
	if err != nil {
		return nil, fmt.Errorf("auth login: refresh user: %w", err)
	}
	return u, nil
}

// ChangePassword updates the user's password after verifying the current one.
// Audits password_change on success.
func (s *AuthService) ChangePassword(userID, current, newPass, ip, ua string) error {
	userRepo := repo.NewUserRepo(s.pool, s.clock)
	auditRepo := repo.NewAuditRepo(s.pool, s.clock)

	u, err := userRepo.Get(userID)
	if err != nil {
		return fmt.Errorf("change password: %w", err)
	}
	if u == nil {
		return ErrInvalidCredentials
	}

	if err := auth.Verify(u.PasswordHash, current); err != nil {
		return ErrInvalidCredentials
	}

	hash, err := auth.Hash(newPass)
	if err != nil {
		return fmt.Errorf("change password: hash: %w", err)
	}

	if err := userRepo.UpdatePassword(userID, hash); err != nil {
		return fmt.Errorf("change password: update: %w", err)
	}

	_ = auditRepo.Append(&model.AuditEntry{
		ID:     uuid.New().String(),
		UserID: userID,
		Action: "password_change",
		IP:     ip, UserAgent: ua,
	})
	return nil
}
