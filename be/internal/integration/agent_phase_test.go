package integration

import (
	"testing"
	"time"

	"be/internal/types"
)

// insertCompletedSession inserts a completed agent session directly into the DB.
func insertCompletedSession(t *testing.T, env *TestEnv, id, ticketID, wfiID, phase, agentType, modelID, status, result string) {
	t.Helper()
	now := time.Now().UTC().Format(time.RFC3339Nano)
	_, err := env.Pool.Exec(`
		INSERT INTO agent_sessions (id, project_id, ticket_id, workflow_instance_id, phase, agent_type,
			model_id, status, result, result_reason, pid, findings,
			context_left, ancestor_session_id, spawn_command, prompt_context,
			started_at, ended_at, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, NULL, NULL, NULL, NULL, NULL, NULL, NULL, ?, ?, ?, ?)`,
		id, env.ProjectID, ticketID, wfiID, phase, agentType,
		nullStr(modelID),
		status, result,
		now, now, now, now,
	)
	if err != nil {
		t.Fatalf("failed to insert completed session %s: %v", id, err)
	}
}

// TestAgentHistoryIncludesPhase verifies that agent_history entries in the
// workflow status response include the "phase" field from agent_sessions.
// This is the core of the bug fix: previously, phase was missing from the
// SELECT query, causing the UI to lose track of which phase an agent belonged to.
func TestAgentHistoryIncludesPhase(t *testing.T) {
	env := NewTestEnv(t)

	env.CreateTicket(t, "PH-1", "Phase in history")
	env.InitWorkflow(t, "PH-1")

	wfiID := env.GetWorkflowInstanceID(t, "PH-1", "test")

	// Insert completed agent sessions in different phases
	insertCompletedSession(t, env, "sess-ph-1", "PH-1", wfiID, "analyzer", "setup-analyzer", "claude:sonnet", "completed", "pass")
	insertCompletedSession(t, env, "sess-ph-2", "PH-1", wfiID, "builder", "implementor", "claude:opus", "completed", "pass")

	// Get workflow status
	status, err := getWorkflowStatus(t, env, "PH-1", &types.WorkflowGetRequest{
		Workflow: "test",
	})
	if err != nil {
		t.Fatalf("failed to get status: %v", err)
	}

	// Verify agent_history has phase field
	history, ok := status["agent_history"].([]interface{})
	if !ok {
		t.Fatalf("expected agent_history to be array, got %T", status["agent_history"])
	}
	if len(history) != 2 {
		t.Fatalf("expected 2 history entries, got %d", len(history))
	}

	// Check each entry has the correct phase
	phaseFound := map[string]bool{}
	for _, entry := range history {
		e, ok := entry.(map[string]interface{})
		if !ok {
			t.Fatalf("expected history entry to be map, got %T", entry)
		}
		phase, ok := e["phase"].(string)
		if !ok {
			t.Fatalf("expected 'phase' field to be string, got %T (value: %v)", e["phase"], e["phase"])
		}
		agentType := e["agent_type"].(string)
		phaseFound[agentType] = true

		switch agentType {
		case "setup-analyzer":
			if phase != "analyzer" {
				t.Fatalf("expected setup-analyzer phase 'analyzer', got %q", phase)
			}
		case "implementor":
			if phase != "builder" {
				t.Fatalf("expected implementor phase 'builder', got %q", phase)
			}
		default:
			t.Fatalf("unexpected agent_type %q", agentType)
		}
	}
	if !phaseFound["setup-analyzer"] || !phaseFound["implementor"] {
		t.Fatal("not all agents found in history")
	}
}

// TestActiveAgentsIncludesPhase verifies that active_agents in the workflow
// status response include the "phase" field.
func TestActiveAgentsIncludesPhase(t *testing.T) {
	env := NewTestEnv(t)

	env.CreateTicket(t, "PH-2", "Phase in active agents")
	env.InitWorkflow(t, "PH-2")

	wfiID := env.GetWorkflowInstanceID(t, "PH-2", "test")

	// Insert a running agent with a specific phase
	env.InsertAgentSession(t, "sess-ph-3", "PH-2", wfiID, "analyzer", "setup-analyzer", "claude:sonnet")

	// Get workflow status
	status, err := getWorkflowStatus(t, env, "PH-2", &types.WorkflowGetRequest{
		Workflow: "test",
	})
	if err != nil {
		t.Fatalf("failed to get status: %v", err)
	}

	// Verify active_agents has phase field
	activeAgents, ok := status["active_agents"].(map[string]interface{})
	if !ok {
		t.Fatalf("expected active_agents to be map, got %T", status["active_agents"])
	}

	agent, ok := activeAgents["setup-analyzer:claude:sonnet"].(map[string]interface{})
	if !ok {
		t.Fatalf("expected agent entry for key 'setup-analyzer:claude:sonnet', got keys: %v", keysOf(activeAgents))
	}

	phase, ok := agent["phase"].(string)
	if !ok {
		t.Fatalf("expected 'phase' field to be string, got %T (value: %v)", agent["phase"], agent["phase"])
	}
	if phase != "analyzer" {
		t.Fatalf("expected phase 'analyzer', got %q", phase)
	}
}

