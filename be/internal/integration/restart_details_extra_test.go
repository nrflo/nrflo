package integration

import (
	"testing"
	"time"

	"be/internal/types"
)

// insertAgentMessage inserts a single agent_messages row for the given session.
func insertAgentMessage(t *testing.T, env *TestEnv, sessionID string, seq int, content string) {
	t.Helper()
	now := env.Clock.Now().UTC().Format(time.RFC3339Nano)
	_, err := env.Pool.Exec(
		`INSERT INTO agent_messages (session_id, seq, content, created_at) VALUES (?, ?, ?, ?)`,
		sessionID, seq, content, now,
	)
	if err != nil {
		t.Fatalf("insertAgentMessage session=%s seq=%d: %v", sessionID, seq, err)
	}
}

// TestRestartDetails_MessageCountFromDB verifies that message_count in each
// RestartDetail is populated from the agent_messages table.
func TestRestartDetails_MessageCountFromDB(t *testing.T) {
	env := NewTestEnv(t)
	env.CreateTicket(t, "RD-MC-1", "Message count test")
	env.InitWorkflow(t, "RD-MC-1")
	wfiID := env.GetWorkflowInstanceID(t, "RD-MC-1", "test")

	t0 := env.Clock.Now()

	// Session A: continued after low_context, 3 messages
	insertRRSession(t, env, "rdmc-sess-a", "RD-MC-1", wfiID, "analyzer",
		"continued", "", "low_context", "", 0, t0, t0.Add(10*time.Second), nil)
	insertAgentMessage(t, env, "rdmc-sess-a", 1, "msg one")
	insertAgentMessage(t, env, "rdmc-sess-a", 2, "msg two")
	insertAgentMessage(t, env, "rdmc-sess-a", 3, "msg three")

	env.Clock.Advance(1 * time.Second)
	t1 := env.Clock.Now()

	// Session B: continued after instant_stall, 1 message
	insertRRSession(t, env, "rdmc-sess-b", "RD-MC-1", wfiID, "analyzer",
		"continued", "", "instant_stall", "rdmc-sess-a", 0, t1, t1.Add(5*time.Second), nil)
	insertAgentMessage(t, env, "rdmc-sess-b", 1, "only msg")

	env.Clock.Advance(1 * time.Second)
	t2 := env.Clock.Now()

	// Session C: final completed session (no messages expected in restart_details)
	insertRRSession(t, env, "rdmc-sess-c", "RD-MC-1", wfiID, "analyzer",
		"completed", "pass", "", "rdmc-sess-a", 2, t2, t2.Add(20*time.Second), nil)

	status, err := getWorkflowStatus(t, env, "RD-MC-1", &types.WorkflowGetRequest{Workflow: "test"})
	if err != nil {
		t.Fatalf("GetStatus: %v", err)
	}

	history, ok := status["agent_history"].([]interface{})
	if !ok || len(history) != 1 {
		t.Fatalf("expected 1 history entry, got %v", status["agent_history"])
	}
	entry, _ := history[0].(map[string]interface{})

	rawDetails, ok := entry["restart_details"].([]interface{})
	if !ok || len(rawDetails) != 2 {
		t.Fatalf("expected 2 restart_details, got %v", entry["restart_details"])
	}

	details := toRestartDetails(t, rawDetails)

	if details[0].MessageCount != 3 {
		t.Errorf("details[0].MessageCount = %d, want 3", details[0].MessageCount)
	}
	if details[1].MessageCount != 1 {
		t.Errorf("details[1].MessageCount = %d, want 1", details[1].MessageCount)
	}
}

// TestRestartDetails_NullEndedAt_ZeroDuration verifies that a continued session
// with NULL ended_at produces duration_sec=0 rather than a negative or error.
func TestRestartDetails_NullEndedAt_ZeroDuration(t *testing.T) {
	env := NewTestEnv(t)
	env.CreateTicket(t, "RD-DUR-1", "Null ended_at duration test")
	env.InitWorkflow(t, "RD-DUR-1")
	wfiID := env.GetWorkflowInstanceID(t, "RD-DUR-1", "test")

	t0 := env.Clock.Now()

	// Session A: continued, but ended_at is zero (NULL in DB)
	insertRRSession(t, env, "rddur-sess-a", "RD-DUR-1", wfiID, "analyzer",
		"continued", "", "stall_restart_start_stall", "", 0, t0, time.Time{}, nil)

	env.Clock.Advance(1 * time.Second)
	t1 := env.Clock.Now()

	// Session B: currently running, ancestor is session A
	insertRRSession(t, env, "rddur-sess-b", "RD-DUR-1", wfiID, "analyzer",
		"running", "", "", "rddur-sess-a", 1, t1, time.Time{}, nil)

	status, err := getWorkflowStatus(t, env, "RD-DUR-1", &types.WorkflowGetRequest{Workflow: "test"})
	if err != nil {
		t.Fatalf("GetStatus: %v", err)
	}

	activeAgents, ok := status["active_agents"].(map[string]interface{})
	if !ok {
		t.Fatalf("expected active_agents map, got %T", status["active_agents"])
	}
	agent, ok := activeAgents["analyzer"].(map[string]interface{})
	if !ok {
		t.Fatalf("expected analyzer in active_agents")
	}

	rawDetails, ok := agent["restart_details"].([]interface{})
	if !ok || len(rawDetails) != 1 {
		t.Fatalf("expected 1 restart_detail, got %v", agent["restart_details"])
	}

	detail, _ := rawDetails[0].(map[string]interface{})
	dur, ok := detail["duration_sec"].(float64)
	if !ok {
		t.Fatalf("duration_sec missing or wrong type in %v", detail)
	}
	if dur != 0 {
		t.Errorf("duration_sec = %v, want 0 for NULL ended_at", dur)
	}
}

