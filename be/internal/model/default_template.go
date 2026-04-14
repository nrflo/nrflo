package model

import "time"

// DefaultTemplate represents a global default agent template
type DefaultTemplate struct {
	ID        string    `json:"id"`
	Name      string    `json:"name"`
	Type      string    `json:"type"`
	Template  string    `json:"template"`
	Readonly        bool      `json:"readonly"`
	DefaultTemplate *string   `json:"default_template,omitempty"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}
