package repo

import (
	"encoding/json"
	"time"

	"be/internal/db"
)

// EventLogEntry represents a persisted WS event
type EventLogEntry struct {
	Seq       int64           `json:"seq"`
	ProjectID string          `json:"project_id"`
	TicketID  string          `json:"ticket_id"`
	EventType string          `json:"event_type"`
	Workflow  string          `json:"workflow"`
	Payload   json.RawMessage `json:"payload"`
	CreatedAt string          `json:"created_at"`
}

// EventLogRepo handles ws_event_log persistence
type EventLogRepo struct {
	db db.Querier
}

// NewEventLogRepo creates a new event log repository
func NewEventLogRepo(database db.Querier) *EventLogRepo {
	return &EventLogRepo{db: database}
}

// Append inserts an event into the log and returns the assigned sequence number.
func (r *EventLogRepo) Append(projectID, ticketID, eventType, workflow string, payload json.RawMessage) (int64, error) {
	now := time.Now().UTC().Format(time.RFC3339Nano)
	if payload == nil {
		payload = json.RawMessage("{}")
	}
	result, err := r.db.Exec(
		`INSERT INTO ws_event_log (project_id, ticket_id, event_type, workflow, payload, created_at)
		 VALUES (?, ?, ?, ?, ?, ?)`,
		projectID, ticketID, eventType, workflow, string(payload), now,
	)
	if err != nil {
		return 0, err
	}
	return result.LastInsertId()
}

// QuerySince returns events after sinceSeq for a given scope, up to limit.
func (r *EventLogRepo) QuerySince(projectID, ticketID string, sinceSeq int64, limit int) ([]*EventLogEntry, error) {
	query := `SELECT seq, project_id, ticket_id, event_type, workflow, payload, created_at
		FROM ws_event_log
		WHERE project_id = ? AND ticket_id = ? AND seq > ?
		ORDER BY seq ASC
		LIMIT ?`
	rows, err := r.db.Query(query, projectID, ticketID, sinceSeq, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var entries []*EventLogEntry
	for rows.Next() {
		e := &EventLogEntry{}
		var payload string
		if err := rows.Scan(&e.Seq, &e.ProjectID, &e.TicketID, &e.EventType, &e.Workflow, &payload, &e.CreatedAt); err != nil {
			return nil, err
		}
		e.Payload = json.RawMessage(payload)
		entries = append(entries, e)
	}
	return entries, nil
}

// LatestSeq returns the latest sequence number for a scope, or 0 if none.
func (r *EventLogRepo) LatestSeq(projectID, ticketID string) (int64, error) {
	var seq int64
	err := r.db.QueryRow(
		`SELECT COALESCE(MAX(seq), 0) FROM ws_event_log WHERE project_id = ? AND ticket_id = ?`,
		projectID, ticketID,
	).Scan(&seq)
	return seq, err
}

// Cleanup deletes events older than the given duration.
func (r *EventLogRepo) Cleanup(olderThan time.Duration) (int64, error) {
	cutoff := time.Now().UTC().Add(-olderThan).Format(time.RFC3339Nano)
	result, err := r.db.Exec(`DELETE FROM ws_event_log WHERE created_at < ?`, cutoff)
	if err != nil {
		return 0, err
	}
	return result.RowsAffected()
}
