// Package ws provides WebSocket functionality for real-time updates
package ws

import (
	"context"
	"encoding/json"
	"strings"
	"sync"
	"time"

	"be/internal/clock"
	"be/internal/logger"
	"be/internal/repo"
)

// Event types for WebSocket messages
const (
	EventAgentStarted           = "agent.started"
	EventAgentCompleted         = "agent.completed"
	EventAgentContinued         = "agent.continued"
	EventPhaseStarted           = "phase.started"
	EventPhaseCompleted         = "phase.completed"
	EventFindingsUpdated        = "findings.updated"
	EventMessagesUpdated        = "messages.updated"
	EventWorkflowUpdated        = "workflow.updated"
	EventWorkflowDefCreated     = "workflow_def.created"
	EventWorkflowDefUpdated     = "workflow_def.updated"
	EventWorkflowDefDeleted     = "workflow_def.deleted"
	EventAgentDefCreated        = "agent_def.created"
	EventAgentDefUpdated        = "agent_def.updated"
	EventAgentDefDeleted            = "agent_def.deleted"
	EventSystemAgentDefCreated      = "system_agent_def.created"
	EventSystemAgentDefUpdated      = "system_agent_def.updated"
	EventSystemAgentDefDeleted      = "system_agent_def.deleted"
	EventTicketUpdated          = "ticket.updated"
	EventOrchestrationStarted   = "orchestration.started"
	EventOrchestrationCompleted = "orchestration.completed"
	EventOrchestrationFailed    = "orchestration.failed"
	EventOrchestrationRetried   = "orchestration.retried"
	EventOrchestrationCallback  = "orchestration.callback"
	EventChainUpdated                = "chain.updated"
	EventProjectFindingsUpdated      = "project_findings.updated"
	EventAgentContextUpdated         = "agent.context_updated"
	EventAgentTakeControl            = "agent.take_control"
	EventAgentKilled                 = "agent.killed"
	EventAgentTakeControlRejected    = "agent.take_control_rejected"
	EventAgentViewerAttached         = "agent.viewer_attached"
	EventLayerSkipped                = "layer.skipped"
	EventAgentRetryWaiting           = "agent.retry_waiting"
	EventAgentStallWaiting           = "agent.stall_waiting"
	EventAgentStallRestart           = "agent.stall_restart"
	EventAgentNudged                 = "agent.nudged"
	EventAgentContextSaving          = "agent.context_saving"
	EventSkipTagAdded                = "skip_tag.added"
	EventMergeConflictResolving      = "merge.conflict_resolving"
	EventMergeConflictResolved       = "merge.conflict_resolved"
	EventMergeConflictFailed         = "merge.conflict_failed"
	EventWorkflowInstanceDeleted     = "workflow_instance.deleted"
	EventDefaultTemplateCreated      = "default_template.created"
	EventDefaultTemplateUpdated      = "default_template.updated"
	EventDefaultTemplateDeleted      = "default_template.deleted"
	EventCLIModelCreated             = "cli_model.created"
	EventCLIModelUpdated             = "cli_model.updated"
	EventCLIModelDeleted             = "cli_model.deleted"
	EventErrorCreated                = "error.created"
	EventScheduleCreated             = "schedule.created"
	EventScheduleDeleted             = "schedule.deleted"
	EventScheduleTriggered           = "schedule.triggered"
	EventScheduleUpdated             = "schedule.updated"
	EventWorkflowPushed              = "workflow.pushed"
	EventWorkflowPushFailed          = "workflow.push_failed"
	EventTestEcho                    = "test.echo"
	EventNotificationChannelCreated  = "notification_channel.created"
	EventNotificationChannelUpdated  = "notification_channel.updated"
	EventNotificationChannelDeleted  = "notification_channel.deleted"
	EventNotificationDelivered       = "notification.delivered"
	EventNotificationFailed          = "notification.failed"
	EventToolDispatched   = "tool.dispatched"
	EventWorkflowChainCreated  = "chain_def.created"
	EventWorkflowChainUpdated  = "chain_def.updated"
	EventWorkflowChainDeleted  = "chain_def.deleted"
	EventChainRunStarted       = "chain.run_started"
	EventChainStepStarted      = "chain.step_started"
	EventChainStepCompleted    = "chain.step_completed"
	EventChainRunCompleted     = "chain.run_completed"
	EventChainRunFailed        = "chain.run_failed"
	EventArtifactCreated       = "artifact.created"
	EventArtifactDeleted       = "artifact.deleted"
)

