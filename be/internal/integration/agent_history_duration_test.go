package integration

import (
	"testing"
	"time"

	"be/internal/types"
)

// insertSessionWithTimestamps inserts an agent session with specific started_at and ended_at timestamps.
func insertSessionWithTimestamps(t *testing.T, env *TestEnv, id, ticketID, wfiID, phase, agentType, modelID, status, result string, startedAt, endedAt time.Time) {
	t.Helper()
	startedAtStr := startedAt.UTC().Format(time.RFC3339Nano)
	endedAtStr := endedAt.UTC().Format(time.RFC3339Nano)
	now := env.Clock.Now().UTC().Format(time.RFC3339Nano)

	_, err := env.Pool.Exec(`
		INSERT INTO agent_sessions (id, project_id, ticket_id, workflow_instance_id, phase, agent_type,
			model_id, status, result, result_reason, pid, findings,
			context_left, ancestor_session_id, spawn_command, prompt_context,
			restart_count, started_at, ended_at, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, NULL, NULL, NULL, NULL, NULL, NULL, NULL, 0, ?, ?, ?, ?)`,
		id, env.ProjectID, ticketID, wfiID, phase, agentType,
		nullStr(modelID),
		status, nullStr(result),
		startedAtStr, endedAtStr, now, now,
	)
	if err != nil {
		t.Fatalf("failed to insert session %s with timestamps: %v", id, err)
	}
}

// TestAgentHistoryDurationBothTimestamps verifies that agent_history entries
// include duration_sec when both started_at and ended_at are present.
func TestAgentHistoryDurationBothTimestamps(t *testing.T) {
	env := NewTestEnv(t)

	env.CreateTicket(t, "DUR-1", "Duration with both timestamps")
	env.InitWorkflow(t, "DUR-1")

	wfiID := env.GetWorkflowInstanceID(t, "DUR-1", "test")

	// Insert session with 5 minutes (300 seconds) duration
	startTime := env.Clock.Now()
	endTime := startTime.Add(5 * time.Minute)
	insertSessionWithTimestamps(t, env, "dur-sess-1", "DUR-1", wfiID,
		"analyzer", "setup-analyzer", "claude:sonnet", "completed", "pass",
		startTime, endTime)

	// Get workflow status
	status, err := getWorkflowStatus(t, env, "DUR-1", &types.WorkflowGetRequest{
		Workflow: "test",
	})
	if err != nil {
		t.Fatalf("failed to get status: %v", err)
	}

	history, ok := status["agent_history"].([]interface{})
	if !ok {
		t.Fatalf("expected agent_history array, got %T", status["agent_history"])
	}
	if len(history) != 1 {
		t.Fatalf("expected 1 history entry, got %d", len(history))
	}

	entry, _ := history[0].(map[string]interface{})

	// Verify duration_sec is present and equals 300
	durationSec, ok := entry["duration_sec"].(float64)
	if !ok {
		t.Fatalf("expected duration_sec to be float64, got %T (value: %v)", entry["duration_sec"], entry["duration_sec"])
	}
	if int(durationSec) != 300 {
		t.Fatalf("expected duration_sec 300, got %v", durationSec)
	}
}

