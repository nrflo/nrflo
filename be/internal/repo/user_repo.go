package repo

import (
	"database/sql"
	"fmt"
	"time"

	"be/internal/clock"
	"be/internal/db"
	"be/internal/model"
)

// UserRepo handles user CRUD operations.
type UserRepo struct {
	db    db.Querier
	clock clock.Clock
}

// NewUserRepo creates a new UserRepo.
func NewUserRepo(database db.Querier, clk clock.Clock) *UserRepo {
	return &UserRepo{db: database, clock: clk}
}

const userCols = `id, email, display_name, password_hash, role, status, must_change_password, created_at, updated_at, last_login_at, system`

func scanUser(s interface{ Scan(...interface{}) error }) (*model.User, error) {
	u := &model.User{}
	var createdAt, updatedAt string
	var lastLoginAt sql.NullString
	err := s.Scan(
		&u.ID, &u.Email, &u.DisplayName, &u.PasswordHash,
		&u.Role, &u.Status, &u.MustChangePassword,
		&createdAt, &updatedAt, &lastLoginAt, &u.System,
	)
	if err != nil {
		return nil, err
	}
	u.CreatedAt, _ = time.Parse(time.RFC3339Nano, createdAt)
	u.UpdatedAt, _ = time.Parse(time.RFC3339Nano, updatedAt)
	if lastLoginAt.Valid && lastLoginAt.String != "" {
		t, _ := time.Parse(time.RFC3339Nano, lastLoginAt.String)
		u.LastLoginAt = &t
	}
	return u, nil
}

// Get returns a user by ID.
func (r *UserRepo) Get(id string) (*model.User, error) {
	row := r.db.QueryRow(`SELECT `+userCols+` FROM users WHERE id = ?`, id)
	u, err := scanUser(row)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	return u, err
}

// GetByEmail returns a user by email (case-insensitive).
func (r *UserRepo) GetByEmail(email string) (*model.User, error) {
	row := r.db.QueryRow(`SELECT `+userCols+` FROM users WHERE email = ? COLLATE NOCASE`, email)
	u, err := scanUser(row)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	return u, err
}

// List returns all users ordered by created_at ASC.
func (r *UserRepo) List() ([]*model.User, error) {
	rows, err := r.db.Query(`SELECT ` + userCols + ` FROM users ORDER BY created_at ASC`)
	if err != nil {
		return nil, fmt.Errorf("list users: %w", err)
	}
	defer rows.Close()

	var result []*model.User
	for rows.Next() {
		u, err := scanUser(rows)
		if err != nil {
			return nil, fmt.Errorf("scan user: %w", err)
		}
		result = append(result, u)
	}
	return result, rows.Err()
}

// Create inserts a new user. CreatedAt and UpdatedAt are set via injected clock.
func (r *UserRepo) Create(u *model.User) error {
	now := r.clock.Now().UTC().Format(time.RFC3339Nano)
	u.CreatedAt, _ = time.Parse(time.RFC3339Nano, now)
	u.UpdatedAt = u.CreatedAt

	_, err := r.db.Exec(
		`INSERT INTO users (`+userCols+`) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		u.ID, u.Email, u.DisplayName, u.PasswordHash,
		u.Role, u.Status, u.MustChangePassword,
		now, now, nil, u.System,
	)
	return err
}

// UpdateProfile updates display_name, role, and status.
func (r *UserRepo) UpdateProfile(id, displayName string, role model.UserRole, status model.UserStatus) error {
	now := r.clock.Now().UTC().Format(time.RFC3339Nano)
	_, err := r.db.Exec(
		`UPDATE users SET display_name = ?, role = ?, status = ?, updated_at = ? WHERE id = ?`,
		displayName, role, status, now, id,
	)
	return err
}

// UpdatePassword updates the password hash and clears must_change_password.
func (r *UserRepo) UpdatePassword(id, passwordHash string) error {
	now := r.clock.Now().UTC().Format(time.RFC3339Nano)
	_, err := r.db.Exec(
		`UPDATE users SET password_hash = ?, must_change_password = 0, updated_at = ? WHERE id = ?`,
		passwordHash, now, id,
	)
	return err
}

// UpdateLastLogin records the login timestamp.
func (r *UserRepo) UpdateLastLogin(id string) error {
	now := r.clock.Now().UTC().Format(time.RFC3339Nano)
	_, err := r.db.Exec(
		`UPDATE users SET last_login_at = ?, updated_at = ? WHERE id = ?`,
		now, now, id,
	)
	return err
}

// CountActiveAdmins returns the number of active admin users.
func (r *UserRepo) CountActiveAdmins() (int, error) {
	var count int
	err := r.db.QueryRow(
		`SELECT COUNT(*) FROM users WHERE role = 'admin' AND status = 'active'`,
	).Scan(&count)
	return count, err
}

// Delete removes a user by ID.
func (r *UserRepo) Delete(id string) error {
	_, err := r.db.Exec(`DELETE FROM users WHERE id = ?`, id)
	return err
}
