package integration

import (
	"testing"

	"be/internal/client"
	"be/internal/socket"
)

func TestErrorMissingProject(t *testing.T) {
	env := NewTestEnv(t)

	// Create a client with empty project ID
	c := client.NewWithAddr("unix", env.SocketPath, "")

	resp, err := c.Execute("findings.get", map[string]interface{}{
		"ticket_id":  "some-ticket",
		"workflow":   "test",
		"agent_type": "analyzer",
	})
	if err != nil {
		t.Fatalf("connection error: %v", err)
	}
	if resp.Error == nil {
		t.Fatal("expected validation error for missing project")
	}
	if resp.Error.Code != socket.ErrCodeValidation {
		t.Fatalf("expected code %d, got %d (%s)", socket.ErrCodeValidation, resp.Error.Code, resp.Error.Message)
	}
}

func TestErrorMethodNotFound(t *testing.T) {
	env := NewTestEnv(t)

	env.ExpectError(t, "unknown.method", nil, socket.ErrCodeMethodNotFound)
}

func TestErrorRemovedMethodNotFound(t *testing.T) {
	env := NewTestEnv(t)

	// Previously supported methods should now return method not found
	env.ExpectError(t, "ticket.list", nil, socket.ErrCodeMethodNotFound)
	env.ExpectError(t, "project.list", nil, socket.ErrCodeMethodNotFound)
	env.ExpectError(t, "workflow.init", map[string]interface{}{
		"ticket_id": "test",
		"workflow":  "test",
	}, socket.ErrCodeMethodNotFound)
}

func TestErrorAgentMissingProject(t *testing.T) {
	env := NewTestEnv(t)

	// Agent commands require project
	c := client.NewWithAddr("unix", env.SocketPath, "")
	resp, err := c.Execute("agent.complete", map[string]interface{}{
		"ticket_id":  "test",
		"workflow":   "test",
		"agent_type": "analyzer",
	})
	if err != nil {
		t.Fatalf("connection error: %v", err)
	}
	if resp.Error == nil {
		t.Fatal("expected validation error for missing project")
	}
	if resp.Error.Code != socket.ErrCodeValidation {
		t.Fatalf("expected code %d, got %d", socket.ErrCodeValidation, resp.Error.Code)
	}
}