// TestAgentHistoryDurationOnlyStarted verifies that duration_sec is absent
// when only started_at is set (ended_at is NULL).
func TestAgentHistoryDurationOnlyStarted(t *testing.T) {
	env := NewTestEnv(t)

	env.CreateTicket(t, "DUR-2", "Duration with only started_at")
	env.InitWorkflow(t, "DUR-2")

	wfiID := env.GetWorkflowInstanceID(t, "DUR-2", "test")

	// Insert session with only started_at set (ended_at NULL)
	startTime := env.Clock.Now()
	startedAtStr := startTime.UTC().Format(time.RFC3339Nano)
	now := env.Clock.Now().UTC().Format(time.RFC3339Nano)

	_, err := env.Pool.Exec(`
		INSERT INTO agent_sessions (id, project_id, ticket_id, workflow_instance_id, phase, agent_type,
			model_id, status, result, result_reason, pid, findings,
			context_left, ancestor_session_id, spawn_command, prompt_context,
			restart_count, started_at, ended_at, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, NULL, NULL, NULL, NULL, NULL, NULL, NULL, 0, ?, NULL, ?, ?)`,
		"dur-sess-2", env.ProjectID, "DUR-2", wfiID, "analyzer", "setup-analyzer",
		"claude:sonnet", "completed", "pass",
		startedAtStr, now, now,
	)
	if err != nil {
		t.Fatalf("failed to insert session: %v", err)
	}

	// Get workflow status
	status, err := getWorkflowStatus(t, env, "DUR-2", &types.WorkflowGetRequest{
		Workflow: "test",
	})
	if err != nil {
		t.Fatalf("failed to get status: %v", err)
	}

	history, ok := status["agent_history"].([]interface{})
	if !ok {
		t.Fatalf("expected agent_history array, got %T", status["agent_history"])
	}
	if len(history) != 1 {
		t.Fatalf("expected 1 history entry, got %d", len(history))
	}

	entry, _ := history[0].(map[string]interface{})

	// Verify duration_sec is absent
	if _, exists := entry["duration_sec"]; exists {
		t.Fatalf("expected duration_sec to be absent when ended_at is NULL, but got %v", entry["duration_sec"])
	}

	// Verify started_at is still present
	if _, exists := entry["started_at"]; !exists {
		t.Fatal("expected started_at to be present")
	}
}

// TestAgentHistoryDurationOnlyEnded verifies that duration_sec is absent
// when only ended_at is set (started_at is NULL).
func TestAgentHistoryDurationOnlyEnded(t *testing.T) {
	env := NewTestEnv(t)

	env.CreateTicket(t, "DUR-3", "Duration with only ended_at")
	env.InitWorkflow(t, "DUR-3")

	wfiID := env.GetWorkflowInstanceID(t, "DUR-3", "test")

	// Insert session with only ended_at set (started_at NULL)
	endTime := env.Clock.Now()
	endedAtStr := endTime.UTC().Format(time.RFC3339Nano)
	now := env.Clock.Now().UTC().Format(time.RFC3339Nano)

	_, err := env.Pool.Exec(`
		INSERT INTO agent_sessions (id, project_id, ticket_id, workflow_instance_id, phase, agent_type,
			model_id, status, result, result_reason, pid, findings,
			context_left, ancestor_session_id, spawn_command, prompt_context,
			restart_count, started_at, ended_at, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, NULL, NULL, NULL, NULL, NULL, NULL, NULL, 0, NULL, ?, ?, ?)`,
		"dur-sess-3", env.ProjectID, "DUR-3", wfiID, "analyzer", "setup-analyzer",
		"claude:sonnet", "completed", "pass",
		endedAtStr, now, now,
	)
	if err != nil {
		t.Fatalf("failed to insert session: %v", err)
	}

	// Get workflow status
	status, err := getWorkflowStatus(t, env, "DUR-3", &types.WorkflowGetRequest{
		Workflow: "test",
	})
	if err != nil {
		t.Fatalf("failed to get status: %v", err)
	}

	history, ok := status["agent_history"].([]interface{})
	if !ok {
		t.Fatalf("expected agent_history array, got %T", status["agent_history"])
	}
	if len(history) != 1 {
		t.Fatalf("expected 1 history entry, got %d", len(history))
	}

	entry, _ := history[0].(map[string]interface{})

	// Verify duration_sec is absent
	if _, exists := entry["duration_sec"]; exists {
		t.Fatalf("expected duration_sec to be absent when started_at is NULL, but got %v", entry["duration_sec"])
	}

	// Verify ended_at is still present
	if _, exists := entry["ended_at"]; !exists {
		t.Fatal("expected ended_at to be present")
	}
}