// TestMultiPhaseWorkflowAgentState tests the actual bug scenario: a multi-phase
// workflow where phase 1 completes and phase 2 starts. The status response should
// show completed agents from phase 1 with their correct phase, and running agents
// from phase 2 with their correct phase.
func TestMultiPhaseWorkflowAgentState(t *testing.T) {
	env := NewTestEnv(t)

	env.CreateTicket(t, "PH-3", "Multi-phase agent state")
	env.InitWorkflow(t, "PH-3")

	wfiID := env.GetWorkflowInstanceID(t, "PH-3", "test")

	// Phase 1: analyzer - completed agent
	insertCompletedSession(t, env, "sess-analyze", "PH-3", wfiID, "analyzer", "setup-analyzer", "claude:sonnet", "completed", "pass")

	// Phase 2: builder - running agent
	env.InsertAgentSession(t, "sess-build", "PH-3", wfiID, "builder", "implementor", "claude:opus")

	// Get workflow status - this is the critical check
	status, err := getWorkflowStatus(t, env, "PH-3", &types.WorkflowGetRequest{
		Workflow: "test",
	})
	if err != nil {
		t.Fatalf("failed to get status: %v", err)
	}

	// Verify agent_history: completed analyzer agent should have phase "analyzer"
	history, ok := status["agent_history"].([]interface{})
	if !ok {
		t.Fatalf("expected agent_history array, got %T", status["agent_history"])
	}
	if len(history) != 1 {
		t.Fatalf("expected 1 history entry (completed analyzer), got %d", len(history))
	}

	histEntry, _ := history[0].(map[string]interface{})
	if histEntry["agent_type"] != "setup-analyzer" {
		t.Fatalf("expected history agent_type 'setup-analyzer', got %v", histEntry["agent_type"])
	}
	if histEntry["phase"] != "analyzer" {
		t.Fatalf("expected history phase 'analyzer', got %v", histEntry["phase"])
	}
	if histEntry["status"] != "completed" {
		t.Fatalf("expected history status 'completed', got %v", histEntry["status"])
	}

	// Verify active_agents: running builder agent should have phase "builder"
	activeAgents, ok := status["active_agents"].(map[string]interface{})
	if !ok {
		t.Fatalf("expected active_agents map, got %T", status["active_agents"])
	}
	if len(activeAgents) != 1 {
		t.Fatalf("expected 1 active agent (running builder), got %d", len(activeAgents))
	}

	agent, ok := activeAgents["implementor:claude:opus"].(map[string]interface{})
	if !ok {
		t.Fatalf("expected active agent entry for key 'implementor:claude:opus', got keys: %v", keysOf(activeAgents))
	}
	if agent["phase"] != "builder" {
		t.Fatalf("expected active agent phase 'builder', got %v", agent["phase"])
	}
	if agent["agent_type"] != "implementor" {
		t.Fatalf("expected active agent_type 'implementor', got %v", agent["agent_type"])
	}
}

