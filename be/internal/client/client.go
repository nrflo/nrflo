package client

import (
	"bufio"
	"encoding/json"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"time"

	"github.com/google/uuid"

	"be/internal/socket"
)

// Client is a socket client for nrflow server (Unix socket)
type Client struct {
	network   string
	address   string
	projectID string
}

// New creates a new client using the Unix socket connection
func New(projectID string) *Client {
	network, address := socket.GetServerAddr()
	return &Client{
		network:   network,
		address:   address,
		projectID: projectID,
	}
}

// NewWithAddr creates a new client with explicit network and address
func NewWithAddr(network, address, projectID string) *Client {
	return &Client{
		network:   network,
		address:   address,
		projectID: projectID,
	}
}

// IsServerRunning checks if the server is running
func (c *Client) IsServerRunning() bool {
	conn, err := net.DialTimeout(c.network, c.address, 2*time.Second)
	if err != nil {
		return false
	}
	conn.Close()
	return true
}

// ServerNotRunningError returns the error message when server is not running
func ServerNotRunningError() error {
	return fmt.Errorf("nrflow server is not running. If you are a spawned agent: DO NOT start the server - call 'nrflow agent fail <ticket> <agent> --reason=\"server not running\"' and exit")
}

// Execute sends a request and returns the response
func (c *Client) Execute(method string, params interface{}) (*socket.Response, error) {
	var conn net.Conn
	var err error

	// Retry connection up to 3 times with backoff for transient errors
	for attempt := 0; attempt < 3; attempt++ {
		conn, err = net.DialTimeout(c.network, c.address, 5*time.Second)
		if err == nil {
			break
		}
		// If socket doesn't exist, server is definitely not running - no retry
		if os.IsNotExist(err) {
			return nil, ServerNotRunningError()
		}
		// For connection refused, retry with backoff (could be transient)
		if isConnectionRefused(err) && attempt < 2 {
			time.Sleep(time.Duration(100*(attempt+1)) * time.Millisecond)
			continue
		}
		if isConnectionRefused(err) {
			return nil, ServerNotRunningError()
		}
		return nil, fmt.Errorf("failed to connect to server: %w", err)
	}
	defer conn.Close()

	// Build request
	req := socket.Request{
		ID:      uuid.New().String(),
		Method:  method,
		Project: c.projectID,
	}

	if params != nil {
		paramsJSON, err := json.Marshal(params)
		if err != nil {
			return nil, fmt.Errorf("failed to marshal params: %w", err)
		}
		req.Params = paramsJSON
	}

	// Send request
	reqJSON, err := json.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}
	reqJSON = append(reqJSON, '\n')

	conn.SetWriteDeadline(time.Now().Add(30 * time.Second))
	if _, err := conn.Write(reqJSON); err != nil {
		return nil, fmt.Errorf("failed to send request: %w", err)
	}

	// Read response
	conn.SetReadDeadline(time.Now().Add(5 * time.Minute))
	reader := bufio.NewReader(conn)
	respLine, err := reader.ReadBytes('\n')
	if err != nil {
		return nil, fmt.Errorf("failed to read response: %w", err)
	}

	var resp socket.Response
	if err := json.Unmarshal(respLine, &resp); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}

	return &resp, nil
}

// ExecuteAndUnmarshal sends a request and unmarshals the result
func (c *Client) ExecuteAndUnmarshal(method string, params interface{}, result interface{}) error {
	resp, err := c.Execute(method, params)
	if err != nil {
		return err
	}

	if resp.Error != nil {
		return resp.Error
	}

	if result != nil && len(resp.Result) > 0 {
		if err := json.Unmarshal(resp.Result, result); err != nil {
			return fmt.Errorf("failed to unmarshal result: %w", err)
		}
	}

	return nil
}

// ExecuteStreaming sends a request and returns a channel for streaming responses
// This is used for agent spawn which has streaming output
func (c *Client) ExecuteStreaming(method string, params interface{}) (<-chan socket.StreamChunk, <-chan error, error) {
	conn, err := net.DialTimeout(c.network, c.address, 5*time.Second)
	if err != nil {
		if os.IsNotExist(err) || isConnectionRefused(err) {
			return nil, nil, ServerNotRunningError()
		}
		return nil, nil, fmt.Errorf("failed to connect to server: %w", err)
	}

	// Build request
	req := socket.Request{
		ID:      uuid.New().String(),
		Method:  method,
		Project: c.projectID,
	}

	if params != nil {
		paramsJSON, err := json.Marshal(params)
		if err != nil {
			conn.Close()
			return nil, nil, fmt.Errorf("failed to marshal params: %w", err)
		}
		req.Params = paramsJSON
	}

	// Send request
	reqJSON, err := json.Marshal(req)
	if err != nil {
		conn.Close()
		return nil, nil, fmt.Errorf("failed to marshal request: %w", err)
	}
	reqJSON = append(reqJSON, '\n')

	conn.SetWriteDeadline(time.Now().Add(30 * time.Second))
	if _, err := conn.Write(reqJSON); err != nil {
		conn.Close()
		return nil, nil, fmt.Errorf("failed to send request: %w", err)
	}

	// Create channels for streaming
	chunks := make(chan socket.StreamChunk, 100)
	errors := make(chan error, 1)

	// Read streaming responses in a goroutine
	go func() {
		defer conn.Close()
		defer close(chunks)
		defer close(errors)

		reader := bufio.NewReader(conn)
		for {
			conn.SetReadDeadline(time.Now().Add(30 * time.Minute)) // Long timeout for spawn
			line, err := reader.ReadBytes('\n')
			if err != nil {
				errors <- err
				return
			}

			var chunk socket.StreamChunk
			if err := json.Unmarshal(line, &chunk); err != nil {
				// Maybe it's a regular response (error or final result)
				var resp socket.Response
				if err := json.Unmarshal(line, &resp); err != nil {
					errors <- fmt.Errorf("failed to parse response: %w", err)
					return
				}
				if resp.Error != nil {
					errors <- resp.Error
				}
				return
			}

			chunks <- chunk

			// Check if this is the complete event
			if chunk.Stream != nil && chunk.Stream.Type == "complete" {
				return
			}
		}
	}()

	return chunks, errors, nil
}

func isConnectionRefused(err error) bool {
	if err == nil {
		return false
	}
	// Check for connection refused error
	return filepath.Base(err.Error()) == "connection refused" ||
		filepath.Base(err.Error()) == "no such file or directory" ||
		err.Error() == "dial unix: connect: connection refused"
}
