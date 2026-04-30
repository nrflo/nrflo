package repo

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"be/internal/clock"
	"be/internal/db"
	"be/internal/model"
)

// ScheduledTaskRepo handles scheduled_tasks CRUD
type ScheduledTaskRepo struct {
	clock clock.Clock
	db    db.Querier
}

// NewScheduledTaskRepo creates a new scheduled task repository
func NewScheduledTaskRepo(database db.Querier, clk clock.Clock) *ScheduledTaskRepo {
	return &ScheduledTaskRepo{db: database, clock: clk}
}

func scanScheduledTask(
	id, projectID, name, description, cronExpr *string,
	workflowsJSON *string,
	enabled *int,
	lastTriggeredAt, nextRunAt sql.NullString,
	createdAt, updatedAt *string,
) (*model.ScheduledTask, error) {
	t := &model.ScheduledTask{
		ID:             *id,
		ProjectID:      *projectID,
		Name:           *name,
		Description:    *description,
		CronExpression: *cronExpr,
		Enabled:        *enabled != 0,
	}

	if err := json.Unmarshal([]byte(*workflowsJSON), &t.Workflows); err != nil {
		t.Workflows = []string{}
	}

	t.CreatedAt, _ = time.Parse(time.RFC3339Nano, *createdAt)
	t.UpdatedAt, _ = time.Parse(time.RFC3339Nano, *updatedAt)

	if lastTriggeredAt.Valid && lastTriggeredAt.String != "" {
		ts, err := time.Parse(time.RFC3339Nano, lastTriggeredAt.String)
		if err == nil {
			t.LastTriggeredAt = &ts
		}
	}
	if nextRunAt.Valid && nextRunAt.String != "" {
		ts, err := time.Parse(time.RFC3339Nano, nextRunAt.String)
		if err == nil {
			t.NextRunAt = &ts
		}
	}

	return t, nil
}

const scheduledTaskCols = `id, project_id, name, description, cron_expression, workflows, enabled, last_triggered_at, next_run_at, created_at, updated_at`

func (r *ScheduledTaskRepo) scanRow(row interface{ Scan(...interface{}) error }) (*model.ScheduledTask, error) {
	var id, projectID, name, description, cronExpr, workflowsJSON, createdAt, updatedAt string
	var enabled int
	var lastTriggeredAt, nextRunAt sql.NullString

	if err := row.Scan(&id, &projectID, &name, &description, &cronExpr, &workflowsJSON, &enabled, &lastTriggeredAt, &nextRunAt, &createdAt, &updatedAt); err != nil {
		return nil, err
	}
	return scanScheduledTask(&id, &projectID, &name, &description, &cronExpr, &workflowsJSON, &enabled, lastTriggeredAt, nextRunAt, &createdAt, &updatedAt)
}

