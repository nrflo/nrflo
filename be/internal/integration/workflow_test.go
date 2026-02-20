package integration

import (
	"encoding/json"
	"testing"
	"time"

	"be/internal/types"
)

// normalizeJSON round-trips a map through JSON to normalize Go native types
// (int -> float64, struct -> map) to match what tests expect.
func normalizeJSON(t *testing.T, v map[string]interface{}) map[string]interface{} {
	t.Helper()
	data, err := json.Marshal(v)
	if err != nil {
		t.Fatalf("failed to marshal: %v", err)
	}
	var result map[string]interface{}
	if err := json.Unmarshal(data, &result); err != nil {
		t.Fatalf("failed to unmarshal: %v", err)
	}
	return result
}

// getWorkflowStatus gets workflow status and normalizes types via JSON round-trip.
func getWorkflowStatus(t *testing.T, env *TestEnv, ticketID string, req *types.WorkflowGetRequest) (map[string]interface{}, error) {
	t.Helper()
	raw, err := env.WorkflowSvc.GetStatus(env.ProjectID, ticketID, req)
	if err != nil {
		return nil, err
	}
	return normalizeJSON(t, raw), nil
}

func TestWorkflowInitAndStatus(t *testing.T) {
	env := NewTestEnv(t)

	env.CreateTicket(t, "WF-1", "Workflow test")
	env.InitWorkflow(t, "WF-1")

	// Get status via service
	status, err := getWorkflowStatus(t, env, "WF-1", &types.WorkflowGetRequest{
		Workflow: "test",
	})
	if err != nil {
		t.Fatalf("failed to get status: %v", err)
	}

	if v, _ := status["version"].(float64); v != 4 {
		t.Fatalf("expected version 4, got %v", v)
	}

	phaseOrder, ok := status["phase_order"].([]interface{})
	if !ok || len(phaseOrder) != 2 {
		t.Fatalf("expected 2-element phase_order, got %v", status["phase_order"])
	}
	if phaseOrder[0] != "analyzer" || phaseOrder[1] != "builder" {
		t.Fatalf("expected [analyzer, builder], got %v", phaseOrder)
	}

	phases, _ := status["phases"].(map[string]interface{})
	for _, name := range []string{"analyzer", "builder"} {
		phase, _ := phases[name].(map[string]interface{})
		if phase["status"] != "pending" {
			t.Fatalf("expected phase '%s' to be pending, got %v", name, phase["status"])
		}
	}
}

func TestWorkflowInitAutoCreatesTicket(t *testing.T) {
	env := NewTestEnv(t)

	// Init workflow on non-existent ticket - should auto-create
	env.InitWorkflow(t, "auto-ticket")

	// Verify ticket was created via service
	ticket, err := env.TicketSvc.Get(env.ProjectID, "auto-ticket")
	if err != nil {
		t.Fatalf("ticket should have been auto-created: %v", err)
	}
	if ticket.ID != "auto-ticket" {
		t.Fatalf("expected id 'auto-ticket', got %v", ticket.ID)
	}
}

