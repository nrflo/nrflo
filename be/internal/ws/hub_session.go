package ws

import (
	"encoding/json"
	"time"
)

// BroadcastSession sends an event to only the subscribers of event.SessionID.
// Used by console-chat deltas/turn/approval events — ephemeral (never
// event-logged, no listener fan-out), mirroring BroadcastGlobal's shape.
func (h *Hub) BroadcastSession(event *Event) {
	select {
	case h.sessionBroadcast <- event:
	default:
	}
}

// broadcastSessionEvent stamps the timestamp and sends to event.SessionID's
// subscribers only. Does NOT persist to the event log and does NOT fan out to
// registered listeners: console-chat deltas are live-only, exactly as the
// engines already treat them.
func (h *Hub) broadcastSessionEvent(event *Event) {
	event.Timestamp = h.clock.Now().UTC().Format(time.RFC3339Nano)

	data, err := json.Marshal(event)
	if err != nil {
		return
	}

	h.mu.RLock()
	defer h.mu.RUnlock()

	for client := range h.sessionSubs[event.SessionID] {
		h.sendToClient(client, data)
	}
}

// SubscribeSession adds a client to a session-keyed subscription.
func (h *Hub) SubscribeSession(client *Client, sessionID string) {
	h.mu.Lock()
	defer h.mu.Unlock()

	if _, ok := h.sessionSubs[sessionID]; !ok {
		h.sessionSubs[sessionID] = make(map[*Client]bool)
	}
	h.sessionSubs[sessionID][client] = true

	client.mu.Lock()
	client.sessionSubs[sessionID] = true
	client.mu.Unlock()
}

// UnsubscribeSession removes a client from a session-keyed subscription.
func (h *Hub) UnsubscribeSession(client *Client, sessionID string) {
	h.mu.Lock()
	defer h.mu.Unlock()

	if clients, ok := h.sessionSubs[sessionID]; ok {
		delete(clients, client)
		if len(clients) == 0 {
			delete(h.sessionSubs, sessionID)
		}
	}

	client.mu.Lock()
	delete(client.sessionSubs, sessionID)
	client.mu.Unlock()
}

// removeClientSessionSubscriptions removes every session-keyed subscription
// for client (must hold h.mu — called from removeClientSubscriptions).
func (h *Hub) removeClientSessionSubscriptions(client *Client) {
	client.mu.Lock()
	sessionSubs := make(map[string]bool)
	for k, v := range client.sessionSubs {
		sessionSubs[k] = v
	}
	client.mu.Unlock()

	for sessionID := range sessionSubs {
		if clients, ok := h.sessionSubs[sessionID]; ok {
			delete(clients, client)
			if len(clients) == 0 {
				delete(h.sessionSubs, sessionID)
			}
		}
	}
}