// TestAgentHistoryEmptyPhase verifies that agent sessions with an empty
// phase string still appear in agent_history without the phase field
// (sql.NullString with empty string scans as Valid=false).
func TestAgentHistoryEmptyPhase(t *testing.T) {
	env := NewTestEnv(t)

	env.CreateTicket(t, "PH-4", "Empty phase")
	env.InitWorkflow(t, "PH-4")

	wfiID := env.GetWorkflowInstanceID(t, "PH-4", "test")

	// Insert completed agent session with explicit empty-string phase via raw SQL
	now := time.Now().UTC().Format(time.RFC3339Nano)
	_, err := env.Pool.Exec(`
		INSERT INTO agent_sessions (id, project_id, ticket_id, workflow_instance_id, phase, agent_type,
			model_id, status, result, result_reason, pid, findings,
			context_left, ancestor_session_id, spawn_command, prompt_context,
			started_at, ended_at, created_at, updated_at)
		VALUES (?, ?, ?, ?, '', ?, ?, 'completed', 'pass', NULL, NULL, NULL, NULL, NULL, NULL, NULL, ?, ?, ?, ?)`,
		"sess-empty-ph", env.ProjectID, "PH-4", wfiID, "analyzer",
		nullStr("claude:sonnet"),
		now, now, now, now,
	)
	if err != nil {
		t.Fatalf("failed to insert session: %v", err)
	}

	// Get workflow status
	status, err := getWorkflowStatus(t, env, "PH-4", &types.WorkflowGetRequest{
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

	// The entry should exist and have agent_type
	entry, _ := history[0].(map[string]interface{})
	if entry["agent_type"] != "analyzer" {
		t.Fatalf("expected agent_type 'analyzer', got %v", entry["agent_type"])
	}
}

// TestActiveAgentPhaseWithNoModel verifies active_agents include phase even
// when model_id is empty (agent key falls back to agent_type only).
func TestActiveAgentPhaseWithNoModel(t *testing.T) {
	env := NewTestEnv(t)

	env.CreateTicket(t, "PH-5", "No model active agent")
	env.InitWorkflow(t, "PH-5")

	wfiID := env.GetWorkflowInstanceID(t, "PH-5", "test")

	// Insert agent session without model_id
	env.InsertAgentSession(t, "sess-nomodel", "PH-5", wfiID, "builder", "implementor", "")

	// Get workflow status
	status, err := getWorkflowStatus(t, env, "PH-5", &types.WorkflowGetRequest{
		Workflow: "test",
	})
	if err != nil {
		t.Fatalf("failed to get status: %v", err)
	}

	activeAgents, ok := status["active_agents"].(map[string]interface{})
	if !ok {
		t.Fatalf("expected active_agents map, got %T", status["active_agents"])
	}

	// Key should be just "implementor" without model suffix
	agent, ok := activeAgents["implementor"].(map[string]interface{})
	if !ok {
		t.Fatalf("expected agent under key 'implementor', got keys: %v", keysOf(activeAgents))
	}

	phase, ok := agent["phase"].(string)
	if !ok {
		t.Fatalf("expected 'phase' field to be string, got %T (value: %v)", agent["phase"], agent["phase"])
	}
	if phase != "builder" {
		t.Fatalf("expected phase 'builder', got %q", phase)
	}
}

// TestAgentHistoryMixedStatuses verifies that failed agents include the phase field
// in agent_history, and that continued sessions are excluded from history.
func TestAgentHistoryMixedStatuses(t *testing.T) {
	env := NewTestEnv(t)

	env.CreateTicket(t, "PH-6", "Mixed statuses")
	env.InitWorkflow(t, "PH-6")

	wfiID := env.GetWorkflowInstanceID(t, "PH-6", "test")

	// Insert sessions with various non-running statuses
	insertCompletedSession(t, env, "sess-pass", "PH-6", wfiID, "analyzer", "setup-analyzer", "claude:sonnet", "completed", "pass")
	insertCompletedSession(t, env, "sess-fail", "PH-6", wfiID, "analyzer", "code-reviewer", "claude:opus", "failed", "fail")
	insertCompletedSession(t, env, "sess-cont", "PH-6", wfiID, "builder", "implementor", "claude:sonnet", "continued", "continue")

	// Get workflow status
	status, err := getWorkflowStatus(t, env, "PH-6", &types.WorkflowGetRequest{
		Workflow: "test",
	})
	if err != nil {
		t.Fatalf("failed to get status: %v", err)
	}

	history, ok := status["agent_history"].([]interface{})
	if !ok {
		t.Fatalf("expected agent_history array, got %T", status["agent_history"])
	}
	// Continued sessions are filtered from history — only completed and failed remain
	if len(history) != 2 {
		t.Fatalf("expected 2 history entries (continued excluded), got %d", len(history))
	}

	// Verify each entry has phase field
	for i, entry := range history {
		e, _ := entry.(map[string]interface{})
		phase, ok := e["phase"].(string)
		if !ok {
			t.Fatalf("entry %d: expected 'phase' to be string, got %T", i, e["phase"])
		}
		agentType := e["agent_type"].(string)
		switch agentType {
		case "setup-analyzer", "code-reviewer":
			if phase != "analyzer" {
				t.Fatalf("entry %d (%s): expected phase 'analyzer', got %q", i, agentType, phase)
			}
		}
	}
}

// keysOf returns the keys of a map as a string slice for debug output.
func keysOf(m map[string]interface{}) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	return keys
}
