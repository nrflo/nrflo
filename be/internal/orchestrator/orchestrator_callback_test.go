package orchestrator

import (
	"context"
	"testing"
	"time"

	"be/internal/model"
	"be/internal/service"
	"be/internal/spawner"
	"be/internal/ws"
)

// TestHandleCallback_LevelMode verifies level-mode callback: plan built, sessions reset, WS broadcast.
func TestHandleCallback_LevelMode(t *testing.T) {
	env := newTestEnv(t)
	env.createWorkflowWithAgents(t, "callback-test", "Callback workflow", "", []struct{ ID string; Layer int }{
		{"analyzer", 0}, {"builder", 1}, {"verifier", 2},
	})
	env.createTicket(t, "CB-1", "Callback test")
	wfiID := insertWFI(t, env, "wfi-cb-1", "CB-1", "callback-test")

	_, asRepo := openAsRepo(t, env)
	createSession(t, asRepo, "sess-builder", env.project, "CB-1", wfiID, "builder", model.AgentSessionCompleted)
	createSession(t, asRepo, "sess-verifier", env.project, "CB-1", wfiID, "verifier", model.AgentSessionCompleted)

	layerGroups := []layerGroup{
		{layer: 0, phases: []service.SpawnerPhaseDef{{Agent: "analyzer", Layer: 0}}},
		{layer: 1, phases: []service.SpawnerPhaseDef{{Agent: "builder", Layer: 1}}},
		{layer: 2, phases: []service.SpawnerPhaseDef{{Agent: "verifier", Layer: 2}}},
	}
	req := RunRequest{ProjectID: env.project, TicketID: "CB-1", WorkflowName: "callback-test", ScopeType: "ticket"}
	ch := env.subscribeWSClient(t, "ws-1", "CB-1")

	count := 0
	cbErrs := []*spawner.CallbackError{{Level: 1, AgentType: "verifier", Instructions: "Fix builder"}}
	ok := env.orch.handleCallback(context.Background(), wfiID, req, layerGroups, 2, cbErrs, &count)
	if !ok {
		t.Fatal("expected handleCallback to return true")
	}
	if count != 2 {
		t.Errorf("count = %d, want 2 (builder + verifier)", count)
	}

	wi := env.getWorkflowInstance(t, wfiID)
	cb, ok := wi.GetFindings()["_callback"].(map[string]interface{})
	if !ok {
		t.Fatal("expected _callback key in findings")
	}
	if cb["from_layer"] != float64(2) {
		t.Errorf("from_layer = %v, want 2", cb["from_layer"])
	}
	if cb["resume_layer"] != float64(3) {
		t.Errorf("resume_layer = %v, want 3", cb["resume_layer"])
	}
	if cb["instructions"] != "Fix builder" {
		t.Errorf("instructions = %v, want 'Fix builder'", cb["instructions"])
	}

	sess, _ := asRepo.Get("sess-builder")
	if sess.Status != model.AgentSessionCallback {
		t.Errorf("builder status = %s, want callback", sess.Status)
	}
	if sess.Findings.String != "{}" {
		t.Errorf("builder findings = %s, want {}", sess.Findings.String)
	}

	event := expectEvent(t, ch, ws.EventOrchestrationCallback, 2*time.Second)
	if event.Data["from_layer"] != float64(2) {
		t.Errorf("event from_layer = %v, want 2", event.Data["from_layer"])
	}
	if event.Data["to_layer"] != float64(1) {
		t.Errorf("event to_layer = %v, want 1", event.Data["to_layer"])
	}
	if event.Data["instructions"] != "Fix builder" {
		t.Errorf("event instructions = %v", event.Data["instructions"])
	}
}

// TestHandleCallback_AgentMode verifies agent-mode callback produces a per-agent plan step.
func TestHandleCallback_AgentMode(t *testing.T) {
	env := newTestEnv(t)
	env.createWorkflowWithAgents(t, "agent-cb", "Agent callback", "", []struct{ ID string; Layer int }{
		{"analyzer", 0}, {"builder", 1}, {"verifier", 2},
	})
	env.createTicket(t, "CB-A", "Agent mode")
	wfiID := insertWFI(t, env, "wfi-agent-cb", "CB-A", "agent-cb")

	layerGroups := []layerGroup{
		{layer: 0, phases: []service.SpawnerPhaseDef{{Agent: "analyzer", Layer: 0}}},
		{layer: 1, phases: []service.SpawnerPhaseDef{{Agent: "builder", Layer: 1}}},
		{layer: 2, phases: []service.SpawnerPhaseDef{{Agent: "verifier", Layer: 2}}},
	}
	req := RunRequest{ProjectID: env.project, TicketID: "CB-A", WorkflowName: "agent-cb", ScopeType: "ticket"}

	count := 0
	cbErrs := []*spawner.CallbackError{{Mode: "agent", TargetAgent: "builder", AgentType: "verifier", Instructions: "Fix it"}}
	ok := env.orch.handleCallback(context.Background(), wfiID, req, layerGroups, 2, cbErrs, &count)
	if !ok {
		t.Fatal("expected true")
	}
	if count != 2 {
		t.Errorf("count = %d, want 2", count)
	}

	cb, _ := env.getWorkflowInstance(t, wfiID).GetFindings()["_callback"].(map[string]interface{})
	plan := cb["plan"].([]interface{})
	if len(plan) != 2 {
		t.Fatalf("plan len = %d, want 2", len(plan))
	}
	step0 := plan[0].(map[string]interface{})
	if step0["layer"] != float64(1) || step0["whole_layer"] != false {
		t.Errorf("step0 = %v, want layer=1 whole_layer=false", step0)
	}
	step1 := plan[1].(map[string]interface{})
	if step1["layer"] != float64(2) || step1["whole_layer"] != true {
		t.Errorf("step1 = %v, want layer=2 whole_layer=true", step1)
	}
}

