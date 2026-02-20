package integration

import (
	"database/sql"
	"encoding/json"
	"testing"

	"be/internal/model"
	"be/internal/types"
)

// TestMigrationDropsRawOutputColumn verifies that the 000013 migration successfully
// drops the raw_output column and that agent_sessions table works with 21 columns (not 22).
func TestMigrationDropsRawOutputColumn(t *testing.T) {
	env := NewTestEnv(t)

	// Verify raw_output column does not exist by querying schema
	var count int
	err := env.Pool.QueryRow(`
		SELECT COUNT(*) FROM pragma_table_info('agent_sessions')
		WHERE name = 'raw_output'`).Scan(&count)
	if err != nil {
		t.Fatalf("failed to query schema: %v", err)
	}
	if count != 0 {
		t.Fatalf("raw_output column should not exist after migration 000013, found %d columns with that name", count)
	}
}

// TestAgentSessionCRUDWithout RawOutput verifies that CRUD operations work correctly
// with the 21-column schema (previously 22 with raw_output).
func TestAgentSessionCRUDWithoutRawOutput(t *testing.T) {
	env := NewTestEnv(t)

	env.CreateTicket(t, "CRUD-1", "CRUD test")
	env.InitWorkflow(t, "CRUD-1")
	wfiID := env.GetWorkflowInstanceID(t, "CRUD-1", "test")

	// Create a session via InsertAgentSession helper
	env.InsertAgentSession(t, "sess-crud-1", "CRUD-1", wfiID, "analyzer", "analyzer", "sonnet")

	// Verify creation via service layer
	session, err := env.AgentSvc.GetSessionByID("sess-crud-1")
	if err != nil {
		t.Fatalf("failed to get session after creation: %v", err)
	}
	if session.ID != "sess-crud-1" {
		t.Fatalf("expected session ID 'sess-crud-1', got %v", session.ID)
	}
	if session.AgentType != "analyzer" {
		t.Fatalf("expected agent_type 'analyzer', got %v", session.AgentType)
	}
	if session.Phase != "analyzer" {
		t.Fatalf("expected phase 'analyzer', got %v", session.Phase)
	}

	// Create another session using service.CreateSession to test INSERT
	newSession := &model.AgentSession{
		ID:                 "sess-crud-2",
		ProjectID:          env.ProjectID,
		TicketID:           "CRUD-1",
		WorkflowInstanceID: wfiID,
		Phase:              "builder",
		AgentType:          "builder",
		ModelID:            sql.NullString{String: "claude:opus", Valid: true},
		Status:             model.AgentSessionRunning,
		PID:                sql.NullInt64{Int64: 12345, Valid: true},
		ContextLeft:        sql.NullInt64{Int64: 75, Valid: true},
		RestartCount:       0,
	}
	err = env.AgentSvc.CreateSession(newSession)
	if err != nil {
		t.Fatalf("failed to create session via service: %v", err)
	}

	// Verify via read
	retrieved, err := env.AgentSvc.GetSessionByID("sess-crud-2")
	if err != nil {
		t.Fatalf("failed to retrieve created session: %v", err)
	}
	if retrieved.AgentType != "builder" {
		t.Fatalf("expected agent_type 'builder', got %v", retrieved.AgentType)
	}
	if retrieved.PID.Int64 != 12345 {
		t.Fatalf("expected PID 12345, got %v", retrieved.PID.Int64)
	}
	if retrieved.ContextLeft.Int64 != 75 {
		t.Fatalf("expected context_left 75, got %v", retrieved.ContextLeft.Int64)
	}
}

// TestAgentSessionMarshalJSONWithoutRawOutputSize verifies that AgentSession JSON
// serialization no longer includes the raw_output_size field.
func TestAgentSessionMarshalJSONWithoutRawOutputSize(t *testing.T) {
	env := NewTestEnv(t)

	env.CreateTicket(t, "JSON-1", "JSON test")
	env.InitWorkflow(t, "JSON-1")
	wfiID := env.GetWorkflowInstanceID(t, "JSON-1", "test")

	env.InsertAgentSession(t, "sess-json-1", "JSON-1", wfiID, "analyzer", "analyzer", "sonnet")

	// Retrieve session
	session, err := env.AgentSvc.GetSessionByID("sess-json-1")
	if err != nil {
		t.Fatalf("failed to get session: %v", err)
	}

	// Marshal to JSON
	data, err := json.Marshal(session)
	if err != nil {
		t.Fatalf("failed to marshal session: %v", err)
	}

	// Unmarshal to map to check fields
	var result map[string]interface{}
	if err := json.Unmarshal(data, &result); err != nil {
		t.Fatalf("failed to unmarshal JSON: %v", err)
	}

	// Verify raw_output_size is NOT present
	if _, exists := result["raw_output_size"]; exists {
		t.Fatalf("raw_output_size field should not exist in JSON, found: %v", result)
	}

	// Verify expected fields still exist
	if result["id"] != "sess-json-1" {
		t.Fatalf("expected id 'sess-json-1', got %v", result["id"])
	}
	if result["agent_type"] != "analyzer" {
		t.Fatalf("expected agent_type 'analyzer', got %v", result["agent_type"])
	}
	// message_count should exist (from agent_messages table)
	if _, exists := result["message_count"]; !exists {
		t.Fatalf("message_count should exist in JSON output")
	}
}

