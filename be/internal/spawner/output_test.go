package spawner

import (
	"encoding/json"
	"testing"
	"time"

	"be/internal/ws"
)

// TestMessageCoalescingWindow verifies messages.updated broadcasts are coalesced per 2s window.
func TestMessageCoalescingWindow(t *testing.T) {
	env := newSpawnerTestEnv(t)
	ticketID := "TEST-COAL-1"
	env.initWorkflow(t, ticketID)

	// Create hub for broadcasting
	hub := ws.NewHub()
	go hub.Run()
	defer hub.Stop()

	// Create spawner with hub
	spawner := New(Config{
		DataPath: env.dbPath,
		Pool:     env.pool,
		WSHub:    hub,
	})

	// Get workflow instance ID
	var wfiID string
	err := env.pool.QueryRow(`SELECT id FROM workflow_instances WHERE LOWER(project_id) = LOWER(?) AND LOWER(ticket_id) = LOWER(?) AND LOWER(workflow_id) = LOWER(?)`,
		env.project, ticketID, "test").Scan(&wfiID)
	if err != nil {
		t.Fatalf("failed to get workflow instance ID: %v", err)
	}

	// Create agent session directly in DB
	sessionID := "sess-coal-1"
	_, err = env.pool.Exec(`
		INSERT INTO agent_sessions (id, project_id, ticket_id, workflow_instance_id, phase, agent_type, model_id, status, created_at, updated_at)
		VALUES (?, ?, ?, ?, 'analyzer', 'analyzer', 'cli:claude-opus-4', 'running', datetime('now'), datetime('now'))
	`, sessionID, env.project, ticketID, wfiID)
	if err != nil {
		t.Fatalf("failed to create session: %v", err)
	}

	// Subscribe a test client to receive broadcasts
	client, sendCh := ws.NewTestClient(hub, "test-client")
	hub.Register(client)
	time.Sleep(50 * time.Millisecond)
	hub.Subscribe(client, env.project, ticketID)

	// Create a process info (simulate running agent)
	proc := &processInfo{
		sessionID:       sessionID,
		projectID:       env.project,
		ticketID:        ticketID,
		agentType:       "analyzer",
		workflowName:    "test",
		modelID:         "cli:claude-opus-4",
		pendingMessages: make([]string, 0),
		nextSeq:         0,
	}

	// Track messages and flush (first broadcast should always happen)
	spawner.trackMessage(proc, "Message 1")
	spawner.saveMessages(proc)

	// Drain first broadcast
	select {
	case msg := <-sendCh:
		var event ws.Event
		if err := json.Unmarshal(msg, &event); err != nil {
			t.Fatalf("failed to unmarshal event: %v", err)
		}
		if event.Type != ws.EventMessagesUpdated {
			t.Fatalf("expected messages.updated event, got: %s", event.Type)
		}
	case <-time.After(500 * time.Millisecond):
		t.Fatal("timeout waiting for first broadcast")
	}

	// Track and flush again immediately (should be suppressed by coalescing)
	spawner.trackMessage(proc, "Message 2")
	spawner.saveMessages(proc)

	select {
	case msg := <-sendCh:
		t.Fatalf("should not receive broadcast within coalesce window, got: %s", string(msg))
	case <-time.After(300 * time.Millisecond):
		// Expected - no broadcast within window
	}

	// Wait for coalesce window to expire (2s + buffer)
	time.Sleep(2200 * time.Millisecond)

	// Track and flush again (should broadcast now)
	spawner.trackMessage(proc, "Message 3")
	spawner.saveMessages(proc)

	select {
	case msg := <-sendCh:
		var event ws.Event
		if err := json.Unmarshal(msg, &event); err != nil {
			t.Fatalf("failed to unmarshal event: %v", err)
		}
		if event.Type != ws.EventMessagesUpdated {
			t.Fatalf("expected messages.updated event, got: %s", event.Type)
		}
		// Verify payload includes required fields
		sid, ok := event.Data["session_id"].(string)
		if !ok || sid != sessionID {
			t.Errorf("expected session_id=%s, got: %v", sessionID, event.Data["session_id"])
		}
	case <-time.After(500 * time.Millisecond):
		t.Fatal("timeout waiting for broadcast after coalesce window")
	}
}

