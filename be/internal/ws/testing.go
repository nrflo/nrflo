package ws

// NewTestClient creates a Client suitable for testing without a real WebSocket connection.
// The returned send channel receives all messages that would be sent to this client.
func NewTestClient(hub *Hub, id string) (*Client, chan []byte) {
	ch := make(chan []byte, 256)
	client := &Client{
		hub:           hub,
		conn:          nil,
		send:          ch,
		id:            id,
		subscriptions: make(map[string]bool),
	}
	return client, ch
}
