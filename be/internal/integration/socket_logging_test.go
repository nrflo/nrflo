package integration

import (
	"bytes"
	"fmt"
	"os"
	"strings"
	"testing"

	"be/internal/logger"
)

// TestSocketHandler_AgentFail_LogsWithWarnLevel verifies agent.fail logs at WARN level.
func TestSocketHandler_AgentFail_LogsWithWarnLevel(t *testing.T) {
	env := NewTestEnv(t)

	var logBuf bytes.Buffer
	logger.SetWriter(&logBuf)
	defer logger.SetWriter(os.Stderr)

	env.CreateTicket(t, "SOCK-2", "Agent fail logging")
	env.InitWorkflow(t, "SOCK-2")

	wfiID := env.GetWorkflowInstanceID(t, "SOCK-2", "test")
	env.InsertAgentSession(t, "sess-2", "SOCK-2", wfiID, "builder", "builder", "opus_4_7")

	// Execute agent.fail via socket
	env.MustExecute(t, "agent.fail", map[string]interface{}{
		"ticket_id":   "SOCK-2",
		"workflow":    "test",
		"agent_type":  "builder",
		"session_id":  "sess-2",
		"instance_id": wfiID,
	}, nil)

	logs := logBuf.String()

	// Verify log line exists with WARN level
	logLines := extractLogLines(logs, "agent fail received")
	if len(logLines) != 1 {
		t.Fatalf("expected 1 log line, got %d", len(logLines))
	}

	if !strings.Contains(logLines[0], "WARN") {
		t.Errorf("expected WARN level, got: %s", logLines[0])
	}

	// Verify details
	if !strings.Contains(logs, "agent_type=builder") {
		t.Errorf("logs missing agent_type: %s", logs)
	}
	if !strings.Contains(logs, "ticket=SOCK-2") {
		t.Errorf("logs missing ticket: %s", logs)
	}
	if !strings.Contains(logs, "workflow=test") {
		t.Errorf("logs missing workflow: %s", logs)
	}

	// Verify trx
	trx := extractTrx(logLines[0])
	if trx == "" {
		t.Errorf("log line missing trx: %s", logLines[0])
	}
}

// TestSocketHandler_AgentContinue_LogsDetails verifies agent.continue logs with details.
func TestSocketHandler_AgentContinue_LogsDetails(t *testing.T) {
	env := NewTestEnv(t)

	var logBuf bytes.Buffer
	logger.SetWriter(&logBuf)
	defer logger.SetWriter(os.Stderr)

	env.CreateTicket(t, "SOCK-3", "Agent continue logging")
	env.InitWorkflow(t, "SOCK-3")

	wfiID := env.GetWorkflowInstanceID(t, "SOCK-3", "test")
	env.InsertAgentSession(t, "sess-3", "SOCK-3", wfiID, "analyzer", "analyzer", "sonnet")

	// Execute agent.continue via socket
	env.MustExecute(t, "agent.continue", map[string]interface{}{
		"ticket_id":   "SOCK-3",
		"workflow":    "test",
		"agent_type":  "analyzer",
		"session_id":  "sess-3",
		"instance_id": wfiID,
	}, nil)

	logs := logBuf.String()

	// Verify log line exists
	if !strings.Contains(logs, "agent continue received") {
		t.Errorf("logs missing 'agent continue received': %s", logs)
	}

	// Verify details
	if !strings.Contains(logs, "agent_type=analyzer") {
		t.Errorf("logs missing agent_type: %s", logs)
	}
	if !strings.Contains(logs, "ticket=SOCK-3") {
		t.Errorf("logs missing ticket: %s", logs)
	}
	if !strings.Contains(logs, "workflow=test") {
		t.Errorf("logs missing workflow: %s", logs)
	}

	// Verify INFO level
	logLines := extractLogLines(logs, "agent continue received")
	if len(logLines) != 1 {
		t.Fatalf("expected 1 log line, got %d", len(logLines))
	}
	if !strings.Contains(logLines[0], "INFO") {
		t.Errorf("expected INFO level, got: %s", logLines[0])
	}
}

// TestSocketHandler_AgentCallback_LogsWithLevel verifies agent.callback logs with level.
func TestSocketHandler_AgentCallback_LogsWithLevel(t *testing.T) {
	env := NewTestEnv(t)

	var logBuf bytes.Buffer
	logger.SetWriter(&logBuf)
	defer logger.SetWriter(os.Stderr)

	env.CreateTicket(t, "SOCK-4", "Agent callback logging")
	env.InitWorkflow(t, "SOCK-4")

	wfiID := env.GetWorkflowInstanceID(t, "SOCK-4", "test")
	env.InsertAgentSession(t, "sess-4", "SOCK-4", wfiID, "verifier", "verifier", "opus_4_7")

	// Execute agent.callback via socket
	env.MustExecute(t, "agent.callback", map[string]interface{}{
		"ticket_id":    "SOCK-4",
		"workflow":     "test",
		"agent_type":   "verifier",
		"session_id":   "sess-4",
		"level":        2,
		"instructions": "Fix implementation",
		"instance_id":  wfiID,
	}, nil)

	logs := logBuf.String()

	// Verify log line exists
	if !strings.Contains(logs, "agent callback received") {
		t.Errorf("logs missing 'agent callback received': %s", logs)
	}

	// Verify details including level
	if !strings.Contains(logs, "agent_type=verifier") {
		t.Errorf("logs missing agent_type: %s", logs)
	}
	if !strings.Contains(logs, "ticket=SOCK-4") {
		t.Errorf("logs missing ticket: %s", logs)
	}
	if !strings.Contains(logs, "level=2") {
		t.Errorf("logs missing level: %s", logs)
	}

	// Verify INFO level
	logLines := extractLogLines(logs, "agent callback received")
	if len(logLines) != 1 {
		t.Fatalf("expected 1 log line, got %d", len(logLines))
	}
	if !strings.Contains(logLines[0], "INFO") {
		t.Errorf("expected INFO level, got: %s", logLines[0])
	}
}

