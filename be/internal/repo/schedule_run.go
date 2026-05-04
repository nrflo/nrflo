package repo

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"time"

	"be/internal/clock"
	"be/internal/db"
	"be/internal/model"
)

// ScheduleRunRepo handles schedule_runs persistence
type ScheduleRunRepo struct {
	clock clock.Clock
	db    db.Querier
}

// NewScheduleRunRepo creates a new schedule run repository
func NewScheduleRunRepo(database db.Querier, clk clock.Clock) *ScheduleRunRepo {
	return &ScheduleRunRepo{db: database, clock: clk}
}

const scheduleRunCols = `id, scheduled_task_id, project_id, triggered_at, status, workflows, chain_runs, error`

func scanScheduleRun(row interface{ Scan(...interface{}) error }) (*model.ScheduleRun, error) {
	r := &model.ScheduleRun{}
	var triggeredAt, workflowsJSON, chainRunsJSON string
	if err := row.Scan(&r.ID, &r.ScheduledTaskID, &r.ProjectID, &triggeredAt, &r.Status, &workflowsJSON, &chainRunsJSON, &r.Error); err != nil {
		return nil, err
	}
	r.TriggeredAt, _ = time.Parse(time.RFC3339Nano, triggeredAt)
	if err := json.Unmarshal([]byte(workflowsJSON), &r.Workflows); err != nil {
		r.Workflows = []model.ScheduleRunWorkflow{}
	}
	if err := json.Unmarshal([]byte(chainRunsJSON), &r.ChainRuns); err != nil {
		r.ChainRuns = []model.ScheduleRunChain{}
	}
	return r, nil
}

// Insert persists a new schedule run record
func (r *ScheduleRunRepo) Insert(run *model.ScheduleRun) error {
	if run.TriggeredAt.IsZero() {
		run.TriggeredAt = r.clock.Now().UTC()
	}

	wJSON, err := json.Marshal(run.Workflows)
	if err != nil {
		return err
	}
	if run.ChainRuns == nil {
		run.ChainRuns = []model.ScheduleRunChain{}
	}
	cJSON, err := json.Marshal(run.ChainRuns)
	if err != nil {
		return err
	}

	_, err = r.db.Exec(
		`INSERT INTO schedule_runs (`+scheduleRunCols+`) VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		run.ID,
		run.ScheduledTaskID,
		run.ProjectID,
		run.TriggeredAt.UTC().Format(time.RFC3339Nano),
		run.Status,
		string(wJSON),
		string(cJSON),
		run.Error,
	)
	return err
}

// UpdateStatus updates the status, workflows JSON, and error of a run
func (r *ScheduleRunRepo) UpdateStatus(id, status, workflowsJSON, errorMsg string) error {
	result, err := r.db.Exec(
		`UPDATE schedule_runs SET status=?, workflows=?, error=? WHERE id=?`,
		status, workflowsJSON, errorMsg, id,
	)
	if err != nil {
		return err
	}
	rowsAffected, _ := result.RowsAffected()
	if rowsAffected == 0 {
		return fmt.Errorf("schedule run not found: %s", id)
	}
	return nil
}

// UpdateStatusFull updates status, workflows JSON, chain_runs JSON, and error of a run.
func (r *ScheduleRunRepo) UpdateStatusFull(id, status, workflowsJSON, chainRunsJSON, errorMsg string) error {
	result, err := r.db.Exec(
		`UPDATE schedule_runs SET status=?, workflows=?, chain_runs=?, error=? WHERE id=?`,
		status, workflowsJSON, chainRunsJSON, errorMsg, id,
	)
	if err != nil {
		return err
	}
	rowsAffected, _ := result.RowsAffected()
	if rowsAffected == 0 {
		return fmt.Errorf("schedule run not found: %s", id)
	}
	return nil
}

// ListByTask returns runs for a task ordered by triggered_at DESC
func (r *ScheduleRunRepo) ListByTask(taskID string, limit, offset int) ([]*model.ScheduleRun, error) {
	rows, err := r.db.Query(
		`SELECT `+scheduleRunCols+` FROM schedule_runs WHERE scheduled_task_id=? ORDER BY triggered_at DESC LIMIT ? OFFSET ?`,
		taskID, limit, offset,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var runs []*model.ScheduleRun
	for rows.Next() {
		run, err := scanScheduleRun(rows)
		if err != nil {
			return nil, err
		}
		runs = append(runs, run)
	}

	if err := rows.Err(); err != nil {
		return nil, err
	}

	// Return empty slice rather than nil for consistent JSON serialization
	if runs == nil {
		runs = []*model.ScheduleRun{}
	}

	return runs, nil
}

// Get retrieves a single schedule run by ID
func (r *ScheduleRunRepo) Get(id string) (*model.ScheduleRun, error) {
	row := r.db.QueryRow(`SELECT `+scheduleRunCols+` FROM schedule_runs WHERE id=?`, id)
	run, err := scanScheduleRun(row)
	if err == sql.ErrNoRows {
		return nil, fmt.Errorf("schedule run not found: %s", id)
	}
	return run, err
}
