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

// TestHandleCallback_InvalidAgent verifies agent-mode callback to unknown agent fails.
func TestHandleCallback_InvalidAgent(t *testing.T) {
	env := newTestEnv(t)
	env.createTicket(t, "CB-INV", "Invalid agent")
	wfiID := insertWFI(t, env, "wfi-inv", "CB-INV", "test")
	layerGroups := []layerGroup{
		{layer: 0, phases: []service.SpawnerPhaseDef{{Agent: "analyzer", Layer: 0}}},
		{layer: 1, phases: []service.SpawnerPhaseDef{{Agent: "builder", Layer: 1}}},
	}
	req := RunRequest{ProjectID: env.project, TicketID: "CB-INV", WorkflowName: "test", ScopeType: "ticket"}
	count := 0
	cbErrs := []*spawner.CallbackError{{Mode: "agent", TargetAgent: "nonexistent", AgentType: "builder"}}
	ok := env.orch.handleCallback(context.Background(), wfiID, req, layerGroups, 1, cbErrs, &count)
	if ok {
		t.Error("expected false for unknown agent")
	}
}

// TestHandleCallback_ChainOutOfOrder verifies chain with descending layers is rejected.
func TestHandleCallback_ChainOutOfOrder(t *testing.T) {
	env := newTestEnv(t)
	env.createTicket(t, "CB-OOO", "Out of order")
	wfiID := insertWFI(t, env, "wfi-ooo", "CB-OOO", "test")
	layerGroups := []layerGroup{
		{layer: 0, phases: []service.SpawnerPhaseDef{{Agent: "analyzer", Layer: 0}}},
		{layer: 1, phases: []service.SpawnerPhaseDef{{Agent: "builder", Layer: 1}}},
	}
	req := RunRequest{ProjectID: env.project, TicketID: "CB-OOO", WorkflowName: "test", ScopeType: "ticket"}
	count := 0
	// chain in wrong order: builder (layer 1) before analyzer (layer 0)
	cbErrs := []*spawner.CallbackError{{Mode: "chain", Chain: []string{"builder", "analyzer"}, AgentType: "builder"}}
	ok := env.orch.handleCallback(context.Background(), wfiID, req, layerGroups, 1, cbErrs, &count)
	if ok {
		t.Error("expected false for out-of-order chain")
	}
}

// TestHandleCallback_ChainExceedsOriginatorLayer verifies chain agent above originator fails.
func TestHandleCallback_ChainExceedsOriginatorLayer(t *testing.T) {
	env := newTestEnv(t)
	env.createWorkflowWithAgents(t, "chain-exceed", "Chain exceed", "", []struct{ ID string; Layer int }{
		{"analyzer", 0}, {"builder", 1}, {"verifier", 2},
	})
	env.createTicket(t, "CB-CE", "Chain exceed")
	wfiID := insertWFI(t, env, "wfi-ce", "CB-CE", "chain-exceed")
	layerGroups := []layerGroup{
		{layer: 0, phases: []service.SpawnerPhaseDef{{Agent: "analyzer", Layer: 0}}},
		{layer: 1, phases: []service.SpawnerPhaseDef{{Agent: "builder", Layer: 1}}},
		{layer: 2, phases: []service.SpawnerPhaseDef{{Agent: "verifier", Layer: 2}}},
	}
	req := RunRequest{ProjectID: env.project, TicketID: "CB-CE", WorkflowName: "chain-exceed", ScopeType: "ticket"}
	count := 0
	// originator is layer 1, but verifier is layer 2 (exceeds originator)
	cbErrs := []*spawner.CallbackError{{Mode: "chain", Chain: []string{"analyzer", "verifier"}, AgentType: "builder"}}
	ok := env.orch.handleCallback(context.Background(), wfiID, req, layerGroups, 1, cbErrs, &count)
	if ok {
		t.Error("expected false for chain item exceeding originator layer")
	}
}

// TestHandleCallback_CapExceeded verifies the cumulative spawn cap (maxCallbacks=10).
func TestHandleCallback_CapExceeded(t *testing.T) {
	env := newTestEnv(t)
	env.createTicket(t, "CB-CAP", "Cap test")
	wfiID := insertWFI(t, env, "wfi-cap", "CB-CAP", "test")
	layerGroups := []layerGroup{
		{layer: 0, phases: []service.SpawnerPhaseDef{{Agent: "analyzer", Layer: 0}}},
		{layer: 1, phases: []service.SpawnerPhaseDef{{Agent: "builder", Layer: 1}}},
	}
	req := RunRequest{ProjectID: env.project, TicketID: "CB-CAP", WorkflowName: "test", ScopeType: "ticket"}
	// pre-fill counter: level 0 callback from layer 1 = 2 agents → total 11 > 10
	count := 9
	cbErrs := []*spawner.CallbackError{{Level: 0, AgentType: "builder"}}
	ok := env.orch.handleCallback(context.Background(), wfiID, req, layerGroups, 1, cbErrs, &count)
	if ok {
		t.Error("expected false when cumulative cap exceeded")
	}
}

