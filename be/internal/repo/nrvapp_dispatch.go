package repo

import (
	"fmt"
	"time"

	"be/internal/clock"
	"be/internal/db"
	"be/internal/id"
	"be/internal/model"
)

var dispatchIDGen = id.New("disp")

// NrvappDispatchRepo handles CRUD for nrvapp_tool_dispatches
type NrvappDispatchRepo struct {
	db    db.Querier
	clock clock.Clock
}

// NewNrvappDispatchRepo creates a new NrvappDispatchRepo
func NewNrvappDispatchRepo(database db.Querier, clk clock.Clock) *NrvappDispatchRepo {
	return &NrvappDispatchRepo{db: database, clock: clk}
}

// Insert records a tool dispatch event
func (r *NrvappDispatchRepo) Insert(d *model.NrvappToolDispatch) error {
	newID, err := dispatchIDGen.Generate()
	if err != nil {
		return fmt.Errorf("generate id: %w", err)
	}
	d.ID = newID

	now := r.clock.Now().UTC()
	d.CreatedAt = now

	_, err = r.db.Exec(`
		INSERT INTO nrvapp_tool_dispatches
			(id, project_id, session_id, tool_name, input, output, status, error_msg, duration_ms, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		d.ID, d.ProjectID, d.SessionID, d.ToolName,
		d.Input, d.Output, d.Status, d.ErrorMsg, d.DurationMs,
		now.Format(time.RFC3339Nano),
	)
	return err
}

// ListSummary returns aggregate stats for dispatches since the given time
func (r *NrvappDispatchRepo) ListSummary(projectID string, since time.Time) (*model.DispatchSummary, error) {
	rows, err := r.db.Query(`
		SELECT duration_ms, status
		FROM nrvapp_tool_dispatches
		WHERE project_id = ? AND created_at >= ?
		ORDER BY duration_ms ASC`,
		projectID, since.UTC().Format(time.RFC3339Nano))
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var durations []int64
	summary := &model.DispatchSummary{}
	for rows.Next() {
		var ms int64
		var status string
		if err := rows.Scan(&ms, &status); err != nil {
			return nil, err
		}
		durations = append(durations, ms)
		summary.Total++
		if status == model.DispatchStatusSuccess {
			summary.Success++
		} else {
			summary.Error++
		}
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	n := len(durations)
	if n > 0 {
		summary.P50Ms = durations[p50Index(n)]
		summary.P95Ms = durations[p95Index(n)]
	}
	return summary, nil
}

// EditRateByTool returns per-tool review outcome ratios by joining dispatches with review items
func (r *NrvappDispatchRepo) EditRateByTool(projectID string, since time.Time) ([]*model.EditRateRow, error) {
	rows, err := r.db.Query(`
		SELECT
			d.tool_name,
			COUNT(CASE WHEN r.status = 'approved' AND (r.draft IS NULL OR r.draft = r.input) THEN 1 END) AS approve_no_edits,
			COUNT(CASE WHEN r.status = 'approved' AND r.draft IS NOT NULL AND r.draft != r.input THEN 1 END) AS approve_with_edits,
			COUNT(CASE WHEN r.status = 'rejected' THEN 1 END) AS rejected
		FROM nrvapp_tool_dispatches d
		LEFT JOIN nrvapp_review_items r
			ON r.project_id = d.project_id
			AND r.session_id = d.session_id
			AND r.tool_name = d.tool_name
		WHERE d.project_id = ? AND d.created_at >= ?
		GROUP BY d.tool_name
		ORDER BY d.tool_name`,
		projectID, since.UTC().Format(time.RFC3339Nano))
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var result []*model.EditRateRow
	for rows.Next() {
		row := &model.EditRateRow{}
		if err := rows.Scan(&row.ToolName, &row.ApproveNoEdits, &row.ApproveWithEdits, &row.Rejected); err != nil {
			return nil, err
		}
		result = append(result, row)
	}
	return result, rows.Err()
}

// Throughput returns bucketed dispatch counts over time
func (r *NrvappDispatchRepo) Throughput(projectID string, since time.Time, bucketSec int) ([]*model.ThroughputPoint, error) {
	if bucketSec <= 0 {
		bucketSec = 60
	}
	rows, err := r.db.Query(`
		SELECT
			(unixepoch(created_at) / ?) * ? AS bucket_ts,
			COUNT(*) AS cnt
		FROM nrvapp_tool_dispatches
		WHERE project_id = ? AND created_at >= ?
		GROUP BY bucket_ts
		ORDER BY bucket_ts`,
		bucketSec, bucketSec,
		projectID, since.UTC().Format(time.RFC3339Nano))
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var points []*model.ThroughputPoint
	for rows.Next() {
		var bucketTs int64
		var cnt int
		if err := rows.Scan(&bucketTs, &cnt); err != nil {
			return nil, err
		}
		points = append(points, &model.ThroughputPoint{
			BucketStart: time.Unix(bucketTs, 0).UTC(),
			Count:       cnt,
		})
	}
	return points, rows.Err()
}

func p50Index(n int) int {
	return percentileIndex(n, 50)
}

func p95Index(n int) int {
	return percentileIndex(n, 95)
}

func percentileIndex(n, p int) int {
	idx := (n * p / 100)
	if idx >= n {
		idx = n - 1
	}
	return idx
}
