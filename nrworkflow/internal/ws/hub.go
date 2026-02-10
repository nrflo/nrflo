// Package ws provides WebSocket functionality for real-time updates
package ws

import (
	"encoding/json"
	"log"
	"strings"
	"sync"
	"time"
)

// Event types for WebSocket messages
const (
	EventAgentStarted    = "agent.started"
	EventAgentCompleted  = "agent.completed"
	EventAgentContinued  = "agent.continued"
	EventPhaseStarted    = "phase.started"
	EventPhaseCompleted  = "phase.completed"
	EventFindingsUpdated = "findings.updated"
	EventMessagesUpdated = "messages.updated"
	EventWorkflowUpdated    = "workflow.updated"
	EventWorkflowDefCreated = "workflow_def.created"
	EventWorkflowDefUpdated = "workflow_def.updated"
	EventWorkflowDefDeleted = "workflow_def.deleted"
	EventAgentDefCreated    = "agent_def.created"
	EventAgentDefUpdated    = "agent_def.updated"
	EventAgentDefDeleted    = "agent_def.deleted"
	EventOrchestrationStarted   = "orchestration.started"
	EventOrchestrationCompleted = "orchestration.completed"
	EventOrchestrationFailed    = "orchestration.failed"
	EventTestEcho           = "test.echo"
)

// Event represents a WebSocket event to broadcast
type Event struct {
	Type      string                 `json:"type"`
	ProjectID string                 `json:"project_id"`
	TicketID  string                 `json:"ticket_id"`
	Workflow  string                 `json:"workflow,omitempty"`
	Timestamp string                 `json:"timestamp"`
	Data      map[string]interface{} `json:"data,omitempty"`
}

// NewEvent creates a new event with current timestamp
func NewEvent(eventType, projectID, ticketID, workflow string, data map[string]interface{}) *Event {
	return &Event{
		Type:      eventType,
		ProjectID: projectID,
		TicketID:  ticketID,
		Workflow:  workflow,
		Timestamp: time.Now().UTC().Format(time.RFC3339),
		Data:      data,
	}
}

// Hub manages WebSocket clients and broadcasts
type Hub struct {
	// Registered clients
	clients map[*Client]bool

	// Client subscriptions: projectID -> ticketID -> clients
	// Empty ticketID means subscribed to all tickets in project
	subscriptions map[string]map[string]map[*Client]bool

	// Broadcast channel for events
	broadcast chan *Event

	// Register requests from clients
	register chan *Client

	// Unregister requests from clients
	unregister chan *Client

	// Mutex for client operations
	mu sync.RWMutex

	// Shutdown channel
	shutdown chan struct{}
}

// NewHub creates a new Hub instance
func NewHub() *Hub {
	return &Hub{
		clients:       make(map[*Client]bool),
		subscriptions: make(map[string]map[string]map[*Client]bool),
		broadcast:     make(chan *Event, 256),
		register:      make(chan *Client),
		unregister:    make(chan *Client),
		shutdown:      make(chan struct{}),
	}
}

// Run starts the hub's main loop
func (h *Hub) Run() {
	for {
		select {
		case client := <-h.register:
			h.mu.Lock()
			h.clients[client] = true
			h.mu.Unlock()
			log.Printf("[ws] client registered: %s", client.id)

		case client := <-h.unregister:
			h.mu.Lock()
			if _, ok := h.clients[client]; ok {
				delete(h.clients, client)
				h.removeClientSubscriptions(client)
				close(client.send)
			}
			h.mu.Unlock()
			log.Printf("[ws] client unregistered: %s", client.id)

		case event := <-h.broadcast:
			h.broadcastEvent(event)

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
		log.Printf("[ws] broadcast channel full, dropping event: %s", event.Type)
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

	log.Printf("[ws] client %s subscribed to project=%s ticket=%s (project has %d subscriptions, %d total clients)",
		client.id, projectID, ticketID, len(h.subscriptions[projectID]), len(h.clients))
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

	log.Printf("[ws] client %s unsubscribed from project=%s ticket=%s", client.id, projectID, ticketID)
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

// broadcastEvent sends an event to all subscribed clients
func (h *Hub) broadcastEvent(event *Event) {
	h.mu.RLock()
	defer h.mu.RUnlock()

	// Normalize to lowercase for case-insensitive matching
	projectID := strings.ToLower(event.ProjectID)
	ticketID := strings.ToLower(event.TicketID)

	// Marshal event once
	data, err := json.Marshal(event)
	if err != nil {
		log.Printf("[ws] failed to marshal event: %v", err)
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

	if len(sent) > 0 {
		log.Printf("[ws] broadcast %s to %d clients (project=%s ticket=%s)",
			event.Type, len(sent), projectID, ticketID)
	} else {
		var projectKeys []string
		for k := range h.subscriptions {
			projectKeys = append(projectKeys, k)
		}
		log.Printf("[ws] broadcast %s: no subscribers (project=%s ticket=%s, known projects: %v)",
			event.Type, projectID, ticketID, projectKeys)
	}
}

// sendToClient sends data to a client (non-blocking)
func (h *Hub) sendToClient(client *Client, data []byte) {
	select {
	case client.send <- data:
	default:
		// Client buffer full, will be disconnected by write pump
		log.Printf("[ws] client %s buffer full", client.id)
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
