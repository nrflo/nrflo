package model

import "time"

// APICredential stores a provider API key reference (env/file/literal).
type APICredential struct {
	ID        string    `json:"id"`
	Provider  string    `json:"provider"`
	ProjectID *string   `json:"project_id,omitempty"`
	SecretRef string    `json:"secret_ref"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}
