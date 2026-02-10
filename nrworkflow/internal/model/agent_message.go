package model

import "time"

// AgentMessage represents a single message from an agent session
type AgentMessage struct {
	ID        int64     `json:"id"`
	SessionID string    `json:"session_id"`
	Seq       int       `json:"seq"`
	Content   string    `json:"content"`
	CreatedAt time.Time `json:"created_at"`
}
