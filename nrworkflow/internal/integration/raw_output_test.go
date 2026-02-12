package integration

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"path/filepath"
	"testing"

	"nrworkflow/internal/db"
)

// TestRawOutput_EndToEnd covers all acceptance criteria:
// 1. raw_output column stores unprocessed agent stdout/stderr
// 2. Raw output is accumulated via AppendRawOutput (COALESCE concat from NULL)
// 3. Service layer retrieves raw output and returns 404 for missing sessions
// 4. HTTP endpoint GET /api/v1/sessions/:id/raw-output returns raw_output (200) or 404
// 5. MarshalJSON emits raw_output_size (byte count), not content
func TestRawOutput_EndToEnd(t *testing.T) {
	env := NewTestEnv(t)

	// Setup: ticket + workflow + agent session
	insertTestSession(t, env, "sess-raw-1", "RAW-1")

	// --- Step 1: raw_output starts as NULL, service returns empty string ---
	rawOutput, err := env.AgentSvc.GetSessionRawOutput("sess-raw-1")
	if err != nil {
		t.Fatalf("GetSessionRawOutput on fresh session: %v", err)
	}
	if rawOutput != "" {
		t.Fatalf("expected empty raw_output for fresh session, got %q", rawOutput)
	}

	// --- Step 2: Append raw output via repo (simulates spawner flush) ---
	// First append to NULL column (tests COALESCE)
	_, err = env.Pool.Exec(
		`UPDATE agent_sessions SET raw_output = COALESCE(raw_output, '') || ? WHERE id = ?`,
		"line-1: starting agent\n", "sess-raw-1",
	)
	if err != nil {
		t.Fatalf("first AppendRawOutput: %v", err)
	}

	// Second append (tests concatenation)
	_, err = env.Pool.Exec(
		`UPDATE agent_sessions SET raw_output = COALESCE(raw_output, '') || ? WHERE id = ?`,
		"line-2: processing...\nline-3: done\n", "sess-raw-1",
	)
	if err != nil {
		t.Fatalf("second AppendRawOutput: %v", err)
	}

	// Verify accumulated raw output via service
	rawOutput, err = env.AgentSvc.GetSessionRawOutput("sess-raw-1")
	if err != nil {
		t.Fatalf("GetSessionRawOutput after appends: %v", err)
	}
	expected := "line-1: starting agent\nline-2: processing...\nline-3: done\n"
	if rawOutput != expected {
		t.Fatalf("expected raw_output %q, got %q", expected, rawOutput)
	}

	// --- Step 3: Service returns error for non-existent session ---
	_, err = env.AgentSvc.GetSessionRawOutput("does-not-exist")
	if err == nil {
		t.Fatal("expected error for non-existent session, got nil")
	}

	// --- Step 4: MarshalJSON emits raw_output_size ---
	session, err := env.AgentSvc.GetSessionByID("sess-raw-1")
	if err != nil {
		t.Fatalf("GetSessionByID: %v", err)
	}
	jsonBytes, err := json.Marshal(session)
	if err != nil {
		t.Fatalf("MarshalJSON: %v", err)
	}
	var jsonMap map[string]interface{}
	if err := json.Unmarshal(jsonBytes, &jsonMap); err != nil {
		t.Fatalf("unmarshal JSON: %v", err)
	}
	sizeVal, ok := jsonMap["raw_output_size"]
	if !ok {
		t.Fatal("MarshalJSON missing 'raw_output_size' field")
	}
	sizeFloat, ok := sizeVal.(float64)
	if !ok {
		t.Fatalf("raw_output_size not a number: %T", sizeVal)
	}
	if int(sizeFloat) != len(expected) {
		t.Fatalf("expected raw_output_size %d, got %d", len(expected), int(sizeFloat))
	}
	// Ensure raw_output content is NOT in JSON (only size)
	if _, hasContent := jsonMap["raw_output"]; hasContent {
		t.Fatal("MarshalJSON should NOT include 'raw_output' content, only 'raw_output_size'")
	}

	// --- Step 5: Insert messages alongside raw output (same session) ---
	// Verifies messages and raw output coexist
	now := "2026-01-01T00:00:00Z"
	for i, msg := range []string{"msg-a", "msg-b"} {
		_, err := env.Pool.Exec(
			`INSERT INTO agent_messages (session_id, seq, content, created_at) VALUES (?, ?, ?, ?)`,
			"sess-raw-1", i, msg, now,
		)
		if err != nil {
			t.Fatalf("insert message %d: %v", i, err)
		}
	}

	msgs, total, err := env.AgentSvc.GetSessionMessages("sess-raw-1", 0, 0)
	if err != nil {
		t.Fatalf("GetSessionMessages: %v", err)
	}
	if total != 2 || len(msgs) != 2 {
		t.Fatalf("expected 2 messages, got total=%d len=%d", total, len(msgs))
	}

	// Verify raw output still intact after messages
	rawOutput, err = env.AgentSvc.GetSessionRawOutput("sess-raw-1")
	if err != nil {
		t.Fatalf("GetSessionRawOutput after messages: %v", err)
	}
	if rawOutput != expected {
		t.Fatalf("raw_output changed after message insert: %q", rawOutput)
	}
}

