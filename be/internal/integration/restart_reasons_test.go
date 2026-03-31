package integration

import (
	"testing"
	"time"

	"be/internal/types"
)

// insertRRSession inserts an agent session for restart_reasons testing.
// result, reasonCode, and ancestorID may be "" to store NULL.
func insertRRSession(t *testing.T, env *TestEnv, id, ticketID, wfiID, agentType, status, result, reasonCode, ancestorID string, restartCount int, startedAt time.Time) {
	t.Helper()
	now := env.Clock.Now().UTC().Format(time.RFC3339Nano)
	startedAtStr := startedAt.UTC().Format(time.RFC3339Nano)

	var ancestorVal, reasonVal, resultVal interface{}
	if ancestorID != "" {
		ancestorVal = ancestorID
	}
	if reasonCode != "" {
		reasonVal = reasonCode
	}
	if result != "" {
		resultVal = result
	}

	_, err := env.Pool.Exec(`
		INSERT INTO agent_sessions (id, project_id, ticket_id, workflow_instance_id, phase, agent_type,
			model_id, status, result, result_reason, pid, findings,
			context_left, ancestor_session_id, spawn_command, prompt_context,
			restart_count, started_at, ended_at, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, NULL, ?, ?, ?, NULL, NULL, NULL, ?, NULL, NULL, ?, ?, NULL, ?, ?)`,
		id, env.ProjectID, ticketID, wfiID, agentType, agentType,
		status, resultVal, reasonVal, ancestorVal, restartCount, startedAtStr, now, now,
	)
	if err != nil {
		t.Fatalf("insertRRSession %s: %v", id, err)
	}
}

// TestRestartReasonsInHistory_TwoRestarts verifies that agent_history entries
// include restart_reasons when the agent has two continued sessions in its chain.
func TestRestartReasonsInHistory_TwoRestarts(t *testing.T) {
	env := NewTestEnv(t)
	env.CreateTicket(t, "RR-1", "Restart reasons history test")
	env.InitWorkflow(t, "RR-1")
	wfiID := env.GetWorkflowInstanceID(t, "RR-1", "test")

	t0 := env.Clock.Now()
	// Session A: first in chain, got low context and was continued
	insertRRSession(t, env, "rr1-sess-a", "RR-1", wfiID, "analyzer",
		"continued", "", "low_context", "", 0, t0)

	env.Clock.Advance(1 * time.Second)
	t1 := env.Clock.Now()
	// Session B: second in chain, stalled and was continued
	insertRRSession(t, env, "rr1-sess-b", "RR-1", wfiID, "analyzer",
		"continued", "", "instant_stall", "rr1-sess-a", 0, t1)

	env.Clock.Advance(1 * time.Second)
	t2 := env.Clock.Now()
	// Session C: final completed session with restart_count=2 and ancestor=first session
	insertRRSession(t, env, "rr1-sess-c", "RR-1", wfiID, "analyzer",
		"completed", "pass", "", "rr1-sess-a", 2, t2)

	status, err := getWorkflowStatus(t, env, "RR-1", &types.WorkflowGetRequest{Workflow: "test"})
	if err != nil {
		t.Fatalf("GetStatus: %v", err)
	}

	history, ok := status["agent_history"].([]interface{})
	if !ok || len(history) != 1 {
		t.Fatalf("expected 1 history entry, got %v", status["agent_history"])
	}

	entry, _ := history[0].(map[string]interface{})
	if rc := entry["restart_count"].(float64); rc != 2 {
		t.Errorf("restart_count = %v, want 2", rc)
	}

	reasons, ok := entry["restart_reasons"].([]interface{})
	if !ok {
		t.Fatalf("expected restart_reasons array, got %T: %v", entry["restart_reasons"], entry["restart_reasons"])
	}
	if len(reasons) != 2 {
		t.Fatalf("expected 2 restart_reasons, got %d: %v", len(reasons), reasons)
	}
	if reasons[0] != "low_context" {
		t.Errorf("restart_reasons[0] = %v, want low_context", reasons[0])
	}
	if reasons[1] != "instant_stall" {
		t.Errorf("restart_reasons[1] = %v, want instant_stall", reasons[1])
	}
}