// TestMessageCoalescingPerSession verifies coalescing is tracked per session.
func TestMessageCoalescingPerSession(t *testing.T) {
	env := newSpawnerTestEnv(t)
	ticketID := "TEST-COAL-2"
	env.initWorkflow(t, ticketID)

	// Create hub for broadcasting
	hub := ws.NewHub()
	go hub.Run()
	defer hub.Stop()

	// Create spawner with hub
	spawner := New(Config{
		DataPath: env.dbPath,
		Pool:     env.pool,
		WSHub:    hub,
	})


	// Get workflow instance ID
	var wfiID string
	err := env.pool.QueryRow(`SELECT id FROM workflow_instances WHERE LOWER(project_id) = LOWER(?) AND LOWER(ticket_id) = LOWER(?) AND LOWER(workflow_id) = LOWER(?)`,
		env.project, ticketID, "test").Scan(&wfiID)
	if err != nil {
		t.Fatalf("failed to get workflow instance ID: %v", err)
	}
	// Create two agent sessions directly in DB
	sessionID1 := "sess-coal-2a"
	_, err = env.pool.Exec(`
		INSERT INTO agent_sessions (id, project_id, ticket_id, workflow_instance_id, phase, agent_type, model_id, status, created_at, updated_at)
		VALUES (?, ?, ?, ?, 'analyzer', 'analyzer', 'cli:claude-opus-4', 'running', datetime('now'), datetime('now'))
	`, sessionID1, env.project, ticketID, wfiID)
	if err != nil {
		t.Fatalf("failed to create session 1: %v", err)
	}

	sessionID2 := "sess-coal-2b"
	_, err = env.pool.Exec(`
		INSERT INTO agent_sessions (id, project_id, ticket_id, workflow_instance_id, phase, agent_type, model_id, status, created_at, updated_at)
		VALUES (?, ?, ?, ?, 'analyzer', 'builder', 'cli:claude-sonnet-4', 'running', datetime('now'), datetime('now'))
	`, sessionID2, env.project, ticketID, wfiID)
	if err != nil {
		t.Fatalf("failed to create session 2: %v", err)
	}

	// Subscribe a test client to receive broadcasts
	client, sendCh := ws.NewTestClient(hub, "test-client")
	hub.Register(client)
	time.Sleep(50 * time.Millisecond)
	hub.Subscribe(client, env.project, ticketID)

	// Create process infos for both sessions
	proc1 := &processInfo{
		sessionID:       sessionID1,
		projectID:       env.project,
		ticketID:        ticketID,
		agentType:       "analyzer",
		workflowName:    "test",
		modelID:         "cli:claude-opus-4",
		pendingMessages: make([]string, 0),
		nextSeq:         0,
	}
	proc2 := &processInfo{
		sessionID:       sessionID2,
		projectID:       env.project,
		ticketID:        ticketID,
		agentType:       "builder",
		workflowName:    "test",
		modelID:         "cli:claude-sonnet-4",
		pendingMessages: make([]string, 0),
		nextSeq:         0,
	}

	// Flush from session 1
	spawner.trackMessage(proc1, "Session 1 message")
	spawner.saveMessages(proc1)

	// Drain first broadcast from session 1
	select {
	case msg := <-sendCh:
		var event ws.Event
		json.Unmarshal(msg, &event)
		sid, _ := event.Data["session_id"].(string)
		if sid != sessionID1 {
			t.Fatalf("expected broadcast from session 1, got session: %s", sid)
		}
	case <-time.After(500 * time.Millisecond):
		t.Fatal("timeout waiting for session 1 broadcast")
	}

	// Flush from session 2 immediately (should broadcast because different session)
	spawner.trackMessage(proc2, "Session 2 message")
	spawner.saveMessages(proc2)

	select {
	case msg := <-sendCh:
		var event ws.Event
		if err := json.Unmarshal(msg, &event); err != nil {
			t.Fatalf("failed to unmarshal event: %v", err)
		}
		sid, ok := event.Data["session_id"].(string)
		if !ok || sid != sessionID2 {
			t.Errorf("expected broadcast from session 2, got: %v", event.Data["session_id"])
		}
	case <-time.After(500 * time.Millisecond):
		t.Fatal("timeout waiting for session 2 broadcast (should not be coalesced)")
	}
}