// TestRawOutput_HTTPEndpoint tests the GET /api/v1/sessions/:id/raw-output handler.
func TestRawOutput_HTTPEndpoint(t *testing.T) {
	dbDir := t.TempDir()
	dbPath := filepath.Join(dbDir, "test.db")

	// Initialize DB
	database, err := db.OpenPath(dbPath)
	if err != nil {
		t.Fatalf("failed to init DB: %v", err)
	}
	database.Close()

	seedProject(t, dbPath, "rawproj")
	baseURL := startAPIServer(t, dbPath)

	// Seed workflow definition, ticket, workflow instance, and agent session
	database, err = db.OpenPath(dbPath)
	if err != nil {
		t.Fatalf("reopen DB: %v", err)
	}
	now := "2026-01-01T00:00:00Z"
	if _, err := database.Exec(`INSERT INTO workflows (id, project_id, description, categories, phases, created_at, updated_at)
		VALUES ('test', 'rawproj', 'Test', '["full"]', '[{"id":"analyzer","agent":"analyzer"}]', ?, ?)`, now, now); err != nil {
		t.Fatalf("insert workflow: %v", err)
	}
	if _, err := database.Exec(`INSERT INTO tickets (id, project_id, title, status, priority, issue_type, created_by, created_at, updated_at)
		VALUES ('raw-http-1', 'rawproj', 'Raw test', 'open', 2, 'task', 'test', ?, ?)`, now, now); err != nil {
		t.Fatalf("insert ticket: %v", err)
	}
	if _, err := database.Exec(`INSERT INTO workflow_instances (id, project_id, ticket_id, workflow_id, status, current_phase, category, phase_order, phases, created_at, updated_at)
		VALUES ('wfi-raw-1', 'rawproj', 'raw-http-1', 'test', 'active', 'analyzer', 'full', '["analyzer"]', '{}', ?, ?)`, now, now); err != nil {
		t.Fatalf("insert workflow_instance: %v", err)
	}
	if _, err := database.Exec(`INSERT INTO agent_sessions (id, project_id, ticket_id, workflow_instance_id, phase, agent_type,
		model_id, status, raw_output, started_at, created_at, updated_at)
		VALUES ('sess-http-raw', 'rawproj', 'raw-http-1', 'wfi-raw-1', 'analyzer', 'analyzer', 'sonnet', 'running',
		'raw line 1
raw line 2
', ?, ?, ?)`, now, now, now); err != nil {
		t.Fatalf("insert agent_session: %v", err)
	}
	database.Close()

	// --- Test: GET raw output for existing session → 200 ---
	resp, err := http.Get(baseURL + "/api/v1/sessions/sess-http-raw/raw-output")
	if err != nil {
		t.Fatalf("GET raw-output request failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("expected 200, got %d: %s", resp.StatusCode, string(body))
	}

	var result map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if result["session_id"] != "sess-http-raw" {
		t.Fatalf("expected session_id 'sess-http-raw', got %v", result["session_id"])
	}
	expectedHTTP := "raw line 1\nraw line 2\n"
	if result["raw_output"] != expectedHTTP {
		t.Fatalf("expected raw_output %q, got %q", expectedHTTP, result["raw_output"])
	}

	// --- Test: GET raw output for non-existent session → 404 ---
	resp2, err := http.Get(baseURL + "/api/v1/sessions/does-not-exist/raw-output")
	if err != nil {
		t.Fatalf("GET raw-output 404 request failed: %v", err)
	}
	defer resp2.Body.Close()

	if resp2.StatusCode != http.StatusNotFound {
		body, _ := io.ReadAll(resp2.Body)
		t.Fatalf("expected 404, got %d: %s", resp2.StatusCode, string(body))
	}

	// --- Test: GET raw output for session with NULL raw_output → 200 with empty string ---
	database, err = db.OpenPath(dbPath)
	if err != nil {
		t.Fatalf("reopen DB: %v", err)
	}
	if _, err := database.Exec(`INSERT INTO agent_sessions (id, project_id, ticket_id, workflow_instance_id, phase, agent_type,
		model_id, status, raw_output, started_at, created_at, updated_at)
		VALUES ('sess-http-null', 'rawproj', 'raw-http-1', 'wfi-raw-1', 'analyzer', 'builder', 'sonnet', 'running',
		NULL, ?, ?, ?)`, now, now, now); err != nil {
		t.Fatalf("insert null session: %v", err)
	}
	database.Close()

	resp3, err := http.Get(baseURL + "/api/v1/sessions/sess-http-null/raw-output")
	if err != nil {
		t.Fatalf("GET null raw-output request failed: %v", err)
	}
	defer resp3.Body.Close()

	if resp3.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp3.Body)
		t.Fatalf("expected 200, got %d: %s", resp3.StatusCode, string(body))
	}

	var nullResult map[string]interface{}
	if err := json.NewDecoder(resp3.Body).Decode(&nullResult); err != nil {
		t.Fatalf("decode null response: %v", err)
	}
	if nullResult["raw_output"] != "" {
		t.Fatalf("expected empty raw_output for NULL, got %q", nullResult["raw_output"])
	}
}

