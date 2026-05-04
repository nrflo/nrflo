package model

import "time"

// ConfigVersion stores a versioned snapshot of a config file
type ConfigVersion struct {
	ID        int64     `json:"id"`
	ProjectID string    `json:"project_id"`
	File      string    `json:"file"`
	Version   int       `json:"version"`
	Content   []byte    `json:"content"`
	Actor     *string   `json:"actor"`
	CreatedAt time.Time `json:"created_at"`
}