// TestRestartReasonsInActiveAgent_OneRestart verifies that active (running) agents
// include restart_reasons when they have a continued predecessor in their chain.
func TestRestartReasonsInActiveAgent_OneRestart(t *testing.T) {
	env := NewTestEnv(t)
	env.CreateTicket(t, "RR-2", "Restart reasons active agent test")
	env.InitWorkflow(t, "RR-2")
	wfiID := env.GetWorkflowInstanceID(t, "RR-2", "test")

	t0 := env.Clock.Now()
	// Session A: original session, stalled and was continued
	insertRRSession(t, env, "rr2-sess-a", "RR-2", wfiID, "analyzer",
		"continued", "", "stall_restart_start_stall", "", 0, t0)

	env.Clock.Advance(1 * time.Second)
	t1 := env.Clock.Now()
	// Session B: currently running, ancestor is session A, restart_count=1
	insertRRSession(t, env, "rr2-sess-b", "RR-2", wfiID, "analyzer",
		"running", "", "", "rr2-sess-a", 1, t1)

	status, err := getWorkflowStatus(t, env, "RR-2", &types.WorkflowGetRequest{Workflow: "test"})
	if err != nil {
		t.Fatalf("GetStatus: %v", err)
	}

	activeAgents, ok := status["active_agents"].(map[string]interface{})
	if !ok {
		t.Fatalf("expected active_agents map, got %T", status["active_agents"])
	}
	agent, ok := activeAgents["analyzer"].(map[string]interface{})
	if !ok {
		t.Fatalf("expected analyzer in active_agents, got keys: %v", activeAgents)
	}

	if rc := agent["restart_count"].(float64); rc != 1 {
		t.Errorf("restart_count = %v, want 1", rc)
	}
	reasons, ok := agent["restart_reasons"].([]interface{})
	if !ok {
		t.Fatalf("expected restart_reasons array, got %T: %v", agent["restart_reasons"], agent["restart_reasons"])
	}
	if len(reasons) != 1 || reasons[0] != "stall_restart_start_stall" {
		t.Errorf("restart_reasons = %v, want [stall_restart_start_stall]", reasons)
	}
}

// TestRestartReasonsAbsent_WhenNoRestarts verifies that restart_reasons is not
// present in the response when restart_count is 0.
func TestRestartReasonsAbsent_WhenNoRestarts(t *testing.T) {
	env := NewTestEnv(t)
	env.CreateTicket(t, "RR-3", "No restarts test")
	env.InitWorkflow(t, "RR-3")
	wfiID := env.GetWorkflowInstanceID(t, "RR-3", "test")

	t0 := env.Clock.Now()
	insertRRSession(t, env, "rr3-sess", "RR-3", wfiID, "analyzer",
		"completed", "pass", "", "", 0, t0)

	status, err := getWorkflowStatus(t, env, "RR-3", &types.WorkflowGetRequest{Workflow: "test"})
	if err != nil {
		t.Fatalf("GetStatus: %v", err)
	}

	history, ok := status["agent_history"].([]interface{})
	if !ok || len(history) != 1 {
		t.Fatalf("expected 1 history entry, got %v", status["agent_history"])
	}
	entry, _ := history[0].(map[string]interface{})

	if _, exists := entry["restart_reasons"]; exists {
		t.Errorf("expected no restart_reasons when restart_count=0, got: %v", entry["restart_reasons"])
	}
	if rc := entry["restart_count"].(float64); rc != 0 {
		t.Errorf("restart_count = %v, want 0", rc)
	}
}

// TestRestartReasonsMultipleAgents_IndependentChains verifies that two agents
// with independent restart chains each receive their own correct restart_reasons.
func TestRestartReasonsMultipleAgents_IndependentChains(t *testing.T) {
	env := NewTestEnv(t)
	env.CreateTicket(t, "RR-4", "Multiple agents independent chains")
	env.InitWorkflow(t, "RR-4")
	wfiID := env.GetWorkflowInstanceID(t, "RR-4", "test")

	t0 := env.Clock.Now()
	// Analyzer chain: low_context restart
	insertRRSession(t, env, "rr4-ana-a", "RR-4", wfiID, "analyzer",
		"continued", "", "low_context", "", 0, t0)
	env.Clock.Advance(1 * time.Second)
	t1 := env.Clock.Now()
	insertRRSession(t, env, "rr4-ana-b", "RR-4", wfiID, "analyzer",
		"completed", "pass", "", "rr4-ana-a", 1, t1)

	env.Clock.Advance(1 * time.Second)
	t2 := env.Clock.Now()
	// Builder chain: explicit restart
	insertRRSession(t, env, "rr4-bld-a", "RR-4", wfiID, "builder",
		"continued", "", "explicit", "", 0, t2)
	env.Clock.Advance(1 * time.Second)
	t3 := env.Clock.Now()
	insertRRSession(t, env, "rr4-bld-b", "RR-4", wfiID, "builder",
		"completed", "pass", "", "rr4-bld-a", 1, t3)

	status, err := getWorkflowStatus(t, env, "RR-4", &types.WorkflowGetRequest{Workflow: "test"})
	if err != nil {
		t.Fatalf("GetStatus: %v", err)
	}

	history, ok := status["agent_history"].([]interface{})
	if !ok || len(history) != 2 {
		t.Fatalf("expected 2 history entries, got %v", status["agent_history"])
	}

	// Build a map of agent_type -> entry for lookup
	byType := make(map[string]map[string]interface{})
	for _, h := range history {
		e, _ := h.(map[string]interface{})
		byType[e["agent_type"].(string)] = e
	}

	for agentType, expectedReason := range map[string]string{
		"analyzer": "low_context",
		"builder":  "explicit",
	} {
		e := byType[agentType]
		reasons, ok := e["restart_reasons"].([]interface{})
		if !ok || len(reasons) != 1 {
			t.Errorf("%s: expected restart_reasons=[%s], got %v", agentType, expectedReason, e["restart_reasons"])
			continue
		}
		if reasons[0] != expectedReason {
			t.Errorf("%s: restart_reasons[0] = %v, want %s", agentType, reasons[0], expectedReason)
		}
	}
}