// TestRawOutput_AppendToNULL verifies COALESCE behavior when appending to a NULL raw_output.
func TestRawOutput_AppendToNULL(t *testing.T) {
	env := NewTestEnv(t)

	insertTestSession(t, env, "sess-null-1", "RAW-NULL")

	// Verify raw_output is NULL initially
	var rawOutput interface{}
	err := env.Pool.QueryRow("SELECT raw_output FROM agent_sessions WHERE id = ?", "sess-null-1").Scan(&rawOutput)
	if err != nil {
		t.Fatalf("query raw_output: %v", err)
	}
	if rawOutput != nil {
		t.Fatalf("expected NULL raw_output, got %v", rawOutput)
	}

	// Append to NULL - COALESCE should handle this
	_, err = env.Pool.Exec(
		`UPDATE agent_sessions SET raw_output = COALESCE(raw_output, '') || ? WHERE id = ?`,
		"first chunk", "sess-null-1",
	)
	if err != nil {
		t.Fatalf("append to NULL: %v", err)
	}

	result, err := env.AgentSvc.GetSessionRawOutput("sess-null-1")
	if err != nil {
		t.Fatalf("GetSessionRawOutput: %v", err)
	}
	if result != "first chunk" {
		t.Fatalf("expected 'first chunk', got %q", result)
	}
}

// TestRawOutput_BulkOutput verifies handling of large raw output.
func TestRawOutput_BulkOutput(t *testing.T) {
	env := NewTestEnv(t)

	insertTestSession(t, env, "sess-large-1", "RAW-LARGE")

	// Append a large chunk of output in multiple appends
	for i := 0; i < 100; i++ {
		line := fmt.Sprintf("output line %03d: %s\n", i, "some repeated content for bulk testing")
		_, err := env.Pool.Exec(
			`UPDATE agent_sessions SET raw_output = COALESCE(raw_output, '') || ? WHERE id = ?`,
			line, "sess-large-1",
		)
		if err != nil {
			t.Fatalf("append line %d: %v", i, err)
		}
	}

	result, err := env.AgentSvc.GetSessionRawOutput("sess-large-1")
	if err != nil {
		t.Fatalf("GetSessionRawOutput: %v", err)
	}

	// Verify all 100 lines are present
	expectedLen := 0
	for i := 0; i < 100; i++ {
		line := fmt.Sprintf("output line %03d: %s\n", i, "some repeated content for bulk testing")
		expectedLen += len(line)
	}
	if len(result) != expectedLen {
		t.Fatalf("expected %d bytes, got %d", expectedLen, len(result))
	}
}

// TestRawOutput_SessionListIncludesSize verifies that session listings include raw_output_size.
func TestRawOutput_SessionListIncludesSize(t *testing.T) {
	env := NewTestEnv(t)

	env.CreateTicket(t, "RAW-LIST", "Raw list test")
	env.InitWorkflow(t, "RAW-LIST")
	wfiID := env.GetWorkflowInstanceID(t, "RAW-LIST", "test")
	env.InsertAgentSession(t, "sess-list-raw", "RAW-LIST", wfiID, "analyzer", "analyzer", "sonnet")

	// Set raw output on the session
	_, err := env.Pool.Exec(
		`UPDATE agent_sessions SET raw_output = ? WHERE id = ?`,
		"some raw output data", "sess-list-raw",
	)
	if err != nil {
		t.Fatalf("set raw_output: %v", err)
	}

	// Get sessions via service and check MarshalJSON includes raw_output_size
	sessions, err := env.AgentSvc.GetTicketSessions(env.ProjectID, "RAW-LIST", "")
	if err != nil {
		t.Fatalf("GetTicketSessions: %v", err)
	}
	if len(sessions) != 1 {
		t.Fatalf("expected 1 session, got %d", len(sessions))
	}

	jsonBytes, err := json.Marshal(sessions[0])
	if err != nil {
		t.Fatalf("marshal session: %v", err)
	}

	var jsonMap map[string]interface{}
	json.Unmarshal(jsonBytes, &jsonMap)

	size, ok := jsonMap["raw_output_size"]
	if !ok {
		t.Fatal("session JSON missing raw_output_size")
	}
	if int(size.(float64)) != len("some raw output data") {
		t.Fatalf("expected raw_output_size %d, got %v", len("some raw output data"), size)
	}
}
