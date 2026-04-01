package integration

import (
	"testing"
	"time"

	"be/internal/service"
	"be/internal/types"
)

// insertRRSession inserts an agent session for restart_details testing.
// result, reasonCode, and ancestorID may be "" to store NULL.
// endedAt is zero-value to store NULL. contextLeft is nil to store NULL.
func insertRRSession(t *testing.T, env *TestEnv, id, ticketID, wfiID, agentType, status, result, reasonCode, ancestorID string, restartCount int, startedAt time.Time, endedAt time.Time, contextLeft *int64) {
	t.Helper()
	now := env.Clock.Now().UTC().Format(time.RFC3339Nano)
	startedAtStr := startedAt.UTC().Format(time.RFC3339Nano)

	var ancestorVal, reasonVal, resultVal, endedAtVal, contextLeftVal interface{}
	if ancestorID != "" {
		ancestorVal = ancestorID
	}
	if reasonCode != "" {
		reasonVal = reasonCode
	}
	if result != "" {
		resultVal = result
	}
	if !endedAt.IsZero() {
		endedAtVal = endedAt.UTC().Format(time.RFC3339Nano)
	}
	if contextLeft != nil {
		contextLeftVal = *contextLeft
	}

	_, err := env.Pool.Exec(`
		INSERT INTO agent_sessions (id, project_id, ticket_id, workflow_instance_id, phase, agent_type,
			model_id, status, result, result_reason, pid, findings,
			context_left, ancestor_session_id, spawn_command, prompt_context,
			restart_count, started_at, ended_at, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, NULL, ?, ?, ?, NULL, NULL, ?, ?, NULL, NULL, ?, ?, ?, ?, ?)`,
		id, env.ProjectID, ticketID, wfiID, agentType, agentType,
		status, resultVal, reasonVal, contextLeftVal, ancestorVal, restartCount, startedAtStr, endedAtVal, now, now,
	)
	if err != nil {
		t.Fatalf("insertRRSession %s: %v", id, err)
	}
}

// assertRestartDetails extracts and validates restart_details from a response entry.
func assertRestartDetails(t *testing.T, entry map[string]interface{}, wantReasons []string) {
	t.Helper()
	rawDetails, ok := entry["restart_details"].([]interface{})
	if !ok {
		t.Fatalf("expected restart_details array, got %T: %v", entry["restart_details"], entry["restart_details"])
	}
	if len(rawDetails) != len(wantReasons) {
		t.Fatalf("expected %d restart_details, got %d: %v", len(wantReasons), len(rawDetails), rawDetails)
	}
	for i, want := range wantReasons {
		detail := rawDetails[i]
		// JSON round-tripped through map[string]interface{}, so check inner map
		var d map[string]interface{}
		switch v := detail.(type) {
		case map[string]interface{}:
			d = v
		default:
			// Might be a RestartDetail struct serialized via JSON
			t.Fatalf("restart_details[%d]: unexpected type %T", i, detail)
		}
		if d["reason"] != want {
			t.Errorf("restart_details[%d].reason = %v, want %s", i, d["reason"], want)
		}
		// duration_sec should be a number
		if _, ok := d["duration_sec"]; !ok {
			t.Errorf("restart_details[%d] missing duration_sec", i)
		}
		// message_count should be a number
		if _, ok := d["message_count"]; !ok {
			t.Errorf("restart_details[%d] missing message_count", i)
		}
	}
}

// toRestartDetails converts []interface{} to []service.RestartDetail for detailed assertions.
func toRestartDetails(t *testing.T, raw []interface{}) []service.RestartDetail {
	t.Helper()
	var details []service.RestartDetail
	for i, item := range raw {
		m, ok := item.(map[string]interface{})
		if !ok {
			t.Fatalf("restart_details[%d]: expected map, got %T", i, item)
		}
		d := service.RestartDetail{
			Reason:      m["reason"].(string),
			DurationSec: m["duration_sec"].(float64),
			MessageCount: int(m["message_count"].(float64)),
		}
		if cl, ok := m["context_left"]; ok && cl != nil {
			v := int64(cl.(float64))
			d.ContextLeft = &v
		}
		details = append(details, d)
	}
	return details
}

