package model

import "time"

// ServiceToken is a long-lived bearer credential scoped to a single project.
// External services use the plaintext token (returned once at creation) to call
// the REST API; the DB stores only the sha256 hash plus a short display hint.
type ServiceToken struct {
	ID          string     `json:"id"`
	ProjectID   string     `json:"project_id"`
	Name        string     `json:"name"`
	TokenHash   string     `json:"-"`
	DisplayHint string     `json:"display_hint"`
	CreatedAt   time.Time  `json:"created_at"`
	CreatedBy   string     `json:"created_by,omitempty"`
	LastUsedAt  *time.Time `json:"last_used_at,omitempty"`
}