// TestRestartReasonsNullReason_Excluded verifies that continued sessions with
// NULL result_reason are excluded from the restart_reasons list.
func TestRestartReasonsNullReason_Excluded(t *testing.T) {
	env := NewTestEnv(t)
	env.CreateTicket(t, "RR-5", "Null reason excluded test")
	env.InitWorkflow(t, "RR-5")
	wfiID := env.GetWorkflowInstanceID(t, "RR-5", "test")

	t0 := env.Clock.Now()
	// Continued session with NULL result_reason (reasonCode="")
	insertRRSession(t, env, "rr5-sess-a", "RR-5", wfiID, "analyzer",
		"continued", "", "", "", 0, t0)

	env.Clock.Advance(1 * time.Second)
	t1 := env.Clock.Now()
	// Final completed session with restart_count=1 but no known reason
	insertRRSession(t, env, "rr5-sess-b", "RR-5", wfiID, "analyzer",
		"completed", "pass", "", "rr5-sess-a", 1, t1)

	status, err := getWorkflowStatus(t, env, "RR-5", &types.WorkflowGetRequest{Workflow: "test"})
	if err != nil {
		t.Fatalf("GetStatus: %v", err)
	}

	history, ok := status["agent_history"].([]interface{})
	if !ok || len(history) != 1 {
		t.Fatalf("expected 1 history entry, got %v", status["agent_history"])
	}
	entry, _ := history[0].(map[string]interface{})

	if rc := entry["restart_count"].(float64); rc != 1 {
		t.Errorf("restart_count = %v, want 1", rc)
	}
	// restart_reasons should be absent since the NULL reason was filtered out
	if _, exists := entry["restart_reasons"]; exists {
		t.Errorf("expected no restart_reasons when all reasons are NULL, got: %v", entry["restart_reasons"])
	}
}

// TestRestartReasonsOrder_Chronological verifies that restart_reasons are returned
// in chronological order (by started_at) regardless of insertion order.
func TestRestartReasonsOrder_Chronological(t *testing.T) {
	env := NewTestEnv(t)
	env.CreateTicket(t, "RR-6", "Chronological order test")
	env.InitWorkflow(t, "RR-6")
	wfiID := env.GetWorkflowInstanceID(t, "RR-6", "test")

	t0 := env.Clock.Now()
	insertRRSession(t, env, "rr6-sess-a", "RR-6", wfiID, "analyzer",
		"continued", "", "low_context", "", 0, t0)

	env.Clock.Advance(2 * time.Second)
	t1 := env.Clock.Now()
	insertRRSession(t, env, "rr6-sess-b", "RR-6", wfiID, "analyzer",
		"continued", "", "stall_restart_running_stall", "rr6-sess-a", 0, t1)

	env.Clock.Advance(2 * time.Second)
	t2 := env.Clock.Now()
	insertRRSession(t, env, "rr6-sess-c", "RR-6", wfiID, "analyzer",
		"continued", "", "explicit", "rr6-sess-a", 0, t2)

	env.Clock.Advance(2 * time.Second)
	t3 := env.Clock.Now()
	insertRRSession(t, env, "rr6-sess-d", "RR-6", wfiID, "analyzer",
		"completed", "pass", "", "rr6-sess-a", 3, t3)

	status, err := getWorkflowStatus(t, env, "RR-6", &types.WorkflowGetRequest{Workflow: "test"})
	if err != nil {
		t.Fatalf("GetStatus: %v", err)
	}

	history, _ := status["agent_history"].([]interface{})
	if len(history) != 1 {
		t.Fatalf("expected 1 history entry, got %d", len(history))
	}

	entry, _ := history[0].(map[string]interface{})
	reasons, ok := entry["restart_reasons"].([]interface{})
	if !ok || len(reasons) != 3 {
		t.Fatalf("expected 3 restart_reasons, got %v", entry["restart_reasons"])
	}

	want := []string{"low_context", "stall_restart_running_stall", "explicit"}
	for i, w := range want {
		if reasons[i] != w {
			t.Errorf("restart_reasons[%d] = %v, want %s", i, reasons[i], w)
		}
	}
}
