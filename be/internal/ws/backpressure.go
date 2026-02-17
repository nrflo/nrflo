package ws

const (
	// clientBufferWarningThreshold is the fraction of send channel capacity at which we log a warning.
	clientBufferWarningPct = 75
	// clientBufferSize is the capacity of the send channel (must match client.go).
	clientBufferSize = 256
)

// checkBackpressure returns true if a client's send buffer is near capacity.
func checkBackpressure(c *Client) bool {
	return len(c.send) >= clientBufferSize*clientBufferWarningPct/100
}
