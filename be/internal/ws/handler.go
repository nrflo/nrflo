package ws

import (
	"log"
	"net/http"

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

// Handler handles WebSocket upgrade requests
type Handler struct {
	hub *Hub
}

// NewHandler creates a new WebSocket handler
func NewHandler(hub *Hub) *Handler {
	return &Handler{hub: hub}
}

// ServeHTTP handles WebSocket upgrade
func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Printf("[ws] upgrade error: %v", err)
		return
	}

	client := NewClient(h.hub, conn)
	h.hub.Register(client)

	// Start client pumps in goroutines
	go client.WritePump()
	go client.ReadPump()
}