// Event represents a WebSocket event to broadcast
type Event struct {
	ProtocolVersion int                    `json:"protocol_version,omitempty"`
	Type            string                 `json:"type"`
	ProjectID       string                 `json:"project_id"`
	TicketID        string                 `json:"ticket_id"`
	Workflow        string                 `json:"workflow,omitempty"`
	Timestamp       string                 `json:"timestamp"`
	Sequence        int64                  `json:"sequence,omitempty"`
	Entity          string                 `json:"entity,omitempty"`
	Data            map[string]interface{} `json:"data,omitempty"`
}

// NewEvent creates a new event. Timestamp is assigned later by Hub.broadcastEvent().
func NewEvent(eventType, projectID, ticketID, workflow string, data map[string]interface{}) *Event {
	return &Event{
		Type:      eventType,
		ProjectID: projectID,
		TicketID:  ticketID,
		Workflow:  workflow,
		Data:      data,
	}
}

// Hub manages WebSocket clients and broadcasts
type Hub struct {
	// Clock for timestamp generation
	clock clock.Clock

	// Registered clients
	clients map[*Client]bool

	// Client subscriptions: projectID -> ticketID -> clients
	// Empty ticketID means subscribed to all tickets in project
	subscriptions map[string]map[string]map[*Client]bool

	// Broadcast channel for events (subscription-scoped)
	broadcast chan *Event

	// Global broadcast channel (sent to ALL connected clients)
	globalBroadcast chan *Event

	// Register requests from clients
	register chan *Client

	// Unregister requests from clients
	unregister chan *Client

	// Mutex for client operations
	mu sync.RWMutex

	// Shutdown channel
	shutdown chan struct{}

	// EventLog for durable event persistence (nil = logging disabled)
	eventLog *repo.EventLogRepo

	// SnapshotProvider builds snapshot data for v2 subscribe-with-cursor
	snapshotProvider SnapshotProvider

	// listeners receive a copy of each broadcast event (non-blocking fan-out)
	listeners []Listener
}

// Listener receives a copy of every broadcast event for out-of-band processing.
// OnEvent is called from a dedicated goroutine per broadcast — never on the
// broadcast loop itself — so a slow implementation cannot stall the WS pipeline.
type Listener interface {
	OnEvent(*Event)
}

// SnapshotProvider builds snapshot data for a given subscription scope.
type SnapshotProvider interface {
	BuildSnapshot(projectID, ticketID string) ([]SnapshotChunk, error)
}

// SnapshotChunk represents a typed section of snapshot data.
type SnapshotChunk struct {
	Entity string                 `json:"entity"`
	Data   map[string]interface{} `json:"data"`
}

// NewHub creates a new Hub instance
func NewHub(clk clock.Clock) *Hub {
	return &Hub{
		clock:         clk,
		clients:       make(map[*Client]bool),
		subscriptions: make(map[string]map[string]map[*Client]bool),
		broadcast:       make(chan *Event, 256),
		globalBroadcast: make(chan *Event, 256),
		register:        make(chan *Client),
		unregister:    make(chan *Client),
		shutdown:      make(chan struct{}),
	}
}

// SetEventLog sets the event log repo for durable event persistence.
func (h *Hub) SetEventLog(el *repo.EventLogRepo) {
	h.eventLog = el
}

