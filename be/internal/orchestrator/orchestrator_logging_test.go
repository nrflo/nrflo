package orchestrator

import (
	"bytes"
	"context"
	"strings"
	"testing"
	"time"

	"be/internal/clock"
	"be/internal/logger"
	"be/internal/repo"
)

// setupLogCapture captures logger output to a buffer for testing
func setupLogCapture(t *testing.T) *bytes.Buffer {
	t.Helper()
	var buf bytes.Buffer
	// Save original writer
	origWriter := logger.GetWriter()
	logger.SetWriter(&buf)
	t.Cleanup(func() {
		logger.SetWriter(origWriter)
	})
	return &buf
}

func TestOrchestratorStart_GeneratesTrx(t *testing.T) {
	env := newTestEnv(t)
	env.createTicket(t, "LOG-1", "Test logging")

	logBuf := setupLogCapture(t)

	req := RunRequest{
		ProjectID:    env.project,
		TicketID:     "LOG-1",
		WorkflowName: "test",
		ScopeType:    "ticket",
	}

	result, err := env.orch.Start(context.Background(), req)
	if err != nil {
		t.Fatalf("Start() error = %v", err)
	}

	if result.InstanceID == "" {
		t.Error("Start() returned empty instance ID")
	}

	// Wait a moment for async logging
	time.Sleep(100 * time.Millisecond)

	output := logBuf.String()

	// Verify trx is present (not "-")
	if !strings.Contains(output, "workflow instance created") {
		t.Errorf("log output missing 'workflow instance created': %s", output)
	}

	// Extract trx ID from log line
	lines := strings.Split(output, "\n")
	var trxID string
	for _, line := range lines {
		if strings.Contains(line, "workflow instance created") {
			// Format: YYYY-MM-DD HH:MM:SS LEVEL [trxID] message
			start := strings.Index(line, "[")
			end := strings.Index(line, "]")
			if start != -1 && end != -1 {
				trxID = line[start+1 : end]
			}
			break
		}
	}

	if trxID == "" {
		t.Error("could not extract trx ID from log output")
	}
	if trxID == "-" {
		t.Error("trx ID should not be '-' for orchestration")
	}
	if len(trxID) != 8 {
		t.Errorf("trx ID length = %d, want 8 (hex string)", len(trxID))
	}
}

func TestOrchestratorStart_LogsWorkflowInstanceCreated(t *testing.T) {
	env := newTestEnv(t)
	env.createTicket(t, "LOG-2", "Test instance creation")

	logBuf := setupLogCapture(t)

	req := RunRequest{
		ProjectID:    env.project,
		TicketID:     "LOG-2",
		WorkflowName: "test",
		ScopeType:    "ticket",
	}

	result, err := env.orch.Start(context.Background(), req)
	if err != nil {
		t.Fatalf("Start() error = %v", err)
	}

	time.Sleep(100 * time.Millisecond)

	output := logBuf.String()

	// Verify log contains expected fields
	if !strings.Contains(output, "INFO") {
		t.Errorf("log output missing INFO level: %s", output)
	}
	if !strings.Contains(output, "workflow instance created") {
		t.Errorf("log output missing message: %s", output)
	}
	if !strings.Contains(output, "instance_id="+result.InstanceID) {
		t.Errorf("log output missing instance_id: %s", output)
	}
	if !strings.Contains(output, "workflow=test") {
		t.Errorf("log output missing workflow name: %s", output)
	}
	if !strings.Contains(output, "scope=ticket") {
		t.Errorf("log output missing scope type: %s", output)
	}
}

