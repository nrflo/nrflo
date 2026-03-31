package integration

import (
	"encoding/json"
	"testing"
	"time"

	"be/internal/types"
)

// setSessionFindings sets the findings JSON on an agent session via direct SQL.
func setSessionFindings(t *testing.T, env *TestEnv, sessionID string, findings map[string]interface{}) {
	t.Helper()
	data, err := json.Marshal(findings)
	if err != nil {
		t.Fatalf("failed to marshal findings: %v", err)
	}
	_, err = env.Pool.Exec(`UPDATE agent_sessions SET findings = ? WHERE id = ?`, string(data), sessionID)
	if err != nil {
		t.Fatalf("failed to set findings on session %s: %v", sessionID, err)
	}
}

// TestWorkflowFinalResult_SingleSession verifies that when one session has
// workflow_final_result, it appears as a top-level field in the state response.
func TestWorkflowFinalResult_SingleSession(t *testing.T) {
	env := NewTestEnv(t)
	env.CreateTicket(t, "WFR-1", "Final result test")
	env.InitWorkflow(t, "WFR-1")
	wfiID := env.GetWorkflowInstanceID(t, "WFR-1", "test")

	env.InsertAgentSession(t, "sess-wfr-1", "WFR-1", wfiID, "analyzer", "analyzer", "")
	setSessionFindings(t, env, "sess-wfr-1", map[string]interface{}{
		"workflow_final_result": "Implementation complete: all tests pass",
	})
	env.CompleteAgentSession(t, "sess-wfr-1", "pass")

	status, err := getWorkflowStatus(t, env, "WFR-1", &types.WorkflowGetRequest{Workflow: "test"})
	if err != nil {
		t.Fatalf("failed to get status: %v", err)
	}

	got, ok := status["workflow_final_result"].(string)
	if !ok {
		t.Fatalf("expected workflow_final_result string in state, got %T: %v", status["workflow_final_result"], status["workflow_final_result"])
	}
	if got != "Implementation complete: all tests pass" {
		t.Errorf("workflow_final_result = %q, want %q", got, "Implementation complete: all tests pass")
	}
}

// TestWorkflowFinalResult_AbsentWhenNoKey verifies that when no session has
// workflow_final_result, the field is absent from the state response.
func TestWorkflowFinalResult_AbsentWhenNoKey(t *testing.T) {
	env := NewTestEnv(t)
	env.CreateTicket(t, "WFR-2", "No final result")
	env.InitWorkflow(t, "WFR-2")
	wfiID := env.GetWorkflowInstanceID(t, "WFR-2", "test")

	env.InsertAgentSession(t, "sess-wfr-2", "WFR-2", wfiID, "analyzer", "analyzer", "")
	setSessionFindings(t, env, "sess-wfr-2", map[string]interface{}{
		"some_other_key": "value",
	})
	env.CompleteAgentSession(t, "sess-wfr-2", "pass")

	status, err := getWorkflowStatus(t, env, "WFR-2", &types.WorkflowGetRequest{Workflow: "test"})
	if err != nil {
		t.Fatalf("failed to get status: %v", err)
	}

	if _, exists := status["workflow_final_result"]; exists {
		t.Errorf("expected workflow_final_result to be absent, but got %v", status["workflow_final_result"])
	}
}

// TestWorkflowFinalResult_MultipleCompleted_LastWins verifies that when multiple
// completed sessions have workflow_final_result, the one with the latest ended_at wins.
func TestWorkflowFinalResult_MultipleCompleted_LastWins(t *testing.T) {
	env := NewTestEnv(t)
	env.CreateTicket(t, "WFR-3", "Last writer wins")
	env.InitWorkflow(t, "WFR-3")
	wfiID := env.GetWorkflowInstanceID(t, "WFR-3", "test")

	// First session: completed at T0
	env.InsertAgentSession(t, "sess-wfr-3a", "WFR-3", wfiID, "analyzer", "analyzer", "")
	setSessionFindings(t, env, "sess-wfr-3a", map[string]interface{}{
		"workflow_final_result": "first result",
	})
	env.CompleteAgentSession(t, "sess-wfr-3a", "pass")

	// Second session: completed at T0 + 1s
	env.Clock.Advance(1 * time.Second)
	env.InsertAgentSession(t, "sess-wfr-3b", "WFR-3", wfiID, "builder", "builder", "")
	setSessionFindings(t, env, "sess-wfr-3b", map[string]interface{}{
		"workflow_final_result": "second result",
	})
	env.CompleteAgentSession(t, "sess-wfr-3b", "pass")

	status, err := getWorkflowStatus(t, env, "WFR-3", &types.WorkflowGetRequest{Workflow: "test"})
	if err != nil {
		t.Fatalf("failed to get status: %v", err)
	}

	got, ok := status["workflow_final_result"].(string)
	if !ok {
		t.Fatalf("expected workflow_final_result string, got %T: %v", status["workflow_final_result"], status["workflow_final_result"])
	}
	// The second session ended later, so its value wins.
	if got != "second result" {
		t.Errorf("workflow_final_result = %q, want %q (last completed should win)", got, "second result")
	}
}

