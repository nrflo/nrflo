package socket

import (
	"encoding/json"
	"testing"
	"time"

	"be/internal/ws"
)

// TestAgentContextUpdateBroadcast verifies that agent.context_update broadcasts
// agent.context_updated with correct session_id and context_left fields.
func TestAgentContextUpdateBroadcast(t *testing.T) {
	env := newHandlerTestEnv(t)
	env.createTicketAndWorkflow(t, "TEST-1")

	// Get workflow instance ID
	var wfiID string
	err := env.pool.QueryRow(`SELECT id FROM workflow_instances WHERE LOWER(project_id) = LOWER(?) AND LOWER(ticket_id) = LOWER(?) AND LOWER(workflow_id) = LOWER(?)`,
		env.project, "TEST-1", "test").Scan(&wfiID)
	if err != nil {
		t.Fatalf("failed to get workflow instance ID: %v", err)
	}

	// Insert running agent session
	sessionID := "sess-ctx-update"
	_, err = env.pool.Exec(`
		INSERT INTO agent_sessions (id, project_id, ticket_id, workflow_instance_id, phase, agent_type, model_id, status, created_at, updated_at)
		VALUES (?, ?, ?, ?, 'analyzer', 'analyzer', 'claude-opus-4', 'running', datetime('now'), datetime('now'))
	`, sessionID, env.project, "TEST-1", wfiID)
	if err != nil {
		t.Fatalf("failed to insert agent session: %v", err)
	}

	// Subscribe WS client
	client, sendCh := ws.NewTestClient(env.hub, "test-client-ctx")
	env.hub.Register(client)
	time.Sleep(50 * time.Millisecond)
	env.hub.Subscribe(client, env.project, "TEST-1")

	// Send agent.context_update
	params := map[string]interface{}{
		"session_id":   sessionID,
		"context_left": 42,
	}
	paramsData, _ := json.Marshal(params)

	req := Request{
		ID:      "req-ctx-1",
		Method:  "agent.context_update",
		Project: env.project,
		Params:  paramsData,
	}

	resp := env.handler.Handle(req)
	if resp.Error != nil {
		t.Fatalf("expected no error, got: %v", resp.Error)
	}

	// Verify response status
	var result map[string]string
	if err := json.Unmarshal(resp.Result, &result); err != nil {
		t.Fatalf("failed to unmarshal response: %v", err)
	}
	if result["status"] != "updated" {
		t.Errorf("expected status=updated, got: %v", result["status"])
	}

	// Verify broadcast
	select {
	case msg := <-sendCh:
		var event ws.Event
		if err := json.Unmarshal(msg, &event); err != nil {
			t.Fatalf("failed to unmarshal event: %v", err)
		}

		if event.Type != ws.EventAgentContextUpdated {
			t.Errorf("expected event type %s, got %s", ws.EventAgentContextUpdated, event.Type)
		}

		gotSession, ok := event.Data["session_id"].(string)
		if !ok || gotSession != sessionID {
			t.Errorf("expected session_id=%s, got: %v", sessionID, event.Data["session_id"])
		}

		gotContextLeft, ok := event.Data["context_left"].(float64)
		if !ok || int(gotContextLeft) != 42 {
			t.Errorf("expected context_left=42, got: %v", event.Data["context_left"])
		}

	case <-time.After(time.Second):
		t.Fatal("timeout waiting for agent.context_updated broadcast")
	}
}

// TestAgentContextUpdateNoBroadcastWhenSessionMissing verifies that when session_id
// does not exist, the handler returns success but does NOT broadcast any event.
func TestAgentContextUpdateNoBroadcastWhenSessionMissing(t *testing.T) {
	env := newHandlerTestEnv(t)

	// Subscribe WS client
	client, sendCh := ws.NewTestClient(env.hub, "test-client-missing")
	env.hub.Register(client)
	time.Sleep(50 * time.Millisecond)
	env.hub.Subscribe(client, env.project, "NONEXISTENT-TICKET")

	// Send agent.context_update with nonexistent session_id
	params := map[string]interface{}{
		"session_id":   "nonexistent-session-id",
		"context_left": 75,
	}
	paramsData, _ := json.Marshal(params)

	req := Request{
		ID:      "req-ctx-missing",
		Method:  "agent.context_update",
		Project: env.project,
		Params:  paramsData,
	}

	resp := env.handler.Handle(req)
	if resp.Error != nil {
		t.Fatalf("expected no error for missing session, got: %v", resp.Error)
	}

	// Expect no broadcast within a short window
	select {
	case msg := <-sendCh:
		var event ws.Event
		_ = json.Unmarshal(msg, &event)
		t.Errorf("expected no broadcast for missing session, got event type: %s", event.Type)
	case <-time.After(200 * time.Millisecond):
		// Correct: no broadcast
	}
}