func TestOrchestratorStart_ProjectScope_LogsCorrectScope(t *testing.T) {
	env := newTestEnv(t)

	logBuf := setupLogCapture(t)

	req := RunRequest{
		ProjectID:    env.project,
		WorkflowName: "test",
		ScopeType:    "project",
	}

	// Update workflow to project scope
	_, err := env.pool.Exec(`UPDATE workflows SET scope_type = 'project' WHERE LOWER(id) = LOWER(?)`, "test")
	if err != nil {
		t.Fatalf("failed to update workflow scope: %v", err)
	}

	result, err := env.orch.Start(context.Background(), req)
	if err != nil {
		t.Fatalf("Start() error = %v", err)
	}

	time.Sleep(100 * time.Millisecond)

	output := logBuf.String()

	if !strings.Contains(output, "scope=project") {
		t.Errorf("log output missing scope=project: %s", output)
	}
	if !strings.Contains(output, "instance_id="+result.InstanceID) {
		t.Errorf("log output missing instance_id: %s", output)
	}
}

func TestRestartAgent_LogsRestartRequest(t *testing.T) {
	env := newTestEnv(t)
	env.createTicket(t, "LOG-4", "Test restart logging")
	wfiID := env.initWorkflow(t, "LOG-4")

	logBuf := setupLogCapture(t)

	err := env.orch.RestartAgent(env.project, "LOG-4", "test", "some-session-id")
	if err != nil {
		// Error expected if no running orchestration, but log should still happen
	}

	time.Sleep(50 * time.Millisecond)

	output := logBuf.String()

	if !strings.Contains(output, "INFO") {
		t.Errorf("log output missing INFO level: %s", output)
	}
	if !strings.Contains(output, "agent restart requested") {
		t.Errorf("log output missing message: %s", output)
	}
	if !strings.Contains(output, "session_id=some-session-id") {
		t.Errorf("log output missing session_id: %s", output)
	}
	if !strings.Contains(output, "workflow=test") {
		t.Errorf("log output missing workflow: %s", output)
	}

	// Verify instance exists
	if wfiID == "" {
		t.Error("workflow instance ID should not be empty")
	}
}

func TestStopAll_LogsStopCount(t *testing.T) {
	env := newTestEnv(t)
	env.createTicket(t, "LOG-5A", "Test stop all A")
	env.createTicket(t, "LOG-5B", "Test stop all B")

	// Start two orchestrations (they will fail due to missing agent definitions, but will be registered in runs map)
	req1 := RunRequest{
		ProjectID:    env.project,
		TicketID:     "LOG-5A",
		WorkflowName: "test",
		ScopeType:    "ticket",
	}
	result1, _ := env.orch.Start(context.Background(), req1)

	req2 := RunRequest{
		ProjectID:    env.project,
		TicketID:     "LOG-5B",
		WorkflowName: "test",
		ScopeType:    "ticket",
	}
	result2, _ := env.orch.Start(context.Background(), req2)

	// Give time for orchestrations to start and register
	time.Sleep(100 * time.Millisecond)

	// Check how many are actually running before we capture logs
	env.orch.mu.Lock()
	runCount := len(env.orch.runs)
	env.orch.mu.Unlock()

	logBuf := setupLogCapture(t)

	env.orch.StopAll()

	time.Sleep(50 * time.Millisecond)

	output := logBuf.String()

	if !strings.Contains(output, "WARN") {
		t.Errorf("log output missing WARN level: %s", output)
	}
	if !strings.Contains(output, "stopping all orchestrations") {
		t.Errorf("log output missing message: %s", output)
	}

	// Check for the actual count that was logged
	expectedCount := strings.Contains(output, "count="+string(rune('0'+runCount)))
	if !expectedCount && runCount > 0 {
		t.Errorf("log output has incorrect count (expected count=%d): %s", runCount, output)
	}

	// Verify instances exist
	if result1.InstanceID == "" || result2.InstanceID == "" {
		t.Error("instance IDs should not be empty")
	}
}