// TestAgentHistoryDurationNeitherTimestamp verifies that duration_sec is absent
// when both started_at and ended_at are NULL.
func TestAgentHistoryDurationNeitherTimestamp(t *testing.T) {
	env := NewTestEnv(t)

	env.CreateTicket(t, "DUR-4", "Duration with neither timestamp")
	env.InitWorkflow(t, "DUR-4")

	wfiID := env.GetWorkflowInstanceID(t, "DUR-4", "test")

	// Insert session with both timestamps NULL
	now := env.Clock.Now().UTC().Format(time.RFC3339Nano)

	_, err := env.Pool.Exec(`
		INSERT INTO agent_sessions (id, project_id, ticket_id, workflow_instance_id, phase, agent_type,
			model_id, status, result, result_reason, pid, findings,
			context_left, ancestor_session_id, spawn_command, prompt_context,
			restart_count, started_at, ended_at, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, NULL, NULL, NULL, NULL, NULL, NULL, NULL, 0, NULL, NULL, ?, ?)`,
		"dur-sess-4", env.ProjectID, "DUR-4", wfiID, "analyzer", "setup-analyzer",
		"claude:sonnet", "completed", "pass",
		now, now,
	)
	if err != nil {
		t.Fatalf("failed to insert session: %v", err)
	}

	// Get workflow status
	status, err := getWorkflowStatus(t, env, "DUR-4", &types.WorkflowGetRequest{
		Workflow: "test",
	})
	if err != nil {
		t.Fatalf("failed to get status: %v", err)
	}

	history, ok := status["agent_history"].([]interface{})
	if !ok {
		t.Fatalf("expected agent_history array, got %T", status["agent_history"])
	}
	if len(history) != 1 {
		t.Fatalf("expected 1 history entry, got %d", len(history))
	}

	entry, _ := history[0].(map[string]interface{})

	// Verify duration_sec is absent
	if _, exists := entry["duration_sec"]; exists {
		t.Fatalf("expected duration_sec to be absent when both timestamps are NULL, but got %v", entry["duration_sec"])
	}

	// Verify both timestamps are absent
	if _, exists := entry["started_at"]; exists {
		t.Fatalf("expected started_at to be absent when NULL, but got %v", entry["started_at"])
	}
	if _, exists := entry["ended_at"]; exists {
		t.Fatalf("expected ended_at to be absent when NULL, but got %v", entry["ended_at"])
	}
}

// TestAgentHistoryDurationNegativeClockSkew verifies that negative duration
// (ended_at < started_at) is clamped to 0.
func TestAgentHistoryDurationNegativeClockSkew(t *testing.T) {
	env := NewTestEnv(t)

	env.CreateTicket(t, "DUR-5", "Duration with clock skew")
	env.InitWorkflow(t, "DUR-5")

	wfiID := env.GetWorkflowInstanceID(t, "DUR-5", "test")

	// Insert session with ended_at BEFORE started_at (clock skew)
	startTime := env.Clock.Now()
	endTime := startTime.Add(-2 * time.Minute) // 2 minutes before start
	insertSessionWithTimestamps(t, env, "dur-sess-5", "DUR-5", wfiID,
		"analyzer", "setup-analyzer", "claude:sonnet", "completed", "pass",
		startTime, endTime)

	// Get workflow status
	status, err := getWorkflowStatus(t, env, "DUR-5", &types.WorkflowGetRequest{
		Workflow: "test",
	})
	if err != nil {
		t.Fatalf("failed to get status: %v", err)
	}

	history, ok := status["agent_history"].([]interface{})
	if !ok {
		t.Fatalf("expected agent_history array, got %T", status["agent_history"])
	}
	if len(history) != 1 {
		t.Fatalf("expected 1 history entry, got %d", len(history))
	}

	entry, _ := history[0].(map[string]interface{})

	// Verify duration_sec is clamped to 0 (not negative)
	durationSec, ok := entry["duration_sec"].(float64)
	if !ok {
		t.Fatalf("expected duration_sec to be float64, got %T", entry["duration_sec"])
	}
	if durationSec != 0 {
		t.Fatalf("expected duration_sec 0 (clamped), got %v", durationSec)
	}
}

