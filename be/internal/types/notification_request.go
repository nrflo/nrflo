package types

// NotificationChannelCreateRequest is the payload for creating a notification channel.
type NotificationChannelCreateRequest struct {
	Name       string                 `json:"name"`
	Kind       string                 `json:"kind"`
	Enabled    *bool                  `json:"enabled,omitempty"`
	Config     map[string]interface{} `json:"config,omitempty"`
	EventTypes []string               `json:"event_types,omitempty"`
}

// NotificationChannelUpdateRequest is the payload for partially updating a channel (PATCH).
type NotificationChannelUpdateRequest struct {
	Name       *string                `json:"name,omitempty"`
	Enabled    *bool                  `json:"enabled,omitempty"`
	Config     map[string]interface{} `json:"config,omitempty"`
	EventTypes *[]string              `json:"event_types,omitempty"`
}