// TestRestartDetails_NoRestartReasonsField verifies that the old restart_reasons
// field is completely absent from both active_agents and agent_history responses.
func TestRestartDetails_NoRestartReasonsField(t *testing.T) {
	env := NewTestEnv(t)
	env.CreateTicket(t, "RD-BC-1", "No restart_reasons backward compat test")
	env.InitWorkflow(t, "RD-BC-1")
	wfiID := env.GetWorkflowInstanceID(t, "RD-BC-1", "test")

	t0 := env.Clock.Now()

	// Session A: continued
	insertRRSession(t, env, "rdbc-sess-a", "RD-BC-1", wfiID, "analyzer",
		"continued", "", "low_context", "", 0, t0, t0.Add(15*time.Second), nil)

	env.Clock.Advance(1 * time.Second)
	t1 := env.Clock.Now()

	// Session B: currently running with restart_count=1
	insertRRSession(t, env, "rdbc-sess-b", "RD-BC-1", wfiID, "analyzer",
		"running", "", "", "rdbc-sess-a", 1, t1, time.Time{}, nil)

	status, err := getWorkflowStatus(t, env, "RD-BC-1", &types.WorkflowGetRequest{Workflow: "test"})
	if err != nil {
		t.Fatalf("GetStatus: %v", err)
	}

	// Check active agent — must have restart_details, must NOT have restart_reasons
	activeAgents, ok := status["active_agents"].(map[string]interface{})
	if !ok {
		t.Fatalf("expected active_agents map")
	}
	agent, ok := activeAgents["analyzer"].(map[string]interface{})
	if !ok {
		t.Fatalf("expected analyzer in active_agents")
	}
	if _, exists := agent["restart_reasons"]; exists {
		t.Errorf("active agent: restart_reasons field must not exist, got %v", agent["restart_reasons"])
	}
	if _, exists := agent["restart_details"]; !exists {
		t.Errorf("active agent: restart_details must exist when restart_count > 0")
	}

	// Now complete session B and verify history also lacks restart_reasons
	env.Clock.Advance(1 * time.Second)
	t2 := env.Clock.Now()
	insertRRSession(t, env, "rdbc-sess-c", "RD-BC-1", wfiID, "analyzer",
		"completed", "pass", "", "rdbc-sess-a", 1, t2, t2.Add(30*time.Second), nil)

	// Replace running session B with non-running so history is built from session C
	_, err = env.Pool.Exec(
		`UPDATE agent_sessions SET status='continued' WHERE id='rdbc-sess-b'`)
	if err != nil {
		t.Fatalf("failed to update session B to continued: %v", err)
	}

	status2, err := getWorkflowStatus(t, env, "RD-BC-1", &types.WorkflowGetRequest{Workflow: "test"})
	if err != nil {
		t.Fatalf("GetStatus (history): %v", err)
	}

	history, ok := status2["agent_history"].([]interface{})
	if !ok || len(history) == 0 {
		t.Fatalf("expected at least 1 history entry, got %v", status2["agent_history"])
	}
	entry, _ := history[0].(map[string]interface{})
	if _, exists := entry["restart_reasons"]; exists {
		t.Errorf("history entry: restart_reasons field must not exist, got %v", entry["restart_reasons"])
	}
	if _, exists := entry["restart_details"]; !exists {
		t.Errorf("history entry: restart_details must exist when restart_count > 0")
	}
}

// TestRestartDetails_ContextLeftOmittedWhenNil verifies that context_left is
// absent from the JSON for a RestartDetail whose session had NULL context_left.
func TestRestartDetails_ContextLeftOmittedWhenNil(t *testing.T) {
	env := NewTestEnv(t)
	env.CreateTicket(t, "RD-CTX-1", "Context left nil omit test")
	env.InitWorkflow(t, "RD-CTX-1")
	wfiID := env.GetWorkflowInstanceID(t, "RD-CTX-1", "test")

	t0 := env.Clock.Now()

	// Session A: continued with no context_left recorded
	insertRRSession(t, env, "rdctx-sess-a", "RD-CTX-1", wfiID, "analyzer",
		"continued", "", "explicit", "", 0, t0, t0.Add(7*time.Second), nil)

	env.Clock.Advance(1 * time.Second)
	t1 := env.Clock.Now()

	insertRRSession(t, env, "rdctx-sess-b", "RD-CTX-1", wfiID, "analyzer",
		"completed", "pass", "", "rdctx-sess-a", 1, t1, t1.Add(20*time.Second), nil)

	status, err := getWorkflowStatus(t, env, "RD-CTX-1", &types.WorkflowGetRequest{Workflow: "test"})
	if err != nil {
		t.Fatalf("GetStatus: %v", err)
	}

	history, ok := status["agent_history"].([]interface{})
	if !ok || len(history) != 1 {
		t.Fatalf("expected 1 history entry, got %v", status["agent_history"])
	}
	entry, _ := history[0].(map[string]interface{})

	rawDetails, ok := entry["restart_details"].([]interface{})
	if !ok || len(rawDetails) != 1 {
		t.Fatalf("expected 1 restart_detail, got %v", entry["restart_details"])
	}
	detail, _ := rawDetails[0].(map[string]interface{})

	// context_left must be absent from the JSON map when the session had NULL context_left
	if cl, exists := detail["context_left"]; exists && cl != nil {
		t.Errorf("context_left should be omitted when NULL, got %v", cl)
	}
}
