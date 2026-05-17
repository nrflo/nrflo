package repo

import (
	"database/sql"
	"time"
)

// HistoryRow represents a single entry in findings_history.
type HistoryRow struct {
	ID          string
	FindingID   sql.NullString
	Scope       string
	ScopeID     string
	Key         string
	Operation   string // add | append | delete
	OldValue    sql.NullString
	NewValue    sql.NullString
	ActorID     string
	ActorSource string
	CreatedAt   time.Time
}

// ListHistory returns findings_history rows ordered by created_at DESC.
// Pass key="" to list all keys for the scope/scope_id pair.
func (r *FindingRepo) ListHistory(scope, scopeID, key string, limit, offset int) ([]HistoryRow, error) {
	query := `SELECT id, finding_id, scope, scope_id, key, operation, old_value, new_value, actor_id, actor_source, created_at
		FROM findings_history
		WHERE scope=? AND scope_id=?`
	args := []interface{}{scope, scopeID}

	if key != "" {
		query += " AND key=?"
		args = append(args, key)
	}
	query += " ORDER BY created_at DESC"
	if limit > 0 {
		query += " LIMIT ? OFFSET ?"
		args = append(args, limit, offset)
	}

	rows, err := r.db.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var result []HistoryRow
	for rows.Next() {
		h, err := scanHistoryRow(rows)
		if err != nil {
			return nil, err
		}
		result = append(result, h)
	}
	return result, rows.Err()
}

// writeHistoryTx inserts a findings_history row within an existing transaction.
func writeHistoryTx(tx *sql.Tx, h HistoryRow) error {
	createdAt := h.CreatedAt.UTC().Format(time.RFC3339Nano)
	_, err := tx.Exec(
		`INSERT INTO findings_history
		 (id, finding_id, scope, scope_id, key, operation, old_value, new_value, actor_id, actor_source, created_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		h.ID, h.FindingID,
		h.Scope, h.ScopeID, h.Key,
		h.Operation, h.OldValue, h.NewValue,
		findingNullStr(h.ActorID), h.ActorSource,
		createdAt,
	)
	return err
}

func scanHistoryRow(s interface{ Scan(...interface{}) error }) (HistoryRow, error) {
	var h HistoryRow
	var createdAt string
	var actorID sql.NullString
	err := s.Scan(
		&h.ID, &h.FindingID,
		&h.Scope, &h.ScopeID, &h.Key,
		&h.Operation, &h.OldValue, &h.NewValue,
		&actorID, &h.ActorSource,
		&createdAt,
	)
	if err != nil {
		return h, err
	}
	if actorID.Valid {
		h.ActorID = actorID.String
	}
	h.CreatedAt, _ = time.Parse(time.RFC3339Nano, createdAt)
	return h, nil
}
