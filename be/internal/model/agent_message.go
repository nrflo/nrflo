package model

import "time"

// Synthetic agent_messages categories shared across socket/refinery/handoff.
const (
	// MsgCategorySystemNotice marks a Claude Notification hook row that is
	// not conversation (idle/unknown notices; excludes the workflow_agent
	// "permission" rewrite, which keeps its own category).
	MsgCategorySystemNotice = "system_notice"
	// MsgCategoryTaskNotification marks a UserPromptSubmit row whose text is
	// a Claude Code CLI harness <task-notification> envelope (a backgrounded
	// MCP get_delegation result), not a human-typed prompt.
	MsgCategoryTaskNotification = "task_notification"
)

// AgentMessage represents a single message from an agent session
type AgentMessage struct {
	ID        int64     `json:"id"`
	SessionID string    `json:"session_id"`
	Seq       int       `json:"seq"`
	Content   string    `json:"content"`
	Payload   string    `json:"payload,omitempty"`
	CreatedAt time.Time `json:"created_at"`
}