// TestHandleCallback_ChainMode verifies chain-mode callback.
func TestHandleCallback_ChainMode(t *testing.T) {
	env := newTestEnv(t)
	env.createWorkflowWithAgents(t, "chain-cb", "Chain callback", "", []struct{ ID string; Layer int }{
		{"analyzer", 0}, {"builder", 1}, {"verifier", 2},
	})
	env.createTicket(t, "CB-C", "Chain mode")
	wfiID := insertWFI(t, env, "wfi-chain-cb", "CB-C", "chain-cb")

	layerGroups := []layerGroup{
		{layer: 0, phases: []service.SpawnerPhaseDef{{Agent: "analyzer", Layer: 0}}},
		{layer: 1, phases: []service.SpawnerPhaseDef{{Agent: "builder", Layer: 1}}},
		{layer: 2, phases: []service.SpawnerPhaseDef{{Agent: "verifier", Layer: 2}}},
	}
	req := RunRequest{ProjectID: env.project, TicketID: "CB-C", WorkflowName: "chain-cb", ScopeType: "ticket"}

	count := 0
	cbErrs := []*spawner.CallbackError{{Mode: "chain", Chain: []string{"analyzer", "builder"}, AgentType: "verifier"}}
	ok := env.orch.handleCallback(context.Background(), wfiID, req, layerGroups, 2, cbErrs, &count)
	if !ok {
		t.Fatal("expected true")
	}
	if count != 2 {
		t.Errorf("count = %d, want 2", count)
	}
	cb, _ := env.getWorkflowInstance(t, wfiID).GetFindings()["_callback"].(map[string]interface{})
	if cb["resume_layer"] != float64(2) {
		t.Errorf("resume_layer = %v, want 2", cb["resume_layer"])
	}
}

// TestHandleCallback_MultipleRequests verifies multiple concurrent callbacks are merged.
func TestHandleCallback_MultipleRequests(t *testing.T) {
	env := newTestEnv(t)
	env.createWorkflowWithAgents(t, "multi-req-cb", "Multi-request callback", "", []struct{ ID string; Layer int }{
		{"analyzer", 0}, {"impl-a", 1}, {"impl-b", 1}, {"verifier", 2},
	})
	env.createTicket(t, "CB-MR", "Multi-request")
	wfiID := insertWFI(t, env, "wfi-multi-req", "CB-MR", "multi-req-cb")

	layerGroups := []layerGroup{
		{layer: 0, phases: []service.SpawnerPhaseDef{{Agent: "analyzer", Layer: 0}}},
		{layer: 1, phases: []service.SpawnerPhaseDef{{Agent: "impl-a", Layer: 1}, {Agent: "impl-b", Layer: 1}}},
		{layer: 2, phases: []service.SpawnerPhaseDef{{Agent: "verifier", Layer: 2}}},
	}
	req := RunRequest{ProjectID: env.project, TicketID: "CB-MR", WorkflowName: "multi-req-cb", ScopeType: "ticket"}

	count := 0
	cbErrs := []*spawner.CallbackError{
		{Level: 0, AgentType: "impl-a", Instructions: "impl-a says fix analyzer"},
		{Level: 0, AgentType: "impl-b", Instructions: "impl-b says fix analyzer"},
	}
	ok := env.orch.handleCallback(context.Background(), wfiID, req, layerGroups, 1, cbErrs, &count)
	if !ok {
		t.Fatal("expected true for merged callbacks")
	}

	cb, _ := env.getWorkflowInstance(t, wfiID).GetFindings()["_callback"].(map[string]interface{})
	// sorted by agentID: impl-a before impl-b
	wantInstr := "impl-a says fix analyzer\n---\nimpl-b says fix analyzer"
	if cb["instructions"] != wantInstr {
		t.Errorf("instructions = %q, want %q", cb["instructions"], wantInstr)
	}
}
