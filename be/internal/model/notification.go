package model

// TODO: encrypt at rest

import "time"

// ChannelKind enumerates supported notification transport kinds.
type ChannelKind string

const (
	ChannelKindSlack    ChannelKind = "slack"
	ChannelKindTelegram ChannelKind = "telegram"
)

// DeliveryStatus enumerates notification delivery states.
type DeliveryStatus string

const (
	DeliveryStatusPending   DeliveryStatus = "pending"
	DeliveryStatusSent      DeliveryStatus = "sent"
	DeliveryStatusFailed    DeliveryStatus = "failed"
	DeliveryStatusGivingUp  DeliveryStatus = "giving_up"
)

// NotificationChannel is a workflow-scoped notification destination.
type NotificationChannel struct {
	ID         string      `json:"id"`
	ProjectID  string      `json:"project_id"`
	WorkflowID string      `json:"workflow_id"`
	Name       string      `json:"name"`
	Kind       ChannelKind `json:"kind"`
	Enabled    bool        `json:"enabled"`
	Config          string      `json:"config"`
	MessageTemplate string      `json:"message_template"`
	EventTypes      []string    `json:"event_types"`
	CreatedAt  time.Time   `json:"created_at"`
	UpdatedAt  time.Time   `json:"updated_at"`
}

// NotificationDelivery tracks a single send attempt for a notification event.
type NotificationDelivery struct {
	ID            string         `json:"id"`
	ChannelID     string         `json:"channel_id"`
	ProjectID     string         `json:"project_id"`
	EventType     string         `json:"event_type"`
	Payload       string         `json:"payload"`
	Status        DeliveryStatus `json:"status"`
	Attempts      int            `json:"attempts"`
	LastError     string         `json:"last_error,omitempty"`
	NextAttemptAt *time.Time     `json:"next_attempt_at,omitempty"`
	CreatedAt     time.Time      `json:"created_at"`
	UpdatedAt     time.Time      `json:"updated_at"`
}