func TestWorkflowPhaseLifecycle(t *testing.T) {
	env := NewTestEnv(t)

	env.CreateTicket(t, "WF-2", "Phase lifecycle")
	env.InitWorkflow(t, "WF-2")

	wfiID := env.GetWorkflowInstanceID(t, "WF-2", "test")

	// Create a running agent session for analyzer (derivation reads agent_sessions)
	env.InsertAgentSession(t, "sess-analyzer", "WF-2", wfiID, "analyzer", "analyzer", "")

	// Verify in_progress (derived from running agent session)
	status, err := getWorkflowStatus(t, env, "WF-2", &types.WorkflowGetRequest{
		Workflow: "test",
	})
	if err != nil {
		t.Fatalf("failed to get status: %v", err)
	}
	phases, _ := status["phases"].(map[string]interface{})
	analyzerPhase, _ := phases["analyzer"].(map[string]interface{})
	if analyzerPhase["status"] != "in_progress" {
		t.Fatalf("expected analyzer in_progress, got %v", analyzerPhase["status"])
	}

	// Complete the analyzer session
	env.CompleteAgentSession(t, "sess-analyzer", "pass")

	// Verify completed
	status, err = getWorkflowStatus(t, env, "WF-2", &types.WorkflowGetRequest{
		Workflow: "test",
	})
	if err != nil {
		t.Fatalf("failed to get status: %v", err)
	}
	phases, _ = status["phases"].(map[string]interface{})
	analyzerPhase, _ = phases["analyzer"].(map[string]interface{})
	if analyzerPhase["result"] != "pass" {
		t.Fatalf("expected analyzer result 'pass', got %v", analyzerPhase["result"])
	}

	// Create and complete builder session
	env.Clock.Advance(1 * time.Second)
	env.InsertAgentSession(t, "sess-builder", "WF-2", wfiID, "builder", "builder", "")
	env.CompleteAgentSession(t, "sess-builder", "pass")

	// Verify both completed
	status, err = getWorkflowStatus(t, env, "WF-2", &types.WorkflowGetRequest{
		Workflow: "test",
	})
	if err != nil {
		t.Fatalf("failed to get status: %v", err)
	}
	phases, _ = status["phases"].(map[string]interface{})
	for _, name := range []string{"analyzer", "builder"} {
		phase, _ := phases[name].(map[string]interface{})
		if phase["status"] != "completed" {
			t.Fatalf("expected phase '%s' completed, got %v", name, phase["status"])
		}
	}
}

func TestWorkflowSet(t *testing.T) {
	env := NewTestEnv(t)

	env.CreateTicket(t, "WF-3", "Set test")
	env.InitWorkflow(t, "WF-3")

	// Set retry_count
	err := env.WorkflowSvc.Set(env.ProjectID, "WF-3", &types.WorkflowSetRequest{
		Workflow: "test",
		Key:     "retry_count",
		Value:   "3",
	})
	if err != nil {
		t.Fatalf("failed to set retry_count: %v", err)
	}

	// Verify
	status, err := getWorkflowStatus(t, env, "WF-3", &types.WorkflowGetRequest{
		Workflow: "test",
	})
	if err != nil {
		t.Fatalf("failed to get status: %v", err)
	}
	if rc, ok := status["retry_count"].(float64); !ok || int(rc) != 3 {
		t.Fatalf("expected retry_count 3, got %v", status["retry_count"])
	}

	// Verify unknown keys are rejected
	err = env.WorkflowSvc.Set(env.ProjectID, "WF-3", &types.WorkflowSetRequest{
		Workflow: "test",
		Key:     "custom_note",
		Value:   "hello world",
	})
	if err == nil {
		t.Fatal("expected error for unknown key")
	}
}

func TestWorkflowDuplicateInit(t *testing.T) {
	env := NewTestEnv(t)

	env.CreateTicket(t, "WF-4", "Duplicate init")
	env.InitWorkflow(t, "WF-4")

	// Second init should fail
	err := env.WorkflowSvc.Init(env.ProjectID, "WF-4", &types.WorkflowInitRequest{
		Workflow: "test",
	})
	if err == nil {
		t.Fatal("expected error for duplicate init")
	}
}

func TestWorkflowInvalidPhaseResult(t *testing.T) {
	env := NewTestEnv(t)

	env.CreateTicket(t, "WF-5", "Bad result")
	env.InitWorkflow(t, "WF-5")
	env.StartPhase(t, "WF-5", "analyzer")

	// Complete with invalid result
	err := env.WorkflowSvc.CompletePhase(env.ProjectID, "WF-5", &types.PhaseUpdateRequest{
		Workflow: "test",
		Phase:   "analyzer",
		Result:  "invalid_result",
	})
	if err == nil {
		t.Fatal("expected error for invalid phase result")
	}
}