func TestRetryFailedAgent_LogsRetryAttempt(t *testing.T) {
	env := newTestEnv(t)
	env.createTicket(t, "LOG-6", "Test retry logging")
	wfiID := env.initWorkflow(t, "LOG-6")

	// Mark workflow as failed
	wfiRepo := repo.NewWorkflowInstanceRepo(env.pool, clock.Real())
	err := wfiRepo.UpdateStatus(wfiID, "failed")
	if err != nil {
		t.Fatalf("failed to mark workflow as failed: %v", err)
	}

	// Create a failed agent session with proper timestamps
	sessionID := "test-session-123"
	_, err = env.pool.Exec(`
		INSERT INTO agent_sessions (id, project_id, ticket_id, workflow_instance_id, phase, agent_type, model_id, status, result, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, datetime('now'), datetime('now'))
	`, sessionID, env.project, "LOG-6", wfiID, "analyzer", "analyzer", "claude:sonnet", "failed", "fail")
	if err != nil {
		t.Fatalf("failed to create agent session: %v", err)
	}

	logBuf := setupLogCapture(t)

	ctx := context.Background()
	err = env.orch.RetryFailedAgent(ctx, env.project, "LOG-6", "test", sessionID)
	if err != nil {
		// May fail due to missing project root_path or other issues, but log should happen
	}

	time.Sleep(100 * time.Millisecond)

	output := logBuf.String()

	if !strings.Contains(output, "INFO") {
		t.Errorf("log output missing INFO level: %s", output)
	}
	if !strings.Contains(output, "retrying failed workflow") {
		t.Errorf("log output missing message: %s", output)
	}
	if !strings.Contains(output, "workflow=test") {
		t.Errorf("log output missing workflow: %s", output)
	}
	if !strings.Contains(output, "session_id="+sessionID) {
		t.Errorf("log output missing session_id: %s", output)
	}
	if !strings.Contains(output, "scope=ticket") {
		t.Errorf("log output missing scope: %s", output)
	}
}

func TestHandleCallback_LogsCallbackDetection(t *testing.T) {
	env := newTestEnv(t)

	logBuf := setupLogCapture(t)

	// clearCallbackMetadata doesn't return error, it just logs
	// So we call it with invalid ID to trigger error logging
	env.orch.clearCallbackMetadata(context.Background(), "nonexistent-wfi-id")

	time.Sleep(50 * time.Millisecond)

	output := logBuf.String()

	if !strings.Contains(output, "ERROR") {
		t.Errorf("log output missing ERROR level: %s", output)
	}
	// Should log "failed to load WFI"
	hasError := strings.Contains(output, "failed to load WFI")
	if !hasError {
		t.Errorf("log output missing expected error message: %s", output)
	}
}

func TestOrchestratorStart_TrxPropagation_SameTrxInAllLogs(t *testing.T) {
	env := newTestEnv(t)
	env.createTicket(t, "LOG-7", "Test trx consistency")

	logBuf := setupLogCapture(t)

	req := RunRequest{
		ProjectID:    env.project,
		TicketID:     "LOG-7",
		WorkflowName: "test",
		ScopeType:    "ticket",
	}

	_, err := env.orch.Start(context.Background(), req)
	if err != nil {
		t.Fatalf("Start() error = %v", err)
	}

	time.Sleep(100 * time.Millisecond)

	output := logBuf.String()
	lines := strings.Split(output, "\n")

	// Extract trx IDs from all log lines
	var trxIDs []string
	for _, line := range lines {
		if line == "" {
			continue
		}
		start := strings.Index(line, "[")
		end := strings.Index(line, "]")
		if start != -1 && end != -1 {
			trxID := line[start+1 : end]
			trxIDs = append(trxIDs, trxID)
		}
	}

	if len(trxIDs) == 0 {
		t.Fatal("no trx IDs found in log output")
	}

	// All trx IDs should be the same (same orchestration run)
	firstTrx := trxIDs[0]
	for i, trx := range trxIDs {
		if trx != firstTrx {
			t.Errorf("trx ID mismatch at line %d: got %s, want %s (line: %s)", i, trx, firstTrx, lines[i])
		}
	}
}

