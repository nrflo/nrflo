package ws

import (
	"context"
	"net/http"

	"be/internal/logger"

	"github.com/gorilla/websocket"
)

var upgrader = websocket.Upgrader{
	ReadBufferSize:  1024,
	WriteBufferSize: 1024,
	// Allow all origins for development - in production, configure appropriately
	CheckOrigin: func(r *http.Request) bool {
		return true
	},
}

// SessionAuthorizer reports whether the principal behind one upgraded
// connection may subscribe to sessionID's session channel. It is built once
// per connection, from the authenticated request, before the upgrade.
type SessionAuthorizer func(sessionID string) bool

// Handler handles WebSocket upgrade requests
type Handler struct {
	hub *Hub

	// sessionAuth builds the per-connection SessionAuthorizer. Nil means no
	// authorizer is configured and every subscribe_session is denied: session
	// channels carry console-chat content that the REST routes gate, so this
	// fails closed.
	sessionAuth func(*http.Request) SessionAuthorizer
}

// NewHandler creates a new WebSocket handler
func NewHandler(hub *Hub) *Handler {
	return &Handler{hub: hub}
}

// SetSessionAuthorizer installs the factory that turns an authenticated
// upgrade request into that connection's SessionAuthorizer.
func (h *Handler) SetSessionAuthorizer(f func(*http.Request) SessionAuthorizer) {
	h.sessionAuth = f
}

// ServeHTTP handles WebSocket upgrade
func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	// Built BEFORE the upgrade, while the request (and its authenticated
	// principal) is still live.
	var auth SessionAuthorizer
	if h.sessionAuth != nil {
		auth = h.sessionAuth(r)
	}

	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		logger.Error(context.Background(), "ws upgrade error", "error", err)
		return
	}

	client := NewClient(h.hub, conn)
	client.sessionAuth = auth
	h.hub.Register(client)

	// Start client pumps in goroutines
	go client.WritePump()
	go client.ReadPump()
}
