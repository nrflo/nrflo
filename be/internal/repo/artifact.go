package repo

import (
	"database/sql"
	"fmt"
	"time"

	"be/internal/clock"
	"be/internal/db"
	"be/internal/model"
)

// ArtifactRepo handles artifacts CRUD operations.
type ArtifactRepo struct {
	clock clock.Clock
	db    db.Querier
}

// NewArtifactRepo creates a new artifact repository.
func NewArtifactRepo(database db.Querier, clk clock.Clock) *ArtifactRepo {
	return &ArtifactRepo{db: database, clock: clk}
}

const artifactCols = "id, project_id, workflow_instance_id, name, type, path_key, size_bytes, content_type, source, created_by_session, created_at, updated_at"

func scanArtifact(row interface {
	Scan(dest ...any) error
}) (*model.Artifact, error) {
	a := &model.Artifact{}
	var createdAt, updatedAt string
	var contentType, createdBySession sql.NullString
	if err := row.Scan(
		&a.ID, &a.ProjectID, &a.WorkflowInstanceID, &a.Name, &a.Type,
		&a.PathKey, &a.SizeBytes, &contentType, &a.Source, &createdBySession,
		&createdAt, &updatedAt,
	); err != nil {
		return nil, err
	}
	a.CreatedAt, _ = time.Parse(time.RFC3339Nano, createdAt)
	a.UpdatedAt, _ = time.Parse(time.RFC3339Nano, updatedAt)
	if contentType.Valid {
		a.ContentType = contentType.String
	}
	if createdBySession.Valid {
		a.CreatedBySession = createdBySession.String
	}
	return a, nil
}

// Create inserts a new artifact row.
func (r *ArtifactRepo) Create(a *model.Artifact) error {
	now := r.clock.Now().UTC().Format(time.RFC3339Nano)
	a.CreatedAt, _ = time.Parse(time.RFC3339Nano, now)
	a.UpdatedAt = a.CreatedAt
	var contentType, createdBySession sql.NullString
	if a.ContentType != "" {
		contentType = sql.NullString{String: a.ContentType, Valid: true}
	}
	if a.CreatedBySession != "" {
		createdBySession = sql.NullString{String: a.CreatedBySession, Valid: true}
	}
	_, err := r.db.Exec(`
		INSERT INTO artifacts (id, project_id, workflow_instance_id, name, type, path_key, size_bytes, content_type, source, created_by_session, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		a.ID, a.ProjectID, a.WorkflowInstanceID, a.Name, a.Type, a.PathKey,
		a.SizeBytes, contentType, a.Source, createdBySession, now, now,
	)
	return err
}

// Get returns an artifact by ID.
func (r *ArtifactRepo) Get(id string) (*model.Artifact, error) {
	row := r.db.QueryRow(`SELECT `+artifactCols+` FROM artifacts WHERE id = ?`, id)
	a, err := scanArtifact(row)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	return a, err
}

// List returns all artifacts for a workflow instance ordered by created_at ASC.
func (r *ArtifactRepo) List(workflowInstanceID string) ([]*model.Artifact, error) {
	rows, err := r.db.Query(`SELECT `+artifactCols+` FROM artifacts WHERE workflow_instance_id = ? ORDER BY created_at ASC`, workflowInstanceID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return collectArtifacts(rows)
}

// ListByProject returns all artifacts for a project ordered by created_at ASC.
func (r *ArtifactRepo) ListByProject(projectID string) ([]*model.Artifact, error) {
	rows, err := r.db.Query(`SELECT `+artifactCols+` FROM artifacts WHERE project_id = ? ORDER BY created_at ASC`, projectID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return collectArtifacts(rows)
}

// Delete removes an artifact by id. Returns an error if no row matched.
func (r *ArtifactRepo) Delete(id string) error {
	result, err := r.db.Exec(`DELETE FROM artifacts WHERE id = ?`, id)
	if err != nil {
		return err
	}
	n, _ := result.RowsAffected()
	if n == 0 {
		return fmt.Errorf("artifact not found: %s", id)
	}
	return nil
}

// ExistsByNameForInstance returns true if an artifact with the given name exists
// for the specified workflow instance.
func (r *ArtifactRepo) ExistsByNameForInstance(workflowInstanceID, name string) (bool, error) {
	var exists int
	err := r.db.QueryRow(
		`SELECT 1 FROM artifacts WHERE workflow_instance_id = ? AND name = ? LIMIT 1`,
		workflowInstanceID, name,
	).Scan(&exists)
	if err == sql.ErrNoRows {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return true, nil
}

func collectArtifacts(rows *sql.Rows) ([]*model.Artifact, error) {
	out := []*model.Artifact{}
	for rows.Next() {
		a, err := scanArtifact(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, a)
	}
	return out, nil
}