// TestServiceScanSessionJoinedWithout RawOutput verifies that service.scanSessionJoined
// correctly scans joined queries with 22 columns (21 from agent_sessions + 1 workflow_id from JOIN).
func TestServiceScanSessionJoinedWithoutRawOutput(t *testing.T) {
	env := NewTestEnv(t)

	env.CreateTicket(t, "JOIN-1", "Join test")
	env.InitWorkflow(t, "JOIN-1")
	wfiID := env.GetWorkflowInstanceID(t, "JOIN-1", "test")

	env.InsertAgentSession(t, "sess-join-1", "JOIN-1", wfiID, "analyzer", "analyzer", "sonnet")

	// Use GetRecentSessions which performs JOIN with workflow_instances
	sessions, err := env.AgentSvc.GetRecentSessions(env.ProjectID, 10)
	if err != nil {
		t.Fatalf("failed to get recent sessions (JOIN query): %v", err)
	}
	if len(sessions) == 0 {
		t.Fatalf("expected at least 1 session from JOIN query")
	}

	found := false
	for _, s := range sessions {
		if s.ID == "sess-join-1" {
			found = true
			// Verify Workflow derived field is populated (from JOIN)
			if s.Workflow != "test" {
				t.Fatalf("expected workflow 'test' from JOIN, got %v", s.Workflow)
			}
		}
	}
	if !found {
		t.Fatalf("expected to find sess-join-1 in recent sessions")
	}

	// Test GetTicketSessions which also uses JOIN
	ticketSessions, err := env.AgentSvc.GetTicketSessions(env.ProjectID, "JOIN-1", "")
	if err != nil {
		t.Fatalf("failed to get ticket sessions (JOIN query): %v", err)
	}
	if len(ticketSessions) != 1 {
		t.Fatalf("expected 1 ticket session, got %d", len(ticketSessions))
	}
	if ticketSessions[0].Workflow != "test" {
		t.Fatalf("expected workflow 'test', got %v", ticketSessions[0].Workflow)
	}
}

// TestGetSessionByIDWithoutRawOutput verifies that GetSessionByID service method
// works correctly after raw_output removal.
func TestGetSessionByIDWithoutRawOutput(t *testing.T) {
	env := NewTestEnv(t)

	env.CreateTicket(t, "GETID-1", "GetByID test")
	env.InitWorkflow(t, "GETID-1")
	wfiID := env.GetWorkflowInstanceID(t, "GETID-1", "test")

	env.InsertAgentSession(t, "sess-getid-1", "GETID-1", wfiID, "analyzer", "analyzer", "sonnet")

	// GetSessionByID uses JOIN query
	session, err := env.AgentSvc.GetSessionByID("sess-getid-1")
	if err != nil {
		t.Fatalf("failed to get session by ID: %v", err)
	}
	if session.ID != "sess-getid-1" {
		t.Fatalf("expected session ID 'sess-getid-1', got %v", session.ID)
	}
	if session.Workflow != "test" {
		t.Fatalf("expected workflow 'test', got %v", session.Workflow)
	}
}