// TestAgentContextUpdateMissingSessionID verifies validation: missing session_id returns error.
func TestAgentContextUpdateMissingSessionID(t *testing.T) {
	env := newHandlerTestEnv(t)

	params := map[string]interface{}{
		"context_left": 50,
		// session_id intentionally omitted
	}
	paramsData, _ := json.Marshal(params)

	req := Request{
		ID:      "req-ctx-nosession",
		Method:  "agent.context_update",
		Project: env.project,
		Params:  paramsData,
	}

	resp := env.handler.Handle(req)
	if resp.Error == nil {
		t.Fatal("expected validation error for missing session_id")
	}
}

// TestAgentContextUpdateZeroContextLeft verifies context_left=0 is broadcast correctly.
func TestAgentContextUpdateZeroContextLeft(t *testing.T) {
	env := newHandlerTestEnv(t)
	env.createTicketAndWorkflow(t, "TEST-2")

	var wfiID string
	err := env.pool.QueryRow(`SELECT id FROM workflow_instances WHERE LOWER(project_id) = LOWER(?) AND LOWER(ticket_id) = LOWER(?) AND LOWER(workflow_id) = LOWER(?)`,
		env.project, "TEST-2", "test").Scan(&wfiID)
	if err != nil {
		t.Fatalf("failed to get workflow instance ID: %v", err)
	}

	sessionID := "sess-ctx-zero"
	_, err = env.pool.Exec(`
		INSERT INTO agent_sessions (id, project_id, ticket_id, workflow_instance_id, phase, agent_type, model_id, status, created_at, updated_at)
		VALUES (?, ?, ?, ?, 'analyzer', 'analyzer', 'claude-sonnet-4', 'running', datetime('now'), datetime('now'))
	`, sessionID, env.project, "TEST-2", wfiID)
	if err != nil {
		t.Fatalf("failed to insert agent session: %v", err)
	}

	client, sendCh := ws.NewTestClient(env.hub, "test-client-zero")
	env.hub.Register(client)
	time.Sleep(50 * time.Millisecond)
	env.hub.Subscribe(client, env.project, "TEST-2")

	params := map[string]interface{}{
		"session_id":   sessionID,
		"context_left": 0,
	}
	paramsData, _ := json.Marshal(params)

	req := Request{
		ID:      "req-ctx-zero",
		Method:  "agent.context_update",
		Project: env.project,
		Params:  paramsData,
	}

	resp := env.handler.Handle(req)
	if resp.Error != nil {
		t.Fatalf("expected no error, got: %v", resp.Error)
	}

	select {
	case msg := <-sendCh:
		var event ws.Event
		if err := json.Unmarshal(msg, &event); err != nil {
			t.Fatalf("failed to unmarshal event: %v", err)
		}
		if event.Type != ws.EventAgentContextUpdated {
			t.Errorf("expected event type %s, got %s", ws.EventAgentContextUpdated, event.Type)
		}
		gotContextLeft, ok := event.Data["context_left"].(float64)
		if !ok || int(gotContextLeft) != 0 {
			t.Errorf("expected context_left=0, got: %v", event.Data["context_left"])
		}
	case <-time.After(time.Second):
		t.Fatal("timeout waiting for agent.context_updated broadcast with context_left=0")
	}
}

// TestAgentContextUpdateDBPersisted verifies context_left is written to the database.
func TestAgentContextUpdateDBPersisted(t *testing.T) {
	env := newHandlerTestEnv(t)
	env.createTicketAndWorkflow(t, "TEST-3")

	var wfiID string
	err := env.pool.QueryRow(`SELECT id FROM workflow_instances WHERE LOWER(project_id) = LOWER(?) AND LOWER(ticket_id) = LOWER(?) AND LOWER(workflow_id) = LOWER(?)`,
		env.project, "TEST-3", "test").Scan(&wfiID)
	if err != nil {
		t.Fatalf("failed to get workflow instance ID: %v", err)
	}

	sessionID := "sess-ctx-db"
	_, err = env.pool.Exec(`
		INSERT INTO agent_sessions (id, project_id, ticket_id, workflow_instance_id, phase, agent_type, model_id, status, created_at, updated_at)
		VALUES (?, ?, ?, ?, 'analyzer', 'analyzer', 'claude-opus-4', 'running', datetime('now'), datetime('now'))
	`, sessionID, env.project, "TEST-3", wfiID)
	if err != nil {
		t.Fatalf("failed to insert agent session: %v", err)
	}

	params := map[string]interface{}{
		"session_id":   sessionID,
		"context_left": 123,
	}
	paramsData, _ := json.Marshal(params)

	req := Request{
		ID:      "req-ctx-db",
		Method:  "agent.context_update",
		Project: env.project,
		Params:  paramsData,
	}

	resp := env.handler.Handle(req)
	if resp.Error != nil {
		t.Fatalf("expected no error, got: %v", resp.Error)
	}

	// Verify DB was updated
	var storedContextLeft int
	err = env.pool.QueryRow(`SELECT context_left FROM agent_sessions WHERE id = ?`, sessionID).Scan(&storedContextLeft)
	if err != nil {
		t.Fatalf("failed to query context_left: %v", err)
	}
	if storedContextLeft != 123 {
		t.Errorf("expected context_left=123 in DB, got: %d", storedContextLeft)
	}
}
