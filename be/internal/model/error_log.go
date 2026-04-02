package model

// ErrorType represents the type of error
type ErrorType string

const (
	ErrorTypeAgent    ErrorType = "agent"
	ErrorTypeWorkflow ErrorType = "workflow"
	ErrorTypeSystem   ErrorType = "system"
)

// ErrorLog represents a tracked error record
type ErrorLog struct {
	ID        string    `json:"id"`
	ProjectID string    `json:"project_id"`
	ErrorType ErrorType `json:"error_type"`
	InstanceID string   `json:"instance_id"`
	Message   string    `json:"message"`
	CreatedAt string    `json:"created_at"`
}