// Create inserts a new scheduled task
func (r *ScheduledTaskRepo) Create(task *model.ScheduledTask) error {
	now := r.clock.Now().UTC().Format(time.RFC3339Nano)
	task.CreatedAt, _ = time.Parse(time.RFC3339Nano, now)
	task.UpdatedAt = task.CreatedAt

	wJSON, err := json.Marshal(task.Workflows)
	if err != nil {
		return err
	}

	_, err = r.db.Exec(
		`INSERT INTO scheduled_tasks (`+scheduledTaskCols+`) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		strings.ToLower(task.ID),
		strings.ToLower(task.ProjectID),
		task.Name,
		task.Description,
		task.CronExpression,
		string(wJSON),
		boolToInt(task.Enabled),
		nullableTime(task.LastTriggeredAt),
		nullableTime(task.NextRunAt),
		now,
		now,
	)
	return err
}

// Get retrieves a scheduled task by ID
func (r *ScheduledTaskRepo) Get(id string) (*model.ScheduledTask, error) {
	row := r.db.QueryRow(
		`SELECT `+scheduledTaskCols+` FROM scheduled_tasks WHERE LOWER(id) = LOWER(?)`, id)
	t, err := r.scanRow(row)
	if err == sql.ErrNoRows {
		return nil, fmt.Errorf("scheduled task not found: %s", id)
	}
	return t, err
}

// List retrieves all scheduled tasks for a project
func (r *ScheduledTaskRepo) List(projectID string) ([]*model.ScheduledTask, error) {
	rows, err := r.db.Query(
		`SELECT `+scheduledTaskCols+` FROM scheduled_tasks WHERE LOWER(project_id) = LOWER(?) ORDER BY created_at ASC`,
		projectID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var tasks []*model.ScheduledTask
	for rows.Next() {
		t, err := r.scanRow(rows)
		if err != nil {
			return nil, err
		}
		tasks = append(tasks, t)
	}
	return tasks, rows.Err()
}

// Update performs a partial update on a scheduled task
func (r *ScheduledTaskRepo) Update(task *model.ScheduledTask) error {
	now := r.clock.Now().UTC().Format(time.RFC3339Nano)

	wJSON, err := json.Marshal(task.Workflows)
	if err != nil {
		return err
	}

	result, err := r.db.Exec(
		`UPDATE scheduled_tasks SET name=?, description=?, cron_expression=?, workflows=?, enabled=?, last_triggered_at=?, next_run_at=?, updated_at=? WHERE LOWER(id) = LOWER(?)`,
		task.Name,
		task.Description,
		task.CronExpression,
		string(wJSON),
		boolToInt(task.Enabled),
		nullableTime(task.LastTriggeredAt),
		nullableTime(task.NextRunAt),
		now,
		task.ID,
	)
	if err != nil {
		return err
	}
	rowsAffected, _ := result.RowsAffected()
	if rowsAffected == 0 {
		return fmt.Errorf("scheduled task not found: %s", task.ID)
	}
	task.UpdatedAt, _ = time.Parse(time.RFC3339Nano, now)
	return nil
}

// Delete removes a scheduled task by ID
func (r *ScheduledTaskRepo) Delete(id string) error {
	result, err := r.db.Exec(`DELETE FROM scheduled_tasks WHERE LOWER(id) = LOWER(?)`, id)
	if err != nil {
		return err
	}
	rowsAffected, _ := result.RowsAffected()
	if rowsAffected == 0 {
		return fmt.Errorf("scheduled task not found: %s", id)
	}
	return nil
}

// ListEnabled returns all enabled scheduled tasks across all projects
func (r *ScheduledTaskRepo) ListEnabled() ([]*model.ScheduledTask, error) {
	rows, err := r.db.Query(
		`SELECT ` + scheduledTaskCols + ` FROM scheduled_tasks WHERE enabled = 1 ORDER BY project_id ASC, created_at ASC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var tasks []*model.ScheduledTask
	for rows.Next() {
		t, err := r.scanRow(rows)
		if err != nil {
			return nil, err
		}
		tasks = append(tasks, t)
	}
	return tasks, rows.Err()
}

// UpdateTriggerTimestamps updates last_triggered_at and next_run_at for a task
func (r *ScheduledTaskRepo) UpdateTriggerTimestamps(id string, lastTriggeredAt, nextRunAt *time.Time) error {
	now := r.clock.Now().UTC().Format(time.RFC3339Nano)
	result, err := r.db.Exec(
		`UPDATE scheduled_tasks SET last_triggered_at=?, next_run_at=?, updated_at=? WHERE LOWER(id) = LOWER(?)`,
		nullableTime(lastTriggeredAt),
		nullableTime(nextRunAt),
		now,
		id,
	)
	if err != nil {
		return err
	}
	rowsAffected, _ := result.RowsAffected()
	if rowsAffected == 0 {
		return fmt.Errorf("scheduled task not found: %s", id)
	}
	return nil
}

func boolToInt(b bool) int {
	if b {
		return 1
	}
	return 0
}

func nullableTime(t *time.Time) interface{} {
	if t == nil {
		return nil
	}
	return t.UTC().Format(time.RFC3339Nano)
}