// TestWorkflowFinalResult_NonStringValueIgnored verifies that a non-string
// workflow_final_result value is ignored and the field is absent from the state.
func TestWorkflowFinalResult_NonStringValueIgnored(t *testing.T) {
	env := NewTestEnv(t)
	env.CreateTicket(t, "WFR-4", "Non-string result")
	env.InitWorkflow(t, "WFR-4")
	wfiID := env.GetWorkflowInstanceID(t, "WFR-4", "test")

	env.InsertAgentSession(t, "sess-wfr-4", "WFR-4", wfiID, "analyzer", "analyzer", "")
	setSessionFindings(t, env, "sess-wfr-4", map[string]interface{}{
		"workflow_final_result": 42, // non-string
	})
	env.CompleteAgentSession(t, "sess-wfr-4", "pass")

	status, err := getWorkflowStatus(t, env, "WFR-4", &types.WorkflowGetRequest{Workflow: "test"})
	if err != nil {
		t.Fatalf("failed to get status: %v", err)
	}

	if _, exists := status["workflow_final_result"]; exists {
		t.Errorf("expected workflow_final_result to be absent for non-string value, got %v", status["workflow_final_result"])
	}
}

// TestWorkflowFinalResult_CompletedSessionWinsOverRunning verifies that a completed
// session's workflow_final_result takes priority over a running session (NULL ended_at).
func TestWorkflowFinalResult_CompletedSessionWinsOverRunning(t *testing.T) {
	env := NewTestEnv(t)
	env.CreateTicket(t, "WFR-5", "Running vs completed")
	env.InitWorkflow(t, "WFR-5")
	wfiID := env.GetWorkflowInstanceID(t, "WFR-5", "test")

	// Completed session with its own final result
	env.InsertAgentSession(t, "sess-wfr-5a", "WFR-5", wfiID, "analyzer", "analyzer", "")
	setSessionFindings(t, env, "sess-wfr-5a", map[string]interface{}{
		"workflow_final_result": "completed result",
	})
	env.CompleteAgentSession(t, "sess-wfr-5a", "pass")

	// Running session (no ended_at) that also has a final result
	env.Clock.Advance(1 * time.Second)
	env.InsertAgentSession(t, "sess-wfr-5b", "WFR-5", wfiID, "builder", "builder", "")
	setSessionFindings(t, env, "sess-wfr-5b", map[string]interface{}{
		"workflow_final_result": "running result",
	})
	// sess-wfr-5b remains running (not completed)

	status, err := getWorkflowStatus(t, env, "WFR-5", &types.WorkflowGetRequest{Workflow: "test"})
	if err != nil {
		t.Fatalf("failed to get status: %v", err)
	}

	// Completed sessions should take priority over running sessions
	got, ok := status["workflow_final_result"].(string)
	if !ok {
		t.Fatalf("expected workflow_final_result string, got %T: %v", status["workflow_final_result"], status["workflow_final_result"])
	}
	if got != "completed result" {
		t.Errorf("workflow_final_result = %q, want %q (completed sessions should win over running)", got, "completed result")
	}
}

// TestWorkflowFinalResult_MultilinePreserved verifies that multi-line result text
// is stored and returned verbatim without truncation.
func TestWorkflowFinalResult_MultilinePreserved(t *testing.T) {
	env := NewTestEnv(t)
	env.CreateTicket(t, "WFR-6", "Multiline result")
	env.InitWorkflow(t, "WFR-6")
	wfiID := env.GetWorkflowInstanceID(t, "WFR-6", "test")

	multiline := "Line one\nLine two\nLine three"
	env.InsertAgentSession(t, "sess-wfr-6", "WFR-6", wfiID, "analyzer", "analyzer", "")
	setSessionFindings(t, env, "sess-wfr-6", map[string]interface{}{
		"workflow_final_result": multiline,
	})
	env.CompleteAgentSession(t, "sess-wfr-6", "pass")

	status, err := getWorkflowStatus(t, env, "WFR-6", &types.WorkflowGetRequest{Workflow: "test"})
	if err != nil {
		t.Fatalf("failed to get status: %v", err)
	}

	got, ok := status["workflow_final_result"].(string)
	if !ok {
		t.Fatalf("expected workflow_final_result string, got %T", status["workflow_final_result"])
	}
	if got != multiline {
		t.Errorf("workflow_final_result = %q, want %q", got, multiline)
	}
}