// TestRestartDetailsInHistory_TwoRestarts verifies that agent_history entries
// include restart_details when the agent has two continued sessions in its chain.
func TestRestartDetailsInHistory_TwoRestarts(t *testing.T) {
	env := NewTestEnv(t)
	env.CreateTicket(t, "RR-1", "Restart details history test")
	env.InitWorkflow(t, "RR-1")
	wfiID := env.GetWorkflowInstanceID(t, "RR-1", "test")

	t0 := env.Clock.Now()
	ctxLeft := int64(50000)
	// Session A: first in chain, got low context and was continued
	insertRRSession(t, env, "rr1-sess-a", "RR-1", wfiID, "analyzer",
		"continued", "", "low_context", "", 0, t0, t0.Add(30*time.Second), &ctxLeft)

	env.Clock.Advance(1 * time.Second)
	t1 := env.Clock.Now()
	// Session B: second in chain, stalled and was continued
	insertRRSession(t, env, "rr1-sess-b", "RR-1", wfiID, "analyzer",
		"continued", "", "instant_stall", "rr1-sess-a", 0, t1, t1.Add(42*time.Second), nil)

	env.Clock.Advance(1 * time.Second)
	t2 := env.Clock.Now()
	// Session C: final completed session with restart_count=2 and ancestor=first session
	insertRRSession(t, env, "rr1-sess-c", "RR-1", wfiID, "analyzer",
		"completed", "pass", "", "rr1-sess-a", 2, t2, t2.Add(60*time.Second), nil)

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

	assertRestartDetails(t, entry, []string{"low_context", "instant_stall"})

	// Verify detailed fields
	rawDetails := entry["restart_details"].([]interface{})
	details := toRestartDetails(t, rawDetails)
	if details[0].DurationSec != 30 {
		t.Errorf("details[0].DurationSec = %v, want 30", details[0].DurationSec)
	}
	if details[0].ContextLeft == nil || *details[0].ContextLeft != 50000 {
		t.Errorf("details[0].ContextLeft = %v, want 50000", details[0].ContextLeft)
	}
	if details[1].DurationSec != 42 {
		t.Errorf("details[1].DurationSec = %v, want 42", details[1].DurationSec)
	}
	if details[1].ContextLeft != nil {
		t.Errorf("details[1].ContextLeft = %v, want nil", details[1].ContextLeft)
	}
}

// TestRestartDetailsInActiveAgent_OneRestart verifies that active (running) agents
// include restart_details when they have a continued predecessor in their chain.
func TestRestartDetailsInActiveAgent_OneRestart(t *testing.T) {
	env := NewTestEnv(t)
	env.CreateTicket(t, "RR-2", "Restart details active agent test")
	env.InitWorkflow(t, "RR-2")
	wfiID := env.GetWorkflowInstanceID(t, "RR-2", "test")

	t0 := env.Clock.Now()
	// Session A: original session, stalled and was continued
	insertRRSession(t, env, "rr2-sess-a", "RR-2", wfiID, "analyzer",
		"continued", "", "stall_restart_start_stall", "", 0, t0, t0.Add(120*time.Second), nil)

	env.Clock.Advance(1 * time.Second)
	t1 := env.Clock.Now()
	// Session B: currently running, ancestor is session A, restart_count=1
	insertRRSession(t, env, "rr2-sess-b", "RR-2", wfiID, "analyzer",
		"running", "", "", "rr2-sess-a", 1, t1, time.Time{}, nil)

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
	assertRestartDetails(t, agent, []string{"stall_restart_start_stall"})
}

// TestRestartDetailsAbsent_WhenNoRestarts verifies that restart_details is not
// present in the response when restart_count is 0.
func TestRestartDetailsAbsent_WhenNoRestarts(t *testing.T) {
	env := NewTestEnv(t)
	env.CreateTicket(t, "RR-3", "No restarts test")
	env.InitWorkflow(t, "RR-3")
	wfiID := env.GetWorkflowInstanceID(t, "RR-3", "test")

	t0 := env.Clock.Now()
	insertRRSession(t, env, "rr3-sess", "RR-3", wfiID, "analyzer",
		"completed", "pass", "", "", 0, t0, t0.Add(10*time.Second), nil)

	status, err := getWorkflowStatus(t, env, "RR-3", &types.WorkflowGetRequest{Workflow: "test"})
	if err != nil {
		t.Fatalf("GetStatus: %v", err)
	}

	history, ok := status["agent_history"].([]interface{})
	if !ok || len(history) != 1 {
		t.Fatalf("expected 1 history entry, got %v", status["agent_history"])
	}
	entry, _ := history[0].(map[string]interface{})

	if _, exists := entry["restart_details"]; exists {
		t.Errorf("expected no restart_details when restart_count=0, got: %v", entry["restart_details"])
	}
	if rc := entry["restart_count"].(float64); rc != 0 {
		t.Errorf("restart_count = %v, want 0", rc)
	}
}