// TestAgentHistoryDurationMultipleEntries verifies that multiple agent
// history entries each have their own correct duration_sec values.
func TestAgentHistoryDurationMultipleEntries(t *testing.T) {
	env := NewTestEnv(t)

	env.CreateTicket(t, "DUR-6", "Multiple agents with different durations")
	env.InitWorkflow(t, "DUR-6")

	wfiID := env.GetWorkflowInstanceID(t, "DUR-6", "test")

	baseTime := env.Clock.Now()

	// Insert three sessions with different durations
	// Agent 1: 2 minutes (120 seconds)
	insertSessionWithTimestamps(t, env, "dur-sess-6a", "DUR-6", wfiID,
		"analyzer", "setup-analyzer", "claude:sonnet", "completed", "pass",
		baseTime, baseTime.Add(2*time.Minute))

	// Agent 2: 10 minutes (600 seconds)
	insertSessionWithTimestamps(t, env, "dur-sess-6b", "DUR-6", wfiID,
		"builder", "implementor", "claude:opus", "completed", "pass",
		baseTime.Add(2*time.Minute), baseTime.Add(12*time.Minute))

	// Agent 3: 30 seconds
	insertSessionWithTimestamps(t, env, "dur-sess-6c", "DUR-6", wfiID,
		"analyzer", "qa-verifier", "claude:sonnet", "completed", "pass",
		baseTime.Add(12*time.Minute), baseTime.Add(12*time.Minute+30*time.Second))

	// Get workflow status
	status, err := getWorkflowStatus(t, env, "DUR-6", &types.WorkflowGetRequest{
		Workflow: "test",
	})
	if err != nil {
		t.Fatalf("failed to get status: %v", err)
	}

	history, ok := status["agent_history"].([]interface{})
	if !ok {
		t.Fatalf("expected agent_history array, got %T", status["agent_history"])
	}
	if len(history) != 3 {
		t.Fatalf("expected 3 history entries, got %d", len(history))
	}

	// Verify each entry has the correct duration
	expectedDurations := map[string]float64{
		"setup-analyzer": 120,
		"implementor":    600,
		"qa-verifier":    30,
	}

	for _, entry := range history {
		e, _ := entry.(map[string]interface{})
		agentType := e["agent_type"].(string)

		durationSec, ok := e["duration_sec"].(float64)
		if !ok {
			t.Fatalf("expected duration_sec for %s to be float64, got %T", agentType, e["duration_sec"])
		}

		expectedDur, exists := expectedDurations[agentType]
		if !exists {
			t.Fatalf("unexpected agent_type %q in history", agentType)
		}

		if durationSec != expectedDur {
			t.Errorf("agent %s: expected duration_sec %v, got %v", agentType, expectedDur, durationSec)
		}
	}
}

// TestAgentHistoryDurationPrecision verifies that duration_sec handles
// sub-second precision correctly (fractional seconds).
func TestAgentHistoryDurationPrecision(t *testing.T) {
	env := NewTestEnv(t)

	env.CreateTicket(t, "DUR-7", "Duration with sub-second precision")
	env.InitWorkflow(t, "DUR-7")

	wfiID := env.GetWorkflowInstanceID(t, "DUR-7", "test")

	// Insert session with 1.5 seconds duration
	startTime := env.Clock.Now()
	endTime := startTime.Add(1500 * time.Millisecond) // 1.5 seconds
	insertSessionWithTimestamps(t, env, "dur-sess-7", "DUR-7", wfiID,
		"analyzer", "setup-analyzer", "claude:sonnet", "completed", "pass",
		startTime, endTime)

	// Get workflow status
	status, err := getWorkflowStatus(t, env, "DUR-7", &types.WorkflowGetRequest{
		Workflow: "test",
	})
	if err != nil {
		t.Fatalf("failed to get status: %v", err)
	}

	history, ok := status["agent_history"].([]interface{})
	if !ok {
		t.Fatalf("expected agent_history array, got %T", status["agent_history"])
	}
	if len(history) != 1 {
		t.Fatalf("expected 1 history entry, got %d", len(history))
	}

	entry, _ := history[0].(map[string]interface{})

	// Verify duration_sec is 1.5 (with floating point tolerance)
	durationSec, ok := entry["duration_sec"].(float64)
	if !ok {
		t.Fatalf("expected duration_sec to be float64, got %T", entry["duration_sec"])
	}
	if durationSec < 1.49 || durationSec > 1.51 {
		t.Fatalf("expected duration_sec ~1.5, got %v", durationSec)
	}
}
