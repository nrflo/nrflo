package repo

import (
	"strings"
	"time"

	"be/internal/clock"
	"be/internal/db"
	"be/internal/model"
)

// TicketRefRepo handles ticket_refs CRUD operations.
type TicketRefRepo struct {
	clock clock.Clock
	db    db.Querier
}

// NewTicketRefRepo creates a new TicketRefRepo.
func NewTicketRefRepo(database db.Querier, clk clock.Clock) *TicketRefRepo {
	return &TicketRefRepo{db: database, clock: clk}
}

// Create inserts a single ticket ref and sets its ID and CreatedAt.
func (r *TicketRefRepo) Create(ref *model.TicketRef) error {
	now := r.clock.Now().UTC().Format(time.RFC3339Nano)
	ref.ProjectID = strings.ToLower(ref.ProjectID)
	ref.TicketID = strings.ToLower(ref.TicketID)

	result, err := r.db.Exec(`
		INSERT INTO ticket_refs (project_id, ticket_id, kind, url, label, created_at)
		VALUES (?, ?, ?, ?, ?, ?)`,
		ref.ProjectID, ref.TicketID, ref.Kind, ref.URL, ref.Label, now,
	)
	if err != nil {
		return err
	}
	id, err := result.LastInsertId()
	if err != nil {
		return err
	}
	ref.ID = id
	ref.CreatedAt, _ = time.Parse(time.RFC3339Nano, now)
	return nil
}

// BulkCreate inserts multiple ticket refs in a single transaction.
func (r *TicketRefRepo) BulkCreate(refs []*model.TicketRef) error {
	if len(refs) == 0 {
		return nil
	}
	now := r.clock.Now().UTC().Format(time.RFC3339Nano)

	tx, err := r.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback() //nolint:errcheck

	stmt, err := tx.Prepare(`
		INSERT INTO ticket_refs (project_id, ticket_id, kind, url, label, created_at)
		VALUES (?, ?, ?, ?, ?, ?)`)
	if err != nil {
		return err
	}
	defer stmt.Close()

	for _, ref := range refs {
		ref.ProjectID = strings.ToLower(ref.ProjectID)
		ref.TicketID = strings.ToLower(ref.TicketID)
		result, err := stmt.Exec(ref.ProjectID, ref.TicketID, ref.Kind, ref.URL, ref.Label, now)
		if err != nil {
			return err
		}
		id, err := result.LastInsertId()
		if err != nil {
			return err
		}
		ref.ID = id
		ref.CreatedAt, _ = time.Parse(time.RFC3339Nano, now)
	}

	return tx.Commit()
}

// ListByTicket returns all refs for a ticket ordered by created_at ASC.
func (r *TicketRefRepo) ListByTicket(projectID, ticketID string) ([]*model.TicketRef, error) {
	rows, err := r.db.Query(`
		SELECT id, project_id, ticket_id, kind, url, label, created_at
		FROM ticket_refs
		WHERE LOWER(project_id) = LOWER(?) AND LOWER(ticket_id) = LOWER(?)
		ORDER BY created_at ASC`, projectID, ticketID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var refs []*model.TicketRef
	for rows.Next() {
		ref := &model.TicketRef{}
		var createdAt string
		if err := rows.Scan(&ref.ID, &ref.ProjectID, &ref.TicketID, &ref.Kind, &ref.URL, &ref.Label, &createdAt); err != nil {
			return nil, err
		}
		ref.CreatedAt, _ = time.Parse(time.RFC3339Nano, createdAt)
		refs = append(refs, ref)
	}
	return refs, nil
}