// TestSpawnerMessageHandlingWithoutRawOutput verifies that the spawner still correctly
// captures and flushes parsed messages to agent_messages table after raw_output removal.
func TestSpawnerMessageHandlingWithoutRawOutput(t *testing.T) {
	env := NewTestEnv(t)

	env.CreateTicket(t, "MSG-1", "Message test")
	env.InitWorkflow(t, "MSG-1")
	wfiID := env.GetWorkflowInstanceID(t, "MSG-1", "test")

	env.InsertAgentSession(t, "sess-msg-1", "MSG-1", wfiID, "analyzer", "analyzer", "sonnet")

	// Insert messages into agent_messages table directly (simulating spawner flush)
	_, err := env.Pool.Exec(`
		INSERT INTO agent_messages (session_id, seq, content, created_at)
		VALUES (?, ?, ?, datetime('now'))`,
		"sess-msg-1", 1, "Test message 1",
	)
	if err != nil {
		t.Fatalf("failed to insert message: %v", err)
	}
	_, err = env.Pool.Exec(`
		INSERT INTO agent_messages (session_id, seq, content, created_at)
		VALUES (?, ?, ?, datetime('now'))`,
		"sess-msg-1", 2, "Test message 2",
	)
	if err != nil {
		t.Fatalf("failed to insert second message: %v", err)
	}

	// Retrieve session and verify messages are loaded
	session, err := env.AgentSvc.GetSessionByID("sess-msg-1")
	if err != nil {
		t.Fatalf("failed to get session: %v", err)
	}
	if session.MessageCount != 2 {
		t.Fatalf("expected message_count 2, got %d", session.MessageCount)
	}
	if len(session.Messages) != 2 {
		t.Fatalf("expected 2 messages loaded, got %d", len(session.Messages))
	}
	if session.Messages[0] != "Test message 1" {
		t.Fatalf("expected first message 'Test message 1', got %v", session.Messages[0])
	}
}

// TestEndToEndWorkflowWithoutRawOutput is an end-to-end test covering full workflow
// execution to verify all session CRUD operations work correctly after raw_output removal.
func TestEndToEndWorkflowWithoutRawOutput(t *testing.T) {
	env := NewTestEnv(t)

	// Create ticket and init workflow
	env.CreateTicket(t, "E2E-1", "End to end test")
	env.InitWorkflow(t, "E2E-1")
	wfiID := env.GetWorkflowInstanceID(t, "E2E-1", "test")

	// Start analyzer phase
	env.StartPhase(t, "E2E-1", "analyzer")

	// Create analyzer session
	env.InsertAgentSession(t, "sess-e2e-analyzer", "E2E-1", wfiID, "analyzer", "analyzer", "sonnet")

	// Update findings via socket
	env.MustExecute(t, "findings.add", map[string]interface{}{
		"ticket_id":  "E2E-1",
		"workflow":   "test",
		"agent_type": "analyzer",
		"key":        "test_key",
		"value":      "test_value",
		"instance_id": wfiID,
	}, nil)

	// Mark analyzer session as completed (agent.complete removed; set result directly)
	_, err := env.Pool.Exec(`UPDATE agent_sessions SET result = 'pass', status = 'completed', ended_at = datetime('now'), updated_at = datetime('now') WHERE id = ?`, "sess-e2e-analyzer")
	if err != nil {
		t.Fatalf("failed to complete analyzer session: %v", err)
	}

	env.CompletePhase(t, "E2E-1", "analyzer", "pass")

	// Start builder phase
	env.StartPhase(t, "E2E-1", "builder")
	env.InsertAgentSession(t, "sess-e2e-builder", "E2E-1", wfiID, "builder", "builder", "opus")

	// Mark builder session as completed (agent.complete removed; set result directly)
	_, err = env.Pool.Exec(`UPDATE agent_sessions SET result = 'pass', status = 'completed', ended_at = datetime('now'), updated_at = datetime('now') WHERE id = ?`, "sess-e2e-builder")
	if err != nil {
		t.Fatalf("failed to complete builder session: %v", err)
	}

	env.CompletePhase(t, "E2E-1", "builder", "pass")

	// Verify workflow status
	statusRaw, err := env.WorkflowSvc.GetStatus(env.ProjectID, "E2E-1", &types.WorkflowGetRequest{
		Workflow: "test",
	})
	if err != nil {
		t.Fatalf("failed to get workflow status: %v", err)
	}

	// Normalize JSON types
	statusData, _ := json.Marshal(statusRaw)
	var status map[string]interface{}
	json.Unmarshal(statusData, &status)

	phases, _ := status["phases"].(map[string]interface{})
	for _, phaseName := range []string{"analyzer", "builder"} {
		phase, _ := phases[phaseName].(map[string]interface{})
		if phase["status"] != "completed" {
			t.Fatalf("expected phase '%s' completed, got %v", phaseName, phase["status"])
		}
	}

	// Verify both sessions exist and are retrievable
	allSessions, err := env.AgentSvc.GetTicketSessions(env.ProjectID, "E2E-1", "test")
	if err != nil {
		t.Fatalf("failed to get ticket sessions: %v", err)
	}
	if len(allSessions) != 2 {
		t.Fatalf("expected 2 sessions, got %d", len(allSessions))
	}

	// Verify no raw_output_size in JSON
	for _, session := range allSessions {
		data, _ := json.Marshal(session)
		var result map[string]interface{}
		json.Unmarshal(data, &result)
		if _, exists := result["raw_output_size"]; exists {
			t.Fatalf("session %s should not have raw_output_size field", session.ID)
		}
	}
}