// TestMessageBroadcastPayloadFields verifies messages.updated event has all required fields.
func TestMessageBroadcastPayloadFields(t *testing.T) {
	env := newSpawnerTestEnv(t)
	ticketID := "TEST-COAL-3"
	env.initWorkflow(t, ticketID)

	// Create hub for broadcasting
	hub := ws.NewHub()
	go hub.Run()
	defer hub.Stop()

	// Create spawner with hub
	spawner := New(Config{
		DataPath: env.dbPath,
		Pool:     env.pool,
		WSHub:    hub,
	})

	// Get workflow instance ID
	var wfiID string
	err := env.pool.QueryRow(`SELECT id FROM workflow_instances WHERE LOWER(project_id) = LOWER(?) AND LOWER(ticket_id) = LOWER(?) AND LOWER(workflow_id) = LOWER(?)`,
		env.project, ticketID, "test").Scan(&wfiID)
	if err != nil {
		t.Fatalf("failed to get workflow instance ID: %v", err)
	}

	// Create agent session directly in DB
	sessionID := "sess-coal-3"
	_, err = env.pool.Exec(`
		INSERT INTO agent_sessions (id, project_id, ticket_id, workflow_instance_id, phase, agent_type, model_id, status, created_at, updated_at)
		VALUES (?, ?, ?, ?, 'analyzer', 'analyzer', 'cli:claude-opus-4', 'running', datetime('now'), datetime('now'))
	`, sessionID, env.project, ticketID, wfiID)
	if err != nil {
		t.Fatalf("failed to create session: %v", err)
	}

	// Subscribe a test client to receive broadcasts
	client, sendCh := ws.NewTestClient(hub, "test-client")
	hub.Register(client)
	time.Sleep(50 * time.Millisecond)
	hub.Subscribe(client, env.project, ticketID)

	// Create a process info
	proc := &processInfo{
		sessionID:       sessionID,
		projectID:       env.project,
		ticketID:        ticketID,
		agentType:       "analyzer",
		workflowName:    "test",
		modelID:         "cli:claude-opus-4",
		pendingMessages: make([]string, 0),
		nextSeq:         0,
	}

	// Track and save messages
	spawner.trackMessage(proc, "Test message")
	spawner.saveMessages(proc)

	// Verify broadcast payload
	select {
	case msg := <-sendCh:
		var event ws.Event
		if err := json.Unmarshal(msg, &event); err != nil {
			t.Fatalf("failed to unmarshal event: %v", err)
		}

		// Verify event type
		if event.Type != ws.EventMessagesUpdated {
			t.Fatalf("expected event type %s, got %s", ws.EventMessagesUpdated, event.Type)
		}

		// Verify required payload fields
		sid, ok := event.Data["session_id"].(string)
		if !ok || sid != sessionID {
			t.Errorf("expected session_id=%s, got: %v", sessionID, event.Data["session_id"])
		}

		agentType, ok := event.Data["agent_type"].(string)
		if !ok || agentType != "analyzer" {
			t.Errorf("expected agent_type=analyzer, got: %v", event.Data["agent_type"])
		}

		modelID, ok := event.Data["model_id"].(string)
		if !ok || modelID != "cli:claude-opus-4" {
			t.Errorf("expected model_id=cli:claude-opus-4, got: %v", event.Data["model_id"])
		}

	case <-time.After(500 * time.Millisecond):
		t.Fatal("timeout waiting for broadcast")
	}
}
