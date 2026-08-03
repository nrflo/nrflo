package repo

import (
	"database/sql"
	"encoding/json"
	"time"

	"be/internal/model"
)

// ToolCallStat is one (tool_name, status) bucket's count — the shape
// service.BuildSessionStats aggregates into a per-tool distribution.
type ToolCallStat struct {
	ToolName string
	Status   string
	Count    int
}

const toolDispatchCols = `id, project_id, session_id, tool_name, input, output, status, error_msg, duration_ms, source, session_kind, workflow_instance_id, created_at`

func scanToolDispatch(scanner interface{ Scan(...interface{}) error }) (*model.ToolDispatch, error) {
	d := &model.ToolDispatch{}
	var input, output, source, sessionKind, wfiID sql.NullString
	var durationMs sql.NullInt64
	var createdAt string
	err := scanner.Scan(
		&d.ID, &d.ProjectID, &d.SessionID, &d.ToolName, &input, &output, &d.Status, &d.ErrorMsg, &durationMs,
		&source, &sessionKind, &wfiID, &createdAt,
	)
	if err != nil {
		return nil, err
	}
	d.Input = input.String
	if output.Valid {
		d.Output = &output.String
	}
	d.DurationMs = durationMs.Int64
	d.Source = source.String
	d.SessionKind = sessionKind.String
	d.WorkflowInstanceID = wfiID.String
	d.CreatedAt, _ = time.Parse(time.RFC3339Nano, createdAt)
	return d, nil
}

// ListBySession returns sessionID's recorded tool calls, newest first.
// limit<=0 defaults to 200.
func (r *DispatchRepo) ListBySession(sessionID string, limit int) ([]*model.ToolDispatch, error) {
	if limit <= 0 {
		limit = 200
	}
	rows, err := r.db.Query(`SELECT `+toolDispatchCols+` FROM tool_dispatches
		WHERE session_id = ? ORDER BY created_at DESC LIMIT ?`, sessionID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []*model.ToolDispatch
	for rows.Next() {
		d, err := scanToolDispatch(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, d)
	}
	return out, rows.Err()
}

// ToolDistribution aggregates calls-per-tool/status over sessionIDs (a flow
// graph's node set) via the canonical json_each unnest (repo/system_agent_runs.go).
// Empty sessionIDs returns nil, nil rather than every row in the table.
func (r *DispatchRepo) ToolDistribution(sessionIDs []string) ([]ToolCallStat, error) {
	if len(sessionIDs) == 0 {
		return nil, nil
	}
	idsJSON, err := json.Marshal(sessionIDs)
	if err != nil {
		return nil, err
	}
	rows, err := r.db.Query(`
		SELECT tool_name, status, COUNT(*) FROM tool_dispatches
		WHERE session_id IN (SELECT value FROM json_each(?))
		GROUP BY tool_name, status
		ORDER BY tool_name, status`, string(idsJSON))
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var stats []ToolCallStat
	for rows.Next() {
		var st ToolCallStat
		if err := rows.Scan(&st.ToolName, &st.Status, &st.Count); err != nil {
			return nil, err
		}
		stats = append(stats, st)
	}
	return stats, rows.Err()
}

// DeleteBefore purges tool_dispatches rows older than cutoff (RFC3339Nano) —
// the retention sweep called from the server's periodic cleanup loop.
// Returns the number of rows deleted.
func (r *DispatchRepo) DeleteBefore(cutoff string) (int64, error) {
	result, err := r.db.Exec(`DELETE FROM tool_dispatches WHERE created_at < ?`, cutoff)
	if err != nil {
		return 0, err
	}
	return result.RowsAffected()
}