// TestRestartDetailsMultipleAgents_IndependentChains verifies that two agents
// with independent restart chains each receive their own correct restart_details.
func TestRestartDetailsMultipleAgents_IndependentChains(t *testing.T) {
	env := NewTestEnv(t)
	env.CreateTicket(t, "RR-4", "Multiple agents independent chains")
	env.InitWorkflow(t, "RR-4")
	wfiID := env.GetWorkflowInstanceID(t, "RR-4", "test")

	t0 := env.Clock.Now()
	// Analyzer chain: low_context restart
	insertRRSession(t, env, "rr4-ana-a", "RR-4", wfiID, "analyzer",
		"continued", "", "low_context", "", 0, t0, t0.Add(5*time.Minute), nil)
	env.Clock.Advance(1 * time.Second)
	t1 := env.Clock.Now()
	insertRRSession(t, env, "rr4-ana-b", "RR-4", wfiID, "analyzer",
		"completed", "pass", "", "rr4-ana-a", 1, t1, t1.Add(3*time.Minute), nil)

	env.Clock.Advance(1 * time.Second)
	t2 := env.Clock.Now()
	// Builder chain: explicit restart
	insertRRSession(t, env, "rr4-bld-a", "RR-4", wfiID, "builder",
		"continued", "", "explicit", "", 0, t2, t2.Add(2*time.Minute), nil)
	env.Clock.Advance(1 * time.Second)
	t3 := env.Clock.Now()
	insertRRSession(t, env, "rr4-bld-b", "RR-4", wfiID, "builder",
		"completed", "pass", "", "rr4-bld-a", 1, t3, t3.Add(1*time.Minute), nil)

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
		assertRestartDetails(t, e, []string{expectedReason})
	}
}

// TestRestartDetailsNullReason_Excluded verifies that continued sessions with
// NULL result_reason are excluded from the restart_details list.
func TestRestartDetailsNullReason_Excluded(t *testing.T) {
	env := NewTestEnv(t)
	env.CreateTicket(t, "RR-5", "Null reason excluded test")
	env.InitWorkflow(t, "RR-5")
	wfiID := env.GetWorkflowInstanceID(t, "RR-5", "test")

	t0 := env.Clock.Now()
	// Continued session with NULL result_reason (reasonCode="")
	insertRRSession(t, env, "rr5-sess-a", "RR-5", wfiID, "analyzer",
		"continued", "", "", "", 0, t0, t0.Add(10*time.Second), nil)

	env.Clock.Advance(1 * time.Second)
	t1 := env.Clock.Now()
	// Final completed session with restart_count=1 but no known reason
	insertRRSession(t, env, "rr5-sess-b", "RR-5", wfiID, "analyzer",
		"completed", "pass", "", "rr5-sess-a", 1, t1, t1.Add(10*time.Second), nil)

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
	// restart_details should be absent since the NULL reason was filtered out
	if _, exists := entry["restart_details"]; exists {
		t.Errorf("expected no restart_details when all reasons are NULL, got: %v", entry["restart_details"])
	}
}

// TestRestartDetailsOrder_Chronological verifies that restart_details are returned
// in chronological order (by started_at) regardless of insertion order.
func TestRestartDetailsOrder_Chronological(t *testing.T) {
	env := NewTestEnv(t)
	env.CreateTicket(t, "RR-6", "Chronological order test")
	env.InitWorkflow(t, "RR-6")
	wfiID := env.GetWorkflowInstanceID(t, "RR-6", "test")

	t0 := env.Clock.Now()
	insertRRSession(t, env, "rr6-sess-a", "RR-6", wfiID, "analyzer",
		"continued", "", "low_context", "", 0, t0, t0.Add(10*time.Second), nil)

	env.Clock.Advance(2 * time.Second)
	t1 := env.Clock.Now()
	insertRRSession(t, env, "rr6-sess-b", "RR-6", wfiID, "analyzer",
		"continued", "", "stall_restart_running_stall", "rr6-sess-a", 0, t1, t1.Add(20*time.Second), nil)

	env.Clock.Advance(2 * time.Second)
	t2 := env.Clock.Now()
	insertRRSession(t, env, "rr6-sess-c", "RR-6", wfiID, "analyzer",
		"continued", "", "explicit", "rr6-sess-a", 0, t2, t2.Add(15*time.Second), nil)

	env.Clock.Advance(2 * time.Second)
	t3 := env.Clock.Now()
	insertRRSession(t, env, "rr6-sess-d", "RR-6", wfiID, "analyzer",
		"completed", "pass", "", "rr6-sess-a", 3, t3, t3.Add(5*time.Second), nil)

	status, err := getWorkflowStatus(t, env, "RR-6", &types.WorkflowGetRequest{Workflow: "test"})
	if err != nil {
		t.Fatalf("GetStatus: %v", err)
	}

	history, _ := status["agent_history"].([]interface{})
	if len(history) != 1 {
		t.Fatalf("expected 1 history entry, got %d", len(history))
	}

	entry, _ := history[0].(map[string]interface{})
	assertRestartDetails(t, entry, []string{"low_context", "stall_restart_running_stall", "explicit"})
}
