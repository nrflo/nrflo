package model

import "time"

// ProjectEnvVar represents a per-project environment variable stored in the database.
type ProjectEnvVar struct {
	ProjectID string    `json:"project_id"`
	Name      string    `json:"name"`
	Value     string    `json:"value"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}
