package types

// ScheduledTaskCreateRequest is the request for creating a scheduled task
type ScheduledTaskCreateRequest struct {
	ID             string   `json:"id"`
	Name           string   `json:"name"`
	Description    string   `json:"description,omitempty"`
	CronExpression string   `json:"cron_expression"`
	Workflows      []string `json:"workflows"`
	Enabled        *bool    `json:"enabled,omitempty"`
}

// ScheduledTaskUpdateRequest is the request for updating a scheduled task (pointer fields for partial update)
type ScheduledTaskUpdateRequest struct {
	Name           *string   `json:"name,omitempty"`
	Description    *string   `json:"description,omitempty"`
	CronExpression *string   `json:"cron_expression,omitempty"`
	Workflows      *[]string `json:"workflows,omitempty"`
	Enabled        *bool     `json:"enabled,omitempty"`
}
