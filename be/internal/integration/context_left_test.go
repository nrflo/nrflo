package integration

import (
	"testing"
	"time"

	"be/internal/types"
)

// insertSessionWithContextLeft inserts an agent session with context_left set.
func insertSessionWithContextLeft(t *testing.T, env *TestEnv, id, ticketID, wfiID, phase, agentType, modelID, status, result string, contextLeft int64) {
	t.Helper()
	now := time.Now().UTC().Format(time.RFC3339Nano)
	_, err := env.Pool.Exec(`
		INSERT INTO agent_sessions (id, project_id, ticket_id, workflow_instance_id, phase, agent_type,
			model_id, status, result, result_reason, pid, findings,
			context_left, ancestor_session_id, spawn_command, prompt_context,
			started_at, ended_at, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, NULL, NULL, NULL, ?, NULL, NULL, NULL, ?, ?, ?, ?)`,
		id, env.ProjectID, ticketID, wfiID, phase, agentType,
		nullStr(modelID),
		status, nullStr(result),
		contextLeft,
		now, now, now, now,
	)
	if err != nil {
		t.Fatalf("failed to insert session %s: %v", id, err)
	}
}

// TestActiveAgentsIncludesContextLeft verifies that active_agents in the
// workflow status response include the context_left field when present.
func TestActiveAgentsIncludesContextLeft(t *testing.T) {
	env := NewTestEnv(t)

	env.CreateTicket(t, "CL-1", "Context left in active agents")
	env.InitWorkflow(t, "CL-1")

	wfiID := env.GetWorkflowInstanceID(t, "CL-1", "test")

	// Insert a running agent with context_left set
	insertSessionWithContextLeft(t, env, "sess-cl-1", "CL-1", wfiID, "analyzer", "setup-analyzer", "claude:sonnet", "running", "", 72)

	// Get workflow status
	status, err := getWorkflowStatus(t, env, "CL-1", &types.WorkflowGetRequest{
		Workflow: "test",
	})
	if err != nil {
		t.Fatalf("failed to get status: %v", err)
	}

	activeAgents, ok := status["active_agents"].(map[string]interface{})
	if !ok {
		t.Fatalf("expected active_agents to be map, got %T", status["active_agents"])
	}

	agent, ok := activeAgents["setup-analyzer:claude:sonnet"].(map[string]interface{})
	if !ok {
		t.Fatalf("expected agent entry, got keys: %v", keysOf(activeAgents))
	}

	cl, ok := agent["context_left"].(float64)
	if !ok {
		t.Fatalf("expected context_left to be number, got %T (value: %v)", agent["context_left"], agent["context_left"])
	}
	if int(cl) != 72 {
		t.Fatalf("expected context_left 72, got %v", cl)
	}
}

// TestActiveAgentsOmitsContextLeftWhenNull verifies that active_agents omit
// context_left when it is NULL in the database.
func TestActiveAgentsOmitsContextLeftWhenNull(t *testing.T) {
	env := NewTestEnv(t)

	env.CreateTicket(t, "CL-2", "No context left")
	env.InitWorkflow(t, "CL-2")

	wfiID := env.GetWorkflowInstanceID(t, "CL-2", "test")

	// Insert a running agent without context_left (uses InsertAgentSession which sets NULL)
	env.InsertAgentSession(t, "sess-cl-2", "CL-2", wfiID, "analyzer", "setup-analyzer", "claude:sonnet")

	// Get workflow status
	status, err := getWorkflowStatus(t, env, "CL-2", &types.WorkflowGetRequest{
		Workflow: "test",
	})
	if err != nil {
		t.Fatalf("failed to get status: %v", err)
	}

	activeAgents, ok := status["active_agents"].(map[string]interface{})
	if !ok {
		t.Fatalf("expected active_agents to be map, got %T", status["active_agents"])
	}

	agent, ok := activeAgents["setup-analyzer:claude:sonnet"].(map[string]interface{})
	if !ok {
		t.Fatalf("expected agent entry, got keys: %v", keysOf(activeAgents))
	}

	if _, exists := agent["context_left"]; exists {
		t.Fatalf("expected context_left to be absent when NULL, but got %v", agent["context_left"])
	}
}

