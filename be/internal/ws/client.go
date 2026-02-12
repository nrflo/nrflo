package ws

import (
	"encoding/json"
	"log"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/gorilla/websocket"
)

const (
	// Time allowed to write a message to the peer
	writeWait = 10 * time.Second

	// Time allowed to read the next pong message from the peer
	pongWait = 60 * time.Second

	// Send pings to peer with this period. Must be less than pongWait.
	pingPeriod = (pongWait * 9) / 10

	// Maximum message size allowed from peer
	maxMessageSize = 4096
)

// Client represents a WebSocket client connection
type Client struct {
	hub  *Hub
	conn *websocket.Conn
	send chan []byte
	id   string

	// Client subscriptions
	subscriptions map[string]bool
	mu            sync.Mutex
}

// ClientMessage represents a message from the client
type ClientMessage struct {
	Action    string `json:"action"`    // subscribe, unsubscribe
	ProjectID string `json:"project_id"`
	TicketID  string `json:"ticket_id"` // optional, empty = all tickets in project
}

// NewClient creates a new client
func NewClient(hub *Hub, conn *websocket.Conn) *Client {
	return &Client{
		hub:           hub,
		conn:          conn,
		send:          make(chan []byte, 256),
		id:            uuid.New().String()[:8],
		subscriptions: make(map[string]bool),
	}
}

// ReadPump pumps messages from the WebSocket connection to the hub
func (c *Client) ReadPump() {
	defer func() {
		c.hub.Unregister(c)
		c.conn.Close()
	}()

	c.conn.SetReadLimit(maxMessageSize)
	c.conn.SetReadDeadline(time.Now().Add(pongWait))
	c.conn.SetPongHandler(func(string) error {
		c.conn.SetReadDeadline(time.Now().Add(pongWait))
		return nil
	})

	for {
		_, message, err := c.conn.ReadMessage()
		if err != nil {
			if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseAbnormalClosure) {
				log.Printf("[ws] client %s read error: %v", c.id, err)
			}
			break
		}

		// Parse client message
		var msg ClientMessage
		if err := json.Unmarshal(message, &msg); err != nil {
			log.Printf("[ws] client %s invalid message: %v", c.id, err)
			continue
		}

		// Handle subscription actions
		switch msg.Action {
		case "subscribe":
			if msg.ProjectID != "" {
				c.hub.Subscribe(c, msg.ProjectID, msg.TicketID)
				// Send acknowledgement
				c.sendAck("subscribed", msg.ProjectID, msg.TicketID)
			}
		case "unsubscribe":
			if msg.ProjectID != "" {
				c.hub.Unsubscribe(c, msg.ProjectID, msg.TicketID)
				c.sendAck("unsubscribed", msg.ProjectID, msg.TicketID)
			}
		case "test":
			if msg.ProjectID != "" {
				log.Printf("[ws] client %s test broadcast: project=%s ticket=%s", c.id, msg.ProjectID, msg.TicketID)
				event := NewEvent(EventTestEcho, msg.ProjectID, msg.TicketID, "", map[string]interface{}{
					"source_client": c.id,
					"message":       "broadcast pipeline test",
				})
				c.hub.Broadcast(event)
			}
		default:
			log.Printf("[ws] client %s unknown action: %s", c.id, msg.Action)
		}
	}
}

// WritePump pumps messages from the hub to the WebSocket connection
func (c *Client) WritePump() {
	ticker := time.NewTicker(pingPeriod)
	defer func() {
		ticker.Stop()
		c.conn.Close()
	}()

	for {
		select {
		case message, ok := <-c.send:
			c.conn.SetWriteDeadline(time.Now().Add(writeWait))
			if !ok {
				// Hub closed the channel
				c.conn.WriteMessage(websocket.CloseMessage, []byte{})
				return
			}

			w, err := c.conn.NextWriter(websocket.TextMessage)
			if err != nil {
				return
			}
			w.Write(message)

			// Add queued messages to the current websocket message
			n := len(c.send)
			for i := 0; i < n; i++ {
				w.Write([]byte{'\n'})
				w.Write(<-c.send)
			}

			if err := w.Close(); err != nil {
				return
			}

		case <-ticker.C:
			c.conn.SetWriteDeadline(time.Now().Add(writeWait))
			if err := c.conn.WriteMessage(websocket.PingMessage, nil); err != nil {
				return
			}
		}
	}
}

// sendAck sends an acknowledgement message to the client
func (c *Client) sendAck(action, projectID, ticketID string) {
	ack := map[string]string{
		"type":       "ack",
		"action":     action,
		"project_id": projectID,
		"ticket_id":  ticketID,
	}
	data, _ := json.Marshal(ack)
	select {
	case c.send <- data:
	default:
		// Buffer full
	}
}
