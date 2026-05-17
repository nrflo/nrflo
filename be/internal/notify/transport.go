// Package notify implements the notification dispatch subsystem.
package notify

import (
	"net/http"
	"time"
)

// Transport delivers a notification to a specific channel kind.
type Transport interface {
	// Kind returns the channel kind this transport handles (e.g. "slack").
	Kind() string
	// Send delivers the notification. Returns nil on success.
	Send(notification *Notification) error
}

// Notification is the send payload handed to a Transport.
type Notification struct {
	ChannelID  string
	DeliveryID string
	Kind       string
	Config     map[string]interface{}
	Body       string
	ProjectID  string
	WorkflowID string
	InstanceID string
	TicketID   string
	EventType  string
	Payload    map[string]interface{}
}

var registry = map[string]Transport{}

// Register adds a transport to the global registry. Called from init() in each transport file.
func Register(t Transport) {
	registry[t.Kind()] = t
}

// Get returns the transport for the given kind, or nil if not found.
func Get(kind string) Transport {
	return registry[kind]
}

// sharedClient is shared across all transport HTTP calls.
var sharedClient = &http.Client{Timeout: 10 * time.Second}
