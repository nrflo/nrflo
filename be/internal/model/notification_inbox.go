package model

import "encoding/json"

type InboxNotification struct {
	ID         string          `json:"id"`
	ProjectID  string          `json:"project_id"`
	TicketID   string          `json:"ticket_id"`
	WorkflowID string          `json:"workflow_id"`
	EventType  string          `json:"event_type"`
	Payload    json.RawMessage `json:"payload"`
	CreatedAt  string          `json:"created_at"`
	ReadAt     *string         `json:"read_at"`
}

type NotificationInbox struct {
	Notifications []*InboxNotification `json:"notifications"`
	Unread        int                  `json:"unread"`
	Total         int                  `json:"total"`
}
