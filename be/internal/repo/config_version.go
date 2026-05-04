package repo

import (
	"database/sql"
	"fmt"
	"time"

	"be/internal/clock"
	"be/internal/model"
)

// ConfigVersionRepo handles versioned config snapshots
type ConfigVersionRepo struct {
	db    *sql.DB
	clock clock.Clock
}

// NewConfigVersionRepo creates a new ConfigVersionRepo.
// Accepts *sql.DB directly because Insert requires a transaction.
func NewConfigVersionRepo(database *sql.DB, clk clock.Clock) *ConfigVersionRepo {
	return &ConfigVersionRepo{db: database, clock: clk}
}

// Insert saves a new config version, auto-incrementing version per (project_id, file).
func (r *ConfigVersionRepo) Insert(v *model.ConfigVersion) error {
	now := r.clock.Now().UTC().Format(time.RFC3339Nano)
	v.CreatedAt = r.clock.Now().UTC()

	tx, err := r.db.Begin()
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback() //nolint:errcheck

	var nextVersion int
	err = tx.QueryRow(`
		SELECT COALESCE(MAX(version), 0) + 1
		FROM customer_config_versions
		WHERE project_id = ? AND file = ?`,
		v.ProjectID, v.File).Scan(&nextVersion)
	if err != nil {
		return fmt.Errorf("compute next version: %w", err)
	}
	v.Version = nextVersion

	result, err := tx.Exec(`
		INSERT INTO customer_config_versions (project_id, file, version, content, actor, created_at)
		VALUES (?, ?, ?, ?, ?, ?)`,
		v.ProjectID, v.File, v.Version, v.Content, v.Actor, now)
	if err != nil {
		return fmt.Errorf("insert config version: %w", err)
	}

	id, err := result.LastInsertId()
	if err != nil {
		return fmt.Errorf("last insert id: %w", err)
	}
	v.ID = id

	return tx.Commit()
}

// LatestVersion returns the highest version number for a (project_id, file) pair.
// Returns 0 when no versions exist.
func (r *ConfigVersionRepo) LatestVersion(projectID, file string) (int, error) {
	var version int
	err := r.db.QueryRow(`
		SELECT COALESCE(MAX(version), 0)
		FROM customer_config_versions
		WHERE project_id = ? AND file = ?`,
		projectID, file).Scan(&version)
	return version, err
}

// Get retrieves a specific version of a config file
func (r *ConfigVersionRepo) Get(projectID, file string, version int) (*model.ConfigVersion, error) {
	v := &model.ConfigVersion{}
	var createdAt string

	err := r.db.QueryRow(`
		SELECT id, project_id, file, version, content, actor, created_at
		FROM customer_config_versions
		WHERE project_id = ? AND file = ? AND version = ?`,
		projectID, file, version).Scan(
		&v.ID, &v.ProjectID, &v.File, &v.Version, &v.Content, &v.Actor, &createdAt)
	if err == sql.ErrNoRows {
		return nil, fmt.Errorf("config version not found: %w", err)
	}
	if err != nil {
		return nil, err
	}
	v.CreatedAt, _ = time.Parse(time.RFC3339Nano, createdAt)
	return v, nil
}

// History returns all versions for a (project_id, file), newest first
func (r *ConfigVersionRepo) History(projectID, file string) ([]*model.ConfigVersion, error) {
	rows, err := r.db.Query(`
		SELECT id, project_id, file, version, content, actor, created_at
		FROM customer_config_versions
		WHERE project_id = ? AND file = ?
		ORDER BY version DESC`,
		projectID, file)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var versions []*model.ConfigVersion
	for rows.Next() {
		v := &model.ConfigVersion{}
		var createdAt string
		if err := rows.Scan(&v.ID, &v.ProjectID, &v.File, &v.Version, &v.Content, &v.Actor, &createdAt); err != nil {
			return nil, err
		}
		v.CreatedAt, _ = time.Parse(time.RFC3339Nano, createdAt)
		versions = append(versions, v)
	}
	return versions, rows.Err()
}
