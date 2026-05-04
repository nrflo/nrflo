package repo

import (
	"database/sql"
	"fmt"
	"time"

	"be/internal/clock"
	"be/internal/db"
	"be/internal/id"
	"be/internal/model"
)

var reviewIDGen = id.New("rev")

// NrvappReviewRepo handles CRUD for nrvapp_review_items
type NrvappReviewRepo struct {
	db    db.Querier
	clock clock.Clock
}

// NewNrvappReviewRepo creates a new NrvappReviewRepo
func NewNrvappReviewRepo(database db.Querier, clk clock.Clock) *NrvappReviewRepo {
	return &NrvappReviewRepo{db: database, clock: clk}
}

// Insert creates a new review item. Sets ID and timestamps; defaults status to pending.
func (r *NrvappReviewRepo) Insert(item *model.NrvappReviewItem) error {
	newID, err := reviewIDGen.Generate()
	if err != nil {
		return fmt.Errorf("generate id: %w", err)
	}
	item.ID = newID

	now := r.clock.Now().UTC()
	item.CreatedAt = now
	item.UpdatedAt = now

	if item.Status == "" {
		item.Status = model.ReviewStatusPending
	}

	_, err = r.db.Exec(`
		INSERT INTO nrvapp_review_items
			(id, project_id, tool_name, session_id, input, output, draft, status, reject_reason, created_at, updated_at, approved_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		item.ID, item.ProjectID, item.ToolName, item.SessionID,
		item.Input, item.Output, item.Draft, item.Status, item.RejectReason,
		now.Format(time.RFC3339Nano), now.Format(time.RFC3339Nano), nil,
	)
	return err
}

// Get retrieves a review item by ID
func (r *NrvappReviewRepo) Get(id string) (*model.NrvappReviewItem, error) {
	row := r.db.QueryRow(`
		SELECT id, project_id, tool_name, session_id, input, output, draft, status, reject_reason,
		       created_at, updated_at, approved_at
		FROM nrvapp_review_items WHERE id = ?`, id)
	return scanReviewItem(row)
}

// List returns review items for a project with optional status filter and pagination
func (r *NrvappReviewRepo) List(projectID, status string, limit, offset int) ([]*model.NrvappReviewItem, error) {
	var rows *sql.Rows
	var err error

	if status != "" {
		rows, err = r.db.Query(`
			SELECT id, project_id, tool_name, session_id, input, output, draft, status, reject_reason,
			       created_at, updated_at, approved_at
			FROM nrvapp_review_items
			WHERE project_id = ? AND status = ?
			ORDER BY created_at DESC
			LIMIT ? OFFSET ?`, projectID, status, limit, offset)
	} else {
		rows, err = r.db.Query(`
			SELECT id, project_id, tool_name, session_id, input, output, draft, status, reject_reason,
			       created_at, updated_at, approved_at
			FROM nrvapp_review_items
			WHERE project_id = ?
			ORDER BY created_at DESC
			LIMIT ? OFFSET ?`, projectID, limit, offset)
	}
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var items []*model.NrvappReviewItem
	for rows.Next() {
		item, err := scanReviewItemRow(rows)
		if err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

// UpdateDraft sets the draft field for a review item
func (r *NrvappReviewRepo) UpdateDraft(id, projectID, draft string) error {
	now := r.clock.Now().UTC().Format(time.RFC3339Nano)
	result, err := r.db.Exec(`
		UPDATE nrvapp_review_items SET draft = ?, updated_at = ?
		WHERE id = ? AND project_id = ?`,
		draft, now, id, projectID)
	if err != nil {
		return err
	}
	return requireOneRow(result, "review item", id)
}

// Approve marks a review item as approved. If draft is non-null and output is null/empty,
// copies draft to output.
func (r *NrvappReviewRepo) Approve(id, projectID string) error {
	now := r.clock.Now().UTC().Format(time.RFC3339Nano)
	result, err := r.db.Exec(`
		UPDATE nrvapp_review_items
		SET status = 'approved',
		    approved_at = ?,
		    updated_at = ?,
		    output = CASE WHEN (output IS NULL OR output = '') AND draft IS NOT NULL THEN draft ELSE output END
		WHERE id = ? AND project_id = ? AND status = 'pending'`,
		now, now, id, projectID)
	if err != nil {
		return err
	}
	return requireOneRow(result, "review item", id)
}

// Reject marks a review item as rejected with an optional reason
func (r *NrvappReviewRepo) Reject(id, projectID, reason string) error {
	now := r.clock.Now().UTC().Format(time.RFC3339Nano)
	result, err := r.db.Exec(`
		UPDATE nrvapp_review_items
		SET status = 'rejected', reject_reason = ?, updated_at = ?
		WHERE id = ? AND project_id = ?`,
		reason, now, id, projectID)
	if err != nil {
		return err
	}
	return requireOneRow(result, "review item", id)
}

func requireOneRow(result sql.Result, entity, id string) error {
	n, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if n == 0 {
		return fmt.Errorf("%s not found: %s", entity, id)
	}
	return nil
}

type rowScanner interface {
	Scan(dest ...any) error
}

func scanReviewItem(row rowScanner) (*model.NrvappReviewItem, error) {
	item := &model.NrvappReviewItem{}
	var createdAt, updatedAt string
	var approvedAt *string

	err := row.Scan(
		&item.ID, &item.ProjectID, &item.ToolName, &item.SessionID,
		&item.Input, &item.Output, &item.Draft, &item.Status, &item.RejectReason,
		&createdAt, &updatedAt, &approvedAt,
	)
	if err == sql.ErrNoRows {
		return nil, fmt.Errorf("review item not found: %w", err)
	}
	if err != nil {
		return nil, err
	}

	item.CreatedAt, _ = time.Parse(time.RFC3339Nano, createdAt)
	item.UpdatedAt, _ = time.Parse(time.RFC3339Nano, updatedAt)
	if approvedAt != nil {
		t, _ := time.Parse(time.RFC3339Nano, *approvedAt)
		item.ApprovedAt = &t
	}
	return item, nil
}

func scanReviewItemRow(rows *sql.Rows) (*model.NrvappReviewItem, error) {
	item := &model.NrvappReviewItem{}
	var createdAt, updatedAt string
	var approvedAt *string

	err := rows.Scan(
		&item.ID, &item.ProjectID, &item.ToolName, &item.SessionID,
		&item.Input, &item.Output, &item.Draft, &item.Status, &item.RejectReason,
		&createdAt, &updatedAt, &approvedAt,
	)
	if err != nil {
		return nil, err
	}

	item.CreatedAt, _ = time.Parse(time.RFC3339Nano, createdAt)
	item.UpdatedAt, _ = time.Parse(time.RFC3339Nano, updatedAt)
	if approvedAt != nil {
		t, _ := time.Parse(time.RFC3339Nano, *approvedAt)
		item.ApprovedAt = &t
	}
	return item, nil
}
