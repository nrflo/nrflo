package repo

import (
	"database/sql"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"

	"be/internal/db"
)

// ResetAgentSessionsInWorkflow resets sessions for an arbitrary list of phases across any
// layer to callback state and deletes their session-scoped findings, so re-run agents start
// from a clean slate and stale values from superseded attempts cannot win later reads.
// Excludes running and continued sessions.
func (r *AgentSessionRepo) ResetAgentSessionsInWorkflow(wfiID string, phases []string) error {
	if len(phases) == 0 {
		return nil
	}
	now := r.clock.Now().UTC()
	nowStr := now.Format(time.RFC3339Nano)
	placeholders := make([]string, len(phases))
	phaseArgs := make([]interface{}, len(phases))
	for i, p := range phases {
		placeholders[i] = "?"
		phaseArgs[i] = p
	}
	sessionFilter := fmt.Sprintf(
		`workflow_instance_id = ? AND phase IN (%s) AND status NOT IN ('running', 'continued')`,
		strings.Join(placeholders, ","))

	return db.WithBusyRetry(func() error {
		tx, err := r.db.Begin()
		if err != nil {
			return err
		}
		defer tx.Rollback() //nolint:errcheck // no-op once Commit succeeds
		filterArgs := append([]interface{}{wfiID}, phaseArgs...)
		if err := deleteSessionFindingsTx(tx, sessionFilter, filterArgs, now); err != nil {
			return err
		}
		updateArgs := append([]interface{}{nowStr, nowStr, wfiID}, phaseArgs...)
		if _, err := tx.Exec(fmt.Sprintf(
			`UPDATE agent_sessions SET status = 'callback', ended_at = COALESCE(ended_at, ?), updated_at = ?
			WHERE %s`, sessionFilter), updateArgs...); err != nil {
			return err
		}
		return tx.Commit()
	})
}

// deleteSessionFindingsTx deletes the session-scoped findings of the sessions matched by
// sessionFilter, recording an operation='delete' findings_history row per finding so the
// audit trail stays consistent with FindingRepo.DeleteKeys.
func deleteSessionFindingsTx(tx *sql.Tx, sessionFilter string, filterArgs []interface{}, now time.Time) error {
	rows, err := tx.Query(fmt.Sprintf(
		`SELECT id, scope_id, key, value FROM findings WHERE scope = 'session' AND scope_id IN
		 (SELECT id FROM agent_sessions WHERE %s)`, sessionFilter), filterArgs...)
	if err != nil {
		return err
	}
	type doomed struct {
		id, scopeID, key string
		value            sql.NullString
	}
	var found []doomed
	for rows.Next() {
		var d doomed
		if err := rows.Scan(&d.id, &d.scopeID, &d.key, &d.value); err != nil {
			rows.Close()
			return err
		}
		found = append(found, d)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return err
	}
	for _, d := range found {
		if _, err := tx.Exec(`DELETE FROM findings WHERE id = ?`, d.id); err != nil {
			return err
		}
		if err := writeHistoryTx(tx, HistoryRow{
			ID:    uuid.New().String(),
			Scope: "session", ScopeID: d.scopeID, Key: d.key,
			Operation:   "delete",
			OldValue:    d.value,
			ActorSource: "orchestrator",
			CreatedAt:   now,
		}); err != nil {
			return err
		}
	}
	return nil
}
