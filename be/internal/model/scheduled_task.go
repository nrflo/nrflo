package model

import "time"

// ScheduledTask represents a recurring workflow trigger
type ScheduledTask struct {
	ID             string     `json:"id"`
	ProjectID      string     `json:"project_id"`
	Name           string     `json:"name"`
	Description    string     `json:"description"`
	CronExpression string     `json:"cron_expression"`
	Workflows      []string   `json:"workflows"`
	Enabled        bool       `json:"enabled"`
	LastTriggeredAt *time.Time `json:"last_triggered_at,omitempty"`
	NextRunAt       *time.Time `json:"next_run_at,omitempty"`
	CreatedAt      time.Time  `json:"created_at"`
	UpdatedAt      time.Time  `json:"updated_at"`
}

// ScheduleRun represents a single execution of a scheduled task
type ScheduleRun struct {
	ID              string              `json:"id"`
	ScheduledTaskID string              `json:"scheduled_task_id"`
	ProjectID       string              `json:"project_id"`
	TriggeredAt     time.Time           `json:"triggered_at"`
	Status          string              `json:"status"`
	Workflows       []ScheduleRunWorkflow `json:"workflows"`
	Error           string              `json:"error"`
}

// ScheduleRunWorkflow records per-workflow outcome within a schedule run
type ScheduleRunWorkflow struct {
	Workflow   string `json:"workflow"`
	InstanceID string `json:"instance_id"`
	Error      string `json:"error,omitempty"`
}