// TestHandleCallback_MetadataPreserved verifies existing findings survive a callback.
func TestHandleCallback_MetadataPreserved(t *testing.T) {
	env := newTestEnv(t)
	env.createTicket(t, "CB-META", "Metadata test")
	wfiID := insertWFIWithFindings(t, env, "wfi-meta", "CB-META", "test", `{"existing_key":"existing_value"}`)
	layerGroups := []layerGroup{
		{layer: 0, phases: []service.SpawnerPhaseDef{{Agent: "analyzer", Layer: 0}}},
		{layer: 1, phases: []service.SpawnerPhaseDef{{Agent: "builder", Layer: 1}}},
	}
	req := RunRequest{ProjectID: env.project, TicketID: "CB-META", WorkflowName: "test", ScopeType: "ticket"}
	count := 0
	env.orch.handleCallback(context.Background(), wfiID, req, layerGroups, 1, []*spawner.CallbackError{{Level: 0, AgentType: "builder", Instructions: "Detailed"}}, &count)

	findings := env.getWorkflowInstance(t, wfiID).GetFindings()
	if findings["existing_key"] != "existing_value" {
		t.Error("existing findings should be preserved")
	}
	if _, ok := findings["_callback"]; !ok {
		t.Error("_callback should be set")
	}
}

// TestHandleCallback_SessionsExcludeRunningAndContinued verifies running/continued sessions are not reset.
func TestHandleCallback_SessionsExcludeRunningAndContinued(t *testing.T) {
	env := newTestEnv(t)
	env.createTicket(t, "CB-EXCL", "Exclusion test")
	wfiID := insertWFI(t, env, "wfi-excl", "CB-EXCL", "test")

	_, asRepo := openAsRepo(t, env)
	createSession(t, asRepo, "sess-completed", env.project, "CB-EXCL", wfiID, "analyzer", model.AgentSessionCompleted)
	createSession(t, asRepo, "sess-running", env.project, "CB-EXCL", wfiID, "builder", model.AgentSessionRunning)
	createSession(t, asRepo, "sess-continued", env.project, "CB-EXCL", wfiID, "analyzer", model.AgentSessionContinued)

	layerGroups := []layerGroup{
		{layer: 0, phases: []service.SpawnerPhaseDef{{Agent: "analyzer", Layer: 0}}},
		{layer: 1, phases: []service.SpawnerPhaseDef{{Agent: "builder", Layer: 1}}},
	}
	req := RunRequest{ProjectID: env.project, TicketID: "CB-EXCL", WorkflowName: "test", ScopeType: "ticket"}
	count := 0
	env.orch.handleCallback(context.Background(), wfiID, req, layerGroups, 1, []*spawner.CallbackError{{Level: 0, AgentType: "builder"}}, &count)

	completed, _ := asRepo.Get("sess-completed")
	if completed.Status != model.AgentSessionCallback {
		t.Errorf("completed session status = %s, want callback", completed.Status)
	}
	running, _ := asRepo.Get("sess-running")
	if running.Status != model.AgentSessionRunning {
		t.Errorf("running session status changed to %s", running.Status)
	}
	continued, _ := asRepo.Get("sess-continued")
	if continued.Status != model.AgentSessionContinued {
		t.Errorf("continued session status changed to %s", continued.Status)
	}
}

// TestHandleCallback_ProjectScope verifies project-scoped workflows broadcast with empty ticket_id.
func TestHandleCallback_ProjectScope(t *testing.T) {
	env := newTestEnv(t)
	env.createWorkflowWithAgents(t, "proj-cb", "Project callback", "project", []struct{ ID string; Layer int }{
		{"analyzer", 0}, {"builder", 1},
	})

	var wfiID string
	err := env.pool.QueryRow(`
		INSERT INTO workflow_instances (id, project_id, ticket_id, workflow_id, status, scope_type, findings, retry_count, created_at, updated_at)
		VALUES ('wfi-proj-cb', ?, '', 'proj-cb', 'active', 'project', '{}', 0, datetime('now'), datetime('now'))
		RETURNING id`, env.project).Scan(&wfiID)
	if err != nil {
		t.Fatalf("create project WFI: %v", err)
	}

	layerGroups := []layerGroup{
		{layer: 0, phases: []service.SpawnerPhaseDef{{Agent: "analyzer", Layer: 0}}},
		{layer: 1, phases: []service.SpawnerPhaseDef{{Agent: "builder", Layer: 1}}},
	}
	req := RunRequest{ProjectID: env.project, TicketID: "", WorkflowName: "proj-cb", ScopeType: "project"}
	ch := env.subscribeWSClient(t, "ws-proj", "")
	count := 0
	ok := env.orch.handleCallback(context.Background(), wfiID, req, layerGroups, 1, []*spawner.CallbackError{{Level: 0, AgentType: "builder"}}, &count)
	if !ok {
		t.Fatal("expected true for project scope")
	}
	event := expectEvent(t, ch, ws.EventOrchestrationCallback, 2*time.Second)
	if event.TicketID != "" {
		t.Errorf("expected empty ticket_id, got %s", event.TicketID)
	}
	if event.ProjectID != env.project {
		t.Errorf("expected project_id=%s, got %s", env.project, event.ProjectID)
	}
}

// TestCallbackChainCountsAgainstCap verifies cumulativeAgentCount detects 11 > maxCallbacks(10).
func TestCallbackChainCountsAgainstCap(t *testing.T) {
	groups := make([]layerGroup, 11)
	for i := 0; i < 11; i++ {
		agent := "agent-x" + string(rune('a'+i))
		groups[i] = layerGroup{layer: i, phases: []service.SpawnerPhaseDef{{Agent: agent, Layer: i}}}
	}
	plan := callbackPlan{steps: make([]callbackPlanStep, 11)}
	for i := 0; i < 11; i++ {
		plan.steps[i] = callbackPlanStep{layer: i, wholeLayer: false, agents: []string{groups[i].phases[0].Agent}}
	}
	count := cumulativeAgentCount(plan, groups)
	if count != 11 {
		t.Errorf("count = %d, want 11", count)
	}
	if count <= maxCallbacks {
		t.Errorf("count %d should exceed maxCallbacks %d", count, maxCallbacks)
	}
}
