package repo

import (
	"database/sql"
	"errors"
	"time"

	"be/internal/clock"
	"be/internal/db"
	"be/internal/model"
)

// ErrPlanStaleOrNotDraft is returned by Approve when there is no draft plan
// head matching the given revision (already approved/cancelled, or a newer
// revision landed concurrently).
var ErrPlanStaleOrNotDraft = errors.New("plan: not a draft head at the given revision")

// PlanRepo handles plan_revisions (append-only) and workflow_plans (mutable
// head) CRUD.
type PlanRepo struct {
	db    db.Querier
	clock clock.Clock
}

// NewPlanRepo creates a new PlanRepo.
func NewPlanRepo(database db.Querier, clk clock.Clock) *PlanRepo {
	return &PlanRepo{db: database, clock: clk}
}

// Append inserts the next revision for instanceID and upserts the head row's
// latest_revision + goal, leaving status untouched on conflict (a caller must
// check head.Status == draft before calling — Append itself does not enforce
// it, since a re-plan after cancel/approve is a service-layer decision).
// Never UPDATEs an existing plan_revisions row.
func (r *PlanRepo) Append(instanceID, manifest, hash, author, plannerSessionID, goal string) (int, error) {
	var revision int
	err := db.WithBusyRetry(func() error {
		rev, appendErr := r.appendOnce(instanceID, manifest, hash, author, plannerSessionID, goal)
		if appendErr != nil {
			return appendErr
		}
		revision = rev
		return nil
	})
	return revision, err
}

func (r *PlanRepo) appendOnce(instanceID, manifest, hash, author, plannerSessionID, goal string) (int, error) {
	tx, err := r.db.Begin()
	if err != nil {
		return 0, err
	}
	defer tx.Rollback() //nolint:errcheck

	var maxRevision sql.NullInt64
	if err := tx.QueryRow(`SELECT MAX(revision) FROM plan_revisions WHERE instance_id = ?`, instanceID).Scan(&maxRevision); err != nil {
		return 0, err
	}
	revision := 1
	if maxRevision.Valid {
		revision = int(maxRevision.Int64) + 1
	}

	now := r.clock.Now().UTC().Format(time.RFC3339Nano)
	if _, err := tx.Exec(
		`INSERT INTO plan_revisions (instance_id, revision, manifest, hash, author, planner_session_id, created_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?)`,
		instanceID, revision, manifest, hash, author, plannerSessionID, now,
	); err != nil {
		return 0, err
	}

	if _, err := tx.Exec(
		`INSERT INTO workflow_plans (instance_id, status, latest_revision, approved_revision, goal, created_at, updated_at)
		 VALUES (?, 'draft', ?, 0, ?, ?, ?)
		 ON CONFLICT(instance_id) DO UPDATE SET
		   latest_revision = excluded.latest_revision,
		   goal = excluded.goal,
		   updated_at = excluded.updated_at`,
		instanceID, revision, goal, now, now,
	); err != nil {
		return 0, err
	}

	if err := tx.Commit(); err != nil {
		return 0, err
	}
	return revision, nil
}

// GetHead returns the mutable plan head row for a workflow instance.
func (r *PlanRepo) GetHead(instanceID string) (*model.WorkflowPlan, error) {
	row := r.db.QueryRow(
		`SELECT instance_id, status, latest_revision, approved_revision, goal, materialized_revision, materialized_hash, created_at, updated_at
		 FROM workflow_plans WHERE instance_id = ?`, instanceID)
	return scanWorkflowPlan(row)
}

// GetRevision returns a single immutable revision.
func (r *PlanRepo) GetRevision(instanceID string, revision int) (*model.PlanRevision, error) {
	row := r.db.QueryRow(
		`SELECT instance_id, revision, manifest, hash, author, planner_session_id, created_at
		 FROM plan_revisions WHERE instance_id = ? AND revision = ?`, instanceID, revision)
	return scanPlanRevision(row)
}

// ListRevisions returns every revision for a workflow instance, oldest first.
func (r *PlanRepo) ListRevisions(instanceID string) ([]*model.PlanRevision, error) {
	rows, err := r.db.Query(
		`SELECT instance_id, revision, manifest, hash, author, planner_session_id, created_at
		 FROM plan_revisions WHERE instance_id = ? ORDER BY revision ASC`, instanceID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []*model.PlanRevision
	for rows.Next() {
		rev, err := scanPlanRevision(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, rev)
	}
	return out, nil
}

// Approve transitions a draft head to approved, guarded on the head still
// being a draft at exactly the given revision. Returns ErrPlanStaleOrNotDraft
// when the guard fails (already approved/cancelled, or a newer revision landed).
func (r *PlanRepo) Approve(instanceID string, revision int) error {
	now := r.clock.Now().UTC().Format(time.RFC3339Nano)
	result, err := r.db.Exec(
		`UPDATE workflow_plans SET status = 'approved', approved_revision = ?, updated_at = ?
		 WHERE instance_id = ? AND status = 'draft' AND latest_revision = ?`,
		revision, now, instanceID, revision,
	)
	if err != nil {
		return err
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if affected == 0 {
		return ErrPlanStaleOrNotDraft
	}
	return nil
}

// Cancel transitions a draft head to cancelled. No-op (no error) if the head
// is missing or already terminal.
func (r *PlanRepo) Cancel(instanceID string) error {
	now := r.clock.Now().UTC().Format(time.RFC3339Nano)
	_, err := r.db.Exec(
		`UPDATE workflow_plans SET status = 'cancelled', updated_at = ? WHERE instance_id = ? AND status = 'draft'`,
		now, instanceID,
	)
	return err
}

// ListExpiredDrafts returns instance ids whose plan head is still 'draft' and
// was last updated before cutoff (RFC3339Nano UTC).
func (r *PlanRepo) ListExpiredDrafts(cutoff string) ([]string, error) {
	rows, err := r.db.Query(`SELECT instance_id FROM workflow_plans WHERE status = 'draft' AND updated_at < ?`, cutoff)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var ids []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	return ids, nil
}

func scanWorkflowPlan(scanner interface{ Scan(...interface{}) error }) (*model.WorkflowPlan, error) {
	p := &model.WorkflowPlan{}
	var createdAt, updatedAt string
	err := scanner.Scan(&p.InstanceID, &p.Status, &p.LatestRevision, &p.ApprovedRevision, &p.Goal,
		&p.MaterializedRevision, &p.MaterializedHash, &createdAt, &updatedAt)
	if err == sql.ErrNoRows {
		return nil, sql.ErrNoRows
	}
	if err != nil {
		return nil, err
	}
	p.CreatedAt, _ = time.Parse(time.RFC3339Nano, createdAt)
	p.UpdatedAt, _ = time.Parse(time.RFC3339Nano, updatedAt)
	return p, nil
}

func scanPlanRevision(scanner interface{ Scan(...interface{}) error }) (*model.PlanRevision, error) {
	rev := &model.PlanRevision{}
	var createdAt string
	err := scanner.Scan(&rev.InstanceID, &rev.Revision, &rev.Manifest, &rev.Hash, &rev.Author, &rev.PlannerSessionID, &createdAt)
	if err == sql.ErrNoRows {
		return nil, sql.ErrNoRows
	}
	if err != nil {
		return nil, err
	}
	rev.CreatedAt, _ = time.Parse(time.RFC3339Nano, createdAt)
	return rev, nil
}
