package repo

import "database/sql"

// PurgeExecer is the minimal write surface shared by *sql.Tx and *db.Pool, letting the
// trace-purge operations below run inside a single transaction (the caller passes the tx).
type PurgeExecer interface {
	Exec(query string, args ...interface{}) (sql.Result, error)
}

// RedactAgentSessions blanks the sensitive columns on every agent_sessions row of a workflow
// instance while keeping the row itself (agent type, model, status, timing, restart counts) as
// a proof-of-execution shell. config is NOT NULL DEFAULT ” so it is cleared to ” (not NULL).
func RedactAgentSessions(q PurgeExecer, wfiID, now string) (int64, error) {
	res, err := q.Exec(`UPDATE agent_sessions
		SET prompt = NULL, system_prompt = NULL, spawn_command = NULL, result_reason = NULL,
		    spawn_token = NULL, config = '', updated_at = ?
		WHERE workflow_instance_id = ?`, now, wfiID)
	if err != nil {
		return 0, err
	}
	n, _ := res.RowsAffected()
	return n, nil
}

// DeleteAgentMessagesByWorkflowInstance deletes all agent_messages for the instance's sessions.
// Required because the sessions are kept (redacted), so ON DELETE CASCADE does not fire.
func DeleteAgentMessagesByWorkflowInstance(q PurgeExecer, wfiID string) (int64, error) {
	res, err := q.Exec(`DELETE FROM agent_messages
		WHERE session_id IN (SELECT id FROM agent_sessions WHERE workflow_instance_id = ?)`, wfiID)
	if err != nil {
		return 0, err
	}
	n, _ := res.RowsAffected()
	return n, nil
}

// DeleteFindingsByWorkflowInstance deletes every finding for the instance. workflow_instance_id
// is denormalized on both workflow_instance- and session-scope rows, so one statement covers
// both (including the workflow_final_result finding).
func DeleteFindingsByWorkflowInstance(q PurgeExecer, wfiID string) (int64, error) {
	res, err := q.Exec(`DELETE FROM findings WHERE workflow_instance_id = ?`, wfiID)
	if err != nil {
		return 0, err
	}
	n, _ := res.RowsAffected()
	return n, nil
}

// DeleteFindingsHistoryByWorkflowInstance deletes finding mutation history for the instance.
// findings_history has no workflow_instance_id column, so it deletes by workflow_instance scope
// and by the instance's session scope_ids.
func DeleteFindingsHistoryByWorkflowInstance(q PurgeExecer, wfiID string) (int64, error) {
	res1, err := q.Exec(`DELETE FROM findings_history WHERE scope = 'workflow_instance' AND scope_id = ?`, wfiID)
	if err != nil {
		return 0, err
	}
	n1, _ := res1.RowsAffected()
	res2, err := q.Exec(`DELETE FROM findings_history WHERE scope = 'session'
		AND scope_id IN (SELECT id FROM agent_sessions WHERE workflow_instance_id = ?)`, wfiID)
	if err != nil {
		return n1, err
	}
	n2, _ := res2.RowsAffected()
	return n1 + n2, nil
}

// DeleteArtifactsByWorkflowInstance deletes the artifact DB rows for the instance. Files on the
// storage backend must be removed separately (before this runs).
func DeleteArtifactsByWorkflowInstance(q PurgeExecer, wfiID string) (int64, error) {
	res, err := q.Exec(`DELETE FROM artifacts WHERE workflow_instance_id = ?`, wfiID)
	if err != nil {
		return 0, err
	}
	n, _ := res.RowsAffected()
	return n, nil
}

// DeleteErrorsByInstance deletes error_log rows recorded for the instance. errors.instance_id is
// a free-text id (not an FK), so it is matched directly.
func DeleteErrorsByInstance(q PurgeExecer, instanceID string) (int64, error) {
	res, err := q.Exec(`DELETE FROM errors WHERE instance_id = ?`, instanceID)
	if err != nil {
		return 0, err
	}
	n, _ := res.RowsAffected()
	return n, nil
}

// ClearWorkflowInstanceCallerData scrubs caller-supplied free-text from the surviving instance
// row. skip_tags is NOT NULL DEFAULT '[]' so it is reset to '[]' (not NULL).
func ClearWorkflowInstanceCallerData(q PurgeExecer, wfiID, now string) error {
	_, err := q.Exec(`UPDATE workflow_instances
		SET external_id = NULL, external_context = NULL, skip_tags = '[]', updated_at = ?
		WHERE id = ?`, now, wfiID)
	return err
}
