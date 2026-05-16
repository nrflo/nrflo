package repo

import (
	"database/sql"
	"fmt"
	"strings"
	"time"

	"be/internal/clock"
	"be/internal/db"
	"be/internal/model"
)

// ServiceTokenRepo handles service_tokens CRUD operations.
type ServiceTokenRepo struct {
	clock clock.Clock
	db    db.Querier
}

// NewServiceTokenRepo creates a new service token repository.
func NewServiceTokenRepo(database db.Querier, clk clock.Clock) *ServiceTokenRepo {
	return &ServiceTokenRepo{db: database, clock: clk}
}

const serviceTokenCols = "id, project_id, name, token_hash, display_hint, created_at, created_by, last_used_at"

func scanServiceToken(row interface {
	Scan(dest ...any) error
}) (*model.ServiceToken, error) {
	t := &model.ServiceToken{}
	var createdAt string
	var createdBy, lastUsedAt sql.NullString
	if err := row.Scan(&t.ID, &t.ProjectID, &t.Name, &t.TokenHash, &t.DisplayHint, &createdAt, &createdBy, &lastUsedAt); err != nil {
		return nil, err
	}
	t.CreatedAt, _ = time.Parse(time.RFC3339Nano, createdAt)
	if createdBy.Valid {
		t.CreatedBy = createdBy.String
	}
	if lastUsedAt.Valid {
		ts, err := time.Parse(time.RFC3339Nano, lastUsedAt.String)
		if err == nil {
			t.LastUsedAt = &ts
		}
	}
	return t, nil
}

// Create inserts a new service token row. The caller is responsible for
// generating the id, the plaintext token, and the sha256 hash + display hint.
func (r *ServiceTokenRepo) Create(t *model.ServiceToken) error {
	now := r.clock.Now().UTC().Format(time.RFC3339Nano)
	t.CreatedAt, _ = time.Parse(time.RFC3339Nano, now)
	var createdBy sql.NullString
	if t.CreatedBy != "" {
		createdBy = sql.NullString{String: t.CreatedBy, Valid: true}
	}
	_, err := r.db.Exec(`
		INSERT INTO service_tokens (id, project_id, name, token_hash, display_hint, created_at, created_by, last_used_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, NULL)`,
		t.ID, strings.ToLower(t.ProjectID), t.Name, t.TokenHash, t.DisplayHint, now, createdBy,
	)
	return err
}

// Delete removes a service token by id. Returns an error if no row matched.
func (r *ServiceTokenRepo) Delete(id string) error {
	result, err := r.db.Exec(`DELETE FROM service_tokens WHERE id = ?`, id)
	if err != nil {
		return err
	}
	n, _ := result.RowsAffected()
	if n == 0 {
		return fmt.Errorf("service token not found: %s", id)
	}
	return nil
}

// ListAll returns every service token ordered by created_at DESC.
func (r *ServiceTokenRepo) ListAll() ([]*model.ServiceToken, error) {
	rows, err := r.db.Query(`SELECT ` + serviceTokenCols + ` FROM service_tokens ORDER BY created_at DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return collectServiceTokens(rows)
}

// ListByProject returns service tokens for a single project.
func (r *ServiceTokenRepo) ListByProject(projectID string) ([]*model.ServiceToken, error) {
	rows, err := r.db.Query(`SELECT `+serviceTokenCols+` FROM service_tokens
		WHERE LOWER(project_id) = LOWER(?) ORDER BY created_at DESC`, projectID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return collectServiceTokens(rows)
}

func collectServiceTokens(rows *sql.Rows) ([]*model.ServiceToken, error) {
	out := []*model.ServiceToken{}
	for rows.Next() {
		t, err := scanServiceToken(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, t)
	}
	return out, nil
}

// GetByHash returns the token matching the sha256 hash, or (nil, nil) if none.
func (r *ServiceTokenRepo) GetByHash(hash string) (*model.ServiceToken, error) {
	if hash == "" {
		return nil, nil
	}
	row := r.db.QueryRow(`SELECT `+serviceTokenCols+` FROM service_tokens WHERE token_hash = ?`, hash)
	t, err := scanServiceToken(row)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return t, nil
}

// TouchLastUsed updates last_used_at for a token. Best-effort; errors are
// returned but callers typically ignore them.
func (r *ServiceTokenRepo) TouchLastUsed(id string) error {
	now := r.clock.Now().UTC().Format(time.RFC3339Nano)
	_, err := r.db.Exec(`UPDATE service_tokens SET last_used_at = ? WHERE id = ?`, now, id)
	return err
}