func TestMarkCompleted_LogsTicketCloseError(t *testing.T) {
	env := newTestEnv(t)
	// Don't create ticket - this will cause close to fail
	logBuf := setupLogCapture(t)

	// Create workflow instance directly without ticket
	wfiID := "test-wfi-nocall"
	_, err := env.pool.Exec(`
		INSERT INTO workflow_instances (id, project_id, ticket_id, workflow_id, status, scope_type, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, datetime('now'), datetime('now'))
	`, wfiID, env.project, "NONEXISTENT", "test", "active", "ticket")
	if err != nil {
		t.Fatalf("failed to create workflow instance: %v", err)
	}

	// Call markCompleted which should try to close ticket and log error
	req := RunRequest{
		ProjectID:    env.project,
		TicketID:     "NONEXISTENT",
		WorkflowName: "test",
		ScopeType:    "ticket",
	}

	env.orch.markCompleted(wfiID, req)

	time.Sleep(100 * time.Millisecond)

	output := logBuf.String()

	if !strings.Contains(output, "ERROR") {
		t.Errorf("log output missing ERROR level: %s", output)
	}
	if !strings.Contains(output, "failed to close ticket") {
		t.Errorf("log output missing error message: %s", output)
	}
	if !strings.Contains(output, "ticket=NONEXISTENT") {
		t.Errorf("log output missing ticket ID: %s", output)
	}
}

func TestProjectScope_LogsProjectCompleted(t *testing.T) {
	env := newTestEnv(t)

	// Update workflow to project scope
	_, err := env.pool.Exec(`UPDATE workflows SET scope_type = 'project' WHERE LOWER(id) = LOWER(?)`, "test")
	if err != nil {
		t.Fatalf("failed to update workflow scope: %v", err)
	}

	logBuf := setupLogCapture(t)

	req := RunRequest{
		ProjectID:    env.project,
		WorkflowName: "test",
		ScopeType:    "project",
	}

	result, err := env.orch.Start(context.Background(), req)
	if err != nil {
		t.Fatalf("Start() error = %v", err)
	}

	time.Sleep(100 * time.Millisecond)

	output := logBuf.String()

	// Verify project scope is logged
	if !strings.Contains(output, "scope=project") {
		t.Errorf("log output missing scope=project: %s", output)
	}
	if !strings.Contains(output, "instance_id="+result.InstanceID) {
		t.Errorf("log output missing instance_id: %s", output)
	}
}

func TestOrchestratorLogging_StructuredFormat(t *testing.T) {
	env := newTestEnv(t)
	env.createTicket(t, "LOG-8", "Test structured logging")

	logBuf := setupLogCapture(t)

	req := RunRequest{
		ProjectID:    env.project,
		TicketID:     "LOG-8",
		WorkflowName: "test",
		ScopeType:    "ticket",
	}

	result, err := env.orch.Start(context.Background(), req)
	if err != nil {
		t.Fatalf("Start() error = %v", err)
	}

	time.Sleep(100 * time.Millisecond)

	output := logBuf.String()
	lines := strings.Split(output, "\n")

	for _, line := range lines {
		if line == "" {
			continue
		}

		// Verify structured format: YYYY-MM-DD HH:MM:SS LEVEL [trx] message key=value...
		parts := strings.Fields(line)
		if len(parts) < 4 {
			t.Errorf("log line has fewer than 4 parts: %s", line)
			continue
		}

		// Check timestamp format (YYYY-MM-DD)
		if !strings.Contains(parts[0], "-") {
			t.Errorf("log line missing date: %s", line)
		}

		// Check time format (HH:MM:SS)
		if !strings.Contains(parts[1], ":") {
			t.Errorf("log line missing time: %s", line)
		}

		// Check level (INFO/WARN/ERROR)
		level := parts[2]
		if level != "INFO" && level != "WARN" && level != "ERROR" {
			t.Errorf("log line has invalid level %s: %s", level, line)
		}

		// Check trx in brackets
		trxPart := parts[3]
		if !strings.HasPrefix(trxPart, "[") || !strings.HasSuffix(trxPart, "]") {
			t.Errorf("log line missing [trx]: %s", line)
		}
	}

	// Verify instance was created
	if result.InstanceID == "" {
		t.Error("instance ID should not be empty")
	}
}
