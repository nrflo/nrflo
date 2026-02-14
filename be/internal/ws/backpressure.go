package ws

import "log"

const (
	// clientBufferWarningThreshold is the fraction of send channel capacity at which we log a warning.
	clientBufferWarningPct = 75
	// clientBufferSize is the capacity of the send channel (must match client.go).
	clientBufferSize = 256
)

// checkBackpressure logs a warning if a client's send buffer is near capacity.
// Returns true if the client should be considered slow.
func checkBackpressure(c *Client) bool {
	queueDepth := len(c.send)
	threshold := clientBufferSize * clientBufferWarningPct / 100
	if queueDepth >= threshold {
		log.Printf("[ws] backpressure: client %s queue depth %d/%d (%.0f%%)",
			c.id, queueDepth, clientBufferSize, float64(queueDepth)/float64(clientBufferSize)*100)
		return true
	}
	return false
}
