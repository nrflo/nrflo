package repo

import (
	"fmt"
	"strings"
	"time"

	"be/internal/clock"
	"be/internal/db"
	"be/internal/model"
)

// AuditRepo handles audit_log persistence.
type AuditRepo struct {
	db    db.Querier
	clock clock.Clock
}

// NewAuditRepo creates a new AuditRepo.
func NewAuditRepo(database db.Querier, clk clock.Clock) *AuditRepo {
	return &AuditRepo{db: database, clock: clk}
}

const auditCols = `id, user_id, action, resource_type, resource_id, ip, user_agent, metadata, created_at`

func scanAuditEntry(s interface{ Scan(...interface{}) error }) (*model.AuditEntry, error) {
	e := &model.AuditEntry{}
	var createdAt string
	err := s.Scan(
		&e.ID, &e.UserID, &e.Action,
		&e.ResourceType, &e.ResourceID,
		&e.IP, &e.UserAgent, &e.Metadata,
		&createdAt,
	)
	if err != nil {
		return nil, err
	}
	e.CreatedAt, _ = time.Parse(time.RFC3339Nano, createdAt)
	return e, nil
}

// Append inserts a new audit entry. CreatedAt is set via injected clock if zero.
func (r *AuditRepo) Append(e *model.AuditEntry) error {
	if e.CreatedAt.IsZero() {
		e.CreatedAt = r.clock.Now().UTC()
	}
	createdAt := e.CreatedAt.UTC().Format(time.RFC3339Nano)

	metadata := e.Metadata
	if metadata == "" {
		metadata = "{}"
	}

	_, err := r.db.Exec(
		`INSERT INTO audit_log (`+auditCols+`) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		e.ID, e.UserID, e.Action,
		e.ResourceType, e.ResourceID,
		e.IP, e.UserAgent, metadata,
		createdAt,
	)
	return err
}

// List returns paginated audit log entries matching the filter.
// Returns entries and total count.
func (r *AuditRepo) List(f model.AuditFilter, page, perPage int) ([]*model.AuditEntry, int, error) {
	where, args := auditWhere(f)

	var total int
	if err := r.db.QueryRow(
		`SELECT COUNT(*) FROM audit_log`+where, args...,
	).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("count audit: %w", err)
	}

	if total == 0 {
		return nil, 0, nil
	}

	offset := (page - 1) * perPage
	query := `SELECT ` + auditCols + ` FROM audit_log` + where +
		` ORDER BY created_at DESC LIMIT ? OFFSET ?`
	queryArgs := append(args, perPage, offset)

	rows, err := r.db.Query(query, queryArgs...)
	if err != nil {
		return nil, 0, fmt.Errorf("list audit: %w", err)
	}
	defer rows.Close()

	var result []*model.AuditEntry
	for rows.Next() {
		e, err := scanAuditEntry(rows)
		if err != nil {
			return nil, 0, fmt.Errorf("scan audit entry: %w", err)
		}
		result = append(result, e)
	}
	return result, total, rows.Err()
}

func auditWhere(f model.AuditFilter) (string, []interface{}) {
	var clauses []string
	var args []interface{}

	if f.UserID != "" {
		clauses = append(clauses, "user_id = ?")
		args = append(args, f.UserID)
	}
	if f.Action != "" {
		clauses = append(clauses, "action = ?")
		args = append(args, f.Action)
	}
	if f.Since != nil {
		clauses = append(clauses, "created_at >= ?")
		args = append(args, f.Since.UTC().Format(time.RFC3339Nano))
	}
	if f.Until != nil {
		clauses = append(clauses, "created_at <= ?")
		args = append(args, f.Until.UTC().Format(time.RFC3339Nano))
	}

	if len(clauses) == 0 {
		return "", args
	}
	return " WHERE " + strings.Join(clauses, " AND "), args
}
