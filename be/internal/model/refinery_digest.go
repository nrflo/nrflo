package model

import "time"

// RefineryDigest is the single-row-per-console-session working-set digest
// produced by the refinery sidecar, keyed to the console-chat agent_sessions
// id so it survives engine rotation within that chat.
type RefineryDigest struct {
	ConsoleSessionID string    `json:"console_session_id"`
	ProjectID        string    `json:"project_id"`
	Version          int       `json:"version"`
	Content          string    `json:"content"`
	FoldCount        int       `json:"fold_count"`
	CreatedAt        time.Time `json:"created_at"`
	UpdatedAt        time.Time `json:"updated_at"`
}
