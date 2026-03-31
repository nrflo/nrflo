package model

import "time"

// DefaultTemplate represents a global default agent template
type DefaultTemplate struct {
	ID        string    `json:"id"`
	Name      string    `json:"name"`
	Template  string    `json:"template"`
	Readonly  bool      `json:"readonly"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}
