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

// Client is a socket client for nrflo server (Unix socket)
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
	return fmt.Errorf("nrflo server is not running. If you are a spawned agent: DO NOT start the server - call the agent_fail tool with a reason and exit")
}

// Execute sends a request and returns the response
func (c *Client) Execute(method string, params interface{}) (*socket.Response, error) {
	return c.executeWithReadDeadline(method, params, 5*time.Minute)
}

func (c *Client) executeWithReadDeadline(method string, params interface{}, readDeadline time.Duration) (*socket.Response, error) {
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
		// Inherit the workflow's trx (set by spawner via NRF_TRX env var) so
		// server-side log lines from this socket call share the parent trx.
		Trx: os.Getenv("NRF_TRX"),
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
	conn.SetReadDeadline(time.Now().Add(readDeadline))
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

// ExecuteAndUnmarshalWithReadDeadline is like ExecuteAndUnmarshal but uses a custom read deadline.
// Use for calls that may block longer than the default 5-minute deadline (e.g. agent.consult).
func (c *Client) ExecuteAndUnmarshalWithReadDeadline(method string, params interface{}, result interface{}, readDeadline time.Duration) error {
	resp, err := c.executeWithReadDeadline(method, params, readDeadline)
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

func isConnectionRefused(err error) bool {
	if err == nil {
		return false
	}
	// Check for connection refused error
	return filepath.Base(err.Error()) == "connection refused" ||
		filepath.Base(err.Error()) == "no such file or directory" ||
		err.Error() == "dial unix: connect: connection refused"
}
