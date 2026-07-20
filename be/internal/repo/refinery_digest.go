package repo

import (
	"database/sql"
	"time"

	"be/internal/clock"
	"be/internal/db"
	"be/internal/model"
)

// RefineryDigestRepo handles the single-row-per-console-session
// refinery_digests head table.
type RefineryDigestRepo struct {
	db    db.Querier
	clock clock.Clock
}

// NewRefineryDigestRepo creates a new RefineryDigestRepo.
func NewRefineryDigestRepo(database db.Querier, clk clock.Clock) *RefineryDigestRepo {
	return &RefineryDigestRepo{db: database, clock: clk}
}

// Get returns the digest row for a console-chat session, or nil (no error)
// when none exists yet.
func (r *RefineryDigestRepo) Get(consoleSessionID string) (*model.RefineryDigest, error) {
	row := r.db.QueryRow(
		`SELECT console_session_id, project_id, version, content, fold_count, created_at, updated_at
		 FROM refinery_digests WHERE console_session_id = ?`, consoleSessionID,
	)
	d := &model.RefineryDigest{}
	var createdAt, updatedAt string
	err := row.Scan(&d.ConsoleSessionID, &d.ProjectID, &d.Version, &d.Content, &d.FoldCount, &createdAt, &updatedAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	d.CreatedAt, _ = time.Parse(time.RFC3339Nano, createdAt)
	d.UpdatedAt, _ = time.Parse(time.RFC3339Nano, updatedAt)
	return d, nil
}

// Upsert writes a fold result: inserts the head row on first fold, otherwise
// bumps version/fold_count and replaces content — single-row upsert
// semantics mirroring workflow_plans. Returns the row's fold_count after the
// write, for per-session fold-count logging.
func (r *RefineryDigestRepo) Upsert(consoleSessionID, projectID, content string) (int, error) {
	now := r.clock.Now().UTC().Format(time.RFC3339Nano)
	_, err := r.db.Exec(
		`INSERT INTO refinery_digests (console_session_id, project_id, version, content, fold_count, created_at, updated_at)
		 VALUES (?, ?, 1, ?, 1, ?, ?)
		 ON CONFLICT(console_session_id) DO UPDATE SET
		   content = excluded.content,
		   version = version + 1,
		   fold_count = fold_count + 1,
		   updated_at = excluded.updated_at`,
		consoleSessionID, projectID, content, now, now,
	)
	if err != nil {
		return 0, err
	}
	d, err := r.Get(consoleSessionID)
	if err != nil {
		return 0, err
	}
	return d.FoldCount, nil
}

// GetSlot returns the autonomous digest row for a (workflow_instance_id,
// node_id) slot, or nil (no error) when none exists yet. The slot is
// relaunch-stable: a kill->relaunch chain keeps the same workflow instance
// and node id, so the next session in the slot reads the prior digest.
func (r *RefineryDigestRepo) GetSlot(workflowInstanceID, nodeID string) (*model.RefineryDigest, error) {
	row := r.db.QueryRow(
		`SELECT workflow_instance_id, node_id, project_id, version, content, fold_count, created_at, updated_at
		 FROM refinery_autonomous_digests WHERE workflow_instance_id = ? AND node_id = ?`,
		workflowInstanceID, nodeID,
	)
	d := &model.RefineryDigest{}
	var createdAt, updatedAt string
	err := row.Scan(&d.WorkflowInstanceID, &d.NodeID, &d.ProjectID, &d.Version, &d.Content, &d.FoldCount, &createdAt, &updatedAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	d.CreatedAt, _ = time.Parse(time.RFC3339Nano, createdAt)
	d.UpdatedAt, _ = time.Parse(time.RFC3339Nano, updatedAt)
	return d, nil
}

// UpsertSlot writes an autonomous fold result keyed by (workflow_instance_id,
// node_id): inserts the head row on first fold, otherwise bumps
// version/fold_count and replaces content. Returns the row's fold_count
// after the write.
func (r *RefineryDigestRepo) UpsertSlot(workflowInstanceID, nodeID, projectID, content string) (int, error) {
	now := r.clock.Now().UTC().Format(time.RFC3339Nano)
	_, err := r.db.Exec(
		`INSERT INTO refinery_autonomous_digests (workflow_instance_id, node_id, project_id, version, content, fold_count, created_at, updated_at)
		 VALUES (?, ?, ?, 1, ?, 1, ?, ?)
		 ON CONFLICT(workflow_instance_id, node_id) DO UPDATE SET
		   content = excluded.content,
		   version = version + 1,
		   fold_count = fold_count + 1,
		   updated_at = excluded.updated_at`,
		workflowInstanceID, nodeID, projectID, content, now, now,
	)
	if err != nil {
		return 0, err
	}
	d, err := r.GetSlot(workflowInstanceID, nodeID)
	if err != nil {
		return 0, err
	}
	return d.FoldCount, nil
}