// TestAgentHistoryIncludesContextLeft verifies that agent_history entries
// include the context_left field when present in the database.
func TestAgentHistoryIncludesContextLeft(t *testing.T) {
	env := NewTestEnv(t)

	env.CreateTicket(t, "CL-3", "Context left in history")
	env.InitWorkflow(t, "CL-3")

	wfiID := env.GetWorkflowInstanceID(t, "CL-3", "test")

	// Insert completed sessions with context_left
	insertSessionWithContextLeft(t, env, "sess-cl-3a", "CL-3", wfiID, "analyzer", "setup-analyzer", "claude:sonnet", "completed", "pass", 45)
	insertSessionWithContextLeft(t, env, "sess-cl-3b", "CL-3", wfiID, "builder", "implementor", "claude:opus", "completed", "pass", 12)

	// Get workflow status
	status, err := getWorkflowStatus(t, env, "CL-3", &types.WorkflowGetRequest{
		Workflow: "test",
	})
	if err != nil {
		t.Fatalf("failed to get status: %v", err)
	}

	history, ok := status["agent_history"].([]interface{})
	if !ok {
		t.Fatalf("expected agent_history array, got %T", status["agent_history"])
	}
	if len(history) != 2 {
		t.Fatalf("expected 2 history entries, got %d", len(history))
	}

	// Verify each entry has correct context_left
	for _, entry := range history {
		e, _ := entry.(map[string]interface{})
		agentType := e["agent_type"].(string)
		cl, ok := e["context_left"].(float64)
		if !ok {
			t.Fatalf("expected context_left for %s, got %T", agentType, e["context_left"])
		}
		switch agentType {
		case "setup-analyzer":
			if int(cl) != 45 {
				t.Fatalf("expected context_left 45 for setup-analyzer, got %v", cl)
			}
		case "implementor":
			if int(cl) != 12 {
				t.Fatalf("expected context_left 12 for implementor, got %v", cl)
			}
		default:
			t.Fatalf("unexpected agent_type %q", agentType)
		}
	}
}