// SetSnapshotProvider sets the provider used for v2 snapshot streaming.
func (h *Hub) SetSnapshotProvider(sp SnapshotProvider) {
	h.snapshotProvider = sp
}

// GetEventLog returns the event log repo (may be nil).
func (h *Hub) GetEventLog() *repo.EventLogRepo {
	return h.eventLog
}

// RegisterListener adds a Listener that receives a copy of every broadcast event.
// Must be called before Hub.Run(). Not thread-safe after Run starts.
func (h *Hub) RegisterListener(l Listener) {
	h.listeners = append(h.listeners, l)
}

// Run starts the hub's main loop
func (h *Hub) Run() {
	for {
		select {
		case client := <-h.register:
			h.mu.Lock()
			h.clients[client] = true
			h.mu.Unlock()

		case client := <-h.unregister:
			h.mu.Lock()
			if _, ok := h.clients[client]; ok {
				delete(h.clients, client)
				h.removeClientSubscriptions(client)
				close(client.send)
			}
			h.mu.Unlock()

		case event := <-h.broadcast:
			h.broadcastEvent(event)

		case event := <-h.globalBroadcast:
			h.broadcastGlobalEvent(event)

		case <-h.shutdown:
			h.mu.Lock()
			for client := range h.clients {
				close(client.send)
				delete(h.clients, client)
			}
			h.mu.Unlock()
			return
		}
	}
}

// Stop gracefully shuts down the hub
func (h *Hub) Stop() {
	close(h.shutdown)
}

// Register registers a new client
func (h *Hub) Register(client *Client) {
	h.register <- client
}

// Unregister unregisters a client
func (h *Hub) Unregister(client *Client) {
	h.unregister <- client
}

// Broadcast sends an event to all subscribed clients
func (h *Hub) Broadcast(event *Event) {
	select {
	case h.broadcast <- event:
	default:
	}
}

// BroadcastGlobal sends an event to ALL connected clients regardless of subscription.
// These are ephemeral signal events — not persisted to event log.
func (h *Hub) BroadcastGlobal(event *Event) {
	select {
	case h.globalBroadcast <- event:
	default:
	}
}

// broadcastGlobalEvent stamps timestamp and sends to all connected clients.
// Does NOT persist to event log (ephemeral notifications).
func (h *Hub) broadcastGlobalEvent(event *Event) {
	event.Timestamp = h.clock.Now().UTC().Format(time.RFC3339Nano)

	data, err := json.Marshal(event)
	if err != nil {
		return
	}

	h.mu.RLock()
	defer h.mu.RUnlock()

	for client := range h.clients {
		h.sendToClient(client, data)
	}
}

// Subscribe adds a client subscription
func (h *Hub) Subscribe(client *Client, projectID, ticketID string) {
	h.mu.Lock()
	defer h.mu.Unlock()

	// Normalize to lowercase for case-insensitive matching
	projectID = strings.ToLower(projectID)
	ticketID = strings.ToLower(ticketID)

	if _, ok := h.subscriptions[projectID]; !ok {
		h.subscriptions[projectID] = make(map[string]map[*Client]bool)
	}
	if _, ok := h.subscriptions[projectID][ticketID]; !ok {
		h.subscriptions[projectID][ticketID] = make(map[*Client]bool)
	}
	h.subscriptions[projectID][ticketID][client] = true

	// Track subscription in client
	client.mu.Lock()
	client.subscriptions[subscriptionKey(projectID, ticketID)] = true
	client.mu.Unlock()
}

// Unsubscribe removes a client subscription
func (h *Hub) Unsubscribe(client *Client, projectID, ticketID string) {
	h.mu.Lock()
	defer h.mu.Unlock()

	// Normalize to lowercase for case-insensitive matching
	projectID = strings.ToLower(projectID)
	ticketID = strings.ToLower(ticketID)

	if projects, ok := h.subscriptions[projectID]; ok {
		if tickets, ok := projects[ticketID]; ok {
			delete(tickets, client)
			if len(tickets) == 0 {
				delete(projects, ticketID)
			}
		}
		if len(projects) == 0 {
			delete(h.subscriptions, projectID)
		}
	}

	// Remove from client tracking
	client.mu.Lock()
	delete(client.subscriptions, subscriptionKey(projectID, ticketID))
	client.mu.Unlock()
}

