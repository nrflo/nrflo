package model

import "time"

// UserRole represents the role of a user.
type UserRole string

const (
	UserRoleAdmin  UserRole = "admin"
	UserRoleViewer UserRole = "viewer"
)

// UserStatus represents the status of a user.
type UserStatus string

const (
	UserStatusActive   UserStatus = "active"
	UserStatusDisabled UserStatus = "disabled"
)

// User represents an nrflo user account.
type User struct {
	ID                 string     `json:"id"`
	Email              string     `json:"email"`
	DisplayName        string     `json:"display_name"`
	PasswordHash       string     `json:"-"` // never serialized
	Role               UserRole   `json:"role"`
	Status             UserStatus `json:"status"`
	MustChangePassword bool       `json:"must_change_password"`
	CreatedAt          time.Time  `json:"created_at"`
	UpdatedAt          time.Time  `json:"updated_at"`
	LastLoginAt        *time.Time `json:"last_login_at,omitempty"`
}
