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

// APICredentialRepo handles api_credentials CRUD operations.
type APICredentialRepo struct {
	clock clock.Clock
	db    db.Querier
}

// NewAPICredentialRepo creates a new api credential repository.
func NewAPICredentialRepo(database db.Querier, clk clock.Clock) *APICredentialRepo {
	return &APICredentialRepo{db: database, clock: clk}
}

// Create inserts a new api credential row.
func (r *APICredentialRepo) Create(cred *model.APICredential) error {
	now := r.clock.Now().UTC().Format(time.RFC3339Nano)
	cred.CreatedAt, _ = time.Parse(time.RFC3339Nano, now)
	cred.UpdatedAt = cred.CreatedAt

	_, err := r.db.Exec(`
		INSERT INTO api_credentials (id, provider, project_id, secret_ref, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?)`,
		strings.ToLower(cred.ID),
		cred.Provider,
		cred.ProjectID,
		cred.SecretRef,
		now,
		now,
	)
	return err
}

func (r *APICredentialRepo) scan(row interface {
	Scan(...interface{}) error
}) (*model.APICredential, error) {
	cred := &model.APICredential{}
	var createdAt, updatedAt string
	var projectID sql.NullString
	if err := row.Scan(
		&cred.ID,
		&cred.Provider,
		&projectID,
		&cred.SecretRef,
		&createdAt,
		&updatedAt,
	); err != nil {
		return nil, err
	}
	if projectID.Valid {
		v := projectID.String
		cred.ProjectID = &v
	}
	cred.CreatedAt, _ = time.Parse(time.RFC3339Nano, createdAt)
	cred.UpdatedAt, _ = time.Parse(time.RFC3339Nano, updatedAt)
	return cred, nil
}

// Get returns a credential by id.
func (r *APICredentialRepo) Get(id string) (*model.APICredential, error) {
	row := r.db.QueryRow(`
		SELECT id, provider, project_id, secret_ref, created_at, updated_at
		FROM api_credentials WHERE LOWER(id) = LOWER(?)`, id)
	cred, err := r.scan(row)
	if err == sql.ErrNoRows {
		return nil, fmt.Errorf("api credential not found: %s", id)
	}
	return cred, err
}

// List returns all credentials ordered by provider, project_id (NULL last).
func (r *APICredentialRepo) List() ([]*model.APICredential, error) {
	rows, err := r.db.Query(`
		SELECT id, provider, project_id, secret_ref, created_at, updated_at
		FROM api_credentials
		ORDER BY provider ASC, project_id IS NULL ASC, project_id ASC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []*model.APICredential{}
	for rows.Next() {
		cred, err := r.scan(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, cred)
	}
	return out, nil
}

// APICredentialUpdateFields lists updatable fields. Nil pointers are skipped.
type APICredentialUpdateFields struct {
	Provider  *string
	ProjectID *string
	SecretRef *string
}

// Update applies the provided fields.
func (r *APICredentialRepo) Update(id string, fields *APICredentialUpdateFields) error {
	updates := []string{}
	args := []interface{}{}

	if fields.Provider != nil {
		updates = append(updates, "provider = ?")
		args = append(args, *fields.Provider)
	}
	if fields.ProjectID != nil {
		updates = append(updates, "project_id = ?")
		args = append(args, *fields.ProjectID)
	}
	if fields.SecretRef != nil {
		updates = append(updates, "secret_ref = ?")
		args = append(args, *fields.SecretRef)
	}

	if len(updates) == 0 {
		return nil
	}

	now := r.clock.Now().UTC().Format(time.RFC3339Nano)
	updates = append(updates, "updated_at = ?")
	args = append(args, now, id)

	query := "UPDATE api_credentials SET " + strings.Join(updates, ", ") + " WHERE LOWER(id) = LOWER(?)"
	result, err := r.db.Exec(query, args...)
	if err != nil {
		return err
	}
	if rows, _ := result.RowsAffected(); rows == 0 {
		return fmt.Errorf("api credential not found: %s", id)
	}
	return nil
}

// Delete removes a credential.
func (r *APICredentialRepo) Delete(id string) error {
	result, err := r.db.Exec("DELETE FROM api_credentials WHERE LOWER(id) = LOWER(?)", id)
	if err != nil {
		return err
	}
	if rows, _ := result.RowsAffected(); rows == 0 {
		return fmt.Errorf("api credential not found: %s", id)
	}
	return nil
}

// Resolve returns the per-project credential for the provider, falling back to
// the global (project_id IS NULL) row. Returns sql.ErrNoRows if neither exists.
func (r *APICredentialRepo) Resolve(provider, projectID string) (*model.APICredential, error) {
	if projectID != "" {
		row := r.db.QueryRow(`
			SELECT id, provider, project_id, secret_ref, created_at, updated_at
			FROM api_credentials
			WHERE provider = ? AND LOWER(project_id) = LOWER(?)
			LIMIT 1`, provider, projectID)
		cred, err := r.scan(row)
		if err == nil {
			return cred, nil
		}
		if err != sql.ErrNoRows {
			return nil, err
		}
	}
	row := r.db.QueryRow(`
		SELECT id, provider, project_id, secret_ref, created_at, updated_at
		FROM api_credentials
		WHERE provider = ? AND project_id IS NULL
		LIMIT 1`, provider)
	return r.scan(row)
}