// TestAgentHistoryOmitsContextLeftWhenNull verifies that agent_history
// entries omit context_left when NULL in the database.
func TestAgentHistoryOmitsContextLeftWhenNull(t *testing.T) {
	env := NewTestEnv(t)

	env.CreateTicket(t, "CL-4", "No context left in history")
	env.InitWorkflow(t, "CL-4")

	wfiID := env.GetWorkflowInstanceID(t, "CL-4", "test")

	// Insert completed session without context_left
	insertCompletedSession(t, env, "sess-cl-4", "CL-4", wfiID, "analyzer", "setup-analyzer", "claude:sonnet", "completed", "pass")

	// Get workflow status
	status, err := getWorkflowStatus(t, env, "CL-4", &types.WorkflowGetRequest{
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
	if _, exists := entry["context_left"]; exists {
		t.Fatalf("expected context_left to be absent when NULL, but got %v", entry["context_left"])
	}
}

// TestAgentHistoryIncludesStartedAtEndedAt verifies that agent_history entries
// include started_at and ended_at timestamps when present.
func TestAgentHistoryIncludesStartedAtEndedAt(t *testing.T) {
	env := NewTestEnv(t)

	env.CreateTicket(t, "CL-5", "Timestamps in history")
	env.InitWorkflow(t, "CL-5")

	wfiID := env.GetWorkflowInstanceID(t, "CL-5", "test")

	// Insert completed session (insertCompletedSession sets both started_at and ended_at)
	insertCompletedSession(t, env, "sess-cl-5", "CL-5", wfiID, "analyzer", "setup-analyzer", "claude:sonnet", "completed", "pass")

	// Get workflow status
	status, err := getWorkflowStatus(t, env, "CL-5", &types.WorkflowGetRequest{
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

	startedAt, ok := entry["started_at"].(string)
	if !ok {
		t.Fatalf("expected started_at to be string, got %T", entry["started_at"])
	}
	if startedAt == "" {
		t.Fatal("expected non-empty started_at")
	}

	endedAt, ok := entry["ended_at"].(string)
	if !ok {
		t.Fatalf("expected ended_at to be string, got %T", entry["ended_at"])
	}
	if endedAt == "" {
		t.Fatal("expected non-empty ended_at")
	}
}

// TestActiveAgentsIncludesStartedAt verifies that active_agents include
// the started_at timestamp.
func TestActiveAgentsIncludesStartedAt(t *testing.T) {
	env := NewTestEnv(t)

	env.CreateTicket(t, "CL-6", "Started at in active agents")
	env.InitWorkflow(t, "CL-6")

	wfiID := env.GetWorkflowInstanceID(t, "CL-6", "test")

	env.InsertAgentSession(t, "sess-cl-6", "CL-6", wfiID, "analyzer", "setup-analyzer", "claude:sonnet")

	// Get workflow status
	status, err := getWorkflowStatus(t, env, "CL-6", &types.WorkflowGetRequest{
		Workflow: "test",
	})
	if err != nil {
		t.Fatalf("failed to get status: %v", err)
	}

	activeAgents, ok := status["active_agents"].(map[string]interface{})
	if !ok {
		t.Fatalf("expected active_agents map, got %T", status["active_agents"])
	}

	agent, ok := activeAgents["setup-analyzer:claude:sonnet"].(map[string]interface{})
	if !ok {
		t.Fatalf("expected agent entry, got keys: %v", keysOf(activeAgents))
	}

	startedAt, ok := agent["started_at"].(string)
	if !ok {
		t.Fatalf("expected started_at to be string, got %T", agent["started_at"])
	}
	if startedAt == "" {
		t.Fatal("expected non-empty started_at")
	}
}

// TestContextLeftBoundaryValues verifies context_left at boundary values (0 and 100).
func TestContextLeftBoundaryValues(t *testing.T) {
	env := NewTestEnv(t)

	env.CreateTicket(t, "CL-7", "Context left boundaries")
	env.InitWorkflow(t, "CL-7")

	wfiID := env.GetWorkflowInstanceID(t, "CL-7", "test")

	// Insert agents with boundary context_left values
	insertSessionWithContextLeft(t, env, "sess-cl-7a", "CL-7", wfiID, "analyzer", "agent-zero", "claude:sonnet", "completed", "pass", 0)
	insertSessionWithContextLeft(t, env, "sess-cl-7b", "CL-7", wfiID, "builder", "agent-full", "claude:opus", "completed", "pass", 100)

	// Get workflow status
	status, err := getWorkflowStatus(t, env, "CL-7", &types.WorkflowGetRequest{
		Workflow: "test",
	})
	if err != nil {
		t.Fatalf("failed to get status: %v", err)
	}

	history, ok := status["agent_history"].([]interface{})
	if !ok {
		t.Fatalf("expected agent_history array, got %T", status["agent_history"])
	}
	if len(history) != 2 {
		t.Fatalf("expected 2 history entries, got %d", len(history))
	}

	for _, entry := range history {
		e, _ := entry.(map[string]interface{})
		agentType := e["agent_type"].(string)
		cl, ok := e["context_left"].(float64)
		if !ok {
			t.Fatalf("expected context_left for %s, got %T", agentType, e["context_left"])
		}
		switch agentType {
		case "agent-zero":
			if int(cl) != 0 {
				t.Fatalf("expected context_left 0, got %v", cl)
			}
		case "agent-full":
			if int(cl) != 100 {
				t.Fatalf("expected context_left 100, got %v", cl)
			}
		}
	}
}