// TestSocketHandler_UnknownMethod_LogsWarning verifies unknown methods log as warnings.
func TestSocketHandler_UnknownMethod_LogsWarning(t *testing.T) {
	env := NewTestEnv(t)

	var logBuf bytes.Buffer
	logger.SetWriter(&logBuf)
	defer logger.SetWriter(os.Stderr)

	// Execute unknown method via socket
	env.ExpectError(t, "agent.unknown", map[string]interface{}{
		"ticket_id":  "SOCK-5",
		"agent_type": "test",
	}, -32601) // Method not found error code

	logs := logBuf.String()

	// Verify warning log
	logLines := extractLogLines(logs, "unknown socket method")
	if len(logLines) == 0 {
		t.Fatalf("expected warning log for unknown method, got none: %s", logs)
	}

	// Verify WARN level
	if !strings.Contains(logLines[0], "WARN") {
		t.Errorf("expected WARN level, got: %s", logLines[0])
	}

	// Verify method is logged
	if !strings.Contains(logs, "method=agent.unknown") {
		t.Errorf("logs missing method: %s", logs)
	}
}

// TestSocketHandler_AgentError_LogsError verifies agent service errors are logged.
func TestSocketHandler_AgentError_LogsError(t *testing.T) {
	env := NewTestEnv(t)

	var logBuf bytes.Buffer
	logger.SetWriter(&logBuf)
	defer logger.SetWriter(os.Stderr)

	// Try to fail non-existent agent (will fail)
	env.ExpectError(t, "agent.fail", map[string]interface{}{
		"ticket_id":  "NONEXISTENT",
		"workflow":   "test",
		"agent_type": "analyzer",
	}, -32603) // Internal error code

	logs := logBuf.String()

	// Verify error log exists
	logLines := extractLogLines(logs, "socket handler error")
	if len(logLines) == 0 {
		t.Fatalf("expected error log for agent service error, got none: %s", logs)
	}

	// Verify ERROR level
	if !strings.Contains(logLines[0], "ERROR") {
		t.Errorf("expected ERROR level, got: %s", logLines[0])
	}

	// Verify method is logged
	if !strings.Contains(logs, "method=agent.fail") {
		t.Errorf("logs missing method: %s", logs)
	}

	// Verify error details
	if !strings.Contains(logs, "error=") {
		t.Errorf("logs missing error details: %s", logs)
	}
}

// TestSocketHandler_WSBroadcast_LogsTypeAndProject verifies ws.broadcast logs type and project.
func TestSocketHandler_WSBroadcast_LogsTypeAndProject(t *testing.T) {
	env := NewTestEnv(t)

	var logBuf bytes.Buffer
	logger.SetWriter(&logBuf)
	defer logger.SetWriter(os.Stderr)

	// Execute ws.broadcast via socket
	env.MustExecute(t, "ws.broadcast", map[string]interface{}{
		"type":       "test.event",
		"project_id": env.ProjectID,
		"ticket_id":  "SOCK-6",
		"data": map[string]interface{}{
			"key": "value",
		},
	}, nil)

	logs := logBuf.String()

	// Verify log line exists
	if !strings.Contains(logs, "ws broadcast") {
		t.Errorf("logs missing 'ws broadcast': %s", logs)
	}

	// Verify details
	if !strings.Contains(logs, "type=test.event") {
		t.Errorf("logs missing type: %s", logs)
	}
	if !strings.Contains(logs, fmt.Sprintf("project=%s", env.ProjectID)) {
		t.Errorf("logs missing project: %s", logs)
	}

	// Verify INFO level
	logLines := extractLogLines(logs, "ws broadcast")
	if len(logLines) != 1 {
		t.Fatalf("expected 1 log line, got %d", len(logLines))
	}
	if !strings.Contains(logLines[0], "INFO") {
		t.Errorf("expected INFO level, got: %s", logLines[0])
	}
}