// removeClientSubscriptions removes all subscriptions for a client (must hold h.mu)
func (h *Hub) removeClientSubscriptions(client *Client) {
	client.mu.Lock()
	subs := make(map[string]bool)
	for k, v := range client.subscriptions {
		subs[k] = v
	}
	client.mu.Unlock()

	for key := range subs {
		projectID, ticketID := parseSubscriptionKey(key)
		if projects, ok := h.subscriptions[projectID]; ok {
			if tickets, ok := projects[ticketID]; ok {
				delete(tickets, client)
				if len(tickets) == 0 {
					delete(projects, ticketID)
				}
			}
			if len(projects) == 0 {
				delete(h.subscriptions, projectID)
			}
		}
	}
}

// broadcastEvent stamps the event timestamp, logs to the durable log (if configured), assigns seq, then sends to clients.
func (h *Hub) broadcastEvent(event *Event) {
	// Stamp timestamp at broadcast time
	event.Timestamp = h.clock.Now().UTC().Format(time.RFC3339Nano)

	// Persist to event log before dispatching
	if h.eventLog != nil {
		payload, _ := json.Marshal(event.Data)
		seq, err := h.eventLog.Append(
			strings.ToLower(event.ProjectID),
			strings.ToLower(event.TicketID),
			event.Type,
			event.Workflow,
			payload,
		)
		if err != nil {
			logger.Error(context.Background(), "event log append failed", "error", err)
		} else {
			event.Sequence = seq
			event.ProtocolVersion = ProtocolVersion
		}
	}

	// Fan out to registered listeners in a separate goroutine so a slow
	// listener cannot stall the WS broadcast pipeline.
	if len(h.listeners) > 0 {
		listeners := make([]Listener, len(h.listeners))
		copy(listeners, h.listeners)
		go func() {
			for _, l := range listeners {
				l.OnEvent(event)
			}
		}()
	}

	h.mu.RLock()
	defer h.mu.RUnlock()

	// Normalize to lowercase for case-insensitive matching
	projectID := strings.ToLower(event.ProjectID)
	ticketID := strings.ToLower(event.TicketID)

	// Marshal event once
	data, err := json.Marshal(event)
	if err != nil {
		return
	}

	// Find all clients subscribed to this project+ticket
	sent := make(map[*Client]bool)

	// Check project-wide subscriptions (ticketID = "")
	if projects, ok := h.subscriptions[projectID]; ok {
		if clients, ok := projects[""]; ok {
			for client := range clients {
				if !sent[client] {
					h.sendToClient(client, data)
					sent[client] = true
				}
			}
		}
		// Check specific ticket subscriptions
		if clients, ok := projects[ticketID]; ok {
			for client := range clients {
				if !sent[client] {
					h.sendToClient(client, data)
					sent[client] = true
				}
			}
		}
	}

}

// sendToClient sends data to a client (non-blocking)
func (h *Hub) sendToClient(client *Client, data []byte) {
	checkBackpressure(client)
	select {
	case client.send <- data:
	default:
		// Client buffer full, will be disconnected by write pump
	}
}

// ClientCount returns the number of connected clients
func (h *Hub) ClientCount() int {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return len(h.clients)
}

func subscriptionKey(projectID, ticketID string) string {
	return projectID + ":" + ticketID
}

func parseSubscriptionKey(key string) (projectID, ticketID string) {
	for i := 0; i < len(key); i++ {
		if key[i] == ':' {
			return key[:i], key[i+1:]
		}
	}
	return key, ""
}
