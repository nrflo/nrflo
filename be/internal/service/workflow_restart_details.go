package service

import (
	"database/sql"
	"fmt"
	"strings"
	"time"
)

// loadRestartDetails queries continued sessions for a workflow instance and returns
// a map of chain root session ID -> ordered list of RestartDetail values.
func (s *WorkflowService) loadRestartDetails(wfiID string) map[string][]RestartDetail {
	details := make(map[string][]RestartDetail)
	rows, err := s.pool.Query(`
		SELECT COALESCE(ancestor_session_id, id) as chain_root, id, result_reason, started_at, ended_at, context_left
		FROM agent_sessions
		WHERE workflow_instance_id = ? AND status = 'continued' AND result_reason IS NOT NULL
		ORDER BY started_at`, wfiID)
	if err != nil {
		return details
	}
	defer rows.Close()

	type rowData struct {
		chainRoot   string
		id          string
		reason      string
		startedAt   sql.NullString
		endedAt     sql.NullString
		contextLeft sql.NullInt64
	}
	var allRows []rowData
	var sessionIDs []string

	for rows.Next() {
		var r rowData
		rows.Scan(&r.chainRoot, &r.id, &r.reason, &r.startedAt, &r.endedAt, &r.contextLeft)
		allRows = append(allRows, r)
		sessionIDs = append(sessionIDs, r.id)
	}

	// Batch-fetch message counts
	msgCounts := s.batchMessageCounts(sessionIDs)

	for _, r := range allRows {
		var durSec float64
		if r.startedAt.Valid && r.endedAt.Valid {
			if start, err := time.Parse(time.RFC3339Nano, r.startedAt.String); err == nil {
				if end, err := time.Parse(time.RFC3339Nano, r.endedAt.String); err == nil {
					durSec = end.Sub(start).Seconds()
					if durSec < 0 {
						durSec = 0
					}
				}
			}
		}
		var ctxLeft *int64
		if r.contextLeft.Valid {
			v := r.contextLeft.Int64
			ctxLeft = &v
		}
		detail := RestartDetail{
			Reason:       r.reason,
			DurationSec:  durSec,
			ContextLeft:  ctxLeft,
			MessageCount: msgCounts[r.id],
		}
		details[r.chainRoot] = append(details[r.chainRoot], detail)
	}
	return details
}

// batchMessageCounts fetches message counts for a batch of session IDs.
func (s *WorkflowService) batchMessageCounts(sessionIDs []string) map[string]int {
	counts := make(map[string]int)
	if len(sessionIDs) == 0 {
		return counts
	}
	placeholders := make([]string, len(sessionIDs))
	args := make([]interface{}, len(sessionIDs))
	for i, id := range sessionIDs {
		placeholders[i] = "?"
		args[i] = id
	}
	query := fmt.Sprintf(
		`SELECT session_id, COUNT(*) FROM agent_messages WHERE session_id IN (%s) GROUP BY session_id`,
		strings.Join(placeholders, ","),
	)
	rows, err := s.pool.Query(query, args...)
	if err != nil {
		return counts
	}
	defer rows.Close()
	for rows.Next() {
		var sessionID string
		var count int
		rows.Scan(&sessionID, &count)
		counts[sessionID] = count
	}
	return counts
}