// TestSocketHandler_FindingsNotLogged verifies findings operations are not logged per-request.
func TestSocketHandler_FindingsNotLogged(t *testing.T) {
	env := NewTestEnv(t)

	var logBuf bytes.Buffer
	logger.SetWriter(&logBuf)
	defer logger.SetWriter(os.Stderr)

	env.CreateTicket(t, "SOCK-7", "Findings test")
	env.InitWorkflow(t, "SOCK-7")

	// Create a session so findings.add works
	wfiID := env.GetWorkflowInstanceID(t, "SOCK-7", "test")
	env.InsertAgentSession(t, "sess-7", "SOCK-7", wfiID, "analyzer", "analyzer", "sonnet")

	// Execute findings.add (should not log per-request)
	env.MustExecute(t, "findings.add", map[string]interface{}{
		"session_id":  "sess-7",
		"instance_id": wfiID,
		"key":         "test_key",
		"value":       "test_value",
	}, nil)

	logs := logBuf.String()

	// Verify no findings-specific logs (excluding potential broadcast logs)
	if strings.Contains(logs, "findings.add") {
		t.Errorf("findings operations should not be logged per-request: %s", logs)
	}
}

// TestSocketHandler_TrxGeneratedPerRequest verifies each socket Handle() call gets a new trx.
func TestSocketHandler_TrxGeneratedPerRequest(t *testing.T) {
	env := NewTestEnv(t)

	var logBuf bytes.Buffer
	logger.SetWriter(&logBuf)
	defer logger.SetWriter(os.Stderr)

	env.CreateTicket(t, "SOCK-8", "Trx generation test")
	env.InitWorkflow(t, "SOCK-8")

	wfiID := env.GetWorkflowInstanceID(t, "SOCK-8", "test")
	env.InsertAgentSession(t, "sess-8a", "SOCK-8", wfiID, "analyzer", "analyzer", "sonnet")
	env.InsertAgentSession(t, "sess-8b", "SOCK-8", wfiID, "builder", "builder", "opus_4_7")

	// Execute two agent commands
	env.MustExecute(t, "agent.fail", map[string]interface{}{
		"ticket_id":   "SOCK-8",
		"workflow":    "test",
		"agent_type":  "analyzer",
		"session_id":  "sess-8a",
		"instance_id": wfiID,
	}, nil)

	env.MustExecute(t, "agent.fail", map[string]interface{}{
		"ticket_id":   "SOCK-8",
		"workflow":    "test",
		"agent_type":  "builder",
		"session_id":  "sess-8b",
		"instance_id": wfiID,
	}, nil)

	logs := logBuf.String()

	// Extract trx from both log lines
	logLines := extractLogLines(logs, "agent fail received")
	if len(logLines) != 2 {
		t.Fatalf("expected 2 log lines, got %d", len(logLines))
	}

	trx1 := extractTrx(logLines[0])
	trx2 := extractTrx(logLines[1])

	if trx1 == "" || trx2 == "" {
		t.Fatalf("failed to extract trx from log lines")
	}

	// Verify different trx IDs (each request gets a new trx)
	if trx1 == trx2 {
		t.Errorf("expected different trx IDs per request, got same: %s", trx1)
	}
}

// TestSocketServer_LogsMigrated verifies socket server's log.Printf calls were migrated to logger.
func TestSocketServer_LogsMigrated(t *testing.T) {
	// Capture logs before creating server
	var logBuf bytes.Buffer
	logger.SetWriter(&logBuf)
	defer logger.SetWriter(os.Stderr)

	// Create test env - this starts the socket server which should log
	env := NewTestEnv(t)

	// Verify the server is started (make a simple request)
	env.CreateTicket(t, "SOCK-9", "Log migration test")

	logs := logBuf.String()

	// Verify structured log format (not log.Printf format)
	if !strings.Contains(logs, "socket server listening") {
		t.Errorf("logs missing 'socket server listening': %s", logs)
	}

	// Verify structured logging format (has trx brackets and key=value pairs)
	logLines := extractLogLines(logs, "socket server listening")
	if len(logLines) == 0 {
		t.Fatalf("expected socket server listening log")
	}

	// Should have INFO level and key=value pairs
	if !strings.Contains(logLines[0], "INFO") {
		t.Errorf("expected INFO level in structured log: %s", logLines[0])
	}
	if !strings.Contains(logLines[0], "path=") {
		t.Errorf("expected path= key-value pair: %s", logLines[0])
	}

	// Should NOT have old log.Printf format (which would start with timestamp but no level)
	if strings.Contains(logLines[0], "[socket]") {
		t.Errorf("log should not have old [socket] prefix: %s", logLines[0])
	}
}

// Helper functions

// extractLogLines returns all log lines containing the given substring.
func extractLogLines(logs, substr string) []string {
	var lines []string
	for _, line := range strings.Split(logs, "\n") {
		if strings.Contains(line, substr) {
			lines = append(lines, line)
		}
	}
	return lines
}

// extractTrx extracts the trx ID from a log line (format: [trxid]).
func extractTrx(logLine string) string {
	start := strings.Index(logLine, "[")
	end := strings.Index(logLine, "]")
	if start == -1 || end == -1 || end <= start {
		return ""
	}
	return logLine[start+1 : end]
}
